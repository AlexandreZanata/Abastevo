package orgapi_test

import (
	"context"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
	"net/http"
	"testing"
)

func TestCompanyFreshnessFollowsCurrentScopeAndCannotBeOmitted(t *testing.T) {
	gateway, state := gatewayFixture(t)
	confirmed := 0
	gateway.WebSessionConfirm = func(_ *http.Request, fresh bool) (string, error) {
		confirmed++
		if fresh {
			return "", orgapi.FreshAuthRequired
		}
		return accountID, nil
	}
	r := chi.NewRouter()
	if err := orgapi.Register(r, route, gateway, orgapi.Rule{Permission: "org.settings.read", Fresh: true}, func(context.Context, orgapi.Principal, readDTO) (any, error) {
		state.work++
		return map[string]bool{"ok": true}, nil
	}); err != nil {
		t.Fatal(err)
	}
	web := func(request *http.Request) {
		request.Header.Del("Signature")
		request.Header.Set("Origin", origin)
		request.Header.Set("X-CSRF", "synthetic-csrf")
		request.AddCookie(&http.Cookie{Name: "__Host-empresas-session", Value: "synthetic-cookie"})
	}
	response := call(r, "/v1/orgs/"+orgA+"/example/read", "{}", web)
	if response.Code != 401 || state.work != 0 || confirmed != 1 {
		t.Fatal("fresh guard bypass", response.Code)
	}
	response = call(r, "/v1/orgs/"+orgB+"/example/read", "{}", web)
	if response.Code != 404 || confirmed != 1 {
		t.Fatal("fresh check preceded current foreign-scope denial", response.Code)
	}
	gateway.WebSessionConfirm = nil
	if err := orgapi.Register(chi.NewRouter(), route, gateway, orgapi.Rule{Permission: "org.settings.read"}, func(context.Context, orgapi.Principal, readDTO) (any, error) { return nil, nil }); err == nil {
		t.Fatal("missing confirm guard accepted")
	}
}
