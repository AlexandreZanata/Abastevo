package apicontract

import (
	"encoding/json"
	"testing"
)

func TestLegalDeclarationContractExamplesAndGuardMetadata(t *testing.T) {
	document, router := companyRoutes(t)
	if err := ValidateOrganizationRoutes(router, document); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"EnterpriseLegalCreateRequest", "EnterpriseLegalWithdrawRequest", "EnterpriseEstablishmentCreateRequest", "EnterpriseEstablishmentChangeRequest", "EnterpriseLegalReadRequest", "EnterpriseLegalListRequest", "EnterpriseEstablishmentListRequest", "EnterpriseLegalAcknowledgement", "EnterpriseLegalEntityView", "EnterpriseLegalIdentifierResult", "EnterpriseEstablishmentView", "EnterpriseLegalPage", "EnterpriseEstablishmentPage"} {
		schema := document.Components.Schemas[name]
		if schema == nil || schema.Value == nil || schema.Value.Example == nil {
			t.Fatal("missing legal schema/example", name)
		}
		raw, err := json.Marshal(schema.Value.Example)
		if err != nil {
			t.Fatal(err)
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		if err := schema.Value.VisitJSON(value); err != nil {
			t.Fatal("invalid legal example", name, err)
		}
		value.(map[string]any)["account_id"] = "7b0e2a54-0000-4000-8000-000000000001"
		if schema.Value.VisitJSON(value) == nil {
			t.Fatal("extra authority accepted", name)
		}
	}
	create := document.Components.Schemas["EnterpriseLegalCreateRequest"].Value
	for _, value := range []map[string]any{
		{"schema_version": 1, "operation_id": "7b0e2a54-0000-4000-8000-000000001004", "kind": "COMPANY", "legal_name": "Synthetic company", "identifier": "EXEMPL01000170", "purpose": "VERIFICATION_AND_AGREEMENTS"},
		{"schema_version": 1, "operation_id": "7b0e2a54-0000-4000-8000-000000001004", "kind": "NATURAL_PERSON", "legal_name": "Synthetic party", "identifier": "synthetic", "purpose": "VERIFICATION_AND_AGREEMENTS", "establishment": map[string]any{}},
		{"schema_version": 1, "operation_id": "7b0e2a54-0000-4000-8000-000000001004", "kind": "NATURAL_PERSON", "legal_name": "Synthetic party", "identifier": "synthetic"},
	} {
		if create.VisitJSON(value) == nil {
			t.Fatal("cross-kind declaration fields accepted")
		}
	}
	const path = "/v1/orgs/{org_id}/legal-entities/{legal_entity_id}/establishments/{establishment_id}/change"
	for _, test := range []string{"cookie", "metadata", "resource parameter"} {
		t.Run(test, func(t *testing.T) {
			doc, r := companyRoutes(t)
			operation := doc.Paths.Value(path).Post
			switch test {
			case "cookie":
				operation.Security = nil
			case "metadata":
				operation.Extensions["x-abastevo-organization"] = map[string]any{"permission": "org.settings.manage", "success": 200}
			case "resource parameter":
				for i, p := range operation.Parameters {
					if p.Value != nil && p.Value.Name == "establishment_id" {
						operation.Parameters = append(operation.Parameters[:i], operation.Parameters[i+1:]...)
						break
					}
				}
			}
			if ValidateOrganizationRoutes(r, doc) == nil {
				t.Fatal("legal contract drift accepted")
			}
		})
	}
}
