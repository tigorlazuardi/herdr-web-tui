// Package push implements global Web Push subscription storage and dispatch.
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

type storeFile struct {
	Subscriptions []webpush.Subscription `json:"subscriptions"`
}

type legacyStoreFile struct {
	Subscription webpush.Subscription `json:"subscription"`
}

// Store atomically persists a global endpoint-deduplicated subscription list.
type Store struct {
	path          string
	mu            sync.RWMutex
	subscriptions []webpush.Subscription
	legacyRaw     []byte
	remove        func(string) error
}

// OpenStore loads current subscription storage plus deployed v1 single-subscription shape.
func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, remove: os.Remove}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "read push subscriptions")
	}
	var current storeFile
	if err := json.Unmarshal(b, &current); err == nil && current.Subscriptions != nil {
		for _, sub := range current.Subscriptions {
			if !validSubscription(sub) {
				return nil, errors.New("invalid push subscription store")
			}
			s.upsert(sub)
		}
		return s, nil
	}
	var legacy legacyStoreFile
	if err := json.Unmarshal(b, &legacy); err != nil || !validSubscription(legacy.Subscription) {
		return nil, errors.New("invalid push subscription store")
	}
	s.subscriptions = []webpush.Subscription{legacy.Subscription}
	s.legacyRaw = append([]byte(nil), b...)
	return s, nil
}

func (s *Store) upsert(sub webpush.Subscription) {
	for i := range s.subscriptions {
		if s.subscriptions[i].Endpoint == sub.Endpoint {
			s.subscriptions[i] = sub
			return
		}
	}
	s.subscriptions = append(s.subscriptions, sub)
}

// Get returns a subscription snapshot safe from later mutations.
func (s *Store) Get() []webpush.Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]webpush.Subscription(nil), s.subscriptions...)
}

func syncParentDirectory(path string) error {
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	if err = dir.Sync(); err != nil {
		_ = dir.Close()
		return err
	}
	return dir.Close()
}

// ensureLegacyBackup preserves the deployed single-subscription file before its first one-way rewrite.
func (s *Store) ensureLegacyBackup() error {
	if len(s.legacyRaw) == 0 {
		return nil
	}
	backup := s.path + ".legacy-v1.bak"
	if existing, err := os.ReadFile(backup); err == nil {
		if string(existing) != string(s.legacyRaw) {
			return errors.New("legacy push subscription backup differs; preserve it before migration")
		}
		if err := syncParentDirectory(backup); err != nil {
			return errors.Wrap(err, "sync legacy push subscription backup directory")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.Wrap(err, "read legacy push subscription backup")
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".push-legacy-*")
	if err != nil {
		return errors.Wrap(err, "create legacy push subscription backup")
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(s.legacyRaw)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Link(name, backup)
	}
	if err != nil {
		return errors.Wrap(err, "persist legacy push subscription backup")
	}
	if err := syncParentDirectory(backup); err != nil {
		return errors.Wrap(err, "sync legacy push subscription backup directory")
	}
	return nil
}

// persist atomically writes current global shape. Caller holds s.mu.
func (s *Store) persist(subscriptions []webpush.Subscription) error {
	if err := s.ensureLegacyBackup(); err != nil {
		return err
	}
	b, err := json.Marshal(storeFile{Subscriptions: subscriptions})
	if err != nil {
		return errors.Wrap(err, "encode push subscriptions")
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
		return errors.Wrap(err, "persist push subscriptions")
	}
	s.subscriptions = subscriptions
	s.legacyRaw = nil
	return nil
}

// Put atomically upserts one subscription keyed by endpoint.
func (s *Store) Put(sub webpush.Subscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := append([]webpush.Subscription(nil), s.subscriptions...)
	for i := range next {
		if next[i].Endpoint == sub.Endpoint {
			next[i] = sub
			return s.persist(next)
		}
	}
	return s.persist(append(next, sub))
}

// Delete atomically removes only matching endpoint.
func (s *Store) Delete(endpoint string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.subscriptions {
		if s.subscriptions[i].Endpoint != endpoint {
			continue
		}
		next := append([]webpush.Subscription(nil), s.subscriptions[:i]...)
		next = append(next, s.subscriptions[i+1:]...)
		if err := s.persistDelete(next); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// persistDelete removes empty storage atomically or persists remaining subscriptions. Caller holds s.mu.
func (s *Store) persistDelete(next []webpush.Subscription) error {
	if len(next) != 0 {
		return s.persist(next)
	}
	if err := s.ensureLegacyBackup(); err != nil {
		return err
	}
	if err := s.remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Wrap(err, "remove push subscriptions")
	}
	s.subscriptions = nil
	s.legacyRaw = nil
	return nil
}

// DeleteIfMatch atomically removes exact stale snapshot, preserving endpoint replacements.
func (s *Store) DeleteIfMatch(snapshot webpush.Subscription) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.subscriptions {
		if s.subscriptions[i] != snapshot {
			continue
		}
		next := append([]webpush.Subscription(nil), s.subscriptions[:i]...)
		next = append(next, s.subscriptions[i+1:]...)
		if err := s.persistDelete(next); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

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
		observer.ObserveInt64(active, int64(len(store.Get())))
		return nil
	}, active)
	return svc
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

// Handler serves the trusted single-owner subscription lifecycle API.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/push/config", s.config)
	mux.HandleFunc("PUT /api/push/subscription", s.put)
	mux.HandleFunc("DELETE /api/push/subscription", s.del)
	mux.HandleFunc("POST /api/push/focus", s.focus)
	return mux
}

