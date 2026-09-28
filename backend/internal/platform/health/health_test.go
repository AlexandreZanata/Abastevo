package health

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/telemetry"
)

const dsnSecret = "postgres://app:S3cr3t-zzz@db.internal:5432/app?sslmode=require"

func testLogger(t *testing.T, buf *bytes.Buffer) *telemetry.Logger {
	t.Helper()
	l, err := telemetry.New(buf, "info", telemetry.RoleAPI)
	if err != nil {
		t.Fatalf("telemetry.New: %v", err)
	}
	return l
}

// decodeStatus asserts the minimal envelope: exactly one "status" key.
func decodeStatus(t *testing.T, body []byte) string {
	t.Helper()
	var rec map[string]any
	if err := json.Unmarshal(body, &rec); err != nil {
		t.Fatalf("health body is not JSON: %v", err)
	}
	if len(rec) != 1 {
		t.Fatalf("health body must carry status only, got %v", rec)
	}
	status, ok := rec["status"].(string)
	if !ok {
		t.Fatalf("status field missing: %v", rec)
	}
	return status
}

func TestLiveIgnoresFailingChecks(t *testing.T) {
	var logs bytes.Buffer
	called := false
	h, err := New(testLogger(t, &logs), 0, Check{
		Name: "db",
		Fn: func(ctx context.Context) error {
			called = true
			return errors.New("dial " + dsnSecret)
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rec := httptest.NewRecorder()
	h.Live(rec, httptest.NewRequest(http.MethodGet, "/health/live", nil))

	res := rec.Result()
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Errorf("live status = %d, want 200", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if got := decodeStatus(t, body); got != "ok" {
		t.Errorf("live body status = %q, want ok", got)
	}
	if called {
		t.Error("liveness must not invoke dependency checks")
	}
}

func TestReadyFailsClosedWithoutDisclosure(t *testing.T) {
	var logs bytes.Buffer
	h, err := New(testLogger(t, &logs), 0, Check{
		Name: "db",
		Fn: func(ctx context.Context) error {
			return errors.New("dial " + dsnSecret)
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rec := httptest.NewRecorder()
	h.Ready(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	res := rec.Result()
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("ready status = %d, want 503", res.StatusCode)
	}
	if got := decodeStatus(t, body); got != "not_ready" {
		t.Errorf("ready body status = %q, want not_ready", got)
	}
	if strings.Contains(string(body), "S3cr3t-zzz") {
		t.Error("readiness body discloses check internals")
	}
	if strings.Contains(logs.String(), "S3cr3t-zzz") {
		t.Error("readiness log discloses check internals")
	}
	if res.Header.Get("Cache-Control") == "" {
		t.Error("readiness must carry Cache-Control")
	}
}

func TestReadyPassesWhenChecksPass(t *testing.T) {
	var logs bytes.Buffer
	h, err := New(testLogger(t, &logs), 0, Check{
		Name: "db",
		Fn:   func(ctx context.Context) error { return nil },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rec := httptest.NewRecorder()
	h.Ready(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	res := rec.Result()
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Errorf("ready status = %d, want 200", res.StatusCode)
	}
	if got := decodeStatus(t, body); got != "ready" {
		t.Errorf("ready body status = %q, want ready", got)
	}
}

func TestSlowCheckTimesOutAsNotReady(t *testing.T) {
	var logs bytes.Buffer
	h, err := New(testLogger(t, &logs), 50*time.Millisecond, Check{
		Name: "db",
		Fn: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	start := time.Now()
	rec := httptest.NewRecorder()
	h.Ready(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("readiness took %v, check timeout not enforced", elapsed)
	}
	if res := rec.Result(); res.StatusCode != http.StatusServiceUnavailable {
		res.Body.Close()
		t.Errorf("ready status = %d, want 503", res.StatusCode)
	} else {
		res.Body.Close()
	}
}

func TestNewRejectsNilLogger(t *testing.T) {
	if _, err := New(nil, 0); err == nil {
		t.Error("expected error for nil logger, got nil")
	}
}
