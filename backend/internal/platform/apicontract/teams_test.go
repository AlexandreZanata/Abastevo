package apicontract

import (
	"encoding/json"
	"testing"
)

func TestTeamContractExamplesAndGuardDrift(t *testing.T) {
	document, router := companyRoutes(t)
	if err := ValidateOrganizationRoutes(router, document); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"EnterpriseTeamCreateRequest", "EnterpriseTeamChangeRequest", "EnterpriseTeamTransitionRequest", "EnterpriseTeamMemberWriteRequest", "EnterpriseTeamReadRequest", "EnterpriseTeamListRequest", "EnterpriseTeamMemberListRequest", "EnterpriseTeamAcknowledgement", "EnterpriseTeamView", "EnterpriseTeamPage", "EnterpriseTeamLinkView", "EnterpriseTeamLinkPage"} {
		schema := document.Components.Schemas[name]
		if schema == nil || schema.Value == nil || schema.Value.Example == nil {
			t.Fatal("missing team schema/example", name)
		}
		raw, err := json.Marshal(schema.Value.Example)
		if err != nil {
			t.Fatal(err)
		}
		var v any
		if json.Unmarshal(raw, &v) != nil {
			t.Fatal("invalid team example JSON")
		}
		if err := schema.Value.VisitJSON(v); err != nil {
			t.Fatal("invalid team example", name, err)
		}
		v.(map[string]any)["account_id"] = "7b0e2a54-0000-4000-8000-000000000201"
		if schema.Value.VisitJSON(v) == nil {
			t.Fatal("extra team authority accepted", name)
		}
	}
	const path = "/v1/orgs/{org_id}/teams/{team_id}/change"
	for _, test := range []string{"cookie", "metadata", "selector"} {
		t.Run(test, func(t *testing.T) {
			doc, r := companyRoutes(t)
			operation := doc.Paths.Value(path).Post
			switch test {
			case "cookie":
				operation.Security = nil
			case "metadata":
				operation.Extensions["x-abastevo-organization"] = map[string]any{"permission": "idn.employee.manage", "success": 200}
			case "selector":
				for i, p := range operation.Parameters {
					if p.Value != nil && p.Value.Name == "team_id" {
						operation.Parameters = append(operation.Parameters[:i], operation.Parameters[i+1:]...)
						break
					}
				}
			}
			if ValidateOrganizationRoutes(r, doc) == nil {
				t.Fatal("team guard contract drift accepted")
			}
		})
	}
}