// config returns server capability without global registration state.
func (s *Service) config(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"publicKey": s.cfg.PublicKey, "enabled": s.cfg.Enabled()})
}

// put validates one bounded subscription document before endpoint-keyed atomic upsert.
func (s *Service) put(w http.ResponseWriter, r *http.Request) {
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
	if err := s.store.Put(sub); err != nil {
		http.Error(w, "could not persist subscription", http.StatusInternalServerError)
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

// focus validates trusted notification input, verifies pane existence, then focuses through protocol-16 socket API.
func (s *Service) focus(w http.ResponseWriter, r *http.Request) {
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
			PaneID      string `json:"pane_id"`
			AgentStatus string `json:"agent_status"`
		} `json:"panes"`
		Layouts []json.RawMessage `json:"layouts"`
		Agents  []json.RawMessage `json:"agents"`
	} `json:"snapshot"`
}

func validSessionSnapshot(s focusSnapshotResult) bool {
	if s.Snapshot == nil || s.Snapshot.Protocol == nil {
		return false
	}
	// ponytail: accept only schema-reviewed Herdr protocols; add future versions after compatibility review.
	protocolSupported := *s.Snapshot.Protocol == 16 || *s.Snapshot.Protocol == 17
	return s.Type == "session_snapshot" && s.Snapshot.Version != nil && protocolSupported && s.Snapshot.Workspaces != nil && s.Snapshot.Tabs != nil && s.Snapshot.Panes != nil && s.Snapshot.Layouts != nil && s.Snapshot.Agents != nil
}

type focusPaneResult struct {
	Type string `json:"type"`
	Pane *struct {
		PaneID string `json:"pane_id"`
	} `json:"pane"`
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

// herdrRoundTrip owns one protocol-16 connection for exactly one request and response.
func (s *Service) herdrRoundTrip(ctx context.Context, path, phase string, request herdrRequest, result any) error {
	raw, err := s.dial(ctx, "unix", path)
	if err != nil {
		return errors.Wrapf(err, "connect Herdr %s socket", phase)
	}
	conn := newEventConn(ctx, raw)
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.Wrapf(err, "request Herdr %s", phase)
	}
	scan := bufio.NewScanner(conn)
	scan.Buffer(make([]byte, 4096), maxHerdrFrame)
	if err := readHerdrResponse(scan, request.ID, result); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.Wrapf(err, "receive Herdr %s", phase)
	}
	if err := conn.Close(); err != nil {
		return errors.Wrapf(err, "close Herdr %s socket", phase)
	}
	return nil
}

// focusPane verifies pane existence, then focuses it over a fresh protocol-16 connection.
func (s *Service) focusPane(ctx context.Context, paneID string) error {
	path := s.socketPath
	if path == "" {
		path = os.Getenv("HERDR_SOCKET_PATH")
	}
	if path == "" {
		return errors.New("HERDR_SOCKET_PATH is required")
	}
	var snap focusSnapshotResult
	if err := s.herdrRoundTrip(ctx, path, "snapshot", herdrRequest{ID: "focus-snapshot", Method: "session.snapshot", Params: map[string]any{}}, &snap); err != nil {
		return err
	}
	if !validSessionSnapshot(snap) {
		return errors.Wrap(errHerdrProtocol, "snapshot result missing required shape")
	}
	for _, pane := range snap.Snapshot.Panes {
		if pane.PaneID != paneID {
			continue
		}
		var focused focusPaneResult
		if err := s.herdrRoundTrip(ctx, path, "pane focus", herdrRequest{ID: "focus-pane", Method: "pane.focus", Params: map[string]any{"pane_id": paneID}}, &focused); err != nil {
			return err
		}
		if focused.Type != "pane_info" || focused.Pane == nil || focused.Pane.PaneID != paneID {
			return errors.Wrap(errHerdrProtocol, "unexpected focus result shape")
		}
		return nil
	}
	return errPaneNotFound
}

type deleteRequest struct {
	Endpoint string `json:"endpoint"`
}

