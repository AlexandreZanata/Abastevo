package apicontract

import (
	"encoding/json"
	"testing"
)

func TestOperatingUnitContractExamplesAndSourceGuardDrift(t *testing.T) {
	document, router := companyRoutes(t)
	if err := ValidateOrganizationRoutes(router, document); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"EnterpriseUnitCreateRequest", "EnterpriseUnitChangeRequest", "EnterpriseUnitTransitionRequest", "EnterpriseUnitReadRequest", "EnterpriseUnitListRequest", "EnterpriseUnitAcknowledgement", "EnterpriseUnitView", "EnterpriseUnitPage"} {
		schema := document.Components.Schemas[name]
		if schema == nil || schema.Value == nil || schema.Value.Example == nil {
			t.Fatal("missing unit schema/example", name)
		}
		raw, err := json.Marshal(schema.Value.Example)
		if err != nil {
			t.Fatal(err)
		}
		var value any
		if json.Unmarshal(raw, &value) != nil {
			t.Fatal("invalid unit example encoding")
		}
		if err := schema.Value.VisitJSON(value); err != nil {
			t.Fatal("invalid unit example", name, err)
		}
		value.(map[string]any)["account_id"] = "7b0e2a54-0000-4000-8000-000000000201"
		if schema.Value.VisitJSON(value) == nil {
			t.Fatal("extra unit authority accepted", name)
		}
	}
	const path = "/v1/orgs/{org_id}/units/{unit_id}/archive"
	for _, test := range []string{"cookie", "metadata", "target parameter"} {
		t.Run(test, func(t *testing.T) {
			doc, r := companyRoutes(t)
			operation := doc.Paths.Value(path).Post
			switch test {
			case "cookie":
				operation.Security = nil
			case "metadata":
				operation.Extensions["x-abastevo-organization"] = map[string]any{"permission": "org.legal_entity.manage", "success": 200}
			case "target parameter":
				for i, p := range operation.Parameters {
					if p.Value != nil && p.Value.Name == "unit_id" {
						operation.Parameters = append(operation.Parameters[:i], operation.Parameters[i+1:]...)
						break
					}
				}
			}
			if ValidateOrganizationRoutes(r, doc) == nil {
				t.Fatal("unit guard contract drift accepted")
			}
		})
	}
}
