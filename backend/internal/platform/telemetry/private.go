package telemetry

import (
	"context"
	"log/slog"
	"time"
)

// ContextWithPrivateRequest strips arbitrary fields from generic logger calls.
// Transport must mark the context before invoking any private handler/recovery.
func ContextWithPrivateRequest(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxPrivateRequest, true)
}

// ContextWithServerRequest is for transport-assigned correlation only, never
// directly for an inbound header. Generic client/job contexts remain untrusted.
func ContextWithServerRequest(ctx context.Context, id string) context.Context {
	return context.WithValue(ContextWithRequest(ctx, id), ctxServerRequestID, id)
}

func privateRequest(ctx context.Context) bool {
	private, _ := ctx.Value(ctxPrivateRequest).(bool)
	return private
}

func (l *Logger) privateAttrs(ctx context.Context, code string) []any {
	attrs := []any{slog.String("code", code)}
	if id, _ := ctx.Value(ctxServerRequestID).(string); id != "" {
		attrs = append(attrs, slog.String("request_id", id))
	}
	return attrs
}

// HTTPMethod folds arbitrary wire verbs into a bounded telemetry vocabulary.
func HTTPMethod(method string) string {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return method
	default:
		return "OTHER"
	}
}

// Request is the transport-owned access record. route must be the matched
// source template or "unmatched", never the raw URL. Generic private Info/
// Warn/Error calls cannot supply these fields or inherit bound-route values.
func (l *Logger) Request(ctx context.Context, method, route string, status int, elapsed time.Duration) {
	fields := []any{slog.String("method", method), slog.String("route", route),
		slog.Int("status", status), slog.Int64("duration_ms", elapsed.Milliseconds())}
	if !privateRequest(ctx) {
		l.Info(ctx, "http.request", "request served", fields...)
		return
	}
	fields[0] = slog.String("method", HTTPMethod(method))
	l.root.Log(ctx, slog.LevelInfo, "request served", append(l.privateAttrs(ctx, "http.request"), fields...)...)
}
