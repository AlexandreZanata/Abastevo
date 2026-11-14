package apicontract

import (
	"net/http"
	"testing"
)

func TestAccountContextContractsRequireOwnAccountEntry(t *testing.T) {
	document, router := companyRoutes(t)
	if e := ValidateOrganizationRoutes(router, document); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"EnterpriseAccountContextListRequest", "EnterpriseAccountContextReadRequest", "EnterpriseAccountContext", "EnterpriseAccountContextListResult", "EnterpriseAccountContextGrant", "EnterpriseAccountContextView"} {
		schema := document.Components.Schemas[name].Value
		if e := schema.VisitJSON(enterpriseContractObject(t, schema.Example)); e != nil {
			t.Fatal(name, e)
		}
	}
	for _, name := range []string{"EnterpriseAccountContextListRequest", "EnterpriseAccountContextReadRequest"} {
		schema := document.Components.Schemas[name].Value
		for _, field := range []string{"account_id", "family_id", "access_token", "session_id", "roles", "permission"} {
			body := enterpriseContractObject(t, schema.Example)
			body[field] = "synthetic"
			if schema.VisitJSON(body) == nil {
				t.Fatal("caller authority accepted", name, field)
			}
		}
	}
	for _, action := range []string{"list", "read"} {
		path := "/v1/org-access/contexts/" + action
		t.Run(action+"/unguarded", func(t *testing.T) {
			d, r := companyRoutes(t)
			r.Post(path, func(http.ResponseWriter, *http.Request) {})
			if ValidateOrganizationRoutes(r, d) == nil {
				t.Fatal("unguarded current contexts")
			}
		})
		t.Run(action+"/binding", func(t *testing.T) {
			d, r := companyRoutes(t)
			d.Paths.Value(path).Post.Extensions["x-abastevo-account-context"] = map[string]any{"action": action, "cookie": true, "csrf": true, "success": 200}
			if ValidateOrganizationRoutes(r, d) == nil {
				t.Fatal("missing account binding")
			}
		})
		t.Run(action+"/alternate", func(t *testing.T) {
			d, r := companyRoutes(t)
			d.Paths.Value(path).Post.Extensions["x-abastevo-personal-invitation"] = map[string]any{}
			if ValidateOrganizationRoutes(r, d) == nil {
				t.Fatal("alternate authority metadata")
			}
		})
	}
}
