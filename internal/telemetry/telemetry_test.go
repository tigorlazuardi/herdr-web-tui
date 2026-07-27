package telemetry

import (
	"bytes"
	"context"
	"go.opentelemetry.io/otel"
	"log/slog"
	"strings"
	"testing"
)

func TestSignalExportEnabledMatrix(t *testing.T) {
	cases := []struct {
		name, generic, traces, metrics string
		wantTrace, wantMetric          bool
	}{
		{name: "none"},
		{name: "generic", generic: "http://collector:4317", wantTrace: true, wantMetric: true},
		{name: "trace only", traces: "http://collector:4317", wantTrace: true},
		{name: "metric only", metrics: "http://collector:4317", wantMetric: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", tc.generic)
			t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", tc.traces)
			t.Setenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", tc.metrics)
			if got := signalExportEnabled("TRACES"); got != tc.wantTrace {
				t.Fatalf("trace enabled=%v want %v", got, tc.wantTrace)
			}
			if got := signalExportEnabled("METRICS"); got != tc.wantMetric {
				t.Fatalf("metric enabled=%v want %v", got, tc.wantMetric)
			}
		})
	}
}

func TestSetupNoExporterCorrelatesLocalLogs(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "")
	var b bytes.Buffer
	log := slog.New(CorrelatingHandler{slog.NewTextHandler(&b, nil)})
	shutdown, err := Setup(context.Background(), log)
	if err != nil {
		t.Fatal(err)
	}
	ctx, span := otel.Tracer("test").Start(context.Background(), "operation")
	log.InfoContext(ctx, "inside")
	span.End()
	if err := shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "inside") || !strings.Contains(out, "trace_id=") || !strings.Contains(out, "span_id=") {
		t.Fatalf("missing local correlated log: %s", out)
	}
}
