package apicontract

import (
	"encoding/json"
	"slices"
	"testing"
)

// EMP-FLEET-007: reservation, block and availability shapes are exact and
// closed; no state, requester, tolerance, instant or authority is chosen by
// the caller, and a conflict detail carries only window instants.
func TestFleetReservationContractExamplesAndExactShapes(t *testing.T) {
	document := loadSpec(t)
	for _, name := range []string{"EnterpriseFleetReservationRequest", "EnterpriseFleetReservationActRequest", "EnterpriseFleetReservationAcknowledgement", "EnterpriseFleetReservation", "EnterpriseFleetReservationPage",
		"EnterpriseFleetBlockCreateRequest", "EnterpriseFleetBlockRemoveRequest", "EnterpriseFleetBlockAcknowledgement", "EnterpriseFleetBlock", "EnterpriseFleetBlockPage", "EnterpriseFleetAvailabilityCause", "EnterpriseFleetAvailability"} {
		t.Run(name, func(t *testing.T) {
			schema := document.Components.Schemas[name]
			if schema == nil || schema.Value == nil || schema.Value.Example == nil {
				t.Fatal("missing reservation schema or explicit example")
			}
			raw, err := json.Marshal(schema.Value.Example)
			if err != nil {
				t.Fatal(err)
			}
			var example map[string]any
			if json.Unmarshal(raw, &example) != nil {
				t.Fatal("invalid reservation object example")
			}
			if err := schema.Value.VisitJSON(example); err != nil {
				t.Fatal("invalid reservation example", err)
			}
			for _, field := range []string{"account_id", "membership_id", "requested_by", "tolerance", "unit_id", "at"} {
				injected := map[string]any{}
				for k, v := range example {
					injected[k] = v
				}
				injected[field] = "synthetic"
				if schema.Value.VisitJSON(injected) == nil {
					t.Fatal("caller-selected reservation authority or data accepted", field)
				}
			}
		})
	}
	request := document.Components.Schemas["EnterpriseFleetReservationRequest"].Value
	base := map[string]any{"schema_version": float64(1), "operation_id": "7b0e2a54-0000-4000-8000-000000009101", "starts_at": "2026-12-01T11:00:00Z", "ends_at": "2026-12-01T15:00:00Z", "purpose": "Synthetic purpose"}
	if err := request.VisitJSON(base); err != nil {
		t.Fatal("minimal request refused", err)
	}
	for name, edit := range map[string]func(map[string]any){
		"state chosen":     func(v map[string]any) { v["state"] = "CONFIRMED" },
		"missing purpose":  func(v map[string]any) { delete(v, "purpose") },
		"empty purpose":    func(v map[string]any) { v["purpose"] = "" },
		"missing window":   func(v map[string]any) { delete(v, "ends_at") },
		"employee by name": func(v map[string]any) { v["employee_id"] = "Synthetic driver" },
		"tolerance chosen": func(v map[string]any) { v["no_show_tolerance_minutes"] = float64(30) },
	} {
		value := map[string]any{}
		for k, v := range base {
			value[k] = v
		}
		edit(value)
		if request.VisitJSON(value) == nil {
			t.Fatal("invalid request accepted by the contract", name)
		}
	}
	remove := document.Components.Schemas["EnterpriseFleetBlockRemoveRequest"].Value
	if remove.VisitJSON(map[string]any{"schema_version": float64(1), "operation_id": "7b0e2a54-0000-4000-8000-000000009105"}) == nil {
		t.Fatal("block removal without a reason accepted by the contract")
	}
	block := document.Components.Schemas["EnterpriseFleetBlockCreateRequest"].Value
	if block.VisitJSON(map[string]any{"schema_version": float64(1), "operation_id": "7b0e2a54-0000-4000-8000-000000009104", "reason_kind": "MAINTENANCE"}) == nil {
		t.Fatal("unknown block reason accepted")
	}
	item := document.Components.Schemas["EnterpriseFleetReservation"].Value
	if item.VisitJSON(map[string]any{"reservation_id": "7b0e2a54-0000-4000-8000-000000009201", "asset_id": "7b0e2a54-0000-4000-8000-000000008201", "purpose": "x", "starts_at": "2026-11-14T11:00:00Z", "ends_at": "2026-11-14T15:00:00Z", "no_show_tolerance_minutes": float64(60), "state": "MISSED", "requested_at": "2026-11-13T10:00:00Z"}) == nil {
		t.Fatal("unknown reservation state accepted")
	}
	availability := document.Components.Schemas["EnterpriseFleetAvailability"].Value
	if availability.VisitJSON(map[string]any{"asset_id": "7b0e2a54-0000-4000-8000-000000008201", "availability": "FREE", "causes": []any{}, "missing_sources": []any{}, "evaluated_at": "2026-11-14T12:30:00Z"}) == nil {
		t.Fatal("unknown availability accepted")
	}
	cause := document.Components.Schemas["EnterpriseFleetAvailabilityCause"].Value
	if cause.VisitJSON(map[string]any{"availability": "RESERVED", "kind": "RESERVATION", "employee_id": "7b0e2a54-0000-4000-8000-000000008601"}) == nil {
		t.Fatal("a cause may not carry a person")
	}
	codes := document.Components.Schemas["EnterpriseError"].Value.AllOf[1].Value.Properties["error"].Value.Properties["code"].Value.Enum
	if !slices.Contains(codes, any("RESERVATION_CONFLICT")) {
		t.Fatal("RESERVATION_CONFLICT is not a registered enterprise error code")
	}
	detail := document.Components.Schemas["ApiError"].Value.Properties["error"].Value.Properties["details"].Value.Items.Value
	if detail.VisitJSON(map[string]any{"field": "conflict_window", "code": "STARTS_AT", "instant": "2026-12-01T11:00:00Z"}) != nil {
		t.Fatal("conflict window instant detail refused")
	}
}

