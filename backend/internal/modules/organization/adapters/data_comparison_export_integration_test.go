//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	datapg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/adapters/preparation"
	exportpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

const comparisonDataset = "market-mt"

func prepareMarketComparison(t *testing.T, env dataUsageFixtureState) string {
	t.Helper()
	ctx := t.Context()
	var activeRole string
	if err := env.pool.QueryRow(ctx, "SELECT current_user").Scan(&activeRole); err != nil {
		t.Fatal(err)
	}
	role := pgx.Identifier{activeRole}.Sanitize()
	const tables = " ON enterprise_regional_series,enterprise_regional_series_samples "
	if _, err := env.fx.f.Admin.Exec(ctx, "GRANT INSERT"+tables+"TO "+role); err != nil {
		t.Fatal(err)
	}
	var reference string
	for index, scenario := range []struct {
		month    string
		stations int
	}{{"01", 5}, {"02", 6}, {"03", 4}} {
		path, m := buildRegionalExport(t, scenario.month, scenario.stations)
		registry, price := m.Files[0].SHA256, m.Sources.Official[0].SourceChecksum
		if _, err := env.fx.f.Admin.Exec(ctx, `INSERT INTO enterprise_data_source_reviews
(source_code,source_digest,revision,status,decision_ref,license_ref,attribution)
VALUES ('S-ESP-01',$1,1,'APPROVED','synthetic-comparison-review','Registry license','Synthetic registry'),
       ('S-ESP-02',$2,1,'APPROVED','synthetic-comparison-review','Survey license','Synthetic survey')`, registry, price); err != nil {
			t.Fatal(err)
		}
		revision := int64(index + 1)
		if _, err := env.fx.f.Admin.Exec(ctx, `INSERT INTO enterprise_regional_datasets
(dataset_key,revision,display_name,state_code,minimum_stations,method_version,snapshot_digest,official_revision,
 registry_digest,registry_review,price_digest,price_review)
VALUES($1,$2,'Synthetic market comparison','MT',5,'abastevo-metrics-v1',$3,$4,$5,1,$6,1)`,
			comparisonDataset, revision, m.ContentSHA256, m.Sources.Official[0].RevisionID, registry, price); err != nil {
			t.Fatal(err)
		}
		if _, err := preparation.Prepare(ctx, env.pool, comparisonDataset, revision, path); err != nil {
			t.Fatal(err)
		}
		if _, err := env.fx.f.Admin.Exec(ctx, `INSERT INTO enterprise_data_plan_datasets
(product_code,plan_code,plan_revision,dataset_key,dataset_revision,approval_ref)
VALUES ('data','synthetic',1,$1,$2,'synthetic-comparison-scope')`, comparisonDataset, revision); err != nil {
			t.Fatal(err)
		}
		if revision == 2 {
			reference = price
		}
	}
	if _, err := env.fx.f.Admin.Exec(ctx, "REVOKE INSERT"+tables+"FROM "+role); err != nil {
		t.Fatal(err)
	}
	return reference
}

