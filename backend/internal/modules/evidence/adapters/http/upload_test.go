package http

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

func TestUploadGatewayRejectsBeforeStorage(t *testing.T) {
	for _, tc := range []struct {
		name                                           string
		data                                           string
		mime                                           string
		chunked, expired, badToken, claimFail, putFail bool
		code                                           int
	}{
		{name: "exact", data: "synthetic", mime: domain.AllowedMIME, code: 204},
		{name: "oversize", data: "syntheticX", mime: domain.AllowedMIME, code: 413},
		{name: "chunked-oversize", data: "syntheticX", mime: domain.AllowedMIME, chunked: true, code: 413},
		{name: "truncated", data: "sy", mime: domain.AllowedMIME, code: 422},
		{name: "substitution", data: "xxxxxxxxx", mime: domain.AllowedMIME, code: 422},
		{name: "mime", data: "synthetic", mime: "text/plain", code: 415},
		{name: "expired", data: "synthetic", mime: domain.AllowedMIME, expired: true, code: 403},
		{name: "token", data: "synthetic", mime: domain.AllowedMIME, badToken: true, code: 403},
		{name: "budget", data: "synthetic", mime: domain.AllowedMIME, claimFail: true, code: 429},
		{name: "storage", data: "synthetic", mime: domain.AllowedMIME, putFail: true, code: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
			body := []byte("synthetic")
			sum := sha256.Sum256(body)
			session := domain.Session{ID: "s1", Status: domain.StateIssued, MIME: domain.AllowedMIME, DeclaredBytes: int64(len(body)), MaxBytes: domain.MaxUploadBytes, ClaimedSHA256: hex.EncodeToString(sum[:]), ExpiresAt: now.Add(time.Hour), QuarantineKey: "q/synthetic"}
			writes := 0
			finishes := 0
			g := NewUploadGateway([]byte(strings.Repeat("k", 32)), "https://api.example.invalid", func() time.Time { return now })
			g.NewNonce = func() (string, error) { return "claim", nil }
			g.Claim = func(context.Context, string, string, time.Time) (domain.Session, bool, error) {
				if tc.claimFail {
					return domain.Session{}, false, domain.ErrQuotaDenied
				}
				return session, false, nil
			}
			g.Finish = func(_ context.Context, _, _ string, success bool, _ int64) error { finishes++; return nil }
			g.Put = func(_ context.Context, key, mime string, data []byte) error {
				writes++
				if key != session.QuarantineKey || !bytes.Equal(data, body) {
					t.Fatal("unbound output")
				}
				if tc.putFail {
					return errors.New("outage")
				}
				return nil
			}
			target, _, _, err := g.Authorize(context.Background(), session)
			if err != nil {
				t.Fatal(err)
			}
			if tc.expired {
				now = now.Add(6 * time.Minute)
			}
			if tc.badToken {
				target += "x"
			}
			req := httptest.NewRequest(http.MethodPut, target, strings.NewReader(tc.data))
			req.Header.Set("Content-Type", tc.mime)
			if tc.chunked {
				req.ContentLength = -1
			}
			w := httptest.NewRecorder()
			g.ServeHTTP(w, req)
			if w.Code != tc.code {
				t.Fatalf("code=%d want=%d body=%s", w.Code, tc.code, w.Body.String())
			}
			if tc.code != 204 && tc.code != 503 && writes != 0 {
				t.Fatalf("rejected bytes reached storage: %d", writes)
			}
			if (tc.code == 204 || tc.code == 503) && finishes != 1 {
				t.Fatal("claim not completed")
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("missing private policy")
			}
		})
	}
}

func TestUploadGatewayBoundedConcurrencyAndReplay(t *testing.T) {
	now := time.Now()
	raw := []byte("synthetic")
	sum := sha256.Sum256(raw)
	session := domain.Session{ID: "s1", Status: domain.StateIssued, MIME: domain.AllowedMIME, DeclaredBytes: int64(len(raw)), ClaimedSHA256: hex.EncodeToString(sum[:]), ExpiresAt: now.Add(time.Hour)}
	g := NewUploadGateway([]byte(strings.Repeat("k", 32)), "https://api.example.invalid", func() time.Time { return now })
	var claims atomic.Int32
	g.Claim = func(context.Context, string, string, time.Time) (domain.Session, bool, error) {
		claims.Add(1)
		return session, false, nil
	}
	g.Finish = func(context.Context, string, string, bool, int64) error { return nil }
	started, release, done := make(chan struct{}, 4), make(chan struct{}), make(chan int, 4)
	g.Put = func(context.Context, string, string, []byte) error { started <- struct{}{}; <-release; return nil }
	target, _, _, err := g.Authorize(context.Background(), session)
	if err != nil {
		t.Fatal(err)
	}
	send := func() int {
		req := httptest.NewRequest("PUT", target, bytes.NewReader(raw))
		req.Header.Set("Content-Type", session.MIME)
		w := httptest.NewRecorder()
		g.ServeHTTP(w, req)
		return w.Code
	}
	for i := 0; i < 4; i++ {
		go func() { done <- send() }()
	}
	for i := 0; i < 4; i++ {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			close(release)
			t.Fatal("transfer failed to start")
		}
	}
	code := send()
	close(release)
	for i := 0; i < 4; i++ {
		if code := <-done; code != 204 {
			t.Errorf("admitted transfer=%d", code)
		}
	}
	if code != 503 || claims.Load() != 4 {
		t.Fatalf("overloaded request reached DB: code=%d claims=%d", code, claims.Load())
	}
	g.Claim = func(context.Context, string, string, time.Time) (domain.Session, bool, error) {
		return session, true, nil
	}
	g.Put = func(context.Context, string, string, []byte) error { t.Error("replay rewrote object"); return nil }
	g.Finish = func(context.Context, string, string, bool, int64) error {
		t.Error("replay mutated receipt")
		return nil
	}
	if code := send(); code != 204 {
		t.Fatal("receipt replay failed", code)
	}
}