func TestFleetReservationContractBindsActualCurrentCompanyGuards(t *testing.T) {
	document, router := companyRoutes(t)
	if err := ValidateOrganizationRoutes(router, document); err != nil {
		t.Fatal(err)
	}
	write := func(success int) map[string]any {
		return map[string]any{"permission": "fleet.reservation.manage", "write": true, "fresh": true, "require_revision": false, "company_only": true, "success": success}
	}
	read := map[string]any{"permission": "fleet.asset.read", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200}
	prefix := "/v1/orgs/{org_id}/fleet/assets/{asset_id}/"
	routes := map[string]map[string]any{
		prefix + "reservations/request":                  write(201),
		prefix + "reservations/{reservation_id}/confirm": write(200),
		prefix + "reservations/{reservation_id}/cancel":  write(200),
		prefix + "blocks/create":                         write(201),
		prefix + "blocks/{block_id}/remove":              write(200),
		prefix + "reservations/list":                     read,
		prefix + "blocks/list":                           read,
		prefix + "availability/read":                     read,
	}
	for path, want := range routes {
		if document.Paths.Value(path) == nil || document.Paths.Value(path).Post == nil {
			t.Fatal("missing actual reservation route", path)
		}
		for _, test := range []string{"cookie", "permission", "write"} {
			t.Run(path+"/"+test, func(t *testing.T) {
				doc, actual := companyRoutes(t)
				operation := doc.Paths.Value(path).Post
				metadata := map[string]any{}
				for k, v := range want {
					metadata[k] = v
				}
				switch test {
				case "cookie":
					operation.Security = nil
				case "permission":
					metadata["permission"] = "fleet.asset.assign"
				case "write":
					metadata["write"], metadata["fresh"] = !want["write"].(bool), !want["fresh"].(bool)
				}
				if test != "cookie" {
					operation.Extensions["x-abastevo-organization"] = metadata
				}
				if ValidateOrganizationRoutes(actual, doc) == nil {
					t.Fatal("reservation authority/contract drift accepted")
				}
			})
		}
	}
}
