package orgapi_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

func TestAndroidFreshRoutesRequireAndRecheckSessionConfirmation(t *testing.T) {
	g, s := gatewayFixture(t)
	g.SessionConfirm = nil
	if err := orgapi.Register(chi.NewRouter(), route, g, orgapi.Rule{Permission: "idn.credential.manage", Fresh: true}, func(context.Context, orgapi.Principal, readDTO) (any, error) { return nil, nil }); err == nil {
		t.Fatal("fresh Android route accepted without confirmation")
	}
	confirmed := 0
	g.SessionConfirm = func(_ context.Context, family, token string, fresh bool) (string, error) {
		confirmed++
		if family != familyID || token != "synthetic-access" || !fresh {
			t.Fatal("wrong proof confirmed")
		}
		return "", orgapi.FreshAuthRequired
	}
	r := chi.NewRouter()
	if err := orgapi.Register(r, route, g, orgapi.Rule{Permission: "idn.credential.manage", Fresh: true}, func(context.Context, orgapi.Principal, readDTO) (any, error) { s.work++; return struct{}{}, nil }); err != nil {
		t.Fatal(err)
	}
	response := call(r, "/v1/orgs/"+orgA+"/example/read", `{"family_id":"`+familyID+`","access_token":"synthetic-access"}`, func(r *http.Request) { r.Header.Set("Signature", "synthetic-proof") })
	if response.Code != 401 || confirmed != 1 || s.work != 0 {
		t.Fatal(response.Code, confirmed, s.work)
	}
	response = call(r, "/v1/orgs/"+orgB+"/example/read", `{"family_id":"`+familyID+`","access_token":"synthetic-access"}`, func(r *http.Request) { r.Header.Set("Signature", "synthetic-proof") })
	if response.Code != 404 || confirmed != 1 {
		t.Fatal("fresh confirmation preceded foreign scope refusal", response.Code, confirmed)
	}
}
