package main

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestShutdownHTTPServer_TimeoutForceClosesActiveRequest(t *testing.T) {
	requestStarted := make(chan struct{})
	requestCanceled := make(chan struct{})
	testServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-r.Context().Done()
		close(requestCanceled)
	}))
	testServer.Start()
	t.Cleanup(testServer.Close)

	requestDone := make(chan error, 1)
	go func() {
		response, err := http.Get(testServer.URL)
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			err = response.Body.Close()
		}
		requestDone <- err
	}()
	<-requestStarted

	started := time.Now()
	forced, err := shutdownHTTPServer(testServer.Config, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("shutdownHTTPServer: %v", err)
	}
	if !forced {
		t.Fatal("shutdown unexpectedly remained graceful with an active request")
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("forced shutdown took %s", elapsed)
	}
	select {
	case <-requestCanceled:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("active request remained connected after shutdown timeout")
	}
	<-requestDone
}

func TestLoadPushLogsRedactedStructuredFailures(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*testing.T)
		kind      string
		field     string
	}{
		{"invalid config", func(t *testing.T) {
			t.Setenv("VAPID_PUBLIC_KEY", "secret-public")
			t.Setenv("VAPID_PRIVATE_KEY", "secret-private")
			t.Setenv("VAPID_SUBJECT", "secret-subject")
		}, "invalid_config", "vapid.private_key"},
		{"store open", func(t *testing.T) {
			t.Setenv("VAPID_PUBLIC_KEY", "")
			t.Setenv("VAPID_PRIVATE_KEY", "")
			t.Setenv("VAPID_SUBJECT", "")
			t.Setenv("WEB_PUSH_STORE_PATH", t.TempDir())
		}, "store_open", "web_push.store_path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.configure(t)
			var output bytes.Buffer
			_, _, err := loadPush(slog.New(slog.NewJSONHandler(&output, nil)))
			if err == nil {
				t.Fatal("startup failure accepted")
			}
			log := output.String()
			if !strings.Contains(log, `"msg":"Web Push configuration failed"`) || !strings.Contains(log, `"error.kind":"`+tt.kind+`"`) || !strings.Contains(log, `"`+tt.field+`":"<redacted>"`) {
				t.Fatalf("missing structured failure fields: %s", log)
			}
			for _, secret := range []string{"secret-public", "secret-private", "secret-subject"} {
				if strings.Contains(log, secret) {
					t.Fatalf("secret leaked in log: %s", log)
				}
			}
		})
	}
}
