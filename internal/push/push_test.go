package push

import (
	"bufio"
	"bytes"
	"context"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func testService(t *testing.T) (*Service, *Store) {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "push.json"))
	if err != nil {
		t.Fatal(err)
	}
	return NewServiceWithSender(Config{PublicKey: "pub", PrivateKey: "private", Subject: "mailto:x@example.com"}, s, slog.New(slog.NewTextHandler(io.Discard, nil)), fakeSender{}), s
}

type fakeSender struct {
	status  int
	err     error
	before  func()
	payload func([]byte)
	options func(*webpush.Options)
}

type sequenceSender struct {
	statuses map[string]int
	seen     []string
}

func (f *sequenceSender) Send(_ context.Context, _ []byte, sub *webpush.Subscription, _ *webpush.Options) (*http.Response, error) {
	f.seen = append(f.seen, sub.Endpoint)
	status := f.statuses[sub.Endpoint]
	if status == 0 {
		status = http.StatusCreated
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(""))}, nil
}

func (f fakeSender) Send(_ context.Context, payload []byte, _ *webpush.Subscription, options *webpush.Options) (*http.Response, error) {
	if f.before != nil {
		f.before()
	}
	if f.payload != nil {
		f.payload(payload)
	}
	if f.options != nil {
		f.options(options)
	}
	if f.status == 0 {
		f.status = http.StatusCreated
	}
	return &http.Response{StatusCode: f.status, Status: http.StatusText(f.status), Body: io.NopCloser(strings.NewReader(""))}, f.err
}

func subWithEndpoint(endpoint string) webpush.Subscription {
	x, y := elliptic.P256().ScalarBaseMult([]byte{1})
	key := elliptic.Marshal(elliptic.P256(), x, y)
	return webpush.Subscription{Endpoint: endpoint, Keys: webpush.Keys{
		Auth:   base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 16)),
		P256dh: base64.RawURLEncoding.EncodeToString(key),
	}}
}

func sub() webpush.Subscription { return subWithEndpoint("https://push.example/x") }

func TestStoreDeduplicatesEndpoints(t *testing.T) {
	_, s := testService(t)
	if err := s.Put(sub()); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(sub()); err != nil {
		t.Fatal(err)
	}
	if got := s.Get(); len(got) != 1 {
		t.Fatalf("subscriptions=%d want 1", len(got))
	}
	info, err := os.Stat(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode %o", info.Mode().Perm())
	}
}

func TestStoreRejectsInvalidPersistedSubscription(t *testing.T) {
	path := filepath.Join(t.TempDir(), "push.json")
	body, _ := json.Marshal(storeFile{Subscriptions: []webpush.Subscription{{Endpoint: "https://push.example/x"}}})
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStore(path); err == nil {
		t.Fatal("invalid persisted key material accepted")
	}
}

func TestStoreLoadsLegacyAndRewritesCurrentShape(t *testing.T) {
	fixtures := map[string]any{
		"identity-free": legacyStoreFile{Subscription: sub()},
		"deployed v1":   map[string]any{"user": "alice", "subscription": sub()},
	}
	for name, fixture := range fixtures {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "push.json")
			legacy, _ := json.Marshal(fixture)
			if err := os.WriteFile(path, legacy, 0600); err != nil {
				t.Fatal(err)
			}
			store, err := OpenStore(path)
			if err != nil || len(store.Get()) != 1 {
				t.Fatalf("legacy load: subscriptions=%v err=%v", store.Get(), err)
			}
			if err := store.Put(subWithEndpoint("https://push.example/new")); err != nil {
				t.Fatal(err)
			}
			var current storeFile
			body, _ := os.ReadFile(path)
			if err := json.Unmarshal(body, &current); err != nil || len(current.Subscriptions) != 2 || bytes.Contains(body, []byte(`"user"`)) {
				t.Fatalf("current rewrite=%s err=%v", body, err)
			}
			backup, err := os.ReadFile(path + ".legacy-v1.bak")
			if err != nil || !bytes.Equal(backup, legacy) {
				t.Fatalf("legacy backup=%s err=%v", backup, err)
			}
			if err := os.WriteFile(path, backup, 0600); err != nil {
				t.Fatal(err)
			}
			rolledBack, err := OpenStore(path)
			if err != nil || len(rolledBack.Get()) != 1 || rolledBack.Get()[0].Endpoint != sub().Endpoint {
				t.Fatalf("rollback recovery: subscriptions=%v err=%v", rolledBack.Get(), err)
			}
		})
	}
}

func TestConfigValidation(t *testing.T) {
	private := make([]byte, 32)
	private[31] = 1
	x, y := elliptic.P256().ScalarBaseMult(private)
	public := elliptic.Marshal(elliptic.P256(), x, y)
	validPublic := base64.RawURLEncoding.EncodeToString(public)
	validPrivate := base64.RawURLEncoding.EncodeToString(private)
	tests := []struct{ name, public, private, subject string }{
		{"partial", validPublic, "", "mailto:x@example.com"},
		{"malformed public base64", "***", validPrivate, "mailto:x@example.com"},
		{"malformed public point", base64.RawURLEncoding.EncodeToString(make([]byte, 65)), validPrivate, "mailto:x@example.com"},
		{"malformed private base64", validPublic, "***", "mailto:x@example.com"},
		{"zero private scalar", validPublic, base64.RawURLEncoding.EncodeToString(make([]byte, 32)), "mailto:x@example.com"},
		{"mismatched keypair", validPublic, base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32)), "mailto:x@example.com"},
		{"empty mailto subject", validPublic, validPrivate, "mailto:"},
		{"relative subject", validPublic, validPrivate, "/contact"},
		{"subject credentials", validPublic, validPrivate, "https://user@example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("VAPID_PUBLIC_KEY", tt.public)
			t.Setenv("VAPID_PRIVATE_KEY", tt.private)
			t.Setenv("VAPID_SUBJECT", tt.subject)
			if _, err := ConfigFromEnv(); err == nil {
				t.Fatal("malformed VAPID config accepted")
			}
		})
	}
	t.Setenv("VAPID_PUBLIC_KEY", validPublic)
	t.Setenv("VAPID_PRIVATE_KEY", validPrivate)
	t.Setenv("VAPID_SUBJECT", "mailto:x@example.com")
	if config, err := ConfigFromEnv(); err != nil || !config.Enabled() {
		t.Fatalf("valid VAPID config rejected: config=%+v err=%v", config, err)
	}
}

func TestPutValidationRejectsMalformedInput(t *testing.T) {
	svc, store := testService(t)
	valid, _ := json.Marshal(subscriptionRequest{Endpoint: sub().Endpoint, Keys: sub().Keys})
	cases := map[string]string{
		"trailing JSON": string(valid) + `{}`,
		"trailing junk": string(valid) + `x`,
		"unknown field": strings.TrimSuffix(string(valid), "}") + `,"extra":true}`,
		"userinfo":      strings.Replace(string(valid), "https://", "https://user@", 1),
		"fragment":      strings.Replace(string(valid), "/x", "/x#secret", 1),
		"loopback":      strings.Replace(string(valid), "push.example", "127.0.0.1", 1),
		"private":       strings.Replace(string(valid), "push.example", "10.0.0.1", 1),
		"bad auth":      strings.Replace(string(valid), sub().Keys.Auth, "YQ", 1),
		"bad p256dh":    strings.Replace(string(valid), sub().Keys.P256dh, "Yg", 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPut, "/api/push/subscription", strings.NewReader(body))
			w := httptest.NewRecorder()
			svc.Handler().ServeHTTP(w, r)
			if w.Code != http.StatusBadRequest || len(store.Get()) != 0 {
				t.Fatalf("code=%d record=%v", w.Code, store.Get())
			}
		})
	}
}

