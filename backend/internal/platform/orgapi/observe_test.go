package orgapi_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/telemetry/metrics"
	"github.com/go-chi/chi/v5"
)

func TestOrganizationObservationUsesRegisteredCodesAndTemplates(t *testing.T) {
	reg := metrics.NewRegistry()
	observe, err := orgapi.Observe(reg)
	if err != nil {
		t.Fatal(err)
	}
	gateway, _ := gatewayFixture(t)
	router := chi.NewRouter()
	router.Use(observe)
	refusal := error(nil)
	if err := orgapi.Register(router, route, gateway, orgapi.Rule{Permission: "org.settings.read"}, func(context.Context, orgapi.Principal, readDTO) (any, error) {
		return struct{}{}, refusal
	}); err != nil {
		t.Fatal(err)
	}
	body := `{"family_id":"` + familyID + `","access_token":"synthetic-access","value":"PRIVATE-DOCUMENT"}`
	for _, failure := range []error{nil, orgapi.Conflict, errors.New("PRIVATE-PROVIDER-ERROR"), orgapi.Code("PRIVATE-CODE")} {
		refusal = failure
		call(router, "/v1/orgs/"+orgA+"/example/read", body, nil)
	}
	out := reg.Expose()
	for _, want := range []string{
		`org_requests_total{code="OK",result="success",route="` + route + `"} 1`,
		`org_requests_total{code="CONFLICT",result="refused",route="` + route + `"} 1`,
		`org_requests_total{code="UNAVAILABLE",result="unavailable",route="` + route + `"} 2`,
		"org_request_seconds_total",
	} {
		if !strings.Contains(out, want) {
			t.Fatal("missing observation", want)
		}
	}
	for _, secret := range []string{orgA, familyID, "synthetic-access", "PRIVATE-"} {
		if strings.Contains(out, secret) {
			t.Fatal("private metric label")
		}
	}
	if _, err := orgapi.Observe(nil); err == nil {
		t.Fatal("nil registry accepted")
	}
}

func TestObserverExcludesPublicRequestsAndRefusesConflictingRegistration(t *testing.T) {
	for _, name := range []string{"org_requests_total", "org_request_seconds_total"} {
		reg := metrics.NewRegistry()
		if _, err := reg.Counter(name, "h", "foreign_label"); err != nil {
			t.Fatal(err)
		}
		if _, err := orgapi.Observe(reg); err == nil {
			t.Fatal("conflicting labels accepted")
		}
	}
	reg := metrics.NewRegistry()
	observe, _ := orgapi.Observe(reg)
	router := chi.NewRouter()
	router.Use(observe)
	router.Get("/public/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	router.Post(route, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(403) })
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/public/PRIVATE-REFERENCE", nil))
	if strings.Contains(reg.Expose(), `route="`) {
		t.Fatal("public request counted as organization telemetry")
	}
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/orgs/"+orgA+"/example/read", nil))
	if !strings.Contains(reg.Expose(), `code="REFUSED"`) {
		t.Fatal("unmapped refusal lost")
	}
}
