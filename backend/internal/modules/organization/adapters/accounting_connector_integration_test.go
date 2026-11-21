//go:build integration

package adapters_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	intel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/intelligence/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	cappg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/adapters"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	exportpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/go-chi/chi/v5"
)

func TestReviewedAccountingConnectorDeliversAnExactPrivateFile(t *testing.T) {
	t.Setenv("ANPFUEL_TEST_RUNTIME_GRANTS", "script")
	x := fuelingFixture(t)
	fx := x.deviceFixtureState
	org := fx.scope.OrgID().String()
	key := bytes.Repeat([]byte{110}, 32)
	dir := t.TempDir()
	ref := onboardingID(t)
	if err := os.WriteFile(filepath.Join(dir, ref), key, 0o600); err != nil {
		t.Fatal(err)
	}
	keys, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer keys.Close()
	catalog, err := capability.NewCatalog(enterprisesuite.CapabilityCodes())
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := audit.NewCatalog(cappg.AuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	flags := cappg.NewStore(fx.f.Runtime, catalog, auditpg.NewStore(fx.f.Runtime, definitions))
	if _, err := flags.Set(t.Context(), fx.scope, capability.Actor{AccountID: fx.actor.AccountID}, capability.Change{
		OperationID: onboardingID(t), Capability: "integrations", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	accounts := *fx.accounts
	accounts.Store = accountpg.NewPGStore(fx.f.Runtime)
	opts := enterprisesuite.Options{Enabled: true, BillingEnabled: true, Origin: deviceTestOrigin,
		CursorSecret: bytes.Repeat([]byte{76}, 32), Pool: fx.f.Runtime, Accounts: &accounts,
		ExportStorage: exportStorageConfig(t), AccountingKeys: keys}
	if err := enterprisesuite.Register(router, opts); err != nil {
		t.Fatal(err)
	}
	handlers, err := enterprisesuite.ExportHandlers(opts)
	if err != nil {
		t.Fatal(err)
	}
	active := make(map[string]jobs.Handler, len(handlers))
	for _, handler := range handlers {
		active[handler.Kind()] = handler
	}
	dispatcher := jobs.Dispatcher{Queue: jobs.NewQueue(fx.f.Runtime), Kind: exportpg.BuildKind, Handlers: active,
		WorkerID: "accounting-connector-test", LeaseTTL: time.Minute, RetryDelay: time.Millisecond}
	owner := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	const day = "2026-01-05"
	for index, fixture := range []struct {
		at, state string
		amount    int64
	}{
		{day + "T14:30:00Z", "SETTLED", 97351},
		{"2026-01-06T14:30:00Z", "SETTLED", 77440},
		{day + "T14:45:00Z", "OPEN", 65330},
	} {
		settled := fixture.amount
		if index == 2 {
			settled = 0
		}
		if err := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_payable_statements
(org_id,id,operation_id,kind,reference_code,period_month,payable_ids_json,total_centavos,settled_centavos,state,created_at,updated_at)
VALUES($1,$2,$3,'SUPPLIER_INVOICE','Synthetic private reference','2025-12','[]'::jsonb,$4,$5,$6,$7::text::timestamptz,$7::text::timestamptz)`,
			org, onboardingID(t), onboardingID(t), fixture.amount, settled, fixture.state, fixture.at); err != nil {
			t.Fatal(err)
		}
	}
	base := "/v1/orgs/" + org
	exports := base + "/reports/exports"
	manual := func(exportID string) []byte {
		t.Helper()
		path := exports + "/" + exportID
		grant := matrixCall(t, fx, router, owner, path+"/access", map[string]any{}, 200, "", "", nil)
		content := matrixCall(t, fx, router, owner, path+"/content", map[string]any{"capability": grant["capability"]}, 200, "", "", nil)
		raw, err := base64.StdEncoding.DecodeString(content["content_base64"].(string))
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	fueling := x.op(t, x.syncFixtureState, x.declaredNow(t), nil)
	ack, err := x.process(t, x.syncFixtureState, fueling)
	if err != nil || ack.Result() != protocol.Accepted {
		t.Fatalf("company fueling was not recorded: %v", err)
	}
	fuelingID := fueling.Snapshot().Entity.ID
	var reportingDay string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT reporting_day::text FROM org_expenses WHERE org_id=$1 AND fueling_id=$2`, org, fuelingID).Scan(&reportingDay); err != nil {
		t.Fatal(err)
	}
	requestCompetence := func() string {
		t.Helper()
		result := matrixCall(t, fx, router, owner, exports+"/request", map[string]any{
			"schema_version": 1, "operation_id": onboardingID(t), "kind": intel.AccountingCompetenceCSVKind,
			"parameters": map[string]any{"layout_version": 1, "period_start": reportingDay, "period_end": reportingDay},
		}, 202, "", "", nil)
		return result["export"].(map[string]any)["export_id"].(string)
	}
	beforeReview := requestCompetence()
	decide := base + "/fuelings/" + fuelingID + "/review/decide"
	matrixCall(t, fx, router, owner, decide, map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "decision": "OPEN",
	}, 200, "", "", nil)
	matrixCall(t, fx, router, owner, decide, map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "revision": 2, "decision": "APPROVE",
	}, 200, "", "", nil)
	afterReview := requestCompetence()
	for range 2 {
		if found, err := dispatcher.RunOnce(t.Context()); err != nil || !found {
			t.Fatalf("competence export worker found=%v: %v", found, err)
		}
	}
	for _, scenario := range []struct {
		id   string
		rows int
	}{{beforeReview, 1}, {afterReview, 2}} {
		records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(manual(scenario.id)), "\ufeff"))).ReadAll()
		if err != nil || len(records) != scenario.rows {
			t.Fatalf("competence request cutoff was ignored: %v (%v)", records, err)
		}
		if scenario.rows == 2 && (records[1][1] != "COMPETENCE" || records[1][2] != "FUELING" || records[1][7] != "97350" || records[1][8] != "97350") {
			t.Fatalf("approved fueling did not preserve its expense amounts: %v", records[1])
		}
	}

	parameters := map[string]any{"layout_version": 1, "basis": "COMPETENCE", "period_start": day, "period_end": day}
	exportRequest := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": intel.AccountingCashCSVKind, "parameters": parameters}
	matrixCall(t, fx, router, owner, exports+"/request", exportRequest, 422, "VALIDATION_FAILED", "", nil)
	parameters["basis"] = "CASH"
	parameters["period_start"] = "2026-01-06"
	matrixCall(t, fx, router, owner, exports+"/request", exportRequest, 422, "VALIDATION_FAILED", "", nil)
	parameters["period_start"] = day
	result := matrixCall(t, fx, router, owner, exports+"/request", exportRequest, 202, "", "", nil)
	exportID := result["export"].(map[string]any)["export_id"].(string)
	if found, err := dispatcher.RunOnce(t.Context()); err != nil || !found {
		t.Fatalf("accounting export worker found=%v: %v", found, err)
	}

	want := manual(exportID)
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(want), "\ufeff"))).ReadAll()
	if err != nil || len(records) != 2 || len(records[1]) != 10 || records[1][1] != "CASH" || records[1][2] != "PAYABLE_STATEMENT" ||
		records[1][5] != day || records[1][7] != "97351" || records[1][8] != "97351" || bytes.Contains(want, []byte("Synthetic private reference")) {
		t.Fatalf("period, basis or amounts changed: %v (%v)", records, err)
	}
	connections := base + "/integrations/connections"
	approval, connection := onboardingID(t), onboardingID(t)
	proof := func(scope, target string) string {
		mac := hmac.New(sha256.New, key)
		_, _ = mac.Write([]byte("abastevo-accounting-v1\n" + scope + "\n" + connection + "\n" + target))
		return ref + "." + hex.EncodeToString(mac.Sum(nil))
	}
	if _, err := fx.f.Admin.Exec(t.Context(), `INSERT INTO org_connector_approvals
(org_id,id,revision,kind,provider_code,resource_id,credential_ref,maximum_bytes,maximum_items,decision,decision_ref,ready_at)
VALUES($1,$2,1,'ACCOUNTING_EXPORT','accounting-csv-v1',$3,$4,4096,1,'APPROVED','synthetic-reviewed-recipient',clock_timestamp())`,
		org, approval, exportID, ref); err != nil {
		t.Fatal(err)
	}
	matrixCall(t, fx, router, owner, connections+"/configure", map[string]any{
		"schema_version": 1, "operation_id": connection, "approval_id": approval, "approval_revision": 1, "label": "Reviewed synthetic accountant",
	}, 200, "", "", nil)
	get := func(scope, target, bearer, etag string, status int) *httptest.ResponseRecorder {
		t.Helper()
		path := deviceTestOrigin + "/v1/integrations/orgs/" + scope + "/connections/" + connection + "/accounting/" + target
		r := httptest.NewRequest(http.MethodGet, path, nil)
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		if etag != "" {
			r.Header.Set("If-None-Match", etag)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != status || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("accounting delivery: status=%d expected=%d body=%s", w.Code, status, w.Body.String())
		}
		return w
	}
	token := proof(org, exportID)
	get(org, exportID, token, "", 404)
	matrixCall(t, fx, router, owner, connections+"/"+connection+"/activate", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "revision": 1,
	}, 200, "", "", nil)
	get(org, exportID, "", "", 401)
	other := onboardingID(t)
	get(org, other, token, "", 401)
	get(org, other, proof(org, other), "", 404)
	foreign := fx.f.Organizations[1].ID
	get(foreign, exportID, proof(foreign, exportID), "", 404)
	external := get(org, exportID, token, "", 200)
	hash := sha256.Sum256(want)
	etag := `"` + hex.EncodeToString(hash[:]) + `"`
	if !bytes.Equal(want, external.Body.Bytes()) || external.Header().Get("ETag") != etag {
		t.Fatal("delivery changed the authorized file")
	}
	if get(org, exportID, token, etag, 304).Body.Len() != 0 {
		t.Fatal("matching validator returned duplicate content")
	}
	matrixAssign(t, fx, owner, "DRIVER", "SELF", "ACTIVE")
	get(org, exportID, token, etag, 404)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	if _, err := fx.f.Admin.Exec(t.Context(), `INSERT INTO org_connector_approvals
(org_id,id,revision,kind,provider_code,resource_id,credential_ref,maximum_bytes,maximum_items,decision,decision_ref)
VALUES($1,$2,2,'ACCOUNTING_EXPORT','accounting-csv-v1',$3,$4,4096,1,'WITHDRAWN','synthetic-rights-withdrawn')`,
		org, approval, exportID, ref); err != nil {
		t.Fatal(err)
	}
	get(org, exportID, token, etag, 404)
	if !bytes.Equal(want, manual(exportID)) {
		t.Fatal("withdrawing a receiver changed the manual file")
	}
	var sourceCount int
	if err := fx.f.Admin.QueryRow(t.Context(), "SELECT count(*) FROM org_payable_statements WHERE org_id=$1", org).Scan(&sourceCount); err != nil || sourceCount != 3 {
		t.Fatalf("file transport changed accounting records: count=%d err=%v", sourceCount, err)
	}
}
