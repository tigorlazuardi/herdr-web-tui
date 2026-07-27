// Package push implements one-user Web Push subscription storage and dispatch.
package push

import (
	"bufio"
	"context"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/go-faster/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
)

const (
	maxBody       = 16 << 10
	maxFocusBody  = 1024
	maxPaneID     = 128
	maxHerdrFrame = 1 << 20
)

// Config holds fixed server-side Web Push settings. PrivateKey is secret.
type Config struct{ PublicKey, PrivateKey, Subject, StorePath string }

// ConfigFromEnv reads Web Push configuration. All-empty disables push; partial config fails closed.
func ConfigFromEnv() (Config, error) {
	c := Config{PublicKey: os.Getenv("VAPID_PUBLIC_KEY"), PrivateKey: os.Getenv("VAPID_PRIVATE_KEY"), Subject: os.Getenv("VAPID_SUBJECT"), StorePath: os.Getenv("WEB_PUSH_STORE_PATH")}
	if c.StorePath == "" {
		c.StorePath = "./web-push-subscription.json"
	}
	if c.PublicKey == "" && c.PrivateKey == "" && c.Subject == "" {
		return Config{StorePath: c.StorePath}, nil
	}
	if c.PublicKey == "" || c.PrivateKey == "" || c.Subject == "" {
		return Config{}, errors.New("VAPID_PUBLIC_KEY, VAPID_PRIVATE_KEY, and VAPID_SUBJECT must all be set")
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(c.PublicKey)
	if err != nil || len(publicKey) != 65 {
		return Config{}, errors.New("VAPID_PUBLIC_KEY must be a URL-safe base64 P-256 public key")
	}
	publicX, publicY := elliptic.Unmarshal(elliptic.P256(), publicKey)
	if publicX == nil || publicY == nil {
		return Config{}, errors.New("VAPID_PUBLIC_KEY must contain valid P-256 key material")
	}
	privateKey, err := base64.RawURLEncoding.DecodeString(c.PrivateKey)
	if err != nil || len(privateKey) != 32 || new(big.Int).SetBytes(privateKey).Sign() == 0 || new(big.Int).SetBytes(privateKey).Cmp(elliptic.P256().Params().N) >= 0 {
		return Config{}, errors.New("VAPID_PRIVATE_KEY must be a URL-safe base64 P-256 private key")
	}
	x, y := elliptic.P256().ScalarBaseMult(privateKey)
	if x.Cmp(publicX) != 0 || y.Cmp(publicY) != 0 {
		return Config{}, errors.New("VAPID public and private keys do not match")
	}
	u, err := url.Parse(c.Subject)
	validMailto := err == nil && u.Scheme == "mailto" && u.Opaque != "" && u.Host == "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
	validHTTPS := err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.Fragment == ""
	if !validMailto && !validHTTPS {
		return Config{}, errors.New("VAPID_SUBJECT must be a non-empty mailto: or https: URI")
	}
	return c, nil
}

// Enabled reports whether complete VAPID configuration is present.
func (c Config) Enabled() bool { return c.PublicKey != "" }

type record struct {
	User         string               `json:"user"`
	Subscription webpush.Subscription `json:"subscription"`
}

// Store atomically persists one authenticated user's subscription.
type Store struct {
	path   string
	mu     sync.RWMutex
	rec    *record
	remove func(string) error
}

// OpenStore loads a subscription store or creates an empty in-memory view when absent.
func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, remove: os.Remove}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "read push subscription")
	}
	if err := json.Unmarshal(b, &s.rec); err != nil {
		return nil, errors.Wrap(err, "decode push subscription")
	}
	if s.rec == nil || s.rec.User == "" {
		return nil, errors.New("invalid push subscription store")
	}
	return s, nil
}

// Get returns a snapshot safe from later store replacement.
func (s *Store) Get() *record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.rec == nil {
		return nil
	}
	r := *s.rec
	return &r
}

// Put atomically replaces this user's subscription without allowing another identity.
func (s *Store) Put(user string, sub webpush.Subscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rec != nil && s.rec.User != user {
		return ErrDifferentUser
	}
	r := &record{User: user, Subscription: sub}
	b, err := json.Marshal(r)
	if err != nil {
		return errors.Wrap(err, "encode push subscription")
	}
	if err = os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return errors.Wrap(err, "create push store directory")
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".push-*")
	if err != nil {
		return errors.Wrap(err, "create push store temp")
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(b)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, s.path)
	}
	if err != nil {
		return errors.Wrap(err, "persist push subscription")
	}
	s.rec = r
	return nil
}

