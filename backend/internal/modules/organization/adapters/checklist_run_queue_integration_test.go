//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"slices"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	maintapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/checklist"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// queueRuns are the fixture runs of the EMP-CHECK-004 queue tests, created
// in this order (each later declared completion is newer).
type queueRuns struct {
	ok, blockA, review, blockB, released string
}

// queuedRuns stores, through the real check.run.complete owner, a plain run,
// two BLOCKING runs, a disagreement held for review (device COMPLETED_OK, a
// non-critical NOT_OK) and a BLOCKING run released by a fleet manager.
func (x runState) queuedRuns(t *testing.T) queueRuns {
	t.Helper()
	d := x.syncFixtureState
	template := x.template(t, checklist.Departure, organizationTarget())
	completed := func(device string, answers ...map[string]any) string {
		t.Helper()
		at := x.declaredNow(t)
		op := x.runOp(t, d, x.runPayload(template.TemplateID, template.Version, template.PolicyRevision, at.Add(-5*time.Minute), device, answers...), at, nil)
		ack, err := x.process(t, d, op)
		x.runAccepted(t, op, ack, err, len(answers))
		return runOf(op)
	}
	var r queueRuns
	r.ok = completed("COMPLETED_OK", answer("OK", nil), answer("OK", nil), answer("OK", nil))
	r.blockA = x.blockingRun(t, template)
	r.review = completed("COMPLETED_OK", answer("OK", nil), answer("OK", nil), answer("NOT_OK", "Pneu traseiro gasto"))
	r.blockB = x.blockingRun(t, template)
	r.released = x.blockingRun(t, template)
	manager := x.handlerFor(t, "FLEET_MANAGER", "ORG", "")
	if _, _, err := x.release(manager, r.released, 1, "Freios verificados na oficina", onboardingID(t)); err != nil {
		t.Fatal("release fixture", err)
	}
	for run, want := range map[string]string{r.ok: "COMPLETED_OK:COMPLETED_OK:COMPLETED_OK:false", r.review: "COMPLETED_OK:COMPLETED_WITH_ISSUES:COMPLETED_WITH_ISSUES:true",
		r.blockA: "BLOCKING:BLOCKING:BLOCKING:false", r.blockB: "BLOCKING:BLOCKING:BLOCKING:false", r.released: "BLOCKING:BLOCKING:COMPLETED_WITH_ISSUES:false"} {
		if got := x.stored(t, run); len(got) < len(want) || got[:len(want)] != want {
			t.Fatal("queue fixture run", run, got)
		}
	}
	return r
}

// foreignRun plants a BLOCKING run of another organization.
func (x runState) foreignRun(t *testing.T) string {
	t.Helper()
	other := x.otherScope(t).OrgID().String()
	id := onboardingID(t)
	if err := pgx.BeginFunc(t.Context(), x.f.Admin, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `SET LOCAL session_replication_role=replica`); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `INSERT INTO org_checklist_runs(org_id,id,operation_id,payload_digest,asset_id,context,template_id,template_version,policy_revision,server_policy_revision,
 started_at,occurred_at,occurred_submicro,entry_zone,device_offset_seconds,device_outcome,recheck_outcome,outcome,outcome_differs,template_superseded,acknowledged,
 author_account_id,author_membership_id,author_employee_id,enrollment_id,revision)
VALUES($1,$2,$3,'sha256:'||repeat('0',64),$4,'DEPARTURE',$5,1,1,1,date_trunc('microseconds',clock_timestamp()),clock_timestamp(),0,'America/Cuiaba',-14400,
 'BLOCKING','BLOCKING','BLOCKING',false,false,false,$6,$6,$6,$6,1)`, other, id, onboardingID(t), onboardingID(t), onboardingID(t), onboardingID(t))
		return err
	}); err != nil {
		t.Fatal("foreign run fixture", err)
	}
	return id
}

