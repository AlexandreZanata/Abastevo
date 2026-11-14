package apicontract

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

func setupContractCopy(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, e := json.Marshal(value)
	if e != nil {
		t.Fatal(e)
	}
	var result map[string]any
	if json.Unmarshal(raw, &result) != nil {
		t.Fatal("invalid setup contract example")
	}
	return result
}

func TestSetupPanelProjectionAndBoundedSelectorContracts(t *testing.T) {
	document := loadSpec(t)
	for _, name := range []string{"EnterpriseSettingsView", "EnterpriseSettingsMembersRequest", "EnterpriseSettingsMembersPage", "EnterpriseOwnedSetupsRequest", "EnterpriseOwnedSetupsPage"} {
		schema := document.Components.Schemas[name]
		if schema == nil || schema.Value.Example == nil {
			t.Fatal("missing frozen setup schema/example", name)
		}
		if e := schema.Value.VisitJSON(setupContractCopy(t, schema.Value.Example)); e != nil {
			t.Fatal("invalid frozen setup example", name, e)
		}
	}
	for _, name := range []string{"EnterpriseSettingsMembersRequest", "EnterpriseOwnedSetupsRequest"} {
		schema := document.Components.Schemas[name].Value
		for _, v := range []map[string]any{{"limit": 0}, {"limit": 101}, {"account_id": "7b0e2a54-0000-4000-8000-000000000201"}, {"organization_id": "7b0e2a54-0000-4000-8000-000000000301"}, {"limit": nil}} {
			if schema.VisitJSON(v) == nil {
				t.Fatal("unbounded or caller-authority selector accepted", name)
			}
		}
	}
	view := document.Components.Schemas["EnterpriseSettingsView"].Value
	for _, mutation := range []func(map[string]any){
		func(v map[string]any) { v["context"].(map[string]any)["roles"] = []any{"OWNER"} },
		func(v map[string]any) { v["context"].(map[string]any)["permissions"] = []any{"fleet.asset.manage"} },
		func(v map[string]any) { v["setup"].(map[string]any)["missing"] = []any{"private-reason"} },
		func(v map[string]any) { v["setup"].(map[string]any)["terms"].(map[string]any)["kind"] = "REAL" },
		func(v map[string]any) { v["organization"].(map[string]any)["state"] = "CLOSED" },
	} {
		copy := setupContractCopy(t, view.Example)
		mutation(copy)
		if view.VisitJSON(copy) == nil {
			t.Fatal("unsafe setup projection accepted")
		}
	}
	member := document.Components.Schemas["EnterpriseSettingsResponsibilityMember"].Value
	if member.VisitJSON(map[string]any{"membership_id": "7b0e2a54-0000-4000-8000-000000000601", "display_name": "Synthetic", "profile_pending": false, "employee_id": "7b0e2a54-0000-4000-8000-000000000602"}) == nil {
		t.Fatal("employee identity leaked through membership picker")
	}
	legacy := setupContractCopy(t, view.Example)
	organization := legacy["organization"].(map[string]any)
	organization["name"] = ""
	organization["time_zone"] = ""
	organization["kinds"] = []any{}
	legacy["setup"].(map[string]any)["ready"] = false
	legacy["setup"].(map[string]any)["missing"] = []any{"name", "time_zone"}
	if e := view.VisitJSON(legacy); e != nil {
		t.Fatal("persisted incomplete setup cannot be represented", e)
	}
}

type ownedSetupsContractHandler struct{ rule orgapi.OwnedSetupsRule }

func (ownedSetupsContractHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}
func (h ownedSetupsContractHandler) OwnedSetupsRule() orgapi.OwnedSetupsRule    { return h.rule }

func TestOwnedSetupsRouteHasOnlyItsBoundAccountBoundary(t *testing.T) {
	const path = "/v1/org-access/organizations/setups/list"
	for _, test := range []string{"cookie", "csrf", "binding", "metadata", "wrong-path", "unguarded"} {
		t.Run(test, func(t *testing.T) {
			document, router := companyRoutes(t)
			switch test {
			case "metadata":
				document.Paths.Value(path).Post.Extensions["x-abastevo-owned-setups"] = orgapi.OwnedSetupsRule{Cookie: true, CSRF: true, Success: 200}
			case "unguarded":
				router.Post(path, func(http.ResponseWriter, *http.Request) {})
			case "wrong-path":
				wrong := "/v1/org-access/organizations/setups/all"
				document.Paths.Set(wrong, document.Paths.Value(path))
				router.Method(http.MethodPost, wrong, ownedSetupsContractHandler{orgapi.OwnedSetupsRule{Cookie: true, CSRF: true, Binding: true, Success: 200}})
			default:
				rule := orgapi.OwnedSetupsRule{Cookie: true, CSRF: true, Binding: true, Success: 200}
				if test == "cookie" {
					rule.Cookie = false
				}
				if test == "csrf" {
					rule.CSRF = false
				}
				if test == "binding" {
					rule.Binding = false
				}
				router.Method(http.MethodPost, path, ownedSetupsContractHandler{rule})
			}
			if ValidateOrganizationRoutes(router, document) == nil {
				t.Fatal("own-created directory escaped its exact frozen account boundary")
			}
		})
	}
}
