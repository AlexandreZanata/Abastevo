package orgapi_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

// Explicit test policy port, never a production default.
type admissionStub struct {
	validateErr error
	checkErr    error
	calls       int
	check       func()
}

func (s *admissionStub) Validate(orgapi.Rule) error { return s.validateErr }
func (s *admissionStub) Check(_ context.Context, p orgapi.Principal, r orgapi.Rule) error {
	s.calls++
	if p.Scope.Validate() != nil || p.AccountID != accountID || !r.Write {
		return errors.New("invalid test principal")
	}
	if s.check != nil {
		s.check()
	}
	return s.checkErr
}

func TestWritesRequireExplicitAdmissionAndPrivateRateFailures(t *testing.T) {
	rule := orgapi.Rule{Permission: "org.settings.manage", Write: true, RequireRevision: true}
	body := `{"family_id":"` + familyID + `","access_token":"synthetic-access","schema_version":1,"operation_id":"` + membershipID + `","revision":1,"value":""}`
	for _, missing := range []bool{true, false} {
		g, _ := gatewayFixture(t)
		g.WriteAdmission = nil
		if !missing {
			g.WriteAdmission = &admissionStub{validateErr: errors.New("PRIVATE-POLICY")}
		}
		if err := orgapi.Register(chi.NewRouter(), route, g, rule, func(context.Context, orgapi.Principal, commandDTO) (any, error) { return nil, nil }); err == nil {
			t.Fatal("write without matching policy registered")
		}
	}
	for _, test := range []struct {
		err    error
		status int
		retry  string
	}{
		{nil, 200, ""},
		{&orgapi.RateLimitError{RetryAfter: 1501 * time.Millisecond}, 429, "2"},
		{&orgapi.RateLimitError{RetryAfter: time.Nanosecond}, 429, "1"},
		{&orgapi.RateLimitError{RetryAfter: 0}, 503, ""},
		{errors.New("PRIVATE-DATABASE-FAILURE"), 503, ""},
		{orgapi.Forbidden, 503, ""},
	} {
		g, state := gatewayFixture(t)
		probe := &admissionStub{checkErr: test.err}
		g.WriteAdmission = probe
		router := chi.NewRouter()
		if err := orgapi.Register(router, route, g, rule, func(context.Context, orgapi.Principal, commandDTO) (any, error) { state.work++; return struct{}{}, nil }); err != nil {
			t.Fatal(err)
		}
		w := call(router, "/v1/orgs/"+orgA+"/example/read", body, nil)
		if w.Code != test.status || w.Header().Get("Retry-After") != test.retry || probe.calls != 1 || strings.Contains(w.Body.String(), "PRIVATE-") {
			t.Fatal("unsafe admission result", w.Code, w.Header())
		}
		if (state.work == 1) != (test.status == 200) {
			t.Fatal("denied quota entered module")
		}
	}
}

func TestDeniedAuthorityAndReadsNeverConsumeWriteBudgets(t *testing.T) {
	g, state := gatewayFixture(t)
	probe := &admissionStub{}
	g.WriteAdmission = probe
	router := chi.NewRouter()
	rule := orgapi.Rule{Permission: "org.settings.manage", Write: true}
	if err := orgapi.Register(router, route, g, rule, func(context.Context, orgapi.Principal, commandDTO) (any, error) { state.work++; return struct{}{}, nil }); err != nil {
		t.Fatal(err)
	}
	body := `{"family_id":"` + familyID + `","access_token":"synthetic-access","schema_version":1,"operation_id":"` + membershipID + `","value":""}`
	call(router, "/v1/orgs/"+orgB+"/example/read", body, nil)
	state.denied = orgapi.MembershipSuspended
	call(router, "/v1/orgs/"+orgA+"/example/read", body, nil)
	state.denied = nil
	call(router, "/v1/orgs/"+orgA+"/example/read", "{}", nil)
	if probe.calls != 0 || state.work != 0 {
		t.Fatal("unresolved authority spent quota")
	}
	router = chi.NewRouter()
	g.WriteAdmission = nil
	if err := orgapi.Register(router, route, g, orgapi.Rule{Permission: "org.settings.read"}, func(context.Context, orgapi.Principal, readDTO) (any, error) { return struct{}{}, nil }); err != nil {
		t.Fatal(err)
	}
	if w := call(router, "/v1/orgs/"+orgA+"/example/read", `{"family_id":"`+familyID+`","access_token":"synthetic-access"}`, nil); w.Code != 200 {
		t.Fatal("read acquired a new write quota")
	}
}

func TestFreshnessAndCancellationPrecedeModuleAdmission(t *testing.T) {
	g, state := gatewayFixture(t)
	probe := &admissionStub{}
	g.WriteAdmission = probe
	g.WebSessionConfirm = func(*http.Request, bool) (string, error) { return "", orgapi.FreshAuthRequired }
	router := chi.NewRouter()
	if err := orgapi.Register(router, route, g, orgapi.Rule{Permission: "org.settings.manage", Write: true, Fresh: true}, func(context.Context, orgapi.Principal, commandDTO) (any, error) { state.work++; return struct{}{}, nil }); err != nil {
		t.Fatal(err)
	}
	body := `{"schema_version":1,"operation_id":"` + membershipID + `","value":""}`
	w := call(router, "/v1/orgs/"+orgA+"/example/read", body, func(r *http.Request) {
		r.Header.Del("Signature")
		r.Header.Set("Origin", origin)
		r.Header.Set("X-CSRF", "synthetic-csrf")
		r.AddCookie(&http.Cookie{Name: "__Host-empresas-session", Value: "synthetic-cookie"})
	})
	if w.Code != 401 || probe.calls != 0 {
		t.Fatal("fresh auth consumed quota")
	}
	g, state = gatewayFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	g.WriteAdmission = &admissionStub{check: cancel}
	router = chi.NewRouter()
	if err := orgapi.Register(router, route, g, orgapi.Rule{Permission: "org.settings.manage", Write: true}, func(context.Context, orgapi.Principal, commandDTO) (any, error) { state.work++; return struct{}{}, nil }); err != nil {
		t.Fatal(err)
	}
	w = call(router, "/v1/orgs/"+orgA+"/example/read", `{"family_id":"`+familyID+`","access_token":"synthetic-access","schema_version":1,"operation_id":"`+membershipID+`","value":""}`, func(r *http.Request) { *r = *r.WithContext(ctx) })
	if w.Code != 503 || state.work != 0 {
		t.Fatal("cancelled admission entered module")
	}
}
