package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// Representative sensitive payloads per B-BR-011: none of these substrings may
// ever appear in a serialized log record.
var sensitiveFixtures = map[string]string{
	"contributor": "ctr_9f2ac41d77e04b1a",
	"gps":         "-23.5558,-46.6396",
	"ip":          "187.44.12.9",
	"exif":        "Canon EOS R5 serial 082031004771",
	"object_key":  "quarantine/01K7ab9q.jpg",
	"signed_url":  "https://media.internal/o/final.jpg?X-Amz-Signature=deadbeef",
	"body":        `{"price_minor":599,"fuel":"GASOLINE"}`,
	"token":       "Bearer eyJhbGciOiJIUzI1NiJ9.payload",
}

func newTestLogger(t *testing.T, buf *bytes.Buffer) *Logger {
	t.Helper()
	l, err := New(buf, "info", RoleAPI)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return l
}

func decodeRecord(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("log output is not JSON: %v\n%s", err, buf.String())
	}
	return rec
}

func TestNewRejectsUnknownRoleAndLevel(t *testing.T) {
	var buf bytes.Buffer
	if _, err := New(&buf, "info", Role("edge-cache")); err == nil {
		t.Error("expected error for unbounded role, got nil")
	}
	if _, err := New(&buf, "verbose", RoleAPI); err == nil {
		t.Error("expected error for unknown level, got nil")
	}
}

func TestRecordHasRoleCodeAndRequestID(t *testing.T) {
	var buf bytes.Buffer
	l := newTestLogger(t, &buf)
	ctx := ContextWithRequest(context.Background(), "req-123")
	l.Info(ctx, "observation.accepted", "observation received", slog.Int64("max_body_bytes", 42))

	rec := decodeRecord(t, &buf)
	if rec["service_role"] != "api" {
		t.Errorf("service_role = %v, want api", rec["service_role"])
	}
	if rec["code"] != "observation.accepted" {
		t.Errorf("code = %v, want observation.accepted", rec["code"])
	}
	if rec["request_id"] != "req-123" {
		t.Errorf("request_id = %v, want req-123", rec["request_id"])
	}
	ts, ok := rec["time"].(string)
	if !ok {
		t.Fatalf("time field missing or not a string: %v", rec)
	}
	parsed, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		t.Fatalf("time %q does not parse: %v", ts, err)
	}
	if _, offset := parsed.Zone(); offset != 0 {
		t.Errorf("timestamp %q is not UTC", ts)
	}
}

func TestJobContextFields(t *testing.T) {
	var buf bytes.Buffer
	l := newTestLogger(t, &buf)
	ctx := ContextWithJob(context.Background(), "evidence-verify", "job-7")
	l.Info(ctx, "job.claimed", "job claimed")

	rec := decodeRecord(t, &buf)
	if rec["job_type"] != "evidence-verify" || rec["job_id"] != "job-7" {
		t.Errorf("job fields missing: %v", rec)
	}
}

func TestEmptyCodeDefaultsToUnspecified(t *testing.T) {
	var buf bytes.Buffer
	l := newTestLogger(t, &buf)
	l.Warn(context.Background(), "", "no code given")

	if rec := decodeRecord(t, &buf); rec["code"] != "unspecified" {
		t.Errorf("code = %v, want unspecified", rec["code"])
	}
}

func TestSensitiveValuesAbsentFromRecords(t *testing.T) {
	var buf bytes.Buffer
	l := newTestLogger(t, &buf).WithRoute("/stations/{id}")
	ctx := ContextWithRequest(context.Background(), "req-456")

	// Callers redact what they hold; the record must then carry no secret.
	l.Error(ctx, "evidence.invalid", "validation failed",
		slog.String("contributor", Redact(sensitiveFixtures["contributor"])),
		slog.String("gps", Redact(sensitiveFixtures["gps"])),
		slog.String("client_ip", Redact(sensitiveFixtures["ip"])),
		slog.String("exif", Redact(sensitiveFixtures["exif"])),
		slog.String("object_key", Redact(sensitiveFixtures["object_key"])),
		slog.String("signed_url", Redact(sensitiveFixtures["signed_url"])),
		slog.String("body", Redact(sensitiveFixtures["body"])),
		slog.String("auth", Redact(sensitiveFixtures["token"])),
	)

	out := buf.String()
	for name, secret := range sensitiveFixtures {
		if strings.Contains(out, secret) {
			t.Errorf("log output leaks %s fixture", name)
		}
	}
	rec := decodeRecord(t, &buf)
	if rec["route"] != "/stations/{id}" {
		t.Errorf("route = %v, want template /stations/{id}", rec["route"])
	}
}

func TestRedact(t *testing.T) {
	if got := Redact("anything-secret"); got != "[redacted]" {
		t.Errorf("Redact = %q, want [redacted]", got)
	}
}
