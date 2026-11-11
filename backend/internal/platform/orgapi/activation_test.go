package orgapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

func TestGlobalOrganizationSurfaceSwitch(t *testing.T) {
	for _, enabled := range []bool{false, true, false, true} {
		gateway, state := gatewayFixture(t)
		gateway.Enabled = enabled
		router := chi.NewRouter()
		if err := orgapi.Register(router, route, gateway, orgapi.Rule{Permission: "org.settings.read"},
			func(context.Context, orgapi.Principal, readDTO) (any, error) {
				state.work++
				return map[string]bool{"ok": true}, nil
			}); err != nil {
			t.Fatal(err)
		}
		response := call(router, "/v1/orgs/"+orgA+"/example/read", `{"family_id":"`+familyID+`","access_token":"synthetic-access"}`, nil)
		if !enabled {
			if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"code":"NOT_FOUND"`) ||
				state.auth != 0 || state.resolved != 0 || state.work != 0 {
				t.Fatalf("global off exposed handler: %d %s %+v", response.Code, response.Body.String(), state)
			}
		} else if response.Code != http.StatusOK || state.work != 1 {
			t.Fatal(response.Code, response.Body.String())
		}
		if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("ETag") != "" {
			t.Fatal("private cache policy lost")
		}
	}
}
