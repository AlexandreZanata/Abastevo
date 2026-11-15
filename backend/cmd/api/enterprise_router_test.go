package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/apicontract"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
)

func TestFullProductionRouterEnterpriseContractAndDefaultOff(t *testing.T) {
	for _, enabled := range []string{"false", "true"} {
		t.Run(enabled, func(t *testing.T) {
			for key, value := range map[string]string{"ANPFUEL_ENV": "development", "ANPFUEL_DATABASE_URL": "postgres://synthetic@127.0.0.1:1/synthetic?sslmode=disable", "ANPFUEL_CANONICAL_HOST": "company.example.invalid", "ANPFUEL_ENTERPRISE_ENABLED": enabled, "ANPFUEL_FORUM_ENABLED": "false", "ANPFUEL_MAIL_MODE": "off", "ANPFUEL_METRICS_ADDR": "", "ANPFUEL_MAP_DIST_DIR": "", "ANPFUEL_MAP_DIST_PUBLIC_KEYS": "", "ANPFUEL_ADMIN_KEY_SHA256": "", "ANPFUEL_ADMIN_KEY_SHA256_NEXT": "", "ANPFUEL_R2_ENDPOINT": "", "ANPFUEL_R2_BUCKET": "", "ANPFUEL_R2_ACCESS_KEY_ID": "", "ANPFUEL_R2_SECRET_ACCESS_KEY": ""} {
				t.Setenv(key, value)
			}
			stopped := errors.New("synthetic composition checkpoint")
			observed := false
			err := runWithRouterCheck(func(router chi.Router) error {
				observed = true
				document, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "..", "contracts", "openapi", "v1.yaml"))
				if err != nil {
					t.Fatal(err)
				}
				count := 0
				if err := chi.Walk(router, func(method, path string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
					count++
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				if count < 100 {
					t.Fatal("not the complete production router", count)
				}
				if enabled == "true" {
					mounted := map[string]bool{}
					if err := chi.Walk(router, func(method, path string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
						mounted[method+" "+path] = true
						return nil
					}); err != nil {
						t.Fatal(err)
					}
					for _, path := range []string{
						"/v1/orgs/{org_id}/fleet/assets/create",
						"/v1/orgs/{org_id}/fleet/assets/list",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/read",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/change",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/discard",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/activate",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/acquisition/record",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/deactivate",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/reactivate",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/dispose",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/disposal/revert",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/archive",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/lifecycle/list",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/unit-allocations/list",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/cost-centre-allocations/list",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/assignments/create",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/assignments/{assignment_id}/end",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/assignments/list",
						"/v1/orgs/{org_id}/fleet/assignments/mine/list",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/reservations/request",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/reservations/{reservation_id}/confirm",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/reservations/{reservation_id}/cancel",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/reservations/list",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/blocks/create",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/blocks/{block_id}/remove",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/blocks/list",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/availability/read",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/attachments/attach",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/attachments/{attachment_id}/detach",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/attachments/list",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/meters/install",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/meters/list",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/meters/{meter_id}/latest",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/meters/{meter_id}/readings/list",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/meters/{meter_id}/readings/suspect/list",
						"/v1/orgs/{org_id}/fleet/assets/{asset_id}/meters/{meter_id}/readings/{reading_id}/decide",
						"/v1/orgs/{org_id}/checklists/templates/create",
						"/v1/orgs/{org_id}/checklists/templates/list",
						"/v1/orgs/{org_id}/checklists/templates/{template_id}/read",
						"/v1/orgs/{org_id}/checklists/templates/{template_id}/publish",
						"/v1/orgs/{org_id}/checklists/templates/{template_id}/policy",
						"/v1/orgs/{org_id}/checklists/templates/{template_id}/archive",
						"/v1/orgs/{org_id}/checklists/assets/{asset_id}/applicable",
						"/v1/orgs/{org_id}/incidents/reports/list",
						"/v1/orgs/{org_id}/incidents/reports/mine",
						"/v1/orgs/{org_id}/incidents/reports/{report_id}/read",
						"/v1/orgs/{org_id}/incidents/reports/{report_id}/decide",
						"/v1/orgs/{org_id}/trips/review/list",
						"/v1/orgs/{org_id}/trips/{trip_id}/review/read",
						"/v1/orgs/{org_id}/trips/{trip_id}/review/decide",
						"/v1/orgs/{org_id}/missions/create",
						"/v1/orgs/{org_id}/missions/list",
						"/v1/orgs/{org_id}/missions/mine/list",
						"/v1/orgs/{org_id}/missions/{mission_id}/read",
						"/v1/orgs/{org_id}/missions/{mission_id}/assign",
						"/v1/orgs/{org_id}/missions/{mission_id}/cancel",
						"/v1/orgs/{org_id}/finance/periods/list",
						"/v1/orgs/{org_id}/finance/periods/{period}/read",
						"/v1/orgs/{org_id}/finance/periods/{period}/close",
						"/v1/orgs/{org_id}/finance/periods/{period}/reopen",
						"/v1/orgs/{org_id}/documents/uploads/create",
						"/v1/orgs/{org_id}/documents/{document_id}/content/upload",
						"/v1/orgs/{org_id}/documents/{document_id}/content/access",
						"/v1/orgs/{org_id}/documents/{document_id}/content/read",
						"/v1/orgs/{org_id}/documents/{document_id}/links/create",
						"/v1/orgs/{org_id}/documents/{document_id}/links/list",
						"/v1/orgs/{org_id}/documents/links/by-target",
						"/v1/orgs/{org_id}/reports/exports/request",
						"/v1/orgs/{org_id}/reports/exports/list",
						"/v1/orgs/{org_id}/reports/exports/{export_id}/read",
						"/v1/orgs/{org_id}/reports/exports/{export_id}/access",
						"/v1/orgs/{org_id}/reports/exports/{export_id}/content",
					} {
						if !mounted[http.MethodPost+" "+path] {
							t.Fatal("fleet or document contract route missing from complete production router", path)
						}
					}
					if err := apicontract.ValidateOrganizationRoutes(router, document); err != nil {
						t.Fatal("fully composed private route drift", err)
					}
					// Adding one unguarded entry to the full router must fail the same gate.
					router.Post("/v1/orgs/{org_id}/synthetic/unguarded", func(http.ResponseWriter, *http.Request) {})
					if err := apicontract.ValidateOrganizationRoutes(router, document); err == nil {
						t.Fatal("unguarded production route accepted")
					}
				} else {
					// Walk the complete router: no organization entry exists by default,
					// not only the sampled paths below.
					if mounted := organizationRoutes(t, router); len(mounted) != 0 {
						t.Fatal("default production router mounts organization routes", mounted)
					}
					probe := chi.NewRouter()
					probe.Mount("/", router)
					probe.Post("/v1/org-access/synthetic", func(http.ResponseWriter, *http.Request) {})
					if len(organizationRoutes(t, probe)) != 1 {
						t.Fatal("organization route walk missed a mounted entry")
					}
					for _, path := range []string{"/v1/org-access/organizations/create", "/v1/orgs/7b0e2a54-0000-4000-8000-000000000301/units/create", "/v1/orgs/7b0e2a54-0000-4000-8000-000000000301/units/7b0e2a54-0000-4000-8000-000000000801/read", "/v1/orgs/7b0e2a54-0000-4000-8000-000000000301/cost-centres/create", "/v1/orgs/7b0e2a54-0000-4000-8000-000000000301/cost-centres/code/read"} {
						w := httptest.NewRecorder()
						router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "https://company.example.invalid"+path, nil))
						if w.Code != 404 {
							t.Fatal("default production surface exposed", w.Code)
						}
					}
				}
				return stopped
			})
			if !observed || !errors.Is(err, stopped) {
				t.Fatal("did not reach full router checkpoint", err)
			}
		})
	}
}

func organizationRoutes(t *testing.T, router chi.Routes) []string {
	t.Helper()
	var mounted []string
	if err := chi.Walk(router, func(method, path string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if orgapi.IsOrganizationPath(path) {
			mounted = append(mounted, method+" "+path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return mounted
}
