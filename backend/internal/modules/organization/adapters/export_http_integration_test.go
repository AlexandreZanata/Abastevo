//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	exportpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/adapters"
	exportapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	exportdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// exportGrant provisions only the disposable ordinary runtime role with the
// privileges proposed for runtime rollout (EMP-RPT-001).
func exportGrant(t *testing.T, fx deviceFixtureState) {
	t.Helper()
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON org_export_jobs TO "+role+"; GRANT SELECT,INSERT ON org_sensitive_access_events TO "+role+"; GRANT SELECT,INSERT,UPDATE ON job_queue TO "+role); err != nil {
		t.Fatal(err)
	}
}

// syntheticExportRows is the test-only kind admitted through the suite
// option; production composes no format. It declares expense reads as the
// data permission (EMP-AUTH-011).
type syntheticExportRows struct{}

func (syntheticExportRows) Kind() string                           { return "test.synthetic_rows" }
func (syntheticExportRows) ReadPermission() permissions.Permission { return "exp.expense.read" }
func (syntheticExportRows) MediaType() string                      { return "text/csv" }
func (syntheticExportRows) Limits() exportdomain.Limits {
	return exportdomain.Limits{Rows: 20, Bytes: 2048, Duration: 5 * time.Second}
}
func (syntheticExportRows) Canonical(raw []byte) ([]byte, error) { return raw, nil }
func (syntheticExportRows) Produce(_ context.Context, in exportapp.ProduceInput, sink *exportdomain.Sink) error {
	var p struct {
		Rows int `json:"rows"`
	}
	if err := json.Unmarshal(in.Parameters, &p); err != nil {
		return err
	}
	if _, err := sink.Write([]byte("row_id,value_centavos\n")); err != nil {
		return err
	}
	for i := 1; i <= p.Rows; i++ {
		if err := sink.Row([]byte(fmt.Sprintf("%d,%d\n", i, i*100))); err != nil {
			return err
		}
	}
	return nil
}

func exportStorageConfig(t *testing.T) *exportpg.StorageConfig {
	t.Helper()
	value := func(name string) string {
		v := os.Getenv(name)
		if v == "" {
			t.Fatalf("explicit disposable %s required", name)
		}
		return v
	}
	return &exportpg.StorageConfig{Endpoint: value("ANPFUEL_TEST_STORAGE_ENDPOINT"), Bucket: value("ANPFUEL_TEST_STORAGE_BUCKET"), Region: value("ANPFUEL_TEST_STORAGE_REGION"),
		Credentials: storage.Credentials{AccessKeyID: value("ANPFUEL_TEST_STORAGE_ACCESS_KEY_ID"), SecretAccessKey: value("ANPFUEL_TEST_STORAGE_SECRET_ACCESS_KEY")}, AllowLoopback: true}
}

// exportRouter mounts the complete enterprise suite with private storage and
// the synthetic kind, and returns the production queue handlers.
func exportRouter(t *testing.T, fx deviceFixtureState) (chi.Router, *jobs.Dispatcher) {
	t.Helper()
	grantEmployeeTables(t, fx.legalFixtureState)
	fx.f.GrantMembershipAccessCommands(t)
	roleSQL := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_settings_commands,org_role_commands TO "+roleSQL+"; GRANT SELECT,INSERT,UPDATE ON org_ownership_transfers,identity_rate_windows TO "+roleSQL); err != nil {
		t.Fatal(err)
	}
	exportGrant(t, fx)
	runtimeAccounts := *fx.accounts
	runtimeAccounts.Store = accountpg.NewPGStore(fx.f.Runtime)
	options := enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{53}, 32), Pool: fx.f.Runtime, Accounts: &runtimeAccounts,
		ExportStorage: exportStorageConfig(t), ExportFormats: []exportapp.Format{syntheticExportRows{}}}
	router := chi.NewRouter()
	if err := enterprisesuite.Register(router, options); err != nil {
		t.Fatal(err)
	}
	handlers, err := enterprisesuite.ExportHandlers(options)
	if err != nil || len(handlers) != 2 {
		t.Fatal("export queue handlers", err)
	}
	registered := map[string]jobs.Handler{}
	for _, h := range handlers {
		registered[h.Kind()] = h
	}
	return router, &jobs.Dispatcher{Queue: jobs.NewQueue(fx.f.Runtime), Handlers: registered, WorkerID: "export-http-test", Kind: exportpg.BuildKind, LeaseTTL: time.Minute, RetryDelay: time.Millisecond}
}