// Delete removes this user's current subscription.
func (s *Store) Delete(user string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rec == nil {
		return nil
	}
	if s.rec.User != user {
		return ErrDifferentUser
	}
	if err := s.remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Wrap(err, "remove push subscription")
	}
	s.rec = nil
	return nil
}

// DeleteIfMatch atomically removes only the exact subscription used by a stale dispatch.
func (s *Store) DeleteIfMatch(snapshot *record) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if snapshot == nil || s.rec == nil || s.rec.User != snapshot.User || s.rec.Subscription != snapshot.Subscription {
		return false, nil
	}
	if err := s.remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, errors.Wrap(err, "remove stale push subscription")
	}
	s.rec = nil
	return true, nil
}

// ErrDifferentUser prevents cross-identity replacement in single-user deployments.
var ErrDifferentUser = errors.New("another authenticated user already owns the active subscription")

// Sender is the narrow outbound seam used by dispatch tests.
type Sender interface {
	Send(context.Context, []byte, *webpush.Subscription, *webpush.Options) (*http.Response, error)
}

type webSender struct{ client *http.Client }

func (s webSender) Send(ctx context.Context, p []byte, sub *webpush.Subscription, o *webpush.Options) (*http.Response, error) {
	o.HTTPClient = s.client
	return webpush.SendNotificationWithContext(ctx, p, sub, o)
}

// Service owns API, persistence, dispatch, and bounded telemetry.
type Service struct {
	cfg                Config
	store              *Store
	log                *slog.Logger
	sender             Sender
	dial               func(context.Context, string, string) (net.Conn, error)
	socketPath         string
	mutations          metric.Int64Counter
	attempts           metric.Int64Counter
	latency            metric.Float64Histogram
	payload            metric.Int64Histogram
	activeRegistration metric.Registration
	focusAttempts      metric.Int64Counter
}

// NewService builds a service whose outbound transport rejects redirects and non-public dial targets.
func NewService(c Config, store *Store, log *slog.Logger) *Service {
	return NewServiceWithSender(c, store, log, webSender{client: publicHTTPClient()})
}

// SetSocketPath sets fixed Herdr socket used by event subscription and pane focus.
func (s *Service) SetSocketPath(path string) { s.socketPath = path }

// NewServiceWithSender builds a service with a testable outbound sender.
func NewServiceWithSender(c Config, store *Store, log *slog.Logger, sender Sender) *Service {
	m := otel.Meter("herdr-web-tui/push")
	mutations, _ := m.Int64Counter("web_push.subscription.mutations")
	attempts, _ := m.Int64Counter("web_push.attempts")
	latency, _ := m.Float64Histogram("web_push.latency", metric.WithUnit("ms"), metric.WithExplicitBucketBoundaries(5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000))
	payload, _ := m.Int64Histogram("web_push.payload.bytes", metric.WithExplicitBucketBoundaries(256, 1024, 4096, 16384, 65536))
	active, _ := m.Int64ObservableGauge("web_push.subscription.active")
	focusAttempts, _ := m.Int64Counter("web_push.pane_focus.attempts")
	svc := &Service{cfg: c, store: store, log: log, sender: sender, dial: (&net.Dialer{}).DialContext, mutations: mutations, attempts: attempts, latency: latency, payload: payload, focusAttempts: focusAttempts}
	svc.activeRegistration, _ = m.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		if store.Get() != nil {
			observer.ObserveInt64(active, 1)
		} else {
			observer.ObserveInt64(active, 0)
		}
		return nil
	}, active)
	return svc
}

// user accepts identity only from gateway-owned Remote-User.
func user(r *http.Request) (string, bool) {
	u := strings.TrimSpace(r.Header.Get("Remote-User"))
	return u, u != ""
}

type subscriptionRequest struct {
	Endpoint       string       `json:"endpoint"`
	ExpirationTime *json.Number `json:"expirationTime"`
	Keys           webpush.Keys `json:"keys"`
}

