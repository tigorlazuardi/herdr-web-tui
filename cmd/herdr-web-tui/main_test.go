package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

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
