package main

import (
	"bytes"
	"context"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpserver"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/telemetry"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func previewBodyConfig() config.Config {
	return config.Config{Env: config.EnvStaging, MaxBodyBytes: 1 << 20, DevelopmentPhotoPreviewUntil: time.Now().Add(time.Hour), R2: &config.R2Config{Bucket: "abastevo-dev-photos"}}
}
func TestPrivateStagingTransferPreservesMaximumAndNarrowScope(t *testing.T) {
	cfg := previewBodyConfig()
	for _, p := range []string{"/abastevo-dev-photos/q/synthetic", "/abastevo-dev-photos/f/synthetic"} {
		if got := requestBodyLimit(cfg)(httptest.NewRequest("PUT", p, nil)); got != 3<<20 {
			t.Fatalf("private photo max = %d, want 3 MiB", got)
		}
	}
	for _, tc := range []struct{ method, path string }{{"POST", "/v1/privacy/training-consent/grant"}, {"POST", "/v1/contributors"}, {"GET", "/abastevo-dev-photos/q/x"}, {"POST", "/abastevo-dev-photos/q/x"}, {"PUT", "/other/q/x"}, {"PUT", "/abastevo-dev-photos/unknown/x"}} {
		if got := requestBodyLimit(cfg)(httptest.NewRequest(tc.method, tc.path, nil)); got != 1<<20 {
			t.Fatal("default request cap widened", tc)
		}
	}
	for _, kind := range []string{"production", "disabled", "wrong-bucket"} {
		c := cfg
		switch kind {
		case "production":
			c.Env = config.EnvProduction
		case "disabled":
			c.DevelopmentPhotoPreviewUntil = time.Time{}
		case "wrong-bucket":
			c.R2 = &config.R2Config{Bucket: "other"}
		}
		if got := requestBodyLimit(c)(httptest.NewRequest("PUT", "/abastevo-dev-photos/q/x", nil)); got != 1<<20 {
			t.Fatal("unsafe mode widened", kind)
		}
	}
}
func TestStagingMaximumAndOverLimitOnRealHTTPStreams(t *testing.T) {
	cfg := previewBodyConfig()
	log, err := telemetry.New(io.Discard, "info", telemetry.RoleAPI)
	if err != nil {
		t.Fatal(err)
	}
	// This suite verifies exact byte limits, not network latency. The full
	// race/PostGIS CI matrix competes for CPU; keep harness deadlines separate
	// from the unchanged 3 MiB/1 MiB admission assertions.
	srv, err := httpserver.New(httpserver.Options{Addr: "127.0.0.1:0", ReadTimeout: 30 * time.Second, ReadHeaderTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, ShutdownTimeout: 5 * time.Second, MaxBodyBytes: cfg.MaxBodyBytes, BodyLimit: requestBodyLimit(cfg), Logger: log}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			w.WriteHeader(413)
			return
		}
		w.WriteHeader(204)
	}))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	client := &http.Client{Timeout: 45 * time.Second}
	for _, chunked := range []bool{false, true} {
		for _, tc := range []struct {
			path       string
			size, code int
		}{{"/abastevo-dev-photos/q/x", 3 << 20, 204}, {"/abastevo-dev-photos/f/x", 3 << 20, 204}, {"/abastevo-dev-photos/q/x", (3 << 20) + 1, 413}, {"/v1/contributors", (1 << 20) + 1, 413}} {
			r, _ := http.NewRequest("PUT", "http://"+srv.Addr().String()+tc.path, bytes.NewReader(make([]byte, tc.size)))
			if chunked {
				r.ContentLength = -1
			}
			res, err := client.Do(r)
			if err != nil {
				t.Fatalf("path=%s chunked=%v size=%d: %v", tc.path, chunked, tc.size, err)
			}
			res.Body.Close()
			if res.StatusCode != tc.code {
				t.Fatalf("%s chunked=%v size=%d status=%d want=%d", tc.path, chunked, tc.size, res.StatusCode, tc.code)
			}
		}
	}
}
