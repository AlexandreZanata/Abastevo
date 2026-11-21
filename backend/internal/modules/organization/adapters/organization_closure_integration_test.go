//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	exportpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/go-chi/chi/v5"
)

func TestOrganizationArchiveAndScheduledClosureKeepExportAndLegalHold(t *testing.T) {
	t.Setenv("ANPFUEL_TEST_RUNTIME_GRANTS", "script")
	x := tripFixture(t)
	fx := x.deviceFixtureState
	ctx, org := t.Context(), fx.scope.OrgID().String()
	accounts := *fx.accounts
	accounts.Store = accountpg.NewPGStore(fx.f.Runtime)
	o := enterprisesuite.Options{Enabled: true, BillingEnabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{11}, 32),
		Pool: fx.f.Runtime, Accounts: &accounts, ExportStorage: exportStorageConfig(t)}
	router := chi.NewRouter()
	if err := enterprisesuite.Register(router, o); err != nil {
		t.Fatal(err)
	}
	owner, driver := matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	matrixAssign(t, fx, driver, "DRIVER", "ORG", "ACTIVE")
	path := "/v1/orgs/" + org
	previewPath := path + "/lifecycle/open-items"
	changePath := path + "/lifecycle/change"
	matrixCall(t, fx, router, driver, previewPath, map[string]any{}, 403, "FORBIDDEN", "org.lifecycle.manage", nil)
	foreign := "/v1/orgs/" + fx.f.Organizations[1].ID + "/lifecycle/open-items"
	matrixCall(t, fx, router, owner, foreign, map[string]any{}, 404, "NOT_FOUND", "", nil)
	report := func() map[string]any {
		t.Helper()
		return matrixCall(t, fx, router, owner, previewPath, map[string]any{}, 200, "", "", nil)
	}
	command := func(action string, revision any) map[string]any {
		return map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "action": action, "revision": revision}
	}
	started := x.startOp(t, x.syncFixtureState, x.declaredNow(t), nil)
	ack, err := x.process(t, x.syncFixtureState, started)
	x.accepted(t, started, ack, err, 1, 1)
	now := report()
	if now["state"] != "ACTIVE" || now["open_items"].(map[string]any)["trips"] != float64(1) {
		t.Fatalf("report did not show the real open trip: %v", now)
	}
	archive := command("ARCHIVE", now["revision"])
	if status := matrixCallStatus(t, fx, router, owner, changePath, archive); status != 409 || report()["state"] != "ACTIVE" {
		t.Fatalf("open work did not prevent archival: %d", status)
	}
	tripID := tripOf(started)
	end := x.finishOp(t, x.syncFixtureState, tripID, x.declaredNow(t), func(_, payload map[string]any) { payload["run_clock"] = nil })
	ack, err = x.process(t, x.syncFixtureState, end)
	x.accepted(t, end, ack, err, 2, 1)

	var name string
	if err := fx.f.Admin.QueryRow(ctx, `SELECT display_name FROM organizations WHERE org_id=$1`, org).Scan(&name); err != nil {
		t.Fatal(err)
	}
	matrixCall(t, fx, router, owner, path+"/ownership-transfer/propose", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "target_membership_id": driver.member, "step_down_to_admin": false,
	}, 200, "", "", nil)
	matrixCall(t, fx, router, owner, path+"/consolidation/grants/save", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "action": "LEGAL_HOLD", "category": "TRIPS",
		"target_id": tripID, "hold_active": true, "reason": "Synthetic court order",
	}, 200, "", "", nil)
	now = report()
	archived := matrixCall(t, fx, router, owner, changePath, command("ARCHIVE", now["revision"]), 200, "", "", nil)
	if archived["state"] != "ARCHIVED" || archived["closes_at"] != nil {
		t.Fatalf("owner archive unexpectedly scheduled closure: %v", archived)
	}
	var revokedTransfer bool
	if err := fx.f.Admin.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM org_ownership_transfers WHERE org_id=$1 AND state='CANCELLED' AND reduction_cause='ORGANIZATION_ARCHIVED')`, org).Scan(&revokedTransfer); err != nil || !revokedTransfer {
		t.Fatalf("archive left an open ownership transfer: %v", err)
	}
	matrixCall(t, fx, router, owner, path+"/stock/tanks/save", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic blocked tank", "product": "DIESEL_S10", "unit": "L", "capacity_milli": 100000,
	}, 403, "ORG_ARCHIVED", "", nil)
	res := matrixCall(t, fx, router, owner, changePath, command("RESTORE", archived["revision"]), 200, "", "", nil)
	if res["state"] != "ACTIVE" {
		t.Fatal("archived company did not restore to its previous active state")
	}
	schedule := command("REQUEST_CLOSURE", res["revision"])
	schedule["confirmation"], schedule["closure_days"] = "Incorrect synthetic name", 7
	matrixCall(t, fx, router, owner, changePath, schedule, 422, "VALIDATION_FAILED", "", nil)
	schedule["confirmation"] = name
	pending := matrixCall(t, fx, router, owner, changePath, schedule, 200, "", "", nil)
	if pending["state"] != "ARCHIVED" || pending["closes_at"] == nil {
		t.Fatalf("owner closure request did not open the archived export window: %v", pending)
	}
	if replay := matrixCall(t, fx, router, owner, changePath, schedule, 200, "", "", nil); replay["replayed"] != true || replay["closes_at"] != pending["closes_at"] {
		t.Fatal("replay changed the closure date")
	}
	var ownerNotices int
	if err := fx.f.Admin.QueryRow(ctx, `SELECT count(*) FROM org_notification_notices n JOIN org_notification_requests r
  ON (r.org_id,r.id)=(n.org_id,n.request_id) WHERE r.org_id=$1 AND r.source_event_id=$2 AND n.membership_id=$3`,
		org, schedule["operation_id"], owner.member).Scan(&ownerNotices); err != nil || ownerNotices != 1 {
		t.Fatalf("closure reminder did not reach the owner exactly once: %d %v", ownerNotices, err)
	}
	matrixCall(t, fx, router, owner, changePath, command("RESTORE", pending["revision"]), 409, "CONFLICT", "", nil)
	jobHandler := adapters.OrganizationClosureWorker{Pool: fx.f.Runtime}
	queue := jobs.NewQueue(fx.f.Runtime)
	if _, err := queue.Claim(ctx, jobHandler.Kind(), "closure-fixture", time.Minute); !errors.Is(err, jobs.ErrNoJob) {
		t.Fatalf("closure was claimable before the scheduled instant: %v", err)
	}
	export := matrixCall(t, fx, router, owner, path+"/reports/exports/request", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "kind": "org.full_export.zip", "parameters": map[string]any{},
	}, 202, "", "", nil)["export"].(map[string]any)["export_id"].(string)
	handlers, err := enterprisesuite.ExportHandlers(o)
	if err != nil {
		t.Fatal(err)
	}
	exportWorker := jobs.Dispatcher{Queue: queue, Kind: exportpg.BuildKind, Handlers: make(map[string]jobs.Handler), WorkerID: "closure-export-fixture"}
	for _, handler := range handlers {
		exportWorker.Handlers[handler.Kind()] = handler
	}
	if found, err := exportWorker.RunOnce(ctx); err != nil || !found {
		t.Fatalf("archived export worker found=%v: %v", found, err)
	}
	exportPath := path + "/reports/exports/" + export
	grant := matrixCall(t, fx, router, owner, exportPath+"/access", map[string]any{}, 200, "", "", nil)["capability"].(string)
	payload := matrixCall(t, fx, router, owner, exportPath+"/content", map[string]any{"capability": grant}, 200, "", "", nil)
	raw, err := base64.StdEncoding.DecodeString(payload["content_base64"].(string))
	if err != nil || !bytes.HasPrefix(raw, []byte("PK")) {
		t.Fatalf("archived owner did not obtain a private ZIP: %v", err)
	}
	cancel := matrixCall(t, fx, router, owner, changePath, command("CANCEL_CLOSURE", pending["revision"]), 200, "", "", nil)
	if cancel["state"] != "ARCHIVED" || cancel["closes_at"] != nil {
		t.Fatal("cancellation changed the archived state")
	}
	cancelledID := schedule["operation_id"].(string)
	if _, err := fx.f.Admin.Exec(ctx, `UPDATE job_queue SET not_before=clock_timestamp() WHERE id=(SELECT job_id FROM org_closure_requests WHERE org_id=$1 AND operation_id=$2)`, org, cancelledID); err != nil {
		t.Fatal(err)
	}
	closeWorker := jobs.Dispatcher{Queue: queue, Kind: jobHandler.Kind(), Handlers: map[string]jobs.Handler{jobHandler.Kind(): jobHandler}, WorkerID: "closure-fixture"}
	if found, err := closeWorker.RunOnce(ctx); err != nil || !found || report()["state"] != "ARCHIVED" {
		t.Fatalf("stale cancelled closure job was not harmless: %v / %v", found, err)
	}
	schedule = command("REQUEST_CLOSURE", cancel["revision"])
	schedule["confirmation"], schedule["closure_days"] = name, 7
	matrixCall(t, fx, router, owner, changePath, schedule, 200, "", "", nil)
	dueID := schedule["operation_id"].(string)
	ageClosureFixture(t, fx, dueID)
	matrixCall(t, fx, router, owner, previewPath, map[string]any{}, 404, "NOT_FOUND", "", nil)
	matrixCall(t, fx, router, owner, exportPath+"/access", map[string]any{}, 404, "NOT_FOUND", "", nil)
	if found, err := closeWorker.RunOnce(ctx); err != nil || !found {
		t.Fatalf("due closure was not claimed: %v / %v", found, err)
	}
	var terminal, result string
	var members, devices, holds, trips int64
	if err := fx.f.Admin.QueryRow(ctx, `SELECT o.state,c.state,c.hold_count,
 (SELECT count(*) FROM org_memberships m WHERE m.org_id=o.org_id AND m.state<>'ENDED'),
 (SELECT count(*) FROM org_device_enrollments e WHERE e.org_id=o.org_id AND e.state='ENROLLED'),
 (SELECT count(*) FROM org_trips t WHERE t.org_id=o.org_id)