func exportState(t *testing.T, fx deviceFixtureState, export string) (string, string) {
	t.Helper()
	var state, failure string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT state,COALESCE(failure_code,'') FROM org_export_jobs WHERE id=$1`, export).Scan(&state, &failure); err != nil {
		t.Fatal(err)
	}
	return state, failure
}

func exportRequestBody(t *testing.T, rows int) map[string]any {
	return map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": "test.synthetic_rows", "parameters": map[string]any{"rows": rows}}
}

// BUC-EMP-116/117, B-BR-EMP-227..230 through the actual company HTTP entry,
// the actual company authority and the production queue handlers.
func TestExportsThroughActualCompanyHTTPAndWorker(t *testing.T) {
	fx := devicesFixture(t)
	router, dispatcher := exportRouter(t, fx)
	owner := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	finance, colleague, driver := matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, finance, "FINANCE", "ORG", "ACTIVE")
	matrixAssign(t, fx, colleague, "FINANCE", "ORG", "ACTIVE")
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	base := "/v1/orgs/" + fx.scope.OrgID().String() + "/reports/exports"
	request := exportRequestBody(t, 3)
	created := matrixCall(t, fx, router, finance, base+"/request", request, 202, "", "", nil)
	job, _ := created["export"].(map[string]any)
	export, _ := job["export_id"].(string)
	if created["replayed"] != false || job["state"] != "QUEUED" || export == "" || job["kind"] != "test.synthetic_rows" {
		t.Fatal("request acknowledgement", created)
	}
	replay := matrixCall(t, fx, router, finance, base+"/request", request, 202, "", "", nil)
	if replay["replayed"] != true || replay["export"].(map[string]any)["export_id"] != export {
		t.Fatal("replay did not return the stored job", replay)
	}
	matrixCall(t, fx, router, driver, base+"/request", exportRequestBody(t, 1), 403, "FORBIDDEN", "rpt.export.run", nil)
	unknown := exportRequestBody(t, 1)
	unknown["kind"] = "test.unknown_rows"
	matrixCall(t, fx, router, finance, base+"/request", unknown, 422, "EXPORT_FORMAT_UNAVAILABLE", "", nil)
	matrixCall(t, fx, router, finance, base+"/"+export+"/access", map[string]any{}, 409, "EXPORT_NOT_READY", "", nil)

	if found, err := dispatcher.RunOnce(t.Context()); err != nil || !found {
		t.Fatal("build not dispatched", err)
	}
	if state, _ := exportState(t, fx, export); state != "READY" {
		t.Fatal("job not published", state)
	}
	read := matrixCall(t, fx, router, finance, base+"/"+export+"/read", map[string]any{}, 200, "", "", nil)
	if read["state"] != "READY" || read["row_count"] != float64(3) || read["sha256"] == nil {
		t.Fatal("read", read)
	}
	listed := matrixCall(t, fx, router, finance, base+"/list", map[string]any{"limit": 5}, 200, "", "", nil)
	if items, _ := listed["items"].([]any); len(items) != 1 {
		t.Fatal("list", listed)
	}
	if items, _ := matrixCall(t, fx, router, colleague, base+"/list", map[string]any{}, 200, "", "", nil)["items"].([]any); len(items) != 0 {
		t.Fatal("colleague listed the requester's job")
	}
	for _, path := range []string{"/read", "/access"} {
		matrixCall(t, fx, router, colleague, base+"/"+export+path, map[string]any{}, 404, "NOT_FOUND", "", nil)
	}
	matrixCall(t, fx, router, finance, "/v1/orgs/"+other.scope.OrgID().String()+"/reports/exports/"+export+"/read", map[string]any{}, 404, "NOT_FOUND", "", nil)
	granted := matrixCall(t, fx, router, finance, base+"/"+export+"/access", map[string]any{}, 200, "", "", nil)
	capability, _ := granted["capability"].(string)
	expires, _ := time.Parse(time.RFC3339Nano, fmt.Sprint(granted["expires_at"]))
	if capability == "" || expires.After(time.Now().Add(exportdomain.CapabilityTTL)) || granted["export_id"] != export {
		t.Fatal("capability", granted)
	}
	matrixCall(t, fx, router, colleague, base+"/"+export+"/content", map[string]any{"capability": capability}, 404, "NOT_FOUND", "", nil)
	matrixCall(t, fx, router, finance, base+"/"+export+"/content", map[string]any{"capability": capability + "x"}, 404, "NOT_FOUND", "", nil)
	content := matrixCall(t, fx, router, finance, base+"/"+export+"/content", map[string]any{"capability": capability}, 200, "", "", nil)
	raw, err := base64.StdEncoding.DecodeString(fmt.Sprint(content["content_base64"]))
	if err != nil || string(raw) != "row_id,value_centavos\n1,100\n2,200\n3,300\n" || content["media_type"] != "text/csv" || content["sha256"] != read["sha256"] {
		t.Fatal("content", err)
	}
	var issued, served int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FILTER(WHERE stage='ISSUED'),count(*) FILTER(WHERE stage='SERVED') FROM org_sensitive_access_events WHERE category='EXPORT_FILE' AND membership_id=$1`, finance.member).Scan(&issued, &served); err != nil || issued != 1 || served != 1 {
		t.Fatal("download not audited once per stage", issued, served)
	}
	for _, body := range []map[string]any{granted, content, read} {
		raw, _ := json.Marshal(body)
		for _, forbidden := range []string{"org/", "/export/", "X-Amz", "http", "validation-private"} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatal("response exposed a storage reference", forbidden)
			}
		}
	}

	// Permission revoked before processing: FAILED, no file.
	revoked := matrixCall(t, fx, router, finance, base+"/request", exportRequestBody(t, 2), 202, "", "", nil)["export"].(map[string]any)["export_id"].(string)
	matrixAssign(t, fx, finance, "FINANCE", "ORG", "NONE")
	if found, err := dispatcher.RunOnce(t.Context()); err != nil || !found {
		t.Fatal("build not dispatched", err)
	}
	if state, failure := exportState(t, fx, revoked); state != "FAILED" || failure != "EXPORT_RIGHTS_LOST" {
		t.Fatal("revoked requester processed", state, failure)
	}
	matrixCall(t, fx, router, finance, base+"/"+export+"/access", map[string]any{}, 403, "FORBIDDEN", "rpt.export.run", nil)
	matrixAssign(t, fx, finance, "FINANCE", "ORG", "ACTIVE")
	failed := matrixCall(t, fx, router, finance, base+"/"+revoked+"/read", map[string]any{}, 200, "", "", nil)
	if failed["state"] != "FAILED" || failed["failure_code"] != "EXPORT_RIGHTS_LOST" || failed["sha256"] != nil {
		t.Fatal("failed job view", failed)
	}
	// Membership suspended before processing: FAILED as well.
	suspended := matrixCall(t, fx, router, colleague, base+"/request", exportRequestBody(t, 1), 202, "", "", nil)["export"].(map[string]any)["export_id"].(string)
	if err := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_memberships SET state='SUSPENDED',suspension_origin='MANAGER',revision=revision+1 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), colleague.member); err != nil {
		t.Fatal(err)
	}
	if found, err := dispatcher.RunOnce(t.Context()); err != nil || !found {
		t.Fatal("build not dispatched", err)
	}
	if state, failure := exportState(t, fx, suspended); state != "FAILED" || failure != "EXPORT_RIGHTS_LOST" {
		t.Fatal("suspended requester processed", state, failure)
	}
	var objects int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_export_jobs WHERE id IN ($1,$2) AND object_sha256 IS NOT NULL`, revoked, suspended).Scan(&objects); err != nil || objects != 0 {
		t.Fatal("refused requester produced a file", objects)
	}
}