// validSubscription validates browser key material before persistence; dial-time DNS is revalidated separately.
func validSubscription(s webpush.Subscription) bool {
	u, err := url.Parse(s.Endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return false
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil && !isPublicAddr(ip) {
		return false
	}
	auth, err := base64.RawURLEncoding.DecodeString(s.Keys.Auth)
	if err != nil || len(auth) != 16 {
		return false
	}
	key, err := base64.RawURLEncoding.DecodeString(s.Keys.P256dh)
	if err != nil || len(key) != 65 {
		return false
	}
	x, y := elliptic.Unmarshal(elliptic.P256(), key)
	return x != nil && y != nil
}

var publicIPv6Prefix = netip.MustParsePrefix("2000::/3")

var nonPublicPrefixes = []netip.Prefix{
	// IANA IPv4 Special-Purpose Address Registry plus multicast.
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.31.196.0/24"), netip.MustParsePrefix("192.52.193.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("192.175.48.0/24"), netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"), netip.MustParsePrefix("240.0.0.0/4"),
	// IANA IPv6 Special-Purpose Address Registry plus deprecated site-local and multicast.
	netip.MustParsePrefix("::/96"), netip.MustParsePrefix("::ffff:0:0:0/96"),
	netip.MustParsePrefix("64:ff9b::/96"), netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"), netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("2620:4f:8000::/48"), netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("5f00::/16"), netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"), netip.MustParsePrefix("fec0::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

// isPublicAddr allows only globally routable addresses, excluding IANA special-purpose space.
func isPublicAddr(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || !ip.IsGlobalUnicast() {
		return false
	}
	// ponytail: IPv6 public allocation is currently 2000::/3; update this gate if IANA allocates another public prefix.
	if ip.Is6() && !publicIPv6Prefix.Contains(ip) {
		return false
	}
	for _, prefix := range nonPublicPrefixes {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

type lookupNetIP func(context.Context, string, string) ([]netip.Addr, error)
type dialContext func(context.Context, string, string) (net.Conn, error)

// publicDialContext validates every resolved address before dialing one, preventing mixed-answer DNS rebinding bypasses.
func publicDialContext(lookup lookupNetIP, dial dialContext) dialContext {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, errors.Wrap(err, "parse push service address")
		}
		addrs, err := lookup(ctx, "ip", host)
		if err != nil {
			return nil, errors.Wrap(err, "resolve push service")
		}
		for _, ip := range addrs {
			if !isPublicAddr(ip.Unmap()) {
				return nil, errors.New("push service resolved to non-public address")
			}
		}
		if len(addrs) == 0 {
			return nil, errors.New("push service resolved no addresses")
		}
		return dial(ctx, network, net.JoinHostPort(addrs[0].String(), port))
	}
}

// publicHTTPClient revalidates DNS results at every transport dial and disables credential-bearing redirects.
func publicHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = publicDialContext(net.DefaultResolver.LookupNetIP, dialer.DialContext)
	return &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// Handler serves minimal authenticated subscription lifecycle API.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/push/config", s.config)
	mux.HandleFunc("PUT /api/push/subscription", s.put)
	mux.HandleFunc("DELETE /api/push/subscription", s.del)
	mux.HandleFunc("POST /api/push/focus", s.focus)
	return mux
}

// config returns public browser configuration without secret subscription material.
func (s *Service) config(w http.ResponseWriter, r *http.Request) {
	u, ok := user(r)
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	rec := s.store.Get()
	if rec != nil && rec.User != u {
		http.Error(w, "different authenticated user owns subscription", http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"publicKey": s.cfg.PublicKey, "enabled": rec != nil})
}

// put validates exactly one bounded subscription document before atomic persistence.
func (s *Service) put(w http.ResponseWriter, r *http.Request) {
	u, ok := user(r)
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	var input subscriptionRequest
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&input); err != nil {
		http.Error(w, "invalid push subscription", http.StatusBadRequest)
		return
	}
	var trailing any
	if err := d.Decode(&trailing); err != io.EOF {
		http.Error(w, "invalid push subscription", http.StatusBadRequest)
		return
	}
	sub := webpush.Subscription{Endpoint: input.Endpoint, Keys: input.Keys}
	if !validSubscription(sub) {
		http.Error(w, "invalid push subscription", http.StatusBadRequest)
		return
	}
	if err := s.store.Put(u, sub); err != nil {
		if errors.Is(err, ErrDifferentUser) {
			http.Error(w, err.Error(), http.StatusConflict)
		} else {
			http.Error(w, "could not persist subscription", http.StatusInternalServerError)
		}
		return
	}
	s.mutations.Add(r.Context(), 1, metric.WithAttributes(attribute.String("outcome", "enabled")))
	s.log.InfoContext(r.Context(), "push subscription enabled", "push.endpoint", "<redacted>")
	w.WriteHeader(http.StatusNoContent)
}