// del removes only endpoint supplied by caller browser.
func (s *Service) del(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	var input deleteRequest
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
	if !validEndpoint(input.Endpoint) {
		http.Error(w, "invalid push subscription", http.StatusBadRequest)
		return
	}
	if _, err := s.store.Delete(input.Endpoint); err != nil {
		http.Error(w, "could not remove subscription", http.StatusInternalServerError)
		return
	}
	s.mutations.Add(r.Context(), 1, metric.WithAttributes(attribute.String("outcome", "disabled")))
	s.log.InfoContext(r.Context(), "push subscription disabled", "push.endpoint", "<redacted>")
	w.WriteHeader(http.StatusNoContent)
}

func validEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	return err == nil && len(endpoint) <= maxBody && u.Scheme == "https" && u.Host != "" && u.User == nil && u.Fragment == ""
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

// Notify sends bounded JSON to every snapshot subscription, then returns one bounded aggregate error.
func (s *Service) Notify(ctx context.Context, agent, state, paneID string) error {
	if state != "done" && state != "blocked" {
		return nil
	}
	subscriptions := s.store.Get()
	if len(subscriptions) == 0 {
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
	failures := 0
	lastStatusClass := "none"
	failureOutcome, failureStatusClass, failureErrorKind := "failure", "none", "none"
	var lastCause error
	for _, sub := range subscriptions {
		outcome, statusClass, errorKind, cause := s.sendOne(ctx, payload, state, sub)
		lastStatusClass = statusClass
		if cause != nil {
			failures++
			failureOutcome, failureStatusClass, failureErrorKind = outcome, statusClass, errorKind
			lastCause = cause
		}
		attrs := metric.WithAttributes(attribute.String("event.type", state), attribute.String("outcome", outcome), attribute.String("status_class", statusClass))
		s.attempts.Add(ctx, 1, attrs)
		s.log.InfoContext(ctx, "push dispatch", "outcome", outcome, "status_class", statusClass, "agent.name", "<redacted>", "push.endpoint", "<redacted>")
	}
	outcome := "success"
	if failures != 0 {
		outcome = "failure"
		err := &broadcastError{failures: failures, total: len(subscriptions), cause: lastCause}
		span.RecordError(err)
		span.SetStatus(codes.Error, "push dispatch failed")
		span.SetAttributes(attribute.String("outcome", failureOutcome), attribute.String("status_class", failureStatusClass), attribute.String("error.kind", failureErrorKind))
		return err
	}
	span.SetAttributes(attribute.String("outcome", outcome), attribute.String("status_class", lastStatusClass), attribute.String("error.kind", "none"))
	return nil
}

type broadcastError struct {
	failures, total int
	cause           error
}

func (e *broadcastError) Error() string {
	return fmt.Sprintf("web push broadcast failed: %d of %d attempts", e.failures, e.total)
}
func (e *broadcastError) Unwrap() error { return e.cause }

// sendOne records one independently classified push-service attempt and stale prune.
func (s *Service) sendOne(ctx context.Context, payload []byte, state string, sub webpush.Subscription) (outcome, statusClass, errorKind string, cause error) {
	attemptCtx, attemptSpan := otel.Tracer("herdr-web-tui/push").Start(ctx, "push-service HTTP attempt")
	attemptSpan.SetAttributes(attribute.String("push.endpoint", "<redacted>"), attribute.String("auth", "<redacted>"), attribute.String("p256dh", "<redacted>"))
	start := time.Now()
	resp, sendErr := s.sender.Send(attemptCtx, payload, &sub, &webpush.Options{Subscriber: strings.TrimPrefix(s.cfg.Subject, "mailto:"), VAPIDPublicKey: s.cfg.PublicKey, VAPIDPrivateKey: s.cfg.PrivateKey, TTL: 60})
	safeErr := sanitizeSendError(sendErr)
	s.latency.Record(ctx, float64(time.Since(start).Microseconds())/1000)
	outcome, statusClass, errorKind = "success", "none", "none"
	if resp != nil {
		statusClass = fmt.Sprintf("%dxx", resp.StatusCode/100)
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
	}
	if safeErr != nil {
		outcome, errorKind, cause = "failure", safeErr.(*sanitizedSendError).kind, safeErr
		attemptSpan.RecordError(safeErr)
	} else if resp == nil {
		outcome, errorKind, cause = "failure", "missing_response", errors.New("web push transport returned no response")
		attemptSpan.RecordError(cause)
	} else if resp.StatusCode >= 300 {
		outcome, errorKind, cause = "failure", "http_status", errors.New("push service returned unsuccessful status class")
		attemptSpan.RecordError(cause)
	}
	if cause != nil {
		attemptSpan.SetStatus(codes.Error, "push failed")
	}
	attemptSpan.SetAttributes(attribute.String("outcome", map[bool]string{true: "failure", false: "success"}[cause != nil]), attribute.String("status_class", statusClass), attribute.String("error.kind", errorKind))
	attemptSpan.End()
	if resp != nil && (resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone) {
		pruned, deleteErr := s.store.DeleteIfMatch(sub)
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
	return
}