func TestAPISingleOwnerLifecycle(t *testing.T) {
	svc, s := testService(t)
	h := svc.Handler()
	r := httptest.NewRequest(http.MethodGet, "/api/push/config", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatal(w.Code)
	}
	body, _ := json.Marshal(subscriptionRequest{Endpoint: sub().Endpoint, Keys: sub().Keys})
	r = httptest.NewRequest(http.MethodPut, "/api/push/subscription", bytes.NewReader(body))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent || len(s.Get()) == 0 {
		t.Fatalf("code=%d record=%v", w.Code, s.Get())
	}
	r = httptest.NewRequest(http.MethodDelete, "/api/push/subscription", strings.NewReader(`{"endpoint":"https://push.example/x"}`))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent || len(s.Get()) != 0 {
		t.Fatal(w.Code)
	}
}

func TestValidSubscriptionRejectsSpecialUseLiterals(t *testing.T) {
	tests := []struct {
		name string
		host string
		want bool
	}{
		{"public IPv4", "8.8.8.8", true},
		{"public IPv6", "[2606:4700:4700::1111]", true},
		{"outside public allocation IPv6", "[4000::1]", false},
		{"unspecified IPv4", "0.1.2.3", false},
		{"private IPv4", "10.0.0.1", false},
		{"CGNAT IPv4", "100.64.0.1", false},
		{"loopback IPv4", "127.0.0.1", false},
		{"link-local IPv4", "169.254.1.1", false},
		{"protocol assignment IPv4", "192.0.0.1", false},
		{"documentation IPv4", "192.0.2.1", false},
		{"benchmark IPv4", "198.18.0.1", false},
		{"reserved IPv4", "240.0.0.1", false},
		{"loopback IPv6", "[::1]", false},
		{"translation IPv6", "[64:ff9b::1]", false},
		{"discard IPv6", "[100::1]", false},
		{"benchmark IPv6", "[2001:2::1]", false},
		{"documentation IPv6", "[2001:db8::1]", false},
		{"6to4 IPv6", "[2002::1]", false},
		{"documentation IPv6 2", "[3fff::1]", false},
		{"reserved IPv6", "[5f00::1]", false},
		{"private IPv6", "[fd00::1]", false},
		{"link-local IPv6", "[fe80::1]", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validSubscription(subWithEndpoint("https://" + tt.host + "/push")); got != tt.want {
				t.Fatalf("validSubscription(%q)=%v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

func TestPublicHTTPClientRejectsPrivateDialAndRedirect(t *testing.T) {
	client := publicHTTPClient()
	if _, err := client.Get("https://127.0.0.1/push"); err == nil || !strings.Contains(err.Error(), "non-public") {
		t.Fatalf("private dial error=%v", err)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://push.example/start", nil)
	redirect, _ := http.NewRequest(http.MethodGet, "https://other.example/end", nil)
	if err := client.CheckRedirect(redirect, []*http.Request{req}); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("redirect allowed: %v", err)
	}
}

func TestPublicDialRevalidatesDNSAnswers(t *testing.T) {
	tests := []struct {
		name  string
		addrs []netip.Addr
		want  bool
	}{
		{"public IPv4", []netip.Addr{netip.MustParseAddr("93.184.216.34")}, true},
		{"public IPv6", []netip.Addr{netip.MustParseAddr("2606:4700:4700::1111")}, true},
		{"outside public allocation IPv6", []netip.Addr{netip.MustParseAddr("4000::1")}, false},
		{"CGNAT IPv4", []netip.Addr{netip.MustParseAddr("100.64.0.1")}, false},
		{"benchmark IPv4", []netip.Addr{netip.MustParseAddr("198.18.0.1")}, false},
		{"documentation IPv4", []netip.Addr{netip.MustParseAddr("203.0.113.1")}, false},
		{"benchmark IPv6", []netip.Addr{netip.MustParseAddr("2001:2::1")}, false},
		{"documentation IPv6", []netip.Addr{netip.MustParseAddr("2001:db8::1")}, false},
		{"reserved IPv6", []netip.Addr{netip.MustParseAddr("5f00::1")}, false},
		{"mixed public and loopback", []netip.Addr{netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("127.0.0.1")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dialed atomic.Bool
			dial := publicDialContext(
				func(context.Context, string, string) ([]netip.Addr, error) { return tt.addrs, nil },
				func(context.Context, string, string) (net.Conn, error) {
					dialed.Store(true)
					return nil, errors.New("dial marker")
				},
			)
			_, err := dial(context.Background(), "tcp", "push.example:443")
			if tt.want {
				if err == nil || err.Error() != "dial marker" || !dialed.Load() {
					t.Fatalf("public DNS answer not dialed: err=%v dialed=%v", err, dialed.Load())
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "non-public") || dialed.Load() {
				t.Fatalf("non-public DNS answer accepted: err=%v dialed=%v", err, dialed.Load())
			}
		})
	}
}

func TestTransitionFiltersSnapshotsAndDuplicates(t *testing.T) {
	states := map[string]string{"p": "done"}
	e := agentEvent{}
	e.Data.PaneID = "p"
	e.Data.AgentStatus = "done"
	if _, _, _, ok := EventTransition(states, nil, e); ok {
		t.Fatal("initial snapshot notified")
	}
	if _, _, _, ok := EventTransition(states, nil, e); ok {
		t.Fatal("duplicate notified")
	}
	e.Data.AgentStatus = "working"
	EventTransition(states, nil, e)
	e.Data.AgentStatus = "blocked"
	if _, state, paneID, ok := EventTransition(states, nil, e); !ok || state != "blocked" || paneID != "p" {
		t.Fatal("transition missed")
	}
	e.Data.PaneID = "../bad"
	e.Data.AgentStatus = "working"
	EventTransition(states, nil, e)
	e.Data.AgentStatus = "done"
	if _, _, _, ok := EventTransition(states, nil, e); ok {
		t.Fatal("invalid pane target notified")
	}
}

func TestTransitionUsesContentLabelBeforeAgent(t *testing.T) {
	agent := "pi"
	states := map[string]string{"p": "working"}
	e := agentEvent{Data: agentEventData{PaneID: "p", AgentStatus: "done", Agent: &agent}}
	if label, _, _, ok := EventTransition(states, map[string]string{"p": "Push content labels"}, e); !ok || label != "Push content labels" {
		t.Fatalf("content label not used: label=%q ok=%v", label, ok)
	}
}

func TestPaneNotificationLabelsPreferNamedTabAndFallbackToWorkspace(t *testing.T) {
	var snapshot focusSnapshotResult
	if err := json.Unmarshal([]byte(`{"type":"session_snapshot","snapshot":{"version":"test","protocol":19,"workspaces":[{"workspace_id":"w1","label":" Project\u0000 Name "}],"tabs":[{"tab_id":"t1","workspace_id":"w1","label":"Content task","number":1},{"tab_id":"t2","workspace_id":"w1","label":"2","number":2}],"panes":[{"pane_id":"p1","tab_id":"t1","workspace_id":"w1"},{"pane_id":"p2","tab_id":"t2","workspace_id":"w1"}],"layouts":[],"agents":[]}}`), &snapshot); err != nil {
		t.Fatal(err)
	}
	labels := paneNotificationLabels(snapshot)
	if labels["p1"] != "Content task" || labels["p2"] != "Project Name" {
		t.Fatalf("labels=%v", labels)
	}
	if got := len([]rune(notificationLabel(strings.Repeat("x", 121)))); got != 120 {
		t.Fatalf("bounded label length=%d", got)
	}
}

func TestTransitionDoesNotSeedUnvalidatedPane(t *testing.T) {
	states := map[string]string{}
	e := agentEvent{Data: agentEventData{PaneID: "p", AgentStatus: "working"}}
	if _, _, _, ok := EventTransition(states, nil, e); ok || len(states) != 0 {
		t.Fatalf("unseeded working event trusted: states=%v", states)
	}
	e.Data.AgentStatus = "done"
	if _, _, _, ok := EventTransition(states, nil, e); ok || len(states) != 0 {
		t.Fatalf("unseeded done event dispatched: states=%v", states)
	}
}

func TestPaneCreatedReplayOnlyResnapshotsForNewPane(t *testing.T) {
	states := map[string]string{"p1": "working"}
	if paneCreationChangesSet(states, agentEvent{Event: "pane_created", Data: agentEventData{Pane: &agentEventPane{PaneID: "p1"}}}) {
		t.Fatal("existing pane replay requested resnapshot")
	}
	if !paneCreationChangesSet(states, agentEvent{Event: "pane_created", Data: agentEventData{Pane: &agentEventPane{PaneID: "p2"}}}) {
		t.Fatal("new pane did not request resnapshot")
	}
}

func TestNotifyBroadcastReportsFailureEvenWhenLaterEndpointSucceeds(t *testing.T) {
	svc, store := testService(t)
	failed := subWithEndpoint("https://push.example/failed")
	live := subWithEndpoint("https://push.example/live")
	if err := store.Put(failed); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(live); err != nil {
		t.Fatal(err)
	}
	svc.sender = &sequenceSender{statuses: map[string]int{failed.Endpoint: http.StatusInternalServerError}}
	if err := svc.Notify(context.Background(), "agent", "done", "w1:p2"); err == nil || err.Error() != "web push broadcast failed: 1 of 2 attempts" {
		t.Fatalf("aggregate error=%v", err)
	}
}

func TestNotifyBroadcastAttemptsAllAndPrunesOnlyStaleEndpoint(t *testing.T) {
	svc, store := testService(t)
	stale := subWithEndpoint("https://push.example/stale")
	live := subWithEndpoint("https://push.example/live")
	if err := store.Put(stale); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(live); err != nil {
		t.Fatal(err)
	}
	sender := &sequenceSender{statuses: map[string]int{stale.Endpoint: http.StatusGone}}
	svc.sender = sender
	if err := svc.Notify(context.Background(), "agent", "done", "w1:p2"); err == nil || err.Error() != "web push broadcast failed: 1 of 2 attempts" {
		t.Fatalf("aggregate error=%v", err)
	}
	if !slices.Equal(sender.seen, []string{stale.Endpoint, live.Endpoint}) {
		t.Fatalf("attempts=%v", sender.seen)
	}
	got := store.Get()
	if len(got) != 1 || got[0].Endpoint != live.Endpoint {
		t.Fatalf("post-prune subscriptions=%v", got)
	}
}

func TestDeleteAPIOnlyRemovesRequestedEndpoint(t *testing.T) {
	svc, store := testService(t)
	first, second := subWithEndpoint("https://push.example/first"), subWithEndpoint("https://push.example/second")
	if err := store.Put(first); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(second); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/push/subscription", strings.NewReader(`{"endpoint":"https://push.example/first"}`))
	response := httptest.NewRecorder()
	svc.Handler().ServeHTTP(response, req)
	got := store.Get()
	if response.Code != http.StatusNoContent || len(got) != 1 || got[0].Endpoint != second.Endpoint {
		t.Fatalf("code=%d subscriptions=%v", response.Code, got)
	}
}

func TestNotifyPayloadContainsPaneIDWithoutURL(t *testing.T) {
	svc, store := testService(t)
	if err := store.Put(sub()); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	svc.sender = fakeSender{payload: func(body []byte) {
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
	}}
	if err := svc.Notify(context.Background(), "agent", "done", "w1:p2"); err != nil {
		t.Fatal(err)
	}
	if payload["pane_id"] != "w1:p2" || payload["url"] != nil || payload["title"] != "agent" || payload["body"] != "Done — tap to open" {
		t.Fatalf("unsafe or incorrect notification payload: %#v", payload)
	}
}

func TestNotifyNormalizesSubscriberForWebpushGo(t *testing.T) {
	for _, test := range []struct {
		subject string
		want    string
	}{
		{subject: "mailto:operator@example.com", want: "operator@example.com"},
		{subject: "https://example.com/push-contact", want: "https://example.com/push-contact"},
	} {
		t.Run(test.subject, func(t *testing.T) {
			svc, store := testService(t)
			if err := store.Put(sub()); err != nil {
				t.Fatal(err)
			}
			var logs bytes.Buffer
			svc.log = slog.New(slog.NewTextHandler(&logs, nil))
			svc.cfg.Subject = test.subject
			svc.cfg.PrivateKey = "private-secret-marker"
			var got string
			svc.sender = fakeSender{options: func(options *webpush.Options) { got = options.Subscriber }}
			if err := svc.Notify(context.Background(), "agent", "done", "w1:p2"); err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("subscriber=%q want %q", got, test.want)
			}
			if strings.Contains(logs.String(), svc.cfg.PrivateKey) {
				t.Fatalf("VAPID private key logged: %s", logs.String())
			}
		})
	}
}

func TestNotifyStaleResponseDoesNotDeleteReplacement(t *testing.T) {
	svc, s := testService(t)
	original := subWithEndpoint("https://push.example/old")
	replacement := subWithEndpoint("https://push.example/new")
	if err := s.Put(original); err != nil {
		t.Fatal(err)
	}
	svc.sender = fakeSender{status: http.StatusGone, before: func() {
		if err := s.Put(replacement); err != nil {
			t.Fatal(err)
		}
	}}
	if err := svc.Notify(context.Background(), "secret-agent", "done", "w1:p1"); err == nil {
		t.Fatal("expected status error")
	}
	if got := s.Get(); len(got) != 1 || got[0].Endpoint != replacement.Endpoint {
		t.Fatalf("replacement deleted: %#v", got)
	}
}

func TestNotifyStaleDeleteFailureRetainsRecord(t *testing.T) {
	svc, s := testService(t)
	if err := s.Put(sub()); err != nil {
		t.Fatal(err)
	}
	s.remove = func(string) error { return errors.New("disk unavailable") }
	svc.sender = fakeSender{status: http.StatusGone}
	if err := svc.Notify(context.Background(), "secret-agent", "done", "w1:p1"); err == nil {
		t.Fatal("expected status error")
	}
	if len(s.Get()) == 0 {
		t.Fatal("record removed despite persistence failure")
	}
}

func TestNotifySanitizesURLBearingError(t *testing.T) {
	const secretEndpoint = "https://push.example/secret-token"
	var logs bytes.Buffer
	svc, s := testService(t)
	svc.log = slog.New(slog.NewTextHandler(&logs, nil))
	if err := s.Put(subWithEndpoint(secretEndpoint)); err != nil {
		t.Fatal(err)
	}
	raw := &url.Error{Op: "Post", URL: secretEndpoint, Err: context.DeadlineExceeded}
	svc.sender = fakeSender{err: raw}
	err := svc.Notify(context.Background(), "secret-agent", "done", "w1:p1")
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("classification lost: %v", err)
	}
	if strings.Contains(err.Error(), secretEndpoint) || strings.Contains(logs.String(), secretEndpoint) {
		t.Fatalf("endpoint leaked: err=%q logs=%q", err, logs.String())
	}
	if !strings.Contains(logs.String(), "push.endpoint=\u003credacted\u003e") && !strings.Contains(logs.String(), "push.endpoint=<redacted>") {
		t.Fatalf("redaction field missing: %s", logs.String())
	}
}

type countingConn struct {
	net.Conn
	closes atomic.Int32
	closed chan struct{}
}

func (c *countingConn) Close() error {
	if c.closes.Add(1) == 1 {
		close(c.closed)
	}
	return c.Conn.Close()
}

func TestFocusAPIValidatesAndUsesVerifiedSocketRequest(t *testing.T) {
	svc, _ := testService(t)
	var logs bytes.Buffer
	svc.log = slog.New(slog.NewTextHandler(&logs, nil))
	snapshotClient, snapshotServer := net.Pipe()
	focusClient, focusServer := net.Pipe()
	connections := make(chan net.Conn, 2)
	connections <- snapshotClient
	connections <- focusClient
	svc.dial = func(context.Context, string, string) (net.Conn, error) { return <-connections, nil }
	svc.SetSocketPath("test.sock")
	requests := make(chan map[string]any, 2)
	serve := func(server net.Conn, response string) {
		defer server.Close()
		scan := bufio.NewScanner(server)
		if scan.Scan() {
			var request map[string]any
			_ = json.Unmarshal(scan.Bytes(), &request)
			requests <- request
			_, _ = io.WriteString(server, response+"\n")
		}
	}
	go serve(snapshotServer, `{"id":"focus-snapshot","result":{"type":"session_snapshot","snapshot":{"version":"test","protocol":16,"workspaces":[],"tabs":[],"panes":[{"pane_id":"w1:p2","terminal_id":"t1","workspace_id":"w1","tab_id":"tab1","focused":false,"agent_status":"idle","revision":1}],"layouts":[],"agents":[]}}}`)
	go serve(focusServer, `{"id":"focus-pane","result":{"type":"pane_info","pane":{"pane_id":"w1:p2"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/push/focus", strings.NewReader(`{"pane_id":"w1:p2"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	svc.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	<-requests
	focus := <-requests
	params, _ := focus["params"].(map[string]any)
	if focus["method"] != "pane.focus" || params["pane_id"] != "w1:p2" {
		t.Fatalf("unexpected focus request: %#v", focus)
	}
	if strings.Contains(logs.String(), "w1:p2") || !strings.Contains(logs.String(), "pane.id=<redacted>") {
		t.Fatalf("pane ID telemetry not redacted: %s", logs.String())
	}

	for name, test := range map[string]struct {
		body string
		want int
	}{
		"arbitrary URL": {body: `{"pane_id":"w1:p2","url":"https://evil.example"}`, want: http.StatusBadRequest},
		"invalid pane":  {body: `{"pane_id":"../p2"}`, want: http.StatusBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/push/focus", strings.NewReader(test.body))
			r.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			svc.Handler().ServeHTTP(response, r)
			if response.Code != test.want {
				t.Fatalf("code=%d want=%d", response.Code, test.want)
			}
		})
	}
}

func TestFocusPane_FreshConnection_FocusSucceeds(t *testing.T) {
	svc, _ := testService(t)
	snapshotClient, snapshotServer := net.Pipe()
	focusClient, focusServer := net.Pipe()
	ownedSnapshot := &countingConn{Conn: snapshotClient, closed: make(chan struct{})}
	ownedFocus := &countingConn{Conn: focusClient, closed: make(chan struct{})}
	connections := make(chan net.Conn, 2)
	connections <- ownedSnapshot
	connections <- ownedFocus
	var dials atomic.Int32
	svc.dial = func(context.Context, string, string) (net.Conn, error) {
		if dials.Add(1) == 2 && ownedSnapshot.closes.Load() != 1 {
			t.Fatalf("snapshot close count before focus dial=%d want 1", ownedSnapshot.closes.Load())
		}
		return <-connections, nil
	}
	svc.SetSocketPath("test.sock")

	go func() {
		defer snapshotServer.Close()
		scan := bufio.NewScanner(snapshotServer)
		if scan.Scan() {
			_, _ = io.WriteString(snapshotServer, `{"id":"focus-snapshot","result":{"type":"session_snapshot","snapshot":{"version":"test","protocol":16,"workspaces":[],"tabs":[],"panes":[{"pane_id":"w1:p2"}],"layouts":[],"agents":[]}}}`+"\n")
		}
	}()
	focusRequest := make(chan herdrRequest, 1)
	go func() {
		defer focusServer.Close()
		var request herdrRequest
		if json.NewDecoder(focusServer).Decode(&request) == nil {
			focusRequest <- request
			_, _ = io.WriteString(focusServer, `{"id":"focus-pane","result":{"type":"pane_info","pane":{"pane_id":"w1:p2"}}}`+"\n")
		}
	}()

	if err := svc.focusPane(context.Background(), "w1:p2"); err != nil {
		t.Fatal(err)
	}
	request := <-focusRequest
	if dials.Load() != 2 || request.Method != "pane.focus" || request.Params["pane_id"] != "w1:p2" {
		t.Fatalf("dials=%d request=%#v", dials.Load(), request)
	}
	if snapshotCloses, focusCloses := ownedSnapshot.closes.Load(), ownedFocus.closes.Load(); snapshotCloses != 1 || focusCloses != 1 {
		t.Fatalf("final close counts: snapshot=%d focus=%d want 1 each", snapshotCloses, focusCloses)
	}
}

func TestFocusPane_ContextCanceledDuringFocus_ClosesConnectionOnce(t *testing.T) {
	svc, _ := testService(t)
	snapshotClient, snapshotServer := net.Pipe()
	focusClient, focusServer := net.Pipe()
	ownedFocus := &countingConn{Conn: focusClient, closed: make(chan struct{})}
	connections := make(chan net.Conn, 2)
	connections <- snapshotClient
	connections <- ownedFocus
	svc.dial = func(context.Context, string, string) (net.Conn, error) { return <-connections, nil }
	svc.SetSocketPath("test.sock")
	go func() {
		defer snapshotServer.Close()
		buf := make([]byte, 1024)
		if _, err := snapshotServer.Read(buf); err == nil {
			_, _ = io.WriteString(snapshotServer, `{"id":"focus-snapshot","result":{"type":"session_snapshot","snapshot":{"version":"test","protocol":16,"workspaces":[],"tabs":[],"panes":[{"pane_id":"w1:p2"}],"layouts":[],"agents":[]}}}`+"\n")
		}
	}()
	focusRead := make(chan struct{})
	go func() {
		defer focusServer.Close()
		buf := make([]byte, 1024)
		if _, err := focusServer.Read(buf); err == nil {
			close(focusRead)
			<-ownedFocus.closed
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- svc.focusPane(ctx, "w1:p2") }()
	<-focusRead
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation classification lost: %v", err)
	}
	if got := ownedFocus.closes.Load(); got != 1 {
		t.Fatalf("focus close count=%d want 1", got)
	}
}

func TestFocusAPIRejectsTextPlainBeforeDial(t *testing.T) {
	svc, _ := testService(t)
	var dials atomic.Int32
	svc.dial = func(context.Context, string, string) (net.Conn, error) {
		dials.Add(1)
		return nil, errors.New("must not dial")
	}
	req := httptest.NewRequest(http.MethodPost, "/api/push/focus", strings.NewReader(`{"pane_id":"w1:p2"}`))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	svc.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnsupportedMediaType || dials.Load() != 0 {
		t.Fatalf("code=%d dials=%d", w.Code, dials.Load())
	}
}

func TestFocusPaneRejectsInvalidProtocolFrames(t *testing.T) {
	validSnapshot := `{"id":"focus-snapshot","result":{"type":"session_snapshot","snapshot":{"version":"test","protocol":16,"workspaces":[],"tabs":[],"panes":[{"pane_id":"w1:p2","terminal_id":"t1","workspace_id":"w1","tab_id":"tab1","focused":false,"agent_status":"idle","revision":1}],"layouts":[],"agents":[]}}}`
	tests := map[string]struct {
		snapshot string
		focus    string
	}{
		"snapshot wrong ID":                 {snapshot: `{"id":"other","result":{"type":"session_snapshot","snapshot":{"version":"test","protocol":16,"workspaces":[],"tabs":[],"panes":[],"layouts":[],"agents":[]}}}`},
		"snapshot error":                    {snapshot: `{"id":"focus-snapshot","error":{"code":-1}}`},
		"snapshot missing result":           {snapshot: `{"id":"focus-snapshot"}`},
		"snapshot malformed":                {snapshot: `{`},
		"snapshot discriminator missing":    {snapshot: `{"id":"focus-snapshot","result":{"snapshot":{"panes":[]}}}`},
		"snapshot discriminator wrong":      {snapshot: `{"id":"focus-snapshot","result":{"type":"ok","snapshot":{"panes":[]}}}`},
		"snapshot result non-object":        {snapshot: `{"id":"focus-snapshot","result":42}`},
		"snapshot discriminator wrong type": {snapshot: `{"id":"focus-snapshot","result":{"type":16,"snapshot":{"panes":[]}}}`},
		"snapshot shape missing":            {snapshot: `{"id":"focus-snapshot","result":{"type":"session_snapshot"}}`},
		"snapshot panes missing":            {snapshot: `{"id":"focus-snapshot","result":{"type":"session_snapshot","snapshot":{}}}`},
		"snapshot EOF":                      {},
		"focus wrong ID":                    {snapshot: validSnapshot, focus: `{"id":"other","result":{"type":"pane_info","pane":{"pane_id":"w1:p2"}}}`},
		"focus error":                       {snapshot: validSnapshot, focus: `{"id":"focus-pane","error":{"code":-1}}`},
		"focus missing result":              {snapshot: validSnapshot, focus: `{"id":"focus-pane"}`},
		"focus discriminator missing":       {snapshot: validSnapshot, focus: `{"id":"focus-pane","result":{}}`},
		"focus discriminator wrong":         {snapshot: validSnapshot, focus: `{"id":"focus-pane","result":{"type":"session_snapshot"}}`},
		"focus result non-object":           {snapshot: validSnapshot, focus: `{"id":"focus-pane","result":42}`},
		"focus discriminator wrong type":    {snapshot: validSnapshot, focus: `{"id":"focus-pane","result":{"type":16}}`},
		"focus pane missing":                {snapshot: validSnapshot, focus: `{"id":"focus-pane","result":{"type":"pane_info"}}`},
		"focus pane mismatched":             {snapshot: validSnapshot, focus: `{"id":"focus-pane","result":{"type":"pane_info","pane":{"pane_id":"w1:p3"}}}`},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			svc, _ := testService(t)
			snapshotClient, snapshotServer := net.Pipe()
			connections := make(chan net.Conn, 2)
			connections <- snapshotClient
			servers := []net.Conn{snapshotServer}
			if test.focus != "" {
				focusClient, focusServer := net.Pipe()
				connections <- focusClient
				servers = append(servers, focusServer)
			}
			svc.dial = func(context.Context, string, string) (net.Conn, error) { return <-connections, nil }
			svc.SetSocketPath("test.sock")
			responses := []string{test.snapshot, test.focus}
			for i, server := range servers {
				response := responses[i]
				go func() {
					defer server.Close()
					buf := make([]byte, 1024)
					if _, err := server.Read(buf); err == nil && response != "" {
						_, _ = io.WriteString(server, response+"\n")
					}
				}()
			}
			if err := svc.focusPane(context.Background(), "w1:p2"); err == nil || strings.Contains(err.Error(), "w1:p2") {
				t.Fatalf("unsafe or missing error: %v", err)
			}
		})
	}
}

func TestFocusPaneRejectsOversizedFrameAndHonorsCancellation(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel=%t", cancel), func(t *testing.T) {
			svc, _ := testService(t)
			client, server := net.Pipe()
			svc.dial = func(context.Context, string, string) (net.Conn, error) { return client, nil }
			svc.SetSocketPath("test.sock")
			ctx, stop := context.WithCancel(context.Background())
			defer stop()
			go func() {
				defer server.Close()
				buf := make([]byte, 1024)
				_, _ = server.Read(buf)
				if cancel {
					stop()
					return
				}
				_, _ = io.WriteString(server, strings.Repeat("x", maxHerdrFrame+1)+"\n")
			}()
			err := svc.focusPane(ctx, "w1:p2")
			if cancel && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation classification lost: %v", err)
			}
			if !cancel && err == nil {
				t.Fatal("oversized frame accepted")
			}
		})
	}
}

