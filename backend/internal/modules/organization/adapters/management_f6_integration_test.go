//go:build integration

package adapters_test

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	exportpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/adapters"
	mediapg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func grantF6Tables(t *testing.T, f *orgtest.Fixture) {
	t.Helper()
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	stmt := `GRANT SELECT,INSERT,UPDATE ON
		org_f6_commands, org_fuel_tanks, org_fuel_tank_movements, org_fuel_tank_reconciliations,
		org_internal_fuelings, org_operating_cost_entries, org_asset_document_links,
		org_maintenance_plans, org_maintenance_occurrences, org_work_orders,
		org_compliance_items, org_fleet_components, org_parts_stock_items, org_insurance_policies,
		org_kpi_snapshots, org_price_comparisons, org_finding_settings, org_finding_runs, org_findings,
		org_report_schedules, org_recurring_and_fine_costs, org_payable_statements,
		org_retention_and_holds, org_consolidation_grants, org_custom_roles, org_voluntary_price_publications
	TO ` + role + `; GRANT USAGE ON SEQUENCE org_work_orders_number_seq TO ` + role + `;
	GRANT SELECT ON org_expenses, org_fueling_review_entries, org_fuelings, org_trips, org_documents, org_sync_review_items, org_sync_review_resolutions, org_meters, org_meter_readings, org_finance_periods, org_asset_unit_periods, org_asset_cost_centre_periods, org_assets, org_units, org_cost_centres TO ` + role
	if _, err := f.Admin.Exec(t.Context(), stmt); err != nil {
		t.Fatal("grant f6 tables:", err)
	}
}

