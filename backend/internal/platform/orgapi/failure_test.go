package orgapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

func TestRegistrationRejectsIncompleteAuthorityAndRule(t *testing.T) {
	gateway, _ := gatewayFixture(t)
	for _, rule := range []orgapi.Rule{{}, {Permission: "raw input"}, {Permission: "org.settings.read", RequireRevision: true}, {Permission: "org.settings.read", Success: 204}, {Permission: "org.settings.read", Success: 500}} {
		if err := orgapi.Register(chi.NewRouter(), route, gateway, rule, func(context.Context, orgapi.Principal, readDTO) (any, error) { return nil, nil }); err == nil {
			t.Fatal("invalid rule was registered")
		}
	}
	for _, change := range []func(*orgapi.Gateway){
		func(g *orgapi.Gateway) { g.Resolve = nil },
		func(g *orgapi.Gateway) { g.Origin = "https://api.example.invalid/path" },
		func(g *orgapi.Gateway) { g.CursorSecret = nil },
		func(g *orgapi.Gateway) { g.SignedOwner = nil },
		func(g *orgapi.Gateway) { g.Sessions = nil; g.WebSession = nil },
	} {
		changed := gateway
		change(&changed)
		if err := orgapi.Register(chi.NewRouter(), route, changed, orgapi.Rule{Permission: "org.settings.read"}, func(context.Context, orgapi.Principal, readDTO) (any, error) { return nil, nil }); err == nil {
			t.Fatal("unsafe gateway was registered")
		}
	}
	for _, path := range []string{"/v1/public", "/v1/orgs/{org_id}/", "/v1/orgs/{org_id}/*", "/v1/orgs/{org_id}/bad?query"} {
		if err := orgapi.Register(chi.NewRouter(), path, gateway, orgapi.Rule{Permission: "org.settings.read"}, func(context.Context, orgapi.Principal, readDTO) (any, error) { return nil, nil }); err == nil {
			t.Fatal("invalid organization path registered")
		}
	}
}

func TestGatewayRefusesAmbiguousTransportAndResolverMismatch(t *testing.T) {
	for _, change := range []func(*http.Request){
		func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") },
		func(r *http.Request) { r.Header.Add("Content-Type", "application/json") },
		func(r *http.Request) { r.URL.RawQuery = "private=value" },
		func(r *http.Request) { r.Header.Add("Origin", origin); r.Header.Add("Origin", origin) },
	} {
		gateway, state := gatewayFixture(t)
		router := chi.NewRouter()
		if err := orgapi.Register(router, route, gateway, orgapi.Rule{Permission: "org.settings.read"}, func(context.Context, orgapi.Principal, readDTO) (any, error) { state.work++; return nil, nil }); err != nil {
			t.Fatal(err)
		}
		body := `{"family_id":"` + familyID + `","access_token":"synthetic-access","value":""}`
		response := call(router, "/v1/orgs/"+orgA+"/example/read", body, change)
		if response.Code < 400 || state.work != 0 {
			t.Fatal("ambiguous request ran work")
		}
	}
	gateway, state := gatewayFixture(t)
	gateway.Resolve = func(_ context.Context, actor orgapi.Actor, scope kernel.OrgScope, _ orgapi.Rule) (orgapi.Membership, error) {
		return orgapi.Membership{ID: membershipID, AccountID: familyID, OrgID: scope.OrgID()}, nil
	}
	router := chi.NewRouter()
	if err := orgapi.Register(router, route, gateway, orgapi.Rule{Permission: "org.settings.read"}, func(context.Context, orgapi.Principal, readDTO) (any, error) { state.work++; return nil, nil }); err != nil {
		t.Fatal(err)
	}
	body := `{"family_id":"` + familyID + `","access_token":"synthetic-access","value":""}`
	checkError(t, call(router, "/v1/orgs/"+orgA+"/example/read", body, nil), orgapi.NotFound, 404)
	if state.work != 0 {
		t.Fatal("mismatched actor entered module")
	}
}

func TestDependencyAndSerializationFailuresStayPrivate(t *testing.T) {
	gateway, state := gatewayFixture(t)
	router := chi.NewRouter()
	if err := orgapi.Register(router, route, gateway, orgapi.Rule{Permission: "org.settings.read"}, func(context.Context, orgapi.Principal, readDTO) (any, error) { return make(chan int), nil }); err != nil {
		t.Fatal(err)
	}
	body := `{"family_id":"` + familyID + `","access_token":"synthetic-access","value":""}`
	checkError(t, call(router, "/v1/orgs/"+orgA+"/example/read", body, nil), orgapi.Unavailable, 503)
	unavailable := gateway
	unavailable.Sessions = func(context.Context, string, string) (string, error) { return "", orgapi.Unavailable }
	router = chi.NewRouter()
	if err := orgapi.Register(router, route, unavailable, orgapi.Rule{Permission: "org.settings.read"}, func(context.Context, orgapi.Principal, readDTO) (any, error) { state.work++; return nil, nil }); err != nil {
		t.Fatal(err)
	}
	response := call(router, "/v1/orgs/"+orgA+"/example/read", body, nil)
	checkError(t, response, orgapi.Unavailable, 503)
	if state.work != 0 || strings.Contains(response.Body.String(), "synthetic-access") {
		t.Fatal("dependency failure leaked or ran work")
	}
}