func TestFocusAPIRejectsMissingPaneBeforeFocus(t *testing.T) {
	svc, _ := testService(t)
	client, server := net.Pipe()
	svc.dial = func(context.Context, string, string) (net.Conn, error) { return client, nil }
	svc.SetSocketPath("test.sock")
	go func() {
		defer server.Close()
		scan := bufio.NewScanner(server)
		if scan.Scan() {
			_, _ = io.WriteString(server, `{"id":"focus-snapshot","result":{"type":"session_snapshot","snapshot":{"version":"test","protocol":16,"workspaces":[],"tabs":[],"panes":[],"layouts":[],"agents":[]}}}`+"\n")
		}
		if scan.Scan() {
			t.Error("focus request sent for missing pane")
		}
	}()
	req := httptest.NewRequest(http.MethodPost, "/api/push/focus", strings.NewReader(`{"pane_id":"stale"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	svc.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestRunEventsOnceRejectsInvalidSeedSnapshot(t *testing.T) {
	validShape := `{"type":"session_snapshot","snapshot":{"version":"test","protocol":16,"workspaces":[],"tabs":[],"panes":[],"layouts":[],"agents":[]}}`
	for name, response := range map[string]string{
		"wrong ID":         `{"id":"other","result":` + validShape + `}`,
		"error envelope":   `{"id":"push-snapshot","error":{"code":-1}}`,
		"missing result":   `{"id":"push-snapshot"}`,
		"malformed":        `{`,
		"wrong type":       `{"id":"push-snapshot","result":{"type":"ok"}}`,
		"unknown protocol": `{"id":"push-snapshot","result":{"type":"session_snapshot","snapshot":{"version":"test","protocol":20,"workspaces":[],"tabs":[],"panes":[],"layouts":[],"agents":[]}}}`,
		"missing panes":    `{"id":"push-snapshot","result":{"type":"session_snapshot","snapshot":{"version":"test","protocol":16,"workspaces":[],"tabs":[],"layouts":[],"agents":[]}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			svc, _ := testService(t)
			client, server := net.Pipe()
			var dials atomic.Int32
			svc.dial = func(context.Context, string, string) (net.Conn, error) {
				dials.Add(1)
				return client, nil
			}
			go func() {
				defer server.Close()
				scan := bufio.NewScanner(server)
				if scan.Scan() {
					_, _ = io.WriteString(server, response+"\n")
				}
			}()
			if err := svc.runEventsOnce(context.Background(), "unused"); err == nil {
				t.Fatal("invalid seed snapshot accepted")
			}
			if got := dials.Load(); got != 1 {
				t.Fatalf("dials=%d want 1", got)
			}
		})
	}
}

func TestPaneExistsRejectsIncompleteReplaySnapshot(t *testing.T) {
	for name, response := range map[string]string{
		"wrong protocol": `{"id":"push-pane-check","result":{"type":"session_snapshot","snapshot":{"version":"test","protocol":15,"workspaces":[],"tabs":[],"panes":[{"pane_id":"p2"}],"layouts":[],"agents":[]}}}`,
		"missing panes":  `{"id":"push-pane-check","result":{"type":"session_snapshot","snapshot":{"version":"test","protocol":16,"workspaces":[],"tabs":[],"layouts":[],"agents":[]}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			svc, _ := testService(t)
			client, server := net.Pipe()
			svc.dial = func(context.Context, string, string) (net.Conn, error) { return client, nil }
			go func() {
				defer server.Close()
				scan := bufio.NewScanner(server)
				if scan.Scan() {
					_, _ = io.WriteString(server, response+"\n")
				}
			}()
			if exists, err := svc.paneExists(context.Background(), "unused", "p2"); err == nil || exists {
				t.Fatalf("incomplete replay snapshot accepted: exists=%t err=%v", exists, err)
			}
		})
	}
}

func TestRunEventsOnceIgnoresStalePaneReplayAndResnapshotsNewPane(t *testing.T) {
	svc, _ := testService(t)
	var logs bytes.Buffer
	svc.log = slog.New(slog.NewTextHandler(&logs, nil))
	snapshotClient, snapshotServer := net.Pipe()
	eventsClient, eventsServer := net.Pipe()
	staleClient, staleServer := net.Pipe()
	newClient, newServer := net.Pipe()
	connections := make(chan net.Conn, 4)
	for _, conn := range []net.Conn{snapshotClient, eventsClient, staleClient, newClient} {
		connections <- conn
	}
	var dials atomic.Int32
	svc.dial = func(context.Context, string, string) (net.Conn, error) {
		dials.Add(1)
		return <-connections, nil
	}
	subscription := make(chan string, 1)
	newPaneRead := make(chan struct{})
	serve := func(server net.Conn, response string) {
		defer server.Close()
		scan := bufio.NewScanner(server)
		if scan.Scan() {
			_, _ = io.WriteString(server, response+"\n")
		}
	}
	go serve(snapshotServer, `{"id":"push-snapshot","result":{"type":"session_snapshot","snapshot":{"version":"test","protocol":19,"workspaces":[],"tabs":[],"panes":[{"pane_id":"p1","agent_status":"working"}],"layouts":[],"agents":[]}}}`)
	go serve(staleServer, `{"id":"push-pane-check","result":{"type":"session_snapshot","snapshot":{"version":"test","protocol":16,"workspaces":[],"tabs":[],"panes":[{"pane_id":"p1"}],"layouts":[],"agents":[]}}}`)
	go serve(newServer, `{"id":"push-pane-check","result":{"type":"session_snapshot","snapshot":{"version":"test","protocol":16,"workspaces":[],"tabs":[],"panes":[{"pane_id":"p1"},{"pane_id":"p2"}],"layouts":[],"agents":[]}}}`)
	go func() {
		defer eventsServer.Close()
		scan := bufio.NewScanner(eventsServer)
		if scan.Scan() {
			subscription <- scan.Text()
			for _, frame := range []string{
				`{"id":"push-events","result":{"type":"subscription_started"}}`,
				`{"event":"pane_created","data":{"pane":{"pane_id":"p1"}}}`,
				`{"event":"pane_created","data":{"pane":{"pane_id":"stale"}}}`,
				`{"event":"pane_created","data":{"pane":{"pane_id":"p2"}}}`,
			} {
				if _, err := io.WriteString(eventsServer, frame+"\n"); err != nil {
					return
				}
			}
			close(newPaneRead)
		}
	}()
	if err := svc.runEventsOnce(context.Background(), "unused"); !errors.Is(err, errPaneSetChanged) {
		t.Fatalf("new pane did not request resnapshot: %v", err)
	}
	select {
	case <-newPaneRead:
	case <-time.After(time.Second):
		t.Fatal("stale pane replay triggered resnapshot before new pane event")
	}
	if got := dials.Load(); got != 4 {
		t.Fatalf("dial count=%d want 4", got)
	}
	if got := logs.String(); !strings.Contains(got, "reason=stale_pane") || !strings.Contains(got, "reason=new_pane") || strings.Contains(got, "p1") || strings.Contains(got, "p2") {
		t.Fatalf("missing or unsafe pane replay diagnostics: %s", got)
	}
	select {
	case request := <-subscription:
		var envelope struct {
			Params struct {
				Subscriptions []map[string]string `json:"subscriptions"`
			} `json:"params"`
		}
		if err := json.Unmarshal([]byte(request), &envelope); err != nil {
			t.Fatal(err)
		}
		if len(envelope.Params.Subscriptions) != 2 || envelope.Params.Subscriptions[0]["type"] != "pane.agent_status_changed" || envelope.Params.Subscriptions[0]["pane_id"] != "p1" || envelope.Params.Subscriptions[1]["type"] != "pane.created" {
			t.Fatalf("unexpected replay-prone lifecycle subscriptions: %s", request)
		}
	case <-time.After(time.Second):
		t.Fatal("subscription request missing")
	}
}

func TestBoundedTelemetrySpansMetricsAndBuckets(t *testing.T) {
	oldTracer, oldMeter := otel.GetTracerProvider(), otel.GetMeterProvider()
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	t.Cleanup(func() {
		otel.SetTracerProvider(oldTracer)
		otel.SetMeterProvider(oldMeter)
		_ = tp.Shutdown(context.Background())
		_ = mp.Shutdown(context.Background())
	})

	svc, store := testService(t)
	if err := store.Put(sub()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Notify(context.Background(), "secret-agent", "done", "secret-pane"); err != nil {
		t.Fatal(err)
	}
	svc.sender = fakeSender{status: http.StatusInternalServerError}
	if err := svc.Notify(context.Background(), "secret-agent", "blocked", "secret-pane"); err == nil {
		t.Fatal("failed HTTP dispatch returned nil")
	}
	svc.sender = fakeSender{status: http.StatusGone}
	if err := svc.Notify(context.Background(), "secret-agent", "done", "secret-pane"); err == nil {
		t.Fatal("stale dispatch returned nil")
	}

	client, server := net.Pipe()
	svc.dial = func(context.Context, string, string) (net.Conn, error) { return client, nil }
	svc.SetSocketPath("test.sock")
	go func() {
		defer server.Close()
		scan := bufio.NewScanner(server)
		if scan.Scan() {
			_, _ = io.WriteString(server, `{"id":"focus-snapshot","result":{"type":"session_snapshot","snapshot":{"version":"test","protocol":16,"workspaces":[],"tabs":[],"panes":[],"layouts":[],"agents":[]}}}`+"\n")
		}
	}()
	req := httptest.NewRequest(http.MethodPost, "/api/push/focus", strings.NewReader(`{"pane_id":"secret-pane"}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	svc.Handler().ServeHTTP(response, req)
	if response.Code != http.StatusNotFound {
		t.Fatalf("focus code=%d", response.Code)
	}

	if err := store.Put(sub()); err != nil {
		t.Fatal(err)
	}
	var eventPayload map[string]any
	svc.sender = fakeSender{status: http.StatusInternalServerError, payload: func(body []byte) {
		if err := json.Unmarshal(body, &eventPayload); err != nil {
			t.Error(err)
		}
	}}
	var eventLogs bytes.Buffer
	svc.log = slog.New(slog.NewTextHandler(&eventLogs, nil))
	eventSnapshotClient, eventSnapshotServer := net.Pipe()
	eventClient, eventServer := net.Pipe()
	labelSnapshotClient, labelSnapshotServer := net.Pipe()
	eventConnections := make(chan net.Conn, 3)
	eventConnections <- eventSnapshotClient
	eventConnections <- eventClient
	eventConnections <- labelSnapshotClient
	svc.dial = func(context.Context, string, string) (net.Conn, error) { return <-eventConnections, nil }
	go func() {
		defer eventSnapshotServer.Close()
		scan := bufio.NewScanner(eventSnapshotServer)
		if scan.Scan() {
			_, _ = io.WriteString(eventSnapshotServer, `{"id":"push-snapshot","result":{"type":"session_snapshot","snapshot":{"version":"test","protocol":16,"workspaces":[],"tabs":[],"panes":[{"pane_id":"secret-pane","agent_status":"working"}],"layouts":[],"agents":[]}}}`+"\n")
		}
	}()
	go func() {
		defer eventServer.Close()
		scan := bufio.NewScanner(eventServer)
		if scan.Scan() {
			_, _ = io.WriteString(eventServer, `{"id":"push-events","result":{"type":"subscription_started"}}`+"\n")
			_, _ = io.WriteString(eventServer, `{"event":"pane.agent_status_changed","data":{"pane_id":"secret-pane","agent_status":"done","agent":"secret-agent"}}`+"\n")
		}
	}()
	go func() {
		defer labelSnapshotServer.Close()
		scan := bufio.NewScanner(labelSnapshotServer)
		if scan.Scan() {
			_, _ = io.WriteString(labelSnapshotServer, `{"id":"push-label-snapshot","result":{"type":"session_snapshot","snapshot":{"version":"test","protocol":16,"workspaces":[{"workspace_id":"w1","label":"Project"}],"tabs":[{"tab_id":"t1","workspace_id":"w1","label":"Current content title","number":1}],"panes":[{"pane_id":"secret-pane","tab_id":"t1","workspace_id":"w1","agent_status":"done"}],"layouts":[],"agents":[]}}}`+"\n")
		}
	}()
	if err := svc.runEventsOnce(context.Background(), "test.sock"); err == nil {
		t.Fatal("closed event stream returned nil")
	}
	if logs := eventLogs.String(); !strings.Contains(logs, "Herdr event subscription started") || !strings.Contains(logs, "Herdr agent event handled") || !strings.Contains(logs, "reason=dispatch") || strings.Contains(logs, "secret-pane") || strings.Contains(logs, "secret-agent") {
		t.Fatalf("missing or unsafe watcher diagnostics: %s", logs)
	}
	if eventPayload["title"] != "Current content title" {
		t.Fatalf("stale notification content title: %#v", eventPayload)
	}
	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/push/subscription", strings.NewReader(`{"endpoint":"https://push.example/x"}`))
	svc.Handler().ServeHTTP(httptest.NewRecorder(), deleteReq)
	body, _ := json.Marshal(subscriptionRequest{Endpoint: sub().Endpoint, Keys: sub().Keys})
	putReq := httptest.NewRequest(http.MethodPut, "/api/push/subscription", bytes.NewReader(body))
	putResponse := httptest.NewRecorder()
	svc.Handler().ServeHTTP(putResponse, putReq)
	if putResponse.Code != http.StatusNoContent {
		t.Fatalf("subscription re-enable code=%d", putResponse.Code)
	}

	ended := recorder.Ended()
	type spanWant struct {
		name   string
		status codes.Code
		attrs  map[string]string
		error  bool
	}
	redactedDispatch := map[string]string{"agent.name": "<redacted>", "push.endpoint": "<redacted>"}
	redactedAttempt := map[string]string{"auth": "<redacted>", "p256dh": "<redacted>", "push.endpoint": "<redacted>"}
	attrs := func(base map[string]string, eventType, outcome, statusClass, errorKind string) map[string]string {
		got := map[string]string{"outcome": outcome, "status_class": statusClass, "error.kind": errorKind}
		if eventType != "" {
			got["event.type"] = eventType
		}
		for key, value := range base {
			got[key] = value
		}
		return got
	}
	wants := []spanWant{
		{name: "push dispatch", attrs: attrs(redactedDispatch, "done", "success", "2xx", "none")},
		{name: "push-service HTTP attempt", attrs: attrs(redactedAttempt, "", "success", "2xx", "none")},
		{name: "push dispatch", status: codes.Error, attrs: attrs(redactedDispatch, "blocked", "failure", "5xx", "http_status"), error: true},
		{name: "push-service HTTP attempt", status: codes.Error, attrs: attrs(redactedAttempt, "", "failure", "5xx", "http_status"), error: true},
		{name: "push dispatch", status: codes.Error, attrs: attrs(redactedDispatch, "done", "stale_pruned", "4xx", "http_status"), error: true},
		{name: "push-service HTTP attempt", status: codes.Error, attrs: attrs(redactedAttempt, "", "failure", "4xx", "http_status"), error: true},
		{name: "pane focus", status: codes.Error, attrs: map[string]string{"pane.id": "<redacted>", "outcome": "not_found", "status_class": "4xx", "error.kind": "not_found"}, error: true},
		{name: "push dispatch", status: codes.Error, attrs: attrs(redactedDispatch, "done", "failure", "5xx", "http_status"), error: true},
		{name: "push-service HTTP attempt", status: codes.Error, attrs: attrs(redactedAttempt, "", "failure", "5xx", "http_status"), error: true},
		{name: "agent event handling", status: codes.Error, attrs: map[string]string{"agent.name": "<redacted>", "pane.id": "<redacted>", "event.type": "done", "outcome": "failure", "status_class": "none", "error.kind": "dispatch"}, error: true},
	}
	if len(ended) != len(wants) {
		t.Fatalf("span count=%d want %d", len(ended), len(wants))
	}
	for _, span := range ended {
		gotAttrs := map[string]string{}
		for _, attr := range span.Attributes() {
			gotAttrs[string(attr.Key)] = attr.Value.AsString()
		}
		matched := -1
		for i, want := range wants {
			if span.Name() != want.name || span.Status().Code != want.status || len(gotAttrs) != len(want.attrs) {
				continue
			}
			equal := true
			for key, value := range want.attrs {
				if gotAttrs[key] != value {
					equal = false
					break
				}
			}
			if equal {
				matched = i
				break
			}
		}
		if matched < 0 {
			t.Fatalf("unexpected span name=%q status=%v attrs=%#v events=%#v", span.Name(), span.Status().Code, gotAttrs, span.Events())
		}
		want := wants[matched]
		wants = append(wants[:matched], wants[matched+1:]...)
		if want.error {
			if gotAttrs["error.kind"] == "none" || len(span.Events()) != 1 || span.Events()[0].Name != "exception" {
				t.Fatalf("failed span %q missing exact error semantics: attrs=%#v events=%#v", span.Name(), gotAttrs, span.Events())
			}
		} else if len(span.Events()) != 0 || gotAttrs["error.kind"] != "none" {
			t.Fatalf("successful span %q has error semantics: attrs=%#v events=%#v", span.Name(), gotAttrs, span.Events())
		}
	}
	if len(wants) != 0 {
		t.Fatalf("missing spans: %#v", wants)
	}

	var metrics metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &metrics); err != nil {
		t.Fatal(err)
	}
	wanted := map[string]bool{"web_push.subscription.mutations": true, "web_push.attempts": true, "web_push.pane_focus.attempts": true, "web_push.latency": true, "web_push.payload.bytes": true, "web_push.subscription.active": true}
	found := map[string]bool{}
	for _, scope := range metrics.ScopeMetrics {
		for _, m := range scope.Metrics {
			if !wanted[m.Name] {
				t.Errorf("unexpected metric %q", m.Name)
			}
			found[m.Name] = true
			switch data := m.Data.(type) {
			case metricdata.Histogram[float64]:
				if m.Name == "web_push.latency" && !slices.Equal(data.DataPoints[0].Bounds, []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}) {
					t.Errorf("latency bounds=%v", data.DataPoints[0].Bounds)
				}
			case metricdata.Histogram[int64]:
				if m.Name == "web_push.payload.bytes" && !slices.Equal(data.DataPoints[0].Bounds, []float64{256, 1024, 4096, 16384, 65536}) {
					t.Errorf("payload bounds=%v", data.DataPoints[0].Bounds)
				}
			case metricdata.Sum[int64]:
				for _, point := range data.DataPoints {
					for _, attr := range point.Attributes.ToSlice() {
						value := attr.Value.AsString()
						switch attr.Key {
						case "event.type":
							if value != "done" && value != "blocked" {
								t.Errorf("metric %q has unbounded event.type=%q", m.Name, value)
							}
						case "outcome":
							allowed := map[string]bool{"enabled": true, "disabled": true, "success": true, "failure": true, "stale_pruned": true, "stale_replaced": true, "stale_prune_failed": true, "not_found": true}
							if !allowed[value] {
								t.Errorf("metric %q has unbounded outcome=%q", m.Name, value)
							}
						case "status_class":
							if !map[string]bool{"none": true, "2xx": true, "4xx": true, "5xx": true}[value] {
								t.Errorf("metric %q has unbounded status_class=%q", m.Name, value)
							}
						default:
							t.Errorf("metric %q has forbidden attribute %q", m.Name, attr.Key)
						}
					}
				}
			case metricdata.Gauge[int64]:
				if m.Name == "web_push.subscription.active" && (len(data.DataPoints) != 1 || data.DataPoints[0].Value != 1) {
					t.Errorf("active gauge=%#v", data.DataPoints)
				}
			}
		}
	}
	for _, name := range []string{"web_push.subscription.mutations", "web_push.attempts", "web_push.pane_focus.attempts", "web_push.latency", "web_push.payload.bytes", "web_push.subscription.active"} {
		if !found[name] {
			t.Errorf("missing metric %q", name)
		}
	}
}

func TestRunEventsOnceCancellationClosesOwnedConnections(t *testing.T) {
	for _, phase := range []string{"snapshot scan", "subscription scan"} {
		t.Run(phase, func(t *testing.T) {
			svc, _ := testService(t)
			snapshotClient, snapshotServer := net.Pipe()
			eventsClient, eventsServer := net.Pipe()
			t.Cleanup(func() {
				_ = snapshotServer.Close()
				_ = eventsServer.Close()
			})
			snapshotConn := &countingConn{Conn: snapshotClient, closed: make(chan struct{})}
			eventsConn := &countingConn{Conn: eventsClient, closed: make(chan struct{})}
			connections := make(chan net.Conn, 2)
			connections <- snapshotConn
			connections <- eventsConn
			svc.dial = func(context.Context, string, string) (net.Conn, error) { return <-connections, nil }
			blocked := make(chan struct{})
			go func() {
				scan := bufio.NewScanner(snapshotServer)
				if !scan.Scan() {
					return
				}
				if phase == "snapshot scan" {
					close(blocked)
					return
				}
				_, _ = io.WriteString(snapshotServer, `{"id":"push-snapshot","result":{"type":"session_snapshot","snapshot":{"version":"test","protocol":16,"workspaces":[],"tabs":[],"panes":[],"layouts":[],"agents":[]}}}`+"\n")
			}()
			if phase == "subscription scan" {
				go func() {
					scan := bufio.NewScanner(eventsServer)
					if scan.Scan() {
						close(blocked)
					}
				}()
			}
			ctx, cancel := context.WithCancel(context.Background())
			result := make(chan error, 1)
			go func() { result <- svc.runEventsOnce(ctx, "unused") }()
			select {
			case <-blocked:
			case <-time.After(time.Second):
				t.Fatalf("runEventsOnce did not block in %s", phase)
			}
			cancel()
			select {
			case err := <-result:
				if err == nil {
					t.Fatal("cancellation returned nil error")
				}
			case <-time.After(time.Second):
				t.Fatal("runEventsOnce did not exit promptly after cancellation")
			}
			owned := map[string]*countingConn{"snapshot": snapshotConn}
			if phase == "subscription scan" {
				owned["subscription"] = eventsConn
			}
			for name, conn := range owned {
				select {
				case <-conn.closed:
				case <-time.After(time.Second):
					t.Fatalf("%s connection did not close", name)
				}
				if got := conn.closes.Load(); got != 1 {
					t.Fatalf("%s close count=%d want 1", name, got)
				}
			}
		})
	}
}

func TestEventConnCloseStopsCancellationHook(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = server.Close() })
	conn := &countingConn{Conn: client, closed: make(chan struct{})}
	stopped := false
	owned := &eventConn{Conn: conn, stopClose: func() bool {
		stopped = true
		return true
	}}

	if err := owned.Close(); err != nil {
		t.Fatal(err)
	}
	if !stopped {
		t.Fatal("Close did not stop cancellation hook")
	}
	if got := conn.closes.Load(); got != 1 {
		t.Fatalf("close count=%d want 1", got)
	}
}

