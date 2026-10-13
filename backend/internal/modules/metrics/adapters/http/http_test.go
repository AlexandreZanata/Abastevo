package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/metrics/application"
	"github.com/go-chi/chi/v5"
)

const family = "11111111-1111-4111-8111-111111111111"
const owner = "22222222-2222-4222-8222-222222222222"

type probe struct{ result application.Comparison }

func (p *probe) Compare(context.Context, string, application.Side, application.Side) (application.Comparison, error) {
	return p.result, nil
}

func call(h Handler, body string) *httptest.ResponseRecorder {
	router := chi.NewRouter()
	h.RegisterRoutes(router)
	r := httptest.NewRequest(http.MethodPost, "/v1/metrics/compare", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func testHandler(p *probe) Handler {
	return Handler{
		Service: p,
		Sessions: func(_ context.Context, f, a string) (string, error) {
			if f != family || a != "synthetic" {
				return "", errFake()
			}
			return owner, nil
		},
		SignedOwner: func(_ *http.Request, _ string) (string, string, error) {
			return "33333333-3333-4333-8333-333333333333", "44444444-4444-4444-8444-444444444444", nil
		},
	}
}

type fakeErr struct{}

func (fakeErr) Error() string { return "metrics: invalid input" }

func errFake() error { return fakeErr{} }

func TestCompareNoWinnerFabrication(t *testing.T) {
	p := &probe{result: application.Comparison{Compatible: true, Baseline: "10", Current: "100/7"}}
	h := testHandler(p)
	side := func(vehicle string) string {
		return `{"vehicle_id":"` + vehicle + `","from":"2026-01-01T00:00:00Z","to":"2026-02-01T00:00:00Z","timezone":"UTC","method":"fuel-purchase-interval"}`
	}
	base := `{"family_id":"` + family + `","access_token":"synthetic","baseline":` + side("11111111-1111-4111-8111-111111111111") + `,"current":` + side("22222222-2222-4222-8222-222222222222") + `}`
	w := call(h, base)
	if w.Code != 200 || w.Header().Get("ETag") != "" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("compare %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"compatible":true`) || strings.Contains(w.Body.String(), "winner") {
		t.Fatal("fabricated winner")
	}
	for _, bad := range []string{
		strings.Replace(base, `"method":"fuel-purchase-interval"`, `"method":"unknown"`, 1),
		strings.Replace(base, `"family_id"`, `"account_id"`, 1),
	} {
		_ = bad
	}
	unsigned := Handler{Service: p, Sessions: testHandler(p).Sessions}
	if w = call(unsigned, base); w.Code == 200 {
		t.Fatal("unsigned admitted")
	}
	p.result = application.Comparison{Compatible: false, Issue: "missing-baseline"}
	if w = call(h, base); w.Code != 200 || !strings.Contains(w.Body.String(), "missing-baseline") {
		t.Fatalf("incomplete explained %d", w.Code)
	}
}
