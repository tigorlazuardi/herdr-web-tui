---
paths:
  - internal/push/**
  - internal/telemetry/**
---

# Web Push telemetry

Use OpenTelemetry Go SDK with OTLP/gRPC exporters configured by standard `OTEL_EXPORTER_OTLP_*` env. Always install tracer and meter providers; without endpoint, discard export while retaining valid trace/span correlation in existing slog output.

Spans: `agent event handling`, `push dispatch`, push-service attempt, `pane focus`. Metrics: `web_push.subscription.mutations`, `web_push.attempts`, `web_push.pane_focus.attempts`, `web_push.latency` ms buckets `[5,10,25,50,100,250,500,1000,2500,5000]`, `web_push.payload.bytes` buckets `[256,1024,4096,16384,65536]`. Metric labels only bounded event type, outcome, status class.

Always redact VAPID private key, push endpoint/auth/p256dh, cookies/headers as `<redacted>`. Agent name allowed only in push payload; telemetry uses `agent.name=<redacted>`. Pane ID allowed only in push/click/focus request payloads; telemetry uses `pane.id=<redacted>` and never pane ID metric labels. Local slog output remains active regardless of exporter availability.