type focusRequest struct {
	PaneID string `json:"pane_id"`
}

// validPaneID accepts Herdr's bounded opaque pane identifiers without permitting control bytes or paths.
func validPaneID(id string) bool {
	if id == "" || len(id) > maxPaneID {
		return false
	}
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == ':' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// focus validates authenticated notification input, verifies pane existence, then focuses through protocol-16 socket API.
func (s *Service) focus(w http.ResponseWriter, r *http.Request) {
	if _, ok := user(r); !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxFocusBody)
	var input focusRequest
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&input); err != nil || !validPaneID(input.PaneID) {
		http.Error(w, "invalid pane focus request", http.StatusBadRequest)
		return
	}
	var trailing any
	if err := d.Decode(&trailing); err != io.EOF {
		http.Error(w, "invalid pane focus request", http.StatusBadRequest)
		return
	}
	ctx, span := otel.Tracer("herdr-web-tui/push").Start(r.Context(), "pane focus")
	defer span.End()
	span.SetAttributes(attribute.String("pane.id", "<redacted>"))
	outcome, errorKind, status := "success", "none", http.StatusNoContent
	if err := s.focusPane(ctx, input.PaneID); err != nil {
		errorKind = focusErrorKind(err)
		if errors.Is(err, errPaneNotFound) {
			outcome, status = "not_found", http.StatusNotFound
			http.Error(w, "pane no longer exists", status)
		} else {
			outcome, status = "failure", http.StatusBadGateway
			http.Error(w, "could not focus pane", status)
		}
		span.RecordError(errors.New("Herdr pane focus failed: " + errorKind))
		span.SetStatus(codes.Error, "pane focus failed")
	} else {
		w.WriteHeader(status)
	}
	statusClass := fmt.Sprintf("%dxx", status/100)
	span.SetAttributes(attribute.String("outcome", outcome), attribute.String("status_class", statusClass), attribute.String("error.kind", errorKind))
	s.focusAttempts.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", outcome)))
	s.log.InfoContext(ctx, "notification pane focus", "outcome", outcome, "error.kind", errorKind, "pane.id", "<redacted>")
}

var (
	errPaneNotFound  = errors.New("pane no longer exists")
	errHerdrProtocol = errors.New("invalid Herdr protocol response")
)

// focusErrorKind returns bounded telemetry classification without dependency details or pane IDs.
func focusErrorKind(err error) string {
	switch {
	case errors.Is(err, errPaneNotFound):
		return "not_found"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "canceled"
	case errors.Is(err, errHerdrProtocol):
		return "protocol"
	default:
		return "dependency"
	}
}

type herdrRequest struct {
	ID     string         `json:"id"`
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

type herdrResponse struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}

type focusSnapshotResult struct {
	Type     string `json:"type"`
	Snapshot *struct {
		Version    *string           `json:"version"`
		Protocol   *uint32           `json:"protocol"`
		Workspaces []json.RawMessage `json:"workspaces"`
		Tabs       []json.RawMessage `json:"tabs"`
		Panes      []struct {
			PaneID string `json:"pane_id"`
		} `json:"panes"`
		Layouts []json.RawMessage `json:"layouts"`
		Agents  []json.RawMessage `json:"agents"`
	} `json:"snapshot"`
}

type focusOKResult struct {
	Type string `json:"type"`
}

// readHerdrResponse decodes one bounded response and verifies correlation, protocol status, and required result.
func readHerdrResponse(scan *bufio.Scanner, expectedID string, result any) error {
	if !scan.Scan() {
		if err := scan.Err(); err != nil {
			return errors.Wrap(err, "read Herdr response")
		}
		return errors.New("Herdr response missing")
	}
	var response herdrResponse
	if err := json.Unmarshal(scan.Bytes(), &response); err != nil {
		return errors.Wrap(errHerdrProtocol, "malformed envelope")
	}
	if response.ID != expectedID {
		return errors.Wrap(errHerdrProtocol, "unexpected response ID")
	}
	if len(response.Error) != 0 && string(response.Error) != "null" {
		return errors.Wrap(errHerdrProtocol, "error envelope")
	}
	if len(response.Result) == 0 || string(response.Result) == "null" {
		return errors.Wrap(errHerdrProtocol, "result missing")
	}
	if result != nil {
		if err := json.Unmarshal(response.Result, result); err != nil {
			return errors.Wrap(errHerdrProtocol, "malformed result")
		}
	}
	return nil
}

