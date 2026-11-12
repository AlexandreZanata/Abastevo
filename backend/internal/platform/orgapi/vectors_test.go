package orgapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

func TestSharedConventionVectorsThroughHTTP(t *testing.T) {
	raw, err := os.ReadFile("../../../../contracts/testdata/enterprise/api-conventions-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Version int `json:"version"`
		Vectors []struct {
			Name, Kind string
			Status     int
			Value      map[string]any
		} `json:"vectors"`
	}
	if err := json.Unmarshal(raw, &data); err != nil || data.Version != 1 || len(data.Vectors) < 10 {
		t.Fatal("missing convention vectors")
	}
	for _, vector := range data.Vectors {
		t.Run(vector.Name, func(t *testing.T) {
			gateway, state := gatewayFixture(t)
			router := chi.NewRouter()
			if vector.Kind == "read" {
				err = orgapi.Register(router, route, gateway, orgapi.Rule{Permission: "org.settings.read"}, func(context.Context, orgapi.Principal, pageDTO) (any, error) { state.work++; return struct{}{}, nil })
			} else {
				err = orgapi.Register(router, route, gateway, orgapi.Rule{Permission: "org.settings.manage", Write: true}, func(context.Context, orgapi.Principal, orgapi.Command) (any, error) {
					state.work++
					return struct{}{}, nil
				})
			}
			if err != nil {
				t.Fatal(err)
			}
			body, err := json.Marshal(vector.Value)
			if err != nil {
				t.Fatal(err)
			}
			response := call(router, "/v1/orgs/"+orgA+"/example/read", string(body), func(r *http.Request) {
				if vector.Value["family_id"] == nil && vector.Value["access_token"] == nil {
					r.Header.Del("Signature")
					r.Header.Set("Origin", origin)
					r.Header.Set("X-CSRF", "synthetic-csrf")
					r.AddCookie(&http.Cookie{Name: "__Host-empresas-session", Value: "synthetic-cookie"})
				}
			})
			if response.Code != vector.Status || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("ETag") != "" {
				t.Fatalf("vector returned %d %s", response.Code, response.Body.String())
			}
			if (state.work == 1) != (vector.Status == 200) {
				t.Fatal("rejected vector reached module")
			}
		})
	}
}
