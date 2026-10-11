package http

import (
	"context"
	"crypto/elliptic"
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/go-chi/chi/v5"
)

// SEC02-C global/per-client split: the deliberate global ceiling sheds
// with a distinct 503 shape before per-subject fairness is consumed;
// nil breakers and zero limits disable global capping entirely.

func TestBreakerDisabledByDefault(t *testing.T) {
	var nilBreaker *GlobalBreaker
	if wait, err := nilBreaker.Check(domain.OperationChallenge, time.Now()); err != nil || wait != 0 {
		t.Fatalf("nil breaker must admit, got %v (%v)", wait, err)
	}
	b := NewGlobalBreaker(0, 0, 0)
	if wait, err := b.Check(domain.OperationChallenge, time.Now()); err != nil || wait != 0 {
		t.Fatalf("zero limits must admit, got %v (%v)", wait, err)
	}
}

func TestBreakerTripsAndResets(t *testing.T) {
	b := NewGlobalBreaker(2, 1, 0)
	now := time.Now()
	for i := 0; i < 2; i++ {
		if wait, err := b.Check(domain.OperationChallenge, now); err != nil || wait != 0 {
			t.Fatalf("admit %d: %v (%v)", i, wait, err)
		}
	}
	wait, err := b.Check(domain.OperationChallenge, now)
	denied, ok := err.(*BreakerError)
	if !ok || err == nil {
		t.Fatalf("over-ceiling must deny BreakerError, got %v", err)
	}
	if wait <= 0 || denied.RetryAfter <= 0 {
		t.Fatalf("denial must carry positive retry, got %v", wait)
	}
	// Other operations are independent.
	if wait, err := b.Check(domain.OperationRegister, now); err != nil || wait != 0 {
		t.Fatalf("register must stay independent, got %v (%v)", wait, err)
	}
	// Hourly rotation re-admits.
	later := now.Add(61 * time.Minute)
	if wait, err := b.Check(domain.OperationChallenge, later); err != nil || wait != 0 {
		t.Fatalf("rotated window must admit, got %v (%v)", wait, err)
	}
}

func TestBreakerConcurrentCannotOvershoot(t *testing.T) {
	b := NewGlobalBreaker(20, 0, 0)
	now := time.Now()
	var wg sync.WaitGroup
	var mu sync.Mutex
	admitted := 0
	for i := 0; i < 60; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := b.Check(domain.OperationChallenge, now); err == nil {
				mu.Lock()
				admitted++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if admitted != 20 {
		t.Fatalf("parallel admitted = %d, want exactly 20", admitted)
	}
}

// TestMultiStepBudgetViability pins the ordinary signed onboarding and
// rotation ceremonies inside per-subject fairness budgets (SEC02-C):
// one challenge + one register for onboarding, two challenges + one
// rotation proof for key rotation. Counts drive the real handler
// quota path with a recording CheckQuota.
func TestMultiStepBudgetViability(t *testing.T) {
	var mu sync.Mutex
	used := map[string]int{}
	fp := "fp:" + strings.Repeat("e", 64)
	h := Handler{
		Registrar:   noRegistrar{},
		Authority:   "test.invalid",
		QuotaSecret: []byte(strings.Repeat("a", 32)),
		CheckQuota: func(context.Context, string, string) (time.Duration, error) {
			mu.Lock()
			defer mu.Unlock()
			used["calls"]++
			return 0, nil
		},
	}
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	post := func(path, body string) int {
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.44:9000"
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}
	// A real P-256 point so JWK parsing passes and quota (which runs
	// after fingerprint validation, before registrar proofs) is billed.
	gx, gy := elliptic.P256().Params().Gx, elliptic.P256().Params().Gy
	pad := func(b []byte) string {
		padded := make([]byte, 32)
		copy(padded[32-len(b):], b)
		return base64.RawURLEncoding.EncodeToString(padded)
	}
	jwk := `"public_jwk":{"kty":"EC","crv":"P-256","x":"` + pad(gx.Bytes()) + `","y":"` + pad(gy.Bytes()) + `"}`
	// Onboarding: challenge issuance then registration (registrar is
	// offline by design here; quota consumption is what we pin).
	post("/v1/identity/challenges", `{"fingerprint":"`+fp+`","purpose":"REGISTER"}`)
	post("/v1/contributors", `{`+jwk+`,"challenge_id":"c","proof":{"lines":[],"signature":"s"}}`)
	// Rotation ceremony issues two more challenges before rotating.
	post("/v1/identity/challenges", `{"fingerprint":"`+fp+`","purpose":"SIGN"}`)
	post("/v1/identity/challenges", `{"fingerprint":"`+fp+`","purpose":"SIGN"}`)
	post("/v1/contributors/me/keys/rotate", `{"old":{"challenge_id":"a","lines":[],"signature":"s"},"new_jwk":{"kty":"EC","crv":"P-256","x":"`+pad(gx.Bytes())+`","y":"`+pad(gy.Bytes())+`"},"new":{"challenge_id":"b","lines":[],"signature":"s"}}`)
	mu.Lock()
	defer mu.Unlock()
	// Each endpoint bills two subjects (IP + fingerprint): 3
	// challenges + 1 register + 1 rotate(register op) = 10 units.
	if used["calls"] != 10 {
		t.Fatalf("ceremony quota calls = %d, want 10", used["calls"])
	}
	policy := domain.DefaultQuotaPolicy()
	challengeBudget, _ := policy.Lookup(domain.OperationChallenge)
	registerBudget, _ := policy.Lookup(domain.OperationRegister)
	// Challenge units: 3 endpoints x 2 subjects = 6 of 60; register
	// units: 2 endpoints x 2 subjects = 4 of 10. Both viable with
	// headroom for legitimate retries.
	if usedChallenge, want := 6, int(challengeBudget.Limit); usedChallenge >= want {
		t.Fatalf("challenge units %d must stay below %d", usedChallenge, want)
	}
	if usedRegister, want := 4, int(registerBudget.Limit); usedRegister >= want {
		t.Fatalf("register units %d must stay below %d", usedRegister, want)
	}
}

// TestGlobalOverloadShape pins the distinct 503 frontend state with a
// retry hint and no identity echo (SEC02-C).
func TestGlobalOverloadShape(t *testing.T) {
	fp := "fp:" + strings.Repeat("f", 64)
	h := Handler{
		Registrar:     noRegistrar{},
		Authority:     "test.invalid",
		QuotaSecret:   []byte(strings.Repeat("a", 32)),
		GlobalBreaker: NewGlobalBreaker(1, 0, 0),
		CheckQuota:    func(context.Context, string, string) (time.Duration, error) { return 0, nil },
	}
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	post := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/v1/identity/challenges",
			strings.NewReader(`{"fingerprint":"`+fp+`","purpose":"REGISTER"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.44:9000"
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	_ = post()
	second := post()
	if second.Code != 503 || !strings.Contains(second.Body.String(), "identity.overloaded") {
		t.Fatalf("overload must be 503 overloaded, got %d (%s)", second.Code, second.Body.String())
	}
	if second.Header().Get("Retry-After") == "" {
		t.Error("overload must carry Retry-After")
	}
	if strings.Contains(second.Body.String(), "203.0.113") {
		t.Error("overload refusal must not echo the client address")
	}
}