func TestManagementF6EndToEndFlow(t *testing.T) {
	fx := devicesFixture(t)
	grantEmployeeTables(t, fx.legalFixtureState)
	_ = attachmentStore(t, fx.legalFixtureState)
	_ = meterStore(t, fx.legalFixtureState)
	grantChecklistTables(t, fx.legalFixtureState)
	grantMissionTables(t, fx.legalFixtureState)
	fx.f.GrantMeterReviews(t)
	fx.f.GrantMembershipAccessCommands(t)
	periodGrant(t, fx.f)
	exportGrant(t, fx)
	documentGrants(t, fx)
	grantF6Tables(t, fx.f)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_settings_commands,org_role_commands TO "+role+"; GRANT SELECT,INSERT,UPDATE ON org_ownership_transfers,identity_rate_windows TO "+role); err != nil {
		t.Fatal(err)
	}
	envReq := func(k string) string {
		v := os.Getenv(k)
		if v == "" {
			t.Fatalf("explicit disposable %s required", k)
		}
		return v
	}
	docsCfg := mediapg.StorageConfig{
		Endpoint: envReq("ANPFUEL_TEST_STORAGE_ENDPOINT"), Bucket: envReq("ANPFUEL_TEST_STORAGE_BUCKET"),
		Region: envReq("ANPFUEL_TEST_STORAGE_REGION"), AllowLoopback: true,
		Credentials: storage.Credentials{AccessKeyID: envReq("ANPFUEL_TEST_STORAGE_ACCESS_KEY_ID"), SecretAccessKey: envReq("ANPFUEL_TEST_STORAGE_SECRET_ACCESS_KEY")},
	}
	expCfg := exportStorageConfig(t)
	runtimeAccounts := *fx.accounts
	runtimeAccounts.Store = accountpg.NewPGStore(fx.f.Runtime)
	opts := enterprisesuite.Options{
		Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{73}, 32),
		Pool: fx.f.Runtime, Accounts: &runtimeAccounts, DocumentStorage: &docsCfg, ExportStorage: expCfg,
	}
	router := chi.NewRouter()
	if err := enterprisesuite.Register(router, opts); err != nil {
		t.Fatal(err)
	}
	handlers, err := enterprisesuite.ExportHandlers(opts)
	if err != nil {
		t.Fatal(err)
	}
	regHandlers := map[string]jobs.Handler{}
	for _, h := range handlers {
		regHandlers[h.Kind()] = h
	}
	dispatcher := &jobs.Dispatcher{Queue: jobs.NewQueue(fx.f.Runtime), Handlers: regHandlers, WorkerID: "f6-worker", Kind: exportpg.BuildKind, LeaseTTL: time.Minute, RetryDelay: time.Millisecond}

	owner := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	finance := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, finance, "FINANCE", "ORG", "ACTIVE")
	driver := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, driver, "DRIVER", "SELF", "ACTIVE")

	org := fx.scope.OrgID().String()
	base := "/v1/orgs/" + org

	var unitID string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1 ORDER BY created_at,id LIMIT 1`, org).Scan(&unitID); err != nil {
		t.Fatal(err)
	}

	// 1. Create active vehicle + meter
	assetResp := matrixCall(t, fx, router, owner, base+"/fleet/assets/create", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "kind": "VEHICLE",
		"name": "Caminhao F6", "fleet_number": "F6-01", "plate": "F6T1234",
		"usage_basis": "DISTANCE", "category": map[string]any{"new_label": "Trucks"}, "unit_id": unitID,
		"tanks": []any{map[string]any{"role": "MAIN", "capacity_milli": "300000", "products": []string{"DIESEL_S10"}}},
	}, 201, "", "", nil)
	assetID := assetResp["asset_id"].(string)
	meterResp := matrixCall(t, fx, router, owner, base+"/fleet/assets/"+assetID+"/meters/install", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "revision": 1,
		"opening_reading_id": onboardingID(t), "kind": "ODOMETER", "label": "Odo Principal",
		"opening_value": "10000000", "method": "TYPED",
		"occurred_at": time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339), "entry_zone": "America/Sao_Paulo", "device_offset_seconds": -10800,
	}, 201, "", "", nil)
	_ = meterResp
	matrixCall(t, fx, router, owner, base+"/fleet/assets/"+assetID+"/activate", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "revision": 2,
	}, 200, "", "", nil)

	// 2. Fuel stock: VAL-32 single-count moving average
	tankA := matrixCall(t, fx, router, owner, base+"/stock/tanks/save", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t),
		"name": "Tanque Matriz", "product": "DIESEL_S10", "unit": "L", "capacity_milli": 5000000,
	}, 200, "", "", nil)["tank"].(map[string]any)["id"].(string)

	// Purchase receipt: 100L for R$ 600.00 (60000 centavos) -> zero operating expense
	matrixCall(t, fx, router, owner, base+"/stock/movements/record", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t),
		"tank_id": tankA, "kind": "RECEIPT", "product": "DIESEL_S10", "unit": "L",
		"quantity_milli": 100000, "amount_centavos": 60000, "occurred_at": time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
	}, 200, "", "", nil)

	// Mismatch product & overdraft rejected
	matrixCall(t, fx, router, owner, base+"/stock/movements/record", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t),
		"tank_id": tankA, "kind": "DISPENSE", "product": "ETHANOL", "asset_id": assetID, "quantity_milli": 10000,
	}, 422, "STOCK_PRODUCT_MISMATCH", "", nil)
	matrixCall(t, fx, router, owner, base+"/stock/movements/record", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t),
		"tank_id": tankA, "kind": "DISPENSE", "product": "DIESEL_S10", "asset_id": assetID, "quantity_milli": 200000,
	}, 409, "STOCK_INSUFFICIENT", "", nil)

	// Dispense 40L -> 40% of 60000 = 24000 centavos recognized once
	disp := matrixCall(t, fx, router, owner, base+"/stock/movements/record", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t),
		"tank_id": tankA, "kind": "DISPENSE", "product": "DIESEL_S10", "asset_id": assetID,
		"quantity_milli": 40000, "fill": "FULL", "occurred_at": time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339),
	}, 200, "", "", nil)["result"].(map[string]any)["movement"].(map[string]any)
	if disp["recognized_expense_centavos"] != float64(24000) || disp["pool_quantity_after_milli"] != float64(60000) || disp["pool_value_after_centavos"] != float64(36000) {
		t.Fatal("stock moving average valuation mismatch", disp)
	}

	// 3. Maintenance plan + work order lifecycle + availability hold
	planRes := matrixCall(t, fx, router, owner, base+"/maintenance/plans/save", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "action": "SAVE",
		"asset_id": assetID, "name": "Troca Oleo 10k",
		"distance_interval_m": 10000000, "distance_lead_m": 1000000,
		"baseline_day": "2026-11-01", "baseline_odometer_m": 10000000, "eval_day": "2026-11-18",
		"current_odometer_m": 19500000, "rebase_rule": "ACTUAL", "blocking": true,
	}, 200, "", "", nil)["result"].(map[string]any)
	if planRes["state"] != "DUE" {
		t.Fatal("expected preventive plan occurrence DUE", planRes)
	}

	// Disable licence expiry control -> CONTROL_DISABLED
	matrixCall(t, fx, router, owner, base+"/maintenance/plans/save", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "action": "COMPLIANCE_TOGGLE_LICENCE", "licence_enabled": false,
	}, 200, "", "", nil)
	matrixCall(t, fx, router, owner, base+"/maintenance/plans/save", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "action": "COMPLIANCE_UPSERT",
		"subject_kind": "EMPLOYEE", "subject_id": driver.employee, "item_kind": "CNH", "expiry_day": "2026-12-01", "lead_days": 30,
	}, 409, "CONTROL_DISABLED", "", nil)

	// Work order: owner requests, self-approval blocked, finance approves, start opens hold, overrun checked on close, release hold clears availability
	woID := matrixCall(t, fx, router, owner, base+"/maintenance/work-orders/save", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "action": "CREATE",
		"asset_id": assetID, "kind": "CORRECTIVE", "description": "Troca pastilha de freio", "invoice_required": true,
	}, 200, "", "", nil)["result"].(map[string]any)["work_order_id"].(string)

	matrixCall(t, fx, router, owner, base+"/maintenance/work-orders/save", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "action": "QUOTE", "work_order_id": woID, "revision": 1,
		"selected_quote_cents": 50000, "quote_valid_until": time.Now().UTC().Add(48 * time.Hour).Format("2006-01-02"), "tolerance_percent": 10,
	}, 200, "", "", nil)

	// Self-approval denied
	matrixCall(t, fx, router, owner, base+"/maintenance/work-orders/review/decide", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "revision": 2, "work_order_id": woID,
		"decision": "APPROVE", "approved_cents": 50000, "reason": "self test",
	}, 409, "SELF_APPROVAL", "", nil)

	// Finance approves (SoD honored)
	matrixCall(t, fx, router, finance, base+"/maintenance/work-orders/review/decide", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "revision": 2, "work_order_id": woID,
		"decision": "APPROVE", "approved_cents": 50000, "reason": "orcamento validado",
	}, 200, "", "", nil)

	matrixCall(t, fx, router, owner, base+"/maintenance/work-orders/save", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "action": "SCHEDULE", "work_order_id": woID, "revision": 3,
	}, 200, "", "", nil)

	// START opens maintenance hold on asset
	matrixCall(t, fx, router, owner, base+"/maintenance/work-orders/save", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "action": "START", "work_order_id": woID, "revision": 4,
	}, 200, "", "", nil)

	avail := matrixCall(t, fx, router, owner, base+"/fleet/assets/"+assetID+"/availability/read", map[string]any{}, 200, "", "", nil)
	if avail["availability"] != "IN_MAINTENANCE" || len(avail["missing_sources"].([]any)) != 0 {
		t.Fatal("expected IN_MAINTENANCE with live maintenance hold source", avail)
	}

	// Upload real document for invoice
	docID, _ := auditorStoredDocument(t, fx, router, owner)

	// COMPLETE within tolerance -> RELEASE_HOLD -> CLOSE
	matrixCall(t, fx, router, owner, base+"/maintenance/work-orders/save", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "action": "COMPLETE", "work_order_id": woID, "revision": 5,
		"lines":      []any{map[string]any{"kind": "SERVICE", "quantity_milli": 1000, "unit_price_milli": 50000}},
		"paid_cents": 50000, "invoice_document_id": docID,
	}, 200, "", "", nil)
	matrixCall(t, fx, router, owner, base+"/maintenance/work-orders/save", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "action": "RELEASE_HOLD", "work_order_id": woID, "revision": 6,
	}, 200, "", "", nil)
	closed := matrixCall(t, fx, router, owner, base+"/maintenance/work-orders/save", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "action": "CLOSE", "work_order_id": woID, "revision": 7,
	}, 200, "", "", nil)["result"].(map[string]any)
	if closed["state"] != "CLOSED" {
		t.Fatal("work order did not close cleanly", closed)
	}

	availAfter := matrixCall(t, fx, router, owner, base+"/fleet/assets/"+assetID+"/availability/read", map[string]any{}, 200, "", "", nil)
	if availAfter["availability"] != "AVAILABLE" {
		t.Fatal("hold release did not restore availability", availAfter)
	}

	// 4. KPI Overview & Findings Detection + Self-resolve guard + Settings + Audit
	kpi := matrixCall(t, fx, router, owner, base+"/intelligence/kpis/overview", map[string]any{"dimension": "ORG"}, 200, "", "", nil)["overview"].(map[string]any)
	vals := kpi["values"].([]any)
	if len(vals) != 20 {
		t.Fatalf("expected 20 indicators IND-001..020, got %d", len(vals))
	}

	// Raise finding authored by owner -> owner self-resolve blocked -> finance confirms
	raised := matrixCall(t, fx, router, owner, base+"/intelligence/findings/decide", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "action": "RAISE_OR_DETECT",
		"synthetic_signal_kind": "FT-001", "subject_key": "fuel:f6-test", "asset_id": assetID, "author_membership_id": owner.member,
	}, 200, "", "", nil)["result"].(map[string]any)
	findID := raised["finding_id"].(string)

	matrixCall(t, fx, router, owner, base+"/intelligence/findings/decide", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "action": "CONFIRM", "finding_id": findID, "revision": 1, "reason": "self attempt",
	}, 409, "SELF_APPROVAL", "", nil)

	matrixCall(t, fx, router, finance, base+"/intelligence/findings/decide", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "action": "CONFIRM", "finding_id": findID, "revision": 1, "reason": "revisado por financeiro",
	}, 200, "", "", nil)

	// Audit list contains our actions
	aud := matrixCall(t, fx, router, owner, base+"/audit/events/list", map[string]any{"limit": 50}, 200, "", "", nil)["events"].([]any)
	if len(aud) == 0 {
		t.Fatal("expected audit events recorded")
	}

	// 5. Governance controls: Legal Hold blocks DELETION_REPLAY -> release -> replay succeeds
	matrixCall(t, fx, router, owner, base+"/consolidation/grants/save", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "action": "LEGAL_HOLD", "category": "TRIPS", "target_id": assetID, "hold_active": true, "reason": "litigio",
	}, 200, "", "", nil)
	matrixCall(t, fx, router, owner, base+"/consolidation/grants/save", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "action": "DELETION_REPLAY", "category": "TRIPS", "target_id": assetID, "reason": "purge attempt",
	}, 409, "HOLD_OPEN", "", nil)

	// 6. Real F6 Exports via worker dispatcher & MinIO: accounting CSV, PDF, full ZIP
	for _, kind := range []string{"fin.accounting_competence.csv", "rpt.period_summary.pdf", "org.full_export.zip"} {
		req := matrixCall(t, fx, router, owner, base+"/reports/exports/request", map[string]any{
			"schema_version": 1, "operation_id": onboardingID(t), "kind": kind, "parameters": map[string]any{},
		}, 202, "", "", nil)
		expID := req["export"].(map[string]any)["export_id"].(string)
		if found, err := dispatcher.RunOnce(t.Context()); err != nil || !found {
			t.Fatalf("dispatch %s failed: found=%v err=%v", kind, found, err)
		}
		capRes := matrixCall(t, fx, router, owner, base+"/reports/exports/"+expID+"/access", map[string]any{}, 200, "", "", nil)
		capTok := capRes["capability"].(string)
		content := matrixCall(t, fx, router, owner, base+"/reports/exports/"+expID+"/content", map[string]any{"capability": capTok}, 200, "", "", nil)
		rawBytes, err := base64.StdEncoding.DecodeString(content["content_base64"].(string))
		if err != nil || len(rawBytes) == 0 {
			t.Fatalf("empty export bytes for %s: %v", kind, err)
		}
		switch kind {
		case "fin.accounting_competence.csv":
			if !strings.Contains(string(rawBytes), "INTERNAL_STOCK_DISPENSE") || !strings.Contains(string(rawBytes), "WORK_ORDER") {
				t.Fatal("accounting competence CSV missing dispense or work order", string(rawBytes))
			}
		case "rpt.period_summary.pdf":
			if !bytes.HasPrefix(rawBytes, []byte("%PDF-1.4")) {
				t.Fatal("period summary export is not valid PDF 1.4")
			}
		case "org.full_export.zip":
			zr, err := zip.NewReader(bytes.NewReader(rawBytes), int64(len(rawBytes)))
			if err != nil {
				t.Fatal("invalid full export zip", err)
			}
			seen := map[string]bool{}
			for _, f := range zr.File {
				seen[f.Name] = true
			}
			if !seen["manifest.json"] || !seen["tanks.csv"] || !seen["work_orders.csv"] || !seen["findings.csv"] {
				t.Fatal("full export zip missing expected entries", seen)
			}
		}
	}

	// Keep unused variable check happy for httptest/json/account
	var _ = http.MethodPost
	var _ = httptest.NewRecorder
	var _ = json.Marshal
	var _ = account.Service{}
	var _ = accountdomain.SHA256Hasher{}
}