// focusPane uses one raw Herdr socket and never interpolates paneID into shell or method names.
func (s *Service) focusPane(ctx context.Context, paneID string) error {
	path := s.socketPath
	if path == "" {
		path = os.Getenv("HERDR_SOCKET_PATH")
	}
	if path == "" {
		return errors.New("HERDR_SOCKET_PATH is required")
	}
	conn, err := s.dial(ctx, "unix", path)
	if err != nil {
		return errors.Wrap(err, "connect Herdr socket")
	}
	defer conn.Close()
	stopClose := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopClose()
	if err := json.NewEncoder(conn).Encode(herdrRequest{ID: "focus-snapshot", Method: "session.snapshot", Params: map[string]any{}}); err != nil {
		return errors.Wrap(err, "request Herdr snapshot")
	}
	scan := bufio.NewScanner(conn)
	scan.Buffer(make([]byte, 4096), maxHerdrFrame)
	var snap focusSnapshotResult
	if err := readHerdrResponse(scan, "focus-snapshot", &snap); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.Wrap(err, "receive Herdr snapshot")
	}
	if snap.Type != "session_snapshot" {
		return errors.Wrap(errHerdrProtocol, "unexpected snapshot result type")
	}
	if snap.Snapshot == nil || snap.Snapshot.Version == nil || snap.Snapshot.Protocol == nil || *snap.Snapshot.Protocol != 16 || snap.Snapshot.Workspaces == nil || snap.Snapshot.Tabs == nil || snap.Snapshot.Panes == nil || snap.Snapshot.Layouts == nil || snap.Snapshot.Agents == nil {
		return errors.Wrap(errHerdrProtocol, "snapshot result missing required shape")
	}
	found := false
	for _, pane := range snap.Snapshot.Panes {
		if pane.PaneID == paneID {
			found = true
			break
		}
	}
	if !found {
		return errPaneNotFound
	}
	if err := json.NewEncoder(conn).Encode(herdrRequest{ID: "focus-pane", Method: "pane.focus", Params: map[string]any{"pane_id": paneID}}); err != nil {
		return errors.Wrap(err, "request Herdr pane focus")
	}
	var focused focusOKResult
	if err := readHerdrResponse(scan, "focus-pane", &focused); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.Wrap(err, "receive Herdr pane focus result")
	}
	if focused.Type != "ok" {
		return errors.Wrap(errHerdrProtocol, "unexpected focus result type")
	}
	return nil
}

// del removes only the authenticated user's subscription.
func (s *Service) del(w http.ResponseWriter, r *http.Request) {
	u, ok := user(r)
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	if err := s.store.Delete(u); err != nil {
		if errors.Is(err, ErrDifferentUser) {
			http.Error(w, err.Error(), http.StatusConflict)
		} else {
			http.Error(w, "could not remove subscription", http.StatusInternalServerError)
		}
		return
	}
	s.mutations.Add(r.Context(), 1, metric.WithAttributes(attribute.String("outcome", "disabled")))
	s.log.InfoContext(r.Context(), "push subscription disabled")
	w.WriteHeader(http.StatusNoContent)
}

type sanitizedSendError struct {
	kind  string
	cause error
}

func (e *sanitizedSendError) Error() string { return "web push transport failed: " + e.kind }
func (e *sanitizedSendError) Unwrap() error { return e.cause }

// sanitizeSendError preserves errors.Is/errors.As classification while removing attacker-controlled URL text.
func sanitizeSendError(err error) error {
	if err == nil {
		return nil
	}
	kind := "transport"
	switch {
	case errors.Is(err, context.Canceled):
		kind = "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		kind = "timeout"
	default:
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			kind = "timeout"
		}
	}
	return &sanitizedSendError{kind: kind, cause: err}
}

