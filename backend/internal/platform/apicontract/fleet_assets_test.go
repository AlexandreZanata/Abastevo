package apicontract

import (
	"encoding/json"
	"slices"
	"testing"

	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
)

func TestFleetAssetContractExamplesAndExactShapes(t *testing.T) {
	document := loadSpec(t)
	for _, name := range []string{"EnterpriseFleetAssetCategoryChoice", "EnterpriseFleetAssetTankRequest", "EnterpriseFleetAssetExpectedRequest", "EnterpriseFleetAssetCreateRequest", "EnterpriseFleetAssetChangeRequest", "EnterpriseFleetAssetTransitionRequest", "EnterpriseFleetAssetReadRequest", "EnterpriseFleetAssetListRequest", "EnterpriseFleetAssetAcknowledgement", "EnterpriseFleetAssetView", "EnterpriseFleetAssetPage"} {
		t.Run(name, func(t *testing.T) {
			schema := document.Components.Schemas[name]
			if schema == nil || schema.Value == nil || schema.Value.Example == nil {
				t.Fatal("missing asset schema or explicit example")
			}
			raw, err := json.Marshal(schema.Value.Example)
			if err != nil {
				t.Fatal(err)
			}
			var example map[string]any
			if json.Unmarshal(raw, &example) != nil {
				t.Fatal("invalid asset object example")
			}
			if err := schema.Value.VisitJSON(example); err != nil {
				t.Fatal("invalid asset example", err)
			}
			example["account_id"] = "7b0e2a54-0000-4000-8000-000000008001"
			if schema.Value.VisitJSON(example) == nil {
				t.Fatal("caller-selected asset authority accepted")
			}
		})
	}
}

func TestFleetAssetContractRejectsAmbiguousCategoryQuantityAndDiscard(t *testing.T) {
	document := loadSpec(t)
	category := document.Components.Schemas["EnterpriseFleetAssetCategoryChoice"].Value
	id := "7b0e2a54-0000-4000-8000-000000008301"
	for _, value := range []map[string]any{{}, {"existing_id": id, "new_label": "Synthetic"}, {"existing_id": nil}, {"new_label": ""}, {"existing_id": id, "state": "ACTIVE"}} {
		if category.VisitJSON(value) == nil {
			t.Fatal("ambiguous category declaration accepted")
		}
	}
	if category.VisitJSON(map[string]any{"existing_id": id}) != nil {
		t.Fatal("existing real category selector refused")
	}
	tank := document.Components.Schemas["EnterpriseFleetAssetTankRequest"].Value
	for _, value := range []any{float64(275000), "0", "01", "1.0", "1000000001"} {
		if tank.VisitJSON(map[string]any{"role": "MAIN", "capacity_milli": value, "products": []any{"DIESEL_S10"}}) == nil {
			t.Fatal("inexact or out-of-bound capacity accepted")
		}
	}
	for _, value := range []any{float64(8500), "0", "01", "1.0"} {
		if document.Components.Schemas["EnterpriseFleetAssetExpectedRequest"].Value.VisitJSON(map[string]any{"product": "DIESEL_S10", "basis": "DISTANCE", "value": value, "source": "Synthetic", "date": "2026-11-14"}) == nil {
			t.Fatal("inexact comparison reference accepted")
		}
	}
	ack := document.Components.Schemas["EnterpriseFleetAssetAcknowledgement"].Value
	discard := map[string]any{"asset_id": "7b0e2a54-0000-4000-8000-000000008201", "revision": float64(2), "discarded": true, "replayed": true, "data_mode": "SYNTHETIC"}
	if err := ack.VisitJSON(discard); err != nil {
		t.Fatal("original discarded receipt refused", err)
	}
	discard["state"] = "ARCHIVED"
	if ack.VisitJSON(discard) == nil {
		t.Fatal("discard invented lifecycle state")
	}
	enum := tank.Properties["products"].Value.Items.Value.Enum
	actual := make([]string, 0, len(enum))
	for _, value := range enum {
		code, ok := value.(string)
		if !ok {
			t.Fatal("product vocabulary type changed")
		}
		actual = append(actual, code)
	}
	slices.Sort(actual)
	expected := make([]string, 0, len(fuel.Products()))
	for _, product := range fuel.Products() {
		expected = append(expected, string(product.Code()))
	}
	slices.Sort(expected)
	if !slices.Equal(actual, expected) {
		t.Fatal("fleet contract differs from owned product catalog")
	}
}

func TestFleetAssetContractBindsActualCurrentCompanyGuards(t *testing.T) {
	document, router := companyRoutes(t)
	if err := ValidateOrganizationRoutes(router, document); err != nil {
		t.Fatal(err)
	}
	const base = "/v1/orgs/{org_id}/fleet/assets"
	for _, suffix := range []string{"/create", "/list", "/{asset_id}/read", "/{asset_id}/change", "/{asset_id}/discard", "/{asset_id}/activate"} {
		path := base + suffix
		if document.Paths.Value(path) == nil || document.Paths.Value(path).Post == nil {
			t.Fatal("missing actual asset route", path)
		}
	}
	for _, test := range []string{"cookie", "fresh", "permission", "revision", "target"} {
		t.Run(test, func(t *testing.T) {
			doc, actual := companyRoutes(t)
			operation := doc.Paths.Value(base + "/{asset_id}/change").Post
			switch test {
			case "cookie":
				operation.Security = nil
			case "fresh", "permission", "revision":
				metadata := map[string]any{"permission": "fleet.asset.manage", "write": true, "fresh": true, "require_revision": true, "company_only": true, "success": 200}
				if test == "fresh" {
					metadata["fresh"] = false
				}
				if test == "permission" {
					metadata["permission"] = "org.structure.manage"
				}
				if test == "revision" {
					metadata["require_revision"] = false
				}
				operation.Extensions["x-abastevo-organization"] = metadata
			case "target":
				for i, parameter := range operation.Parameters {
					if parameter.Value != nil && parameter.Value.Name == "asset_id" {
						operation.Parameters = append(operation.Parameters[:i], operation.Parameters[i+1:]...)
						break
					}
				}
			}
			if ValidateOrganizationRoutes(actual, doc) == nil {
				t.Fatal("asset authority/contract drift accepted")
			}
		})
	}
}