func TestRunEventsOnceClosesConnectionsOnStreamEnd(t *testing.T) {
	svc, _ := testService(t)
	snapshotClient, snapshotServer := net.Pipe()
	eventsClient, eventsServer := net.Pipe()
	snapshotConn := &countingConn{Conn: snapshotClient, closed: make(chan struct{})}
	eventsConn := &countingConn{Conn: eventsClient, closed: make(chan struct{})}
	connections := make(chan net.Conn, 2)
	connections <- snapshotConn
	connections <- eventsConn
	svc.dial = func(context.Context, string, string) (net.Conn, error) { return <-connections, nil }
	go func() {
		defer snapshotServer.Close()
		scan := bufio.NewScanner(snapshotServer)
		if scan.Scan() {
			_, _ = io.WriteString(snapshotServer, `{"id":"push-snapshot","result":{"type":"session_snapshot","snapshot":{"version":"test","protocol":16,"workspaces":[],"tabs":[],"panes":[],"layouts":[],"agents":[]}}}`+"\n")
		}
	}()
	go func() {
		defer eventsServer.Close()
		scan := bufio.NewScanner(eventsServer)
		_ = scan.Scan()
	}()
	if err := svc.runEventsOnce(context.Background(), "unused"); err == nil {
		t.Fatal("expected closed stream error")
	}
	for name, conn := range map[string]*countingConn{"snapshot": snapshotConn, "subscription": eventsConn} {
		if got := conn.closes.Load(); got != 1 {
			t.Fatalf("%s close count=%d want 1", name, got)
		}
	}
}