FROM organizations o JOIN org_closure_requests c ON c.org_id=o.org_id WHERE o.org_id=$1 AND c.operation_id=$2`, org, dueID).
		Scan(&terminal, &result, &holds, &members, &devices, &trips); err != nil ||
		terminal != "CLOSED" || result != "COMPLETED" || holds != 1 || members != 0 || devices != 0 || trips != 1 {
		t.Fatalf("closure did not retain held trip or end access: state=%s request=%s members=%d devices=%d holds=%d trips=%d err=%v",
			terminal, result, members, devices, holds, trips, err)
	}
	matrixCall(t, fx, router, owner, previewPath, map[string]any{}, 404, "NOT_FOUND", "", nil)
	matrixCall(t, fx, router, owner, exportPath+"/content", map[string]any{"capability": grant}, 404, "NOT_FOUND", "", nil)
	matrixCall(t, fx, router, owner, changePath, command("RESTORE", cancel["revision"]), 404, "NOT_FOUND", "", nil)
}

// Only this disposable fixture advances already recorded deadlines. The
// ordinary worker receives the archived organization and a due job under the
// unmodified production rules.
func ageClosureFixture(t *testing.T, fx deviceFixtureState, operation string) {
	t.Helper()
	ctx := t.Context()
	tx, err := fx.f.Admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `ALTER TABLE org_closure_requests DISABLE TRIGGER org_closure_request_guard`); err != nil {
		t.Fatal(err)
	}
	var job string
	if err := tx.QueryRow(ctx, `UPDATE org_closure_requests SET requested_at=clock_timestamp()-interval '192 hours',
closes_at=clock_timestamp()-interval '12 hours' WHERE org_id=$1 AND operation_id=$2 AND state='PENDING' RETURNING job_id::text`,
		fx.scope.OrgID().String(), operation).Scan(&job); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE job_queue SET not_before=clock_timestamp()-interval '12 hours' WHERE id=$1`, job); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `ALTER TABLE org_closure_requests ENABLE TRIGGER org_closure_request_guard`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}
