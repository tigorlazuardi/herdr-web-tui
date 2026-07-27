// Package telemetry installs always-on OpenTelemetry providers and slog correlation.
package telemetry

import (
	"context"
	"log/slog"
	"os"

	"github.com/go-faster/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
	apiTrace "go.opentelemetry.io/otel/trace"
)

func signalExportEnabled(signal string) bool {
	return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" || os.Getenv("OTEL_EXPORTER_OTLP_"+signal+"_ENDPOINT") != ""
}

// Setup independently installs trace and metric exporters selected by standard OTLP environment variables.
func Setup(ctx context.Context, log *slog.Logger) (func(context.Context) error, error) {
	traceExporting := signalExportEnabled("TRACES")
	metricExporting := signalExportEnabled("METRICS")

	var tp *trace.TracerProvider
	if traceExporting {
		exporter, err := otlptracegrpc.New(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "create OTLP trace exporter")
		}
		tp = trace.NewTracerProvider(trace.WithBatcher(exporter))
	} else {
		tp = trace.NewTracerProvider(trace.WithSampler(trace.AlwaysSample()))
	}

	var mp *metric.MeterProvider
	if metricExporting {
		exporter, err := otlpmetricgrpc.New(ctx)
		if err != nil {
			_ = tp.Shutdown(ctx)
			return nil, errors.Wrap(err, "create OTLP metric exporter")
		}
		mp = metric.NewMeterProvider(metric.WithReader(metric.NewPeriodicReader(exporter)))
	} else {
		mp = metric.NewMeterProvider()
	}

	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	shutdown := func(ctx context.Context) error {
		metricErr := mp.Shutdown(ctx)
		traceErr := tp.Shutdown(ctx)
		if metricErr != nil {
			return metricErr
		}
		return traceErr
	}
	log.Info("OpenTelemetry ready", "traces_exporting", traceExporting, "metrics_exporting", metricExporting)
	return shutdown, nil
}

// CorrelatingHandler keeps local slog output unconditional and adds active trace correlation.
type CorrelatingHandler struct{ slog.Handler }

// Handle adds valid active trace IDs without changing untraced records.
func (h CorrelatingHandler) Handle(ctx context.Context, r slog.Record) error {
	sc := apiTrace.SpanContextFromContext(ctx)
	if sc.IsValid() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs preserves correlation behavior on derived handlers.
func (h CorrelatingHandler) WithAttrs(a []slog.Attr) slog.Handler {
	return CorrelatingHandler{h.Handler.WithAttrs(a)}
}

// WithGroup preserves correlation behavior on grouped handlers.
func (h CorrelatingHandler) WithGroup(n string) slog.Handler {
	return CorrelatingHandler{h.Handler.WithGroup(n)}
}
