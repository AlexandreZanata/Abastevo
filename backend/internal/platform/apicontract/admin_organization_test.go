package apicontract

import (
	"encoding/json"
	"net/http"
	"testing"

	adminpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/adapters"
	adminhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/adapters/http"
	adminapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/application"
	admindomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func operatorRoutes(t *testing.T) (*openapi3.T, chi.Router) {
	t.Helper()
	pool, e := pgxpool.New(t.Context(), "postgres://synthetic@127.0.0.1:1/synthetic?sslmode=disable")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	service := &adminapp.Service{Store: adminpg.NewStore(pool), Keys: []admindomain.ConfiguredKey{{}}}
	h := &adminhttp.Handler{Service: service, Origin: "https://company.example.invalid"}
	router := chi.NewRouter()
	if e := enterprisesuite.RegisterAdmin(router, enterprisesuite.Options{Enabled: true, Pool: pool}, h); e != nil {
		t.Fatal(e)
	}
	return loadSpec(t), router
}
func TestOrganizationOperatorSchemasAndActualGuardDrift(t *testing.T) {
	doc, router := operatorRoutes(t)
	if e := ValidateOrganizationOperators(router, doc); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"AdminOrganizationSuspensionRequest", "AdminOrganizationSuspensionAcknowledgement"} {
		schema := doc.Components.Schemas[name]
		if schema == nil || schema.Value == nil || schema.Value.Example == nil {
			t.Fatal("missing operator example")
		}
		raw, _ := json.Marshal(schema.Value.Example)
		var v any
		if json.Unmarshal(raw, &v) != nil || schema.Value.VisitJSON(v) != nil {
			t.Fatal("invalid operator example")
		}
		v.(map[string]any)["account_id"] = "synthetic"
		if schema.Value.VisitJSON(v) == nil {
			t.Fatal("caller authority accepted")
		}
	}
	const path = "/v1/admin/organizations/{org_id}/suspend"
	for _, kind := range []string{"cookie", "fresh", "header", "raw", "missing"} {
		t.Run(kind, func(t *testing.T) {
			doc, r := operatorRoutes(t)
			op := doc.Paths.Value(path).Post
			switch kind {
			case "cookie":
				op.Security = nil
			case "fresh":
				op.Extensions["x-abastevo-organization-operator"] = map[string]any{"fresh": false}
			case "header":
				op.Parameters = nil
			case "raw":
				r.Post(path, func(http.ResponseWriter, *http.Request) {})
			case "missing":
				doc.Paths.Delete(path)
			}
			if ValidateOrganizationOperators(r, doc) == nil {
				t.Fatal("operator drift accepted")
			}
		})
	}
}