// Notify sends bounded JSON notification payload and atomically prunes only its stale snapshot.
func (s *Service) Notify(ctx context.Context, agent, state, paneID string) error {
	if state != "done" && state != "blocked" {
		return nil
	}
	rec := s.store.Get()
	if rec == nil {
		return nil
	}
	ctx, span := otel.Tracer("herdr-web-tui/push").Start(ctx, "push dispatch")
	defer span.End()
	span.SetAttributes(attribute.String("event.type", state), attribute.String("agent.name", "<redacted>"), attribute.String("push.endpoint", "<redacted>"))
	payload, err := json.Marshal(map[string]string{"title": "Herdr agent " + state, "body": agent, "state": state, "pane_id": paneID})
	if err != nil {
		return errors.Wrap(err, "encode push payload")
	}
	if len(payload) > 4096 {
		err := errors.New("push payload exceeds bounded size")
		span.SetAttributes(attribute.String("outcome", "failure"), attribute.String("status_class", "none"), attribute.String("error.kind", "payload_size"))
		span.RecordError(err)
		span.SetStatus(codes.Error, "push dispatch failed")
		return err
	}
	s.payload.Record(ctx, int64(len(payload)))
	attemptCtx, attemptSpan := otel.Tracer("herdr-web-tui/push").Start(ctx, "push-service HTTP attempt")
	attemptSpan.SetAttributes(attribute.String("push.endpoint", "<redacted>"), attribute.String("auth", "<redacted>"), attribute.String("p256dh", "<redacted>"))
	start := time.Now()
	resp, sendErr := s.sender.Send(attemptCtx, payload, &rec.Subscription, &webpush.Options{Subscriber: strings.TrimPrefix(s.cfg.Subject, "mailto:"), VAPIDPublicKey: s.cfg.PublicKey, VAPIDPrivateKey: s.cfg.PrivateKey, TTL: 60})
	safeErr := sanitizeSendError(sendErr)
	ms := float64(time.Since(start).Microseconds()) / 1000
	s.latency.Record(ctx, ms)
	outcome, statusClass, errorKind := "success", "none", "none"
	if resp != nil {
		defer resp.Body.Close()
		statusClass = fmt.Sprintf("%dxx", resp.StatusCode/100)
	}
	if safeErr != nil {
		outcome, errorKind = "failure", safeErr.(*sanitizedSendError).kind
		attemptSpan.RecordError(safeErr)
		attemptSpan.SetStatus(codes.Error, "push failed")
	} else if resp == nil {
		outcome, errorKind = "failure", "missing_response"
		err := errors.New("web push transport returned no response")
		attemptSpan.RecordError(err)
		attemptSpan.SetStatus(codes.Error, "push failed")
	} else if resp.StatusCode >= 300 {
		outcome, errorKind = "failure", "http_status"
		err := errors.New("push service returned unsuccessful status class")
		attemptSpan.RecordError(err)
		attemptSpan.SetStatus(codes.Error, "push failed")
	}
	attemptSpan.SetAttributes(attribute.String("outcome", outcome), attribute.String("status_class", statusClass), attribute.String("error.kind", errorKind))
	attemptSpan.End()
	if resp != nil && (resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone) {
		pruned, deleteErr := s.store.DeleteIfMatch(rec)
		if deleteErr != nil {
			outcome, errorKind = "stale_prune_failed", "persistence"
			s.log.ErrorContext(ctx, "stale push subscription removal failed", "error", deleteErr, "push.endpoint", "<redacted>")
		} else if pruned {
			outcome = "stale_pruned"
			s.log.WarnContext(ctx, "stale push subscription pruned", "push.endpoint", "<redacted>")
		} else {
			outcome = "stale_replaced"
		}
	}
	if outcome != "success" {
		spanErr := errors.New("web push dispatch failed: " + errorKind)
		span.RecordError(spanErr)
		span.SetStatus(codes.Error, "push dispatch failed")
	}
	span.SetAttributes(attribute.String("outcome", outcome), attribute.String("status_class", statusClass), attribute.String("error.kind", errorKind))
	attrs := metric.WithAttributes(attribute.String("event.type", state), attribute.String("outcome", outcome), attribute.String("status_class", statusClass))
	s.attempts.Add(ctx, 1, attrs)
	s.log.InfoContext(ctx, "push dispatch", "outcome", outcome, "status_class", statusClass, "agent.name", "<redacted>", "push.endpoint", "<redacted>")
	if safeErr != nil {
		return safeErr
	}
	if resp == nil {
		return errors.New("web push transport returned no response")
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("push service returned status class %s", statusClass)
	}
	return nil
}
