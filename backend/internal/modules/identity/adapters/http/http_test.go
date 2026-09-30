package http

import (
	"context"
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/go-chi/chi/v5"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type noRegistrar struct{}

func (noRegistrar) IssueChallenge(context.Context, string, string) (domain.Challenge, error) {
	return domain.Challenge{}, errors.New("offline")
}
func (noRegistrar) Challenge(context.Context, string, string) (domain.Challenge, error) {
	return domain.Challenge{}, errors.New("offline")
}
func (noRegistrar) Register(context.Context, domain.RegistrationRequest) (domain.Registration, error) {
	return domain.Registration{}, errors.New("offline")
}
func (noRegistrar) Rotate(context.Context, domain.RotationRequest) (domain.Rotation, error) {
	return domain.Rotation{}, errors.New("offline")
}
func TestAnonymousIdentityFailClosed(t *testing.T) {
	for _, tc := range []struct {
		body  string
		quota bool
		want  int
	}{
		{`{"fingerprint":"fp:` + strings.Repeat("a", 64) + `","purpose":"REGISTER"}`, false, 503},
		{`{"fingerprint":"fp:` + strings.Repeat("a", 64) + `","purpose":"REGISTER"}`, true, 429},
		{`{"fingerprint":"bad","purpose":"REGISTER"}`, false, 400},
		{`{"fingerprint":"fp:` + strings.Repeat("a", 64) + `","purpose":"UNKNOWN"}`, false, 400},
		{`{"fingerprint":"bad","purpose":"REGISTER"} {}`, false, 400},
	} {
		r := chi.NewRouter()
		h := Handler{Registrar: noRegistrar{}, Authority: "test.invalid", QuotaSecret: []byte(strings.Repeat("a", 32)), CheckQuota: func(context.Context, string, string) (time.Duration, error) {
			if tc.quota {
				return time.Minute, domain.ErrQuotaExceeded
			}
			return 0, nil
		}}
		h.RegisterRoutes(r)
		req := httptest.NewRequest("POST", "/v1/identity/challenges", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("want %d got %d", tc.want, rec.Code)
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Error("identity error cacheable")
		}
	}
}