func TestMarketComparisonThroughPrivateExportAndRevokedRights(t *testing.T) {
	t.Setenv("ANPFUEL_TEST_RUNTIME_GRANTS", "script")
	fx := devicesFixture(t)
	env := dataUsageFixtureOn(t, fx.legalFixtureState, 20, time.Hour)
	reference := prepareMarketComparison(t, env)
	router := chi.NewRouter()
	accounts := *fx.accounts
	accounts.Store = accountpg.NewPGStore(fx.f.Runtime)
	opts := enterprisesuite.Options{Enabled: true, BillingEnabled: true, Origin: deviceTestOrigin,
		CursorSecret: bytes.Repeat([]byte{76}, 32), Pool: fx.f.Runtime, Accounts: &accounts, ExportStorage: exportStorageConfig(t)}
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
	dispatcher := &jobs.Dispatcher{Queue: jobs.NewQueue(fx.f.Runtime), Handlers: active, WorkerID: "market-report-test",
		Kind: exportpg.BuildKind, LeaseTTL: time.Minute, RetryDelay: time.Millisecond}
	owner := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	colleague := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, colleague, "OWNER", "ORG", "ACTIVE")
	org := fx.scope.OrgID().String()
	base := "/v1/orgs/" + org + "/reports/exports"
	command := func(revision int) map[string]any {
		return map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": datapg.MarketComparisonKind,
			"parameters": map[string]any{"fuel": "GASOLINE_REGULAR",
				"baseline":  map[string]any{"dataset": comparisonDataset, "revision": 1},
				"reference": map[string]any{"dataset": comparisonDataset, "revision": revision}}}
	}
	build := func(id string) string {
		t.Helper()
		ok, err := dispatcher.RunOnce(t.Context())
		if err != nil || !ok {
			t.Fatalf("production export handler did not consume the job: %v", err)
		}
		state, failure := exportState(t, fx, id)
		if state != "READY" || failure != "" {
			t.Fatalf("comparison was not published: %s %s", state, failure)
		}
		path := base + "/" + id + "/access"
		matrixCall(t, fx, router, colleague, path, map[string]any{}, 404, "NOT_FOUND", "", nil)
		grant := matrixCall(t, fx, router, owner, path, map[string]any{}, 200, "", "", nil)
		return grant["capability"].(string)
	}
	content := func(id, token string) map[string]any {
		t.Helper()
		view := matrixCall(t, fx, router, owner, base+"/"+id+"/content", map[string]any{"capability": token}, 200, "", "", nil)
		raw, err := base64.StdEncoding.DecodeString(view["content_base64"].(string))
		if err != nil || view["media_type"] != "application/json" || bytes.Contains(raw, []byte(marketProbe)) {
			t.Fatalf("comparison content disclosed an unexpected representation: %v", err)
		}
		var report map[string]any
		if err := json.Unmarshal(raw, &report); err != nil {
			t.Fatal(err)
		}
		return report
	}
	queue := func(input map[string]any) string {
		t.Helper()
		job := matrixCall(t, fx, router, owner, base+"/request", input, http.StatusAccepted, "", "", nil)
		return job["export"].(map[string]any)["export_id"].(string)
	}
	bad := command(2)
	bad["parameters"].(map[string]any)["company_transaction_id"] = onboardingID(t)
	matrixCall(t, fx, router, owner, base+"/request", bad, 422, "VALIDATION_FAILED", "", nil)
	matrixCall(t, fx, router, owner, base+"/request", command(99), 404, "NOT_FOUND", "", nil)

	approved := command(2)
	id := queue(approved)
	if again := queue(approved); again != id {
		t.Fatal("retry did not keep the same exact report request")
	}
	token := build(id)
	available := content(id, token)
	right := available["reference"].(map[string]any)
	if available["readiness"] != "AVAILABLE" || available["difference_milli_brl_per_unit"] != "50" ||
		right["region"] != "MT" || right["method"] != "abastevo-metrics-v1" || right["survey_end"] != "2026-02-01" ||
		len(right["sources"].([]any)) != 2 || right["minimum_stations"] != float64(5) {
		t.Fatal("comparison lost its exact source basis, threshold, period or difference")
	}
	sparse := queue(command(3))
	insufficient := content(sparse, build(sparse))
	if insufficient["readiness"] != "INSUFFICIENT_COVERAGE" || insufficient["difference_milli_brl_per_unit"] != nil ||
		len(insufficient["reference"].(map[string]any)["samples"].([]any)) != 0 {
		t.Fatal("comparison exposed a value for a suppressed cohort")
	}
	waiting := queue(command(2))
	if _, err := fx.f.Admin.Exec(t.Context(), `INSERT INTO enterprise_data_source_reviews(source_code,source_digest,revision,status,decision_ref)
VALUES ('S-ESP-02',$1,2,'WITHDRAWN','synthetic-comparison-withdrawal')`, reference); err != nil {
		t.Fatal(err)
	}
	matrixCall(t, fx, router, owner, base+"/request", approved, 404, "NOT_FOUND", "", nil)
	matrixCall(t, fx, router, owner, base+"/"+id+"/access", map[string]any{}, 404, "NOT_FOUND", "", nil)
	matrixCall(t, fx, router, owner, base+"/"+id+"/content", map[string]any{"capability": token}, 404, "NOT_FOUND", "", nil)
	if ok, err := dispatcher.RunOnce(t.Context()); err != nil || !ok {
		t.Fatalf("withdrawn request was not settled: %v", err)
	}
	if state, code := exportState(t, fx, waiting); state != "FAILED" || code != "EXPORT_RIGHTS_LOST" {
		t.Fatalf("queued withdrawn report was processed as %s / %s", state, code)
	}
}