// queueState counts every row a queue read could touch.
func (x runState) queueState(t *testing.T) string {
	t.Helper()
	var v string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT concat_ws(':',(SELECT count(*) FROM org_checklist_runs),(SELECT count(*) FROM org_checklist_run_answers),
 (SELECT count(*) FROM org_checklist_run_releases),(SELECT count(*) FROM org_audit_events),(SELECT count(*) FROM org_sync_operations),
 (SELECT count(*) FROM org_sensitive_access_events),(SELECT string_agg(id::text||outcome||revision||updated_at::text,',' ORDER BY id) FROM org_checklist_runs))`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func (x runState) queue(h incidentHandler, limit int, after *maintapp.RunQueueKey) (maintapp.RunQueuePage, error) {
	return x.runs.ListRuns(context.Background(), x.scope, h.actor, h.member, maintapp.RunQueueFilter{After: after, Limit: limit})
}

func runIDs(items []maintapp.RunView) []string {
	out := make([]string, 0, len(items))
	for _, v := range items {
		out = append(out, v.RunID)
	}
	return out
}

// B-BR-EMP-506 on real PostGIS: the organization's runs in queue order
// (BLOCKING, then review_required, then the rest; newest completion first),
// the run read's exact view, keyset pages that partition the same order, only
// an ORG grant of check.incident.manage, no other organization's run and no
// write of any kind.
func TestChecklistRunQueueOrdersAttentionFirstAndIsOrganizationScoped(t *testing.T) {
	x := runFixture(t)
	r := x.queuedRuns(t)
	foreign := x.foreignRun(t)
	owner := x.handlerFor(t, "OWNER", "ORG", "")
	manager := x.handlerFor(t, "FLEET_MANAGER", "ORG", "")
	before := x.queueState(t)
	want := []string{r.blockB, r.blockA, r.review, r.released, r.ok}
	page, err := x.queue(owner, 100, nil)
	if err != nil || page.Next != nil || !slices.Equal(runIDs(page.Items), want) {
		t.Fatal("queue order", runIDs(page.Items), want, page.Next, err)
	}
	if slices.Contains(runIDs(page.Items), foreign) {
		t.Fatal("the queue listed another organization's run")
	}
	for _, v := range page.Items {
		read, err := x.runs.ReadRun(t.Context(), x.scope, owner.actor, owner.member, v.AssetID, v.RunID)
		if err != nil || read.Revision != v.Revision || read.Outcome != v.Outcome || read.DeviceOutcome != v.DeviceOutcome || read.RecheckOutcome != v.RecheckOutcome ||
			read.Differs != v.Differs || !slices.Equal(read.Issues, v.Issues) || !slices.Equal(read.Critical, v.Critical) || !read.CompletedAt.Equal(v.CompletedAt) ||
			(read.ReleasedAt == nil) != (v.ReleasedAt == nil) {
			t.Fatal("a queue item differs from the run read", v.RunID, err)
		}
	}
	groups := map[string]int{}
	for _, v := range page.Items {
		groups[v.RunID] = maintapp.RunGroup(v)
	}
	if groups[r.blockA] != maintapp.RunGroupBlocking || groups[r.review] != maintapp.RunGroupReview || groups[r.released] != maintapp.RunGroupOther || groups[r.ok] != maintapp.RunGroupOther {
		t.Fatal("queue groups", groups)
	}
	if released := page.Items[3]; released.Revision != 2 || released.ReleasedAt == nil || released.RecheckOutcome != checklist.Blocking || released.Outcome != checklist.CompletedWithIssues {
		t.Fatal("the released run lost its history", released.Revision, released.Outcome)
	}
	// Keyset pages of every size partition the same order.
	for _, limit := range []int{1, 2, 3} {
		var got []string
		var after *maintapp.RunQueueKey
		for i := 0; i < 10; i++ {
			next, err := x.queue(manager, limit, after)
			if err != nil || len(next.Items) > limit {
				t.Fatal("queue page", limit, err)
			}
			got = append(got, runIDs(next.Items)...)
			if next.Next == nil {
				break
			}
			if len(next.Items) != limit || !next.Next.Same(maintapp.KeyOf(next.Items[len(next.Items)-1])) {
				t.Fatal("queue next key", limit)
			}
			after = next.Next
		}
		if !slices.Equal(got, want) {
			t.Fatal("paged queue order", limit, got)
		}
	}
	// Only an ORG grant covers the organization queue (EDR-328): structural,
	// SELF, AUDITOR, FINANCE and DRIVER members are refused and see nothing.
	second := onboardingID(t)
	if err := unitSchemaExec(t, x.legalFixtureState, `INSERT INTO org_units(org_id,id,name,code,time_zone,state) VALUES($1,$2,'Synthetic queue unit','QUE2','UTC','ACTIVE')`, x.scope.OrgID().String(), second); err != nil {
		t.Fatal(err)
	}
	for name, h := range map[string]incidentHandler{
		"unit fleet manager": x.handlerFor(t, "FLEET_MANAGER", "UNIT", x.unit),
		"unit supervisor":    x.handlerFor(t, "SUPERVISOR", "UNIT", second),
		"auditor":            x.handlerFor(t, "AUDITOR", "ORG", ""),
		"finance":            x.handlerFor(t, "FINANCE", "ORG", ""),
		"driver":             x.handlerFor(t, "DRIVER", "SELF", ""),
	} {
		if refused, err := x.queue(h, 100, nil); err == nil || len(refused.Items) != 0 {
			t.Fatal("queue without an ORG check.incident.manage grant", name, err, len(refused.Items))
		}
	}
	// A key past the end is an empty page; an invalid filter reaches nothing.
	last := maintapp.KeyOf(page.Items[len(page.Items)-1])
	if tail, err := x.queue(owner, 10, &last); err != nil || len(tail.Items) != 0 || tail.Next != nil {
		t.Fatal("queue after the last key", err, len(tail.Items))
	}
	for name, filter := range map[string]maintapp.RunQueueFilter{"zero": {Limit: 0}, "large": {Limit: 101}} {
		if _, err := x.runs.ListRuns(t.Context(), x.scope, owner.actor, owner.member, filter); err == nil {
			t.Fatal("invalid queue filter", name)
		}
	}
	if after := x.queueState(t); after != before {
		t.Fatal("the queue read changed state", before, after)
	}
}

// B-BR-EMP-506/509 through the actual composed company routes the panel
// uses: the queue page and its sealed cursor, the author's SELF_APPROVAL,
// another member's release, the queue and the FLEET-007 availability read
// again (the block is the server's), refusals and malformed requests.
func TestChecklistRunQueueThroughComposedCompanyHTTP(t *testing.T) {
	x := runFixture(t)
	fx := x.deviceFixtureState
	router := chi.NewRouter()
	runtimeAccounts := *x.accounts
	runtimeAccounts.Store = accountpg.NewPGStore(x.f.Runtime)
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{37}, 32), Pool: x.f.Runtime, Accounts: &runtimeAccounts}); err != nil {
		t.Fatal("actual run queue composition failed", err)
	}
	template := x.template(t, checklist.Departure, organizationTarget())
	blocking := x.blockingRun(t, template)
	d := x.syncFixtureState
	at := x.declaredNow(t)
	plain := x.runOp(t, d, x.runPayload(template.TemplateID, template.Version, template.PolicyRevision, at.Add(-time.Minute), "COMPLETED_OK", answer("OK", nil), answer("OK", nil), answer("OK", nil)), at, nil)
	ack, err := x.process(t, d, plain)
	x.runAccepted(t, plain, ack, err, 3)
	owner, auditor := matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	matrixAssign(t, fx, auditor, "AUDITOR", "ORG", "ACTIVE")
	author := incidentLogin(t, d.person, router)
	driver := x.driver(t)
	x.assignDriver(t, driver)
	driverWeb := incidentLogin(t, driver.person, router)
	org := x.scope.OrgID().String()
	path := "/v1/orgs/" + org + "/checklists/runs/list"
	call := func(actor matrixPerson, path string, body any, want int, code, key string) map[string]any {
		t.Helper()
		return matrixCall(t, fx, router, actor, path, body, want, code, key, nil)
	}
	items := func(v map[string]any) []map[string]any {
		raw, _ := v["items"].([]any)
		out := []map[string]any{}
		for _, item := range raw {
			out = append(out, item.(map[string]any))
		}
		return out
	}
	fields := []string{"acknowledged", "asset_id", "completed_at", "context", "critical_issues", "data_mode", "device_outcome", "issues", "outcome", "outcome_differs",
		"policy_revision", "received_at", "recheck_outcome", "review_required", "revision", "run_id", "server_policy_revision", "started_at", "template_id",
		"template_superseded", "template_version"}
	first := call(owner, path, map[string]any{"limit": 1}, 200, "", "")
	page := items(first)
	cursor, _ := first["next_cursor"].(string)
	if len(page) != 1 || page[0]["run_id"] != blocking || page[0]["outcome"] != "BLOCKING" || cursor == "" || first["data_mode"] != "SYNTHETIC" {
		t.Fatal("first queue page", page, cursor)
	}
	keys := make([]string, 0, len(page[0]))
	for key := range page[0] {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	if !slices.Equal(keys, fields) {
		t.Fatal("queue item shape", keys)
	}
	second := call(owner, path, map[string]any{"limit": 1, "cursor": cursor}, 200, "", "")
	if rest := items(second); len(rest) != 1 || rest[0]["run_id"] != runOf(plain) || second["next_cursor"] != "" {
		t.Fatal("second queue page", rest, second["next_cursor"])
	}
	// The cursor is bound to the route and the limit; forged and malformed
	// requests are MALFORMED; members without an ORG grant are FORBIDDEN.
	call(owner, path, map[string]any{"limit": 2, "cursor": cursor}, 400, "MALFORMED", "")
	call(owner, path, map[string]any{"limit": 1, "cursor": "Zm9yZ2Vk"}, 400, "MALFORMED", "")
	call(owner, path, map[string]any{"limit": 101}, 400, "MALFORMED", "")
	call(owner, path, map[string]any{"limit": 1, "outcome": "BLOCKING"}, 400, "MALFORMED", "")
	call(owner, "/v1/orgs/"+org+"/incidents/reports/list", map[string]any{"limit": 1, "cursor": cursor}, 400, "MALFORMED", "")
	call(auditor, path, map[string]any{"limit": 10}, 403, "FORBIDDEN", "check.incident.manage")
	call(driverWeb, path, map[string]any{"limit": 10}, 403, "FORBIDDEN", "check.incident.manage")
	// The block is the server's: BLOCKED by this run through FLEET-007.
	availability := func() map[string]any {
		return call(owner, "/v1/orgs/"+org+"/fleet/assets/"+x.vehicle.AssetID+"/availability/read", map[string]any{}, 200, "", "")
	}
	blocked := availability()
	causes, _ := blocked["causes"].([]any)
	if blocked["availability"] != "BLOCKED" || len(causes) != 1 || causes[0].(map[string]any)["kind"] != "CHECKLIST_RUN" || causes[0].(map[string]any)["record_id"] != blocking {
		t.Fatal("the asset block is not the run's", blocked["availability"], causes)
	}
	release := func(actor matrixPerson, want int, code string) map[string]any {
		t.Helper()
		return call(actor, "/v1/orgs/"+org+"/checklists/runs/"+blocking+"/release", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "reason": "Freios verificados na oficina"}, want, code, "")
	}
	release(author, 409, "SELF_APPROVAL")
	if still := items(call(author, path, map[string]any{"limit": 10}, 200, "", "")); len(still) != 2 || still[0]["run_id"] != blocking || still[0]["revision"] != float64(1) {
		t.Fatal("a refused self release changed the queue", still)
	}
	if again := availability(); again["availability"] != "BLOCKED" {
		t.Fatal("a refused self release lifted the block", again["availability"])
	}
	released := release(owner, 200, "")
	if released["revision"] != float64(2) || released["outcome"] != "COMPLETED_WITH_ISSUES" || released["recheck_outcome"] != "BLOCKING" || released["replayed"] != false {
		t.Fatal("release through the route", released)
	}
	release(owner, 409, "CONFLICT")
	// Released, the run joins the other runs, after the newer plain run.
	after := items(call(owner, path, map[string]any{"limit": 10}, 200, "", ""))
	if len(after) != 2 || after[0]["run_id"] != runOf(plain) || after[1]["run_id"] != blocking {
		t.Fatal("queue order after the release", after)
	}
	if got := after[1]; got["revision"] != float64(2) || got["released_at"] == nil || got["outcome"] != "COMPLETED_WITH_ISSUES" || got["device_outcome"] != "BLOCKING" ||
		got["recheck_outcome"] != "BLOCKING" || !slices.Equal(got["issues"].([]any), []any{float64(2)}) || !slices.Equal(got["critical_issues"].([]any), []any{float64(2)}) {
		t.Fatal("the released run in the queue", got)
	}
	if free := availability(); free["availability"] != "AVAILABLE" || len(free["causes"].([]any)) != 0 {
		t.Fatal("the released vehicle is still blocked", free["availability"])
	}
	// B-BR-EMP-507: the content hint follows ORG coverage of the key; a
	// SUPERVISOR handles reports but gets no content hint.
	supervisor := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, supervisor, "SUPERVISOR", "ORG", "ACTIVE")
	hints := func(p matrixPerson) []any {
		t.Helper()
		v, _ := call(p, "/v1/org-access/contexts/read", map[string]any{"organization_id": org, "membership_id": p.member}, 200, "", "")["permissions"].([]any)
		return v
	}
	if got := hints(owner); !slices.Contains(got, any("doc.document.view_content")) {
		t.Fatal("owner without the content hint", got)
	}
	for name, p := range map[string]matrixPerson{"supervisor": supervisor, "driver": driverWeb} {
		if got := hints(p); slices.Contains(got, any("doc.document.view_content")) {
			t.Fatal("content hint without ORG coverage", name, got)
		}
	}
	items(call(supervisor, path, map[string]any{"limit": 10}, 200, "", ""))
}
