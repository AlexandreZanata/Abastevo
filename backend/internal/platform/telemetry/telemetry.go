// Package telemetry is the slog factory with request/job context and redaction.
//
// Every record carries the bounded fields service_role and code plus, when
// present in the context, request_id and job_type/job_id. Per B-BR-011 no
// contributor identity, exact GPS, IP, EXIF, private object key or signed URL
// may be emitted: callers wrap anything sensitive with Redact, and timestamps
// are UTC. Roles are allowlisted to prevent high-cardinality labels; routes
// are templates ("/stations/{id}"), never raw paths.
package telemetry

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"
)

// Role is the process role emitting the record. Bounded by allowlist.
type Role string

const (
	RoleAPI     Role = "api"
	RoleWorker  Role = "worker"
	RoleMigrate Role = "migrate"
)

// Redacted is the only rendering a wrapped secret ever takes.
const Redacted = "[redacted]"

// Redact replaces a sensitive value with the redaction marker.
func Redact(string) string { return Redacted }

type ctxKey int

const (
	ctxRequestID ctxKey = iota
	ctxJobType
	ctxJobID
)

// ContextWithRequest carries the request ID for subsequent log records.
func ContextWithRequest(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, ctxRequestID, requestID)
}

// ContextWithJob carries the job type and ID for subsequent log records.
func ContextWithJob(ctx context.Context, jobType, jobID string) context.Context {
	ctx = context.WithValue(ctx, ctxJobType, jobType)
	return context.WithValue(ctx, ctxJobID, jobID)
}

// Logger is a JSON slog logger with the process role pre-attached.
type Logger struct {
	base *slog.Logger
}

// New builds a Logger writing JSON records to w. The sink is injected so
// tests observe output without touching process streams.
func New(w io.Writer, level string, role Role) (*Logger, error) {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		return nil, fmt.Errorf("telemetry: level must be debug, info, warn or error")
	}
	switch role {
	case RoleAPI, RoleWorker, RoleMigrate:
	default:
		return nil, fmt.Errorf("telemetry: role must be api, worker or migrate")
	}
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:     lvl,
		AddSource: false,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey && len(groups) == 0 {
				if t, ok := a.Value.Any().(time.Time); ok {
					a.Value = slog.StringValue(t.UTC().Format(time.RFC3339Nano))
				}
			}
			return a
		},
	})
	return &Logger{base: slog.New(handler).With("service_role", string(role))}, nil
}

// WithRoute returns a Logger with a route template attached. Templates only.
func (l *Logger) WithRoute(template string) *Logger {
	return &Logger{base: l.base.With("route", template)}
}

// Info emits a stable-coded informational record.
func (l *Logger) Info(ctx context.Context, code, msg string, args ...any) {
	l.log(ctx, slog.LevelInfo, code, msg, args...)
}

// Warn emits a stable-coded warning record.
func (l *Logger) Warn(ctx context.Context, code, msg string, args ...any) {
	l.log(ctx, slog.LevelWarn, code, msg, args...)
}

// Error emits a stable-coded error record.
func (l *Logger) Error(ctx context.Context, code, msg string, args ...any) {
	l.log(ctx, slog.LevelError, code, msg, args...)
}

func (l *Logger) log(ctx context.Context, level slog.Level, code, msg string, args ...any) {
	if code == "" {
		code = "unspecified"
	}
	attrs := []any{slog.String("code", code)}
	if v, ok := ctx.Value(ctxRequestID).(string); ok && v != "" {
		attrs = append(attrs, slog.String("request_id", v))
	}
	if v, ok := ctx.Value(ctxJobType).(string); ok && v != "" {
		attrs = append(attrs, slog.String("job_type", v))
	}
	if v, ok := ctx.Value(ctxJobID).(string); ok && v != "" {
		attrs = append(attrs, slog.String("job_id", v))
	}
	l.base.Log(ctx, level, msg, append(attrs, args...)...)
}
