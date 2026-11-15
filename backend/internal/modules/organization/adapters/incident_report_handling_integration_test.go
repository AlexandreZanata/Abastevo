//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	maintpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/adapters"
	maintapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/incident"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// incidentHandler is one company member with a current web proof.
type incidentHandler struct {
	actor                     maintapp.IncidentActor
	member, employee, account string
}

// handlingState is the intake fixture plus the real company owner.
type handlingState struct {
	incidentState
	company *maintpg.IncidentStore
}

func handlingFixture(t *testing.T) handlingState {
	t.Helper()
	x := handlingState{incidentState: incidentFixture(t)}
	tripReviewGrants(t, x.f)
	catalog, err := audit.NewCatalog(maintpg.IncidentAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	x.company = &maintpg.IncidentStore{Pool: x.f.Runtime, Authority: &orgpg.IncidentAuthority{Memberships: x.legalFixtureState.store.Memberships, Roles: x.legalFixtureState.store.Roles, Capabilities: x.flags,
		Structure: &orgpg.ResourceStructure{Kind: orgpg.IncidentStructure, Pool: x.f.Runtime}}, Audit: auditpg.NewStore(x.f.Runtime, catalog)}
	return x
}

// handlerFor is a new member whose only grant is the given role and scope.
func (x handlingState) handlerFor(t *testing.T, role, scopeType, target string) incidentHandler {
	t.Helper()
	p := deviceNewPerson(t, x.deviceFixtureState)
	org := x.scope.OrgID().String()
	switch scopeType {
	case "ORG":
		target = org
	case "SELF":
		target = p.member
	}
	if err := unitSchemaExec(t, x.legalFixtureState, `UPDATE org_role_assignments SET ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND membership_id=$2 AND ended_at IS NULL`, org, p.member); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, x.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'Synthetic incident handler')`,
		org, onboardingID(t), p.member, role, scopeType, target, time.Now().UTC(), x.member); err != nil {
		t.Fatal(role, scopeType, err)
	}
	return incidentHandler{actor: maintapp.IncidentActor{AccountID: p.web.AccountID, SessionID: p.web.SessionID}, member: p.member, employee: p.employee, account: p.auth.Account.ID}
}

// reporterHandler is the intake member d acting on the company plane.
func reporterHandler(d syncFixtureState) incidentHandler {
	p := d.person
	return incidentHandler{actor: maintapp.IncidentActor{AccountID: p.web.AccountID, SessionID: p.web.SessionID}, member: p.member, employee: p.employee, account: p.auth.Account.ID}
}

func text(v string) *string { return &v }

// decide never calls t.Fatal, so it is safe inside a goroutine.
func (x handlingState) decide(h incidentHandler, report string, revision int64, action incident.Action, note *string, duplicate, operation string) (maintapp.DecisionResult, bool, error) {
	command, err := maintapp.NewDecisionCommand(maintapp.DecisionInput{OperationID: operation, ReportID: report, ExpectedRevision: revision, Action: action, Note: note, DuplicateOf: duplicate})
	if err != nil {
		return maintapp.DecisionResult{}, false, err
	}
	return x.company.DecideReport(context.Background(), x.scope, h.actor, h.member, command)
}

func (x handlingState) reported(t *testing.T, payload map[string]any) string {
	t.Helper()
	d := x.syncFixtureState
	op := x.reportOp(t, d, payload, x.declaredNow(t), nil)
	ack, err := x.process(t, d, op)
	x.reportAccepted(t, op, ack, err)
	return reportOf(op)
}

// history renders the stored steps and counts the step audit events.
func (x handlingState) history(t *testing.T, report string) (string, int, string) {
	t.Helper()
	var steps, state string
	var audits int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT COALESCE((SELECT string_agg(action||':'||to_state||':'||revision,',' ORDER BY revision) FROM org_incident_report_transitions WHERE org_id=$1 AND report_id=$2),''),
 (SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND target_type='incident_report' AND target_id=$2),(SELECT state||':'||revision FROM org_incident_reports WHERE org_id=$1 AND id=$2)`,
		x.scope.OrgID().String(), report).Scan(&steps, &audits, &state); err != nil {
		t.Fatal(err)
	}
	return steps, audits, state
}

// BUC-EMP-188 / B-BR-EMP-373 on real PostGIS through the real owner.
func TestIncidentReportHandlingStepsRefusalsAndReplay(t *testing.T) {
	x := handlingFixture(t)
	d := x.syncFixtureState
	before := x.costSnapshot(t)
	blocking := damagePayload(x.vehicle.AssetID, nil)
	blocking["blocks_use"] = true
	report := x.reported(t, blocking)
	other := x.reported(t, damagePayload(x.vehicle.AssetID, nil))
	owner := x.handlerFor(t, "OWNER", "ORG", "")
	ack := onboardingID(t)
	first, replayed, err := x.decide(owner, report, 1, incident.Acknowledge, nil, "", ack)
	if err != nil || replayed || first.Step.To != incident.Acknowledged || first.Step.Revision != 2 || first.RecordedAt.IsZero() {
		t.Fatal("acknowledgement", err, first.Step)
	}
	again, replayed, err := x.decide(owner, report, 1, incident.Acknowledge, nil, "", ack)
	if err != nil || !replayed || again.Step != first.Step || !again.RecordedAt.Equal(first.RecordedAt) {
		t.Fatal("replayed acknowledgement", err, replayed)
	}
	if _, _, err := x.decide(owner, report, 2, incident.Treat, text("Oficina agendada"), "", ack); !errors.Is(err, maintapp.ErrOperationReused) {
		t.Fatal("same operation with another digest", err)
	}
	for name, c := range map[string]struct {
		revision int64
		action   incident.Action
		note     *string
		want     error
	}{
		"stale revision":          {1, incident.Treat, text("Oficina agendada"), maintapp.ErrConflict},
		"future revision":         {3, incident.Treat, text("Oficina agendada"), maintapp.ErrConflict},
		"acknowledge twice":       {2, incident.Acknowledge, nil, maintapp.ErrConflict},
		"treat without note":      {2, incident.Treat, nil, incident.ErrInvalid},
		"resolve blank note":      {2, incident.Resolve, text("   "), incident.ErrInvalid},
		"acknowledge with note":   {2, incident.Acknowledge, text("Ciente"), incident.ErrInvalid},
		"resolve note too long":   {2, incident.Resolve, text(strings.Repeat("n", 501)), incident.ErrInvalid},
		"self resolves blocking":  {2, incident.Resolve, text("Resolvido"), maintapp.ErrSelfApproval},
		"self dismisses blocking": {2, incident.Dismiss, text("Descartado"), maintapp.ErrSelfApproval},
	} {
		h := owner
		if strings.HasPrefix(name, "self") {
			h = reporterHandler(d)
		}
		if _, _, err := x.decide(h, report, c.revision, c.action, c.note, "", onboardingID(t)); !errors.Is(err, c.want) {
			t.Fatal(name, err)
		}
	}
	if steps, audits, state := x.history(t, report); steps != "ACKNOWLEDGE:ACKNOWLEDGED:2" || audits != 1 || state != "ACKNOWLEDGED:2" {
		t.Fatal("refusals changed the history", steps, audits, state)
	}
	// The reporter (ADMIN holds the key) may record an action on its own
	// blocking report; only the release-like steps need someone else.
	if _, _, err := x.decide(reporterHandler(d), report, 2, incident.Treat, text("Levado para a oficina"), "", onboardingID(t)); err != nil {
		t.Fatal("reporter's own action", err)
	}
	if resolved, _, err := x.decide(owner, report, 3, incident.Resolve, text("Lanterna trocada"), "", onboardingID(t)); err != nil || resolved.Step.To != incident.Resolved || resolved.Step.Revision != 4 {
		t.Fatal("independent resolution", err)
	}
	if _, _, err := x.decide(owner, report, 4, incident.Dismiss, text("Depois"), "", onboardingID(t)); !errors.Is(err, maintapp.ErrConflict) {
		t.Fatal("a terminal report changed", err)
	}
	if steps, audits, state := x.history(t, report); steps != "ACKNOWLEDGE:ACKNOWLEDGED:2,TREAT:IN_TREATMENT:3,RESOLVE:RESOLVED:4" || audits != 3 || state != "RESOLVED:4" {
		t.Fatal("handling history", steps, audits, state)
	}
	var notes bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT bool_and(a.request_hash=s.request_hash AND a.reason_code='' AND a.actor_id=s.actor_account_id) AND count(*)=3
 FROM org_incident_report_transitions s JOIN org_audit_events a ON(a.org_id,a.operation_id)=(s.org_id,s.operation_id) WHERE s.org_id=$1 AND s.report_id=$2`, x.scope.OrgID().String(), report).Scan(&notes); err != nil || !notes {
		t.Fatal("each step is not bound to its own audit event", err)
	}
	// Duplicates: same asset only, never itself; unknown → not found.
	if _, _, err := x.decide(owner, other, 1, incident.Dismiss, text("Repete o relato"), onboardingID(t), onboardingID(t)); !errors.Is(err, maintapp.ErrNotFound) {
		t.Fatal("unknown duplicate", err)
	}
	implement := x.reported(t, damagePayload(x.otherAsset(t), nil))
	if _, _, err := x.decide(owner, other, 1, incident.Dismiss, text("Repete o relato"), implement, onboardingID(t)); !errors.Is(err, incident.ErrInvalid) {
		t.Fatal("duplicate of another asset", err)
	}
	if _, _, err := x.decide(owner, other, 1, incident.Dismiss, text("Repete o relato"), other, onboardingID(t)); !errors.Is(err, incident.ErrInvalid) {
		t.Fatal("duplicate of itself", err)
	}
	if dismissed, _, err := x.decide(owner, other, 1, incident.Dismiss, text("Repete o relato anterior"), report, onboardingID(t)); err != nil || dismissed.Step.To != incident.Dismissed {
		t.Fatal("duplicate dismissal", err)
	}
	var duplicate string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT duplicate_of::text FROM org_incident_report_transitions WHERE org_id=$1 AND report_id=$2`, x.scope.OrgID().String(), other).Scan(&duplicate); err != nil || duplicate != report {
		t.Fatal("dismissal did not keep the original", duplicate, err)
	}
	// BR-CHECK-018: handling books nothing.
	if after := x.costSnapshot(t); !sameCounts(before, after) {
		t.Fatal("handling changed cost or document rows", before, after)
	}
}

// otherAsset is an ACTIVE implement of the same organization in the unit.
func (x incidentState) otherAsset(t *testing.T) string {
	t.Helper()
	input := fleetCreateInput()
	input.UnitID = x.unit
	created, _, err := fleetWrite(t, x.legalFixtureState, x.assets, fleetCommand(t, input))
	if err != nil {
		t.Fatal(err)
	}
	if active, _, err := fleetWrite(t, x.legalFixtureState, x.assets, fleetCommand(t, fa.AssetWriteInput{Kind: fa.ActivateAsset, AssetID: created.AssetID, ExpectedRevision: 1})); err != nil || active.State != fleet.Active {
		t.Fatal("second asset not active", err)
	}
	return created.AssetID
}

// Authorization (V-AUTHZ/V-ISO): structural coverage through the stored
// snapshot, AUDITOR and DRIVER without the key, another organization.
func TestIncidentReportHandlingAuthorityAndIsolation(t *testing.T) {
	x := handlingFixture(t)
	report := x.reported(t, damagePayload(x.vehicle.AssetID, nil))
	org := x.scope.OrgID().String()
	second := onboardingID(t)
	if err := unitSchemaExec(t, x.legalFixtureState, `INSERT INTO org_units(org_id,id,name,code,time_zone,state) VALUES($1,$2,'Synthetic second unit','INC2','UTC','ACTIVE')`, org, second); err != nil {
		t.Fatal(err)
	}
	inScope := x.handlerFor(t, "SUPERVISOR", "UNIT", x.unit)
	outOfScope := x.handlerFor(t, "SUPERVISOR", "UNIT", second)
	assetScope := x.handlerFor(t, "FLEET_MANAGER", "ASSET", x.vehicle.AssetID)
	auditor := x.handlerFor(t, "AUDITOR", "ORG", "")
	driver := x.handlerFor(t, "DRIVER", "SELF", "")
	forbidden := func(name string, err error) {
		t.Helper()
		var denial *permission.PermissionDenial
		if !errors.As(err, &denial) || denial.Permission() != "check.incident.manage" {
			t.Fatal(name, "expected FORBIDDEN check.incident.manage", err)
		}
	}
	for name, h := range map[string]incidentHandler{"out-of-scope": outOfScope, "auditor": auditor, "driver": driver} {
		_, _, err := x.decide(h, report, 1, incident.Acknowledge, nil, "", onboardingID(t))
		forbidden(name+" decide", err)
		_, err = x.company.ReadReport(context.Background(), x.scope, h.actor, h.member, report)
		forbidden(name+" read", err)
	}
	for name, h := range map[string]incidentHandler{"unit": inScope, "asset": assetScope, "auditor": auditor} {
		_, err := x.company.ListReports(context.Background(), x.scope, h.actor, h.member, maintapp.ReportFilter{Limit: 10})
		forbidden(name+" queue", err)
	}
	if _, err := x.company.ReadReport(context.Background(), x.scope, inScope.actor, inScope.member, report); err != nil {
		t.Fatal("in-scope read", err)
	}
	if done, _, err := x.decide(assetScope, report, 1, incident.Acknowledge, nil, "", onboardingID(t)); err != nil || done.Step.To != incident.Acknowledged {
		t.Fatal("asset-scoped handling", err)
	}
	if done, _, err := x.decide(inScope, report, 2, incident.Resolve, text("Resolvido no local"), "", onboardingID(t)); err != nil || done.Step.To != incident.Resolved {
		t.Fatal("unit-scoped handling", err)
	}
	// Another organization's report is not found for every member.
	owner := x.handlerFor(t, "OWNER", "ORG", "")
	if _, err := x.company.ReadReport(context.Background(), x.scope, owner.actor, owner.member, onboardingID(t)); !errors.Is(err, maintapp.ErrNotFound) && !errors.Is(err, access.ErrNotFound) {
		t.Fatal("unknown report", err)
	}
	foreign := x.foreignReport(t)
	if _, err := x.company.ReadReport(context.Background(), x.scope, owner.actor, owner.member, foreign); !errors.Is(err, maintapp.ErrNotFound) && !errors.Is(err, access.ErrNotFound) {
		t.Fatal("foreign report", err)
	}
	if _, _, err := x.decide(owner, foreign, 1, incident.Acknowledge, nil, "", onboardingID(t)); !errors.Is(err, maintapp.ErrNotFound) && !errors.Is(err, access.ErrNotFound) {
		t.Fatal("foreign report decided", err)
	}
	page, err := x.company.ListReports(context.Background(), x.scope, owner.actor, owner.member, maintapp.ReportFilter{Limit: 10})
	if err != nil || len(page.Items) != 1 || page.Items[0].ReportID != report {
		t.Fatal("queue leaked another organization's report", err, len(page.Items))
	}
}

// foreignReport plants a report of another organization.
func (x handlingState) foreignReport(t *testing.T) string {
	t.Helper()
	other, _, err := x.onboarding.Create(t.Context(), x.actor, onboardingCommand(t, onboardingID(t)))
	if err != nil {
		t.Fatal(err)
	}
	id := onboardingID(t)
	if err := pgx.BeginFunc(t.Context(), x.f.Admin, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `SET LOCAL session_replication_role=replica`); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `INSERT INTO org_incident_reports(org_id,id,operation_id,payload_digest,report_type,asset_id,description,blocks_use,reporter_account_id,reporter_membership_id,
 reporter_employee_id,enrollment_id,occurred_at,occurred_submicro,entry_zone,device_offset_seconds,state,revision)
VALUES($1,$2,$3,'sha256:'||repeat('0',64),'DAMAGE',$4,'Foreign damage',false,$5,$6,$5,$5,clock_timestamp(),0,'America/Cuiaba',-14400,'REPORTED',1)`,
			other.OrgID.String(), id, onboardingID(t), onboardingID(t), onboardingID(t), other.MembershipID)
		return err
	}); err != nil {
		t.Fatal("foreign report fixture", err)
	}
	return id
}

// Two handlers on one revision leave exactly one step, in both orders; the
// loser is CONFLICT and wrote no row or audit event.
func TestIncidentReportHandlingRaceLeavesOneStepInBothOrders(t *testing.T) {
	x := handlingFixture(t)
	a, b := x.handlerFor(t, "OWNER", "ORG", ""), x.handlerFor(t, "FLEET_MANAGER", "ORG", "")
	type attempt struct {
		h      incidentHandler
		action incident.Action
		note   *string
	}
	for name, order := range map[string][2]attempt{
		"acknowledge-first": {{a, incident.Acknowledge, nil}, {b, incident.Dismiss, text("Sem dano")}},
		"dismiss-first":     {{b, incident.Dismiss, text("Sem dano")}, {a, incident.Acknowledge, nil}},
	} {
		t.Run(name, func(t *testing.T) {
			report := x.reported(t, damagePayload(x.vehicle.AssetID, nil))
			holder := make(chan int, 1)
			release := make(chan struct{})
			var once sync.Once
			free := func() { once.Do(func() { close(release) }) }
			defer free()
			done := make(chan error, 1)
			go func() {
				done <- database.WithOrgTx(context.Background(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
					if _, err := tx.Exec(context.Background(), `SELECT state FROM organizations WHERE org_id=$1 FOR UPDATE`, x.scope.OrgID().String()); err != nil {
						return err
					}
					var pid int
					if err := tx.QueryRow(context.Background(), `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
						return err
					}
					holder <- pid
					<-release
					return nil
				})
			}()
			var pid int
			select {
			case pid = <-holder:
			case err := <-done:
				t.Fatal("holder failed", err)
			}
			results := make(chan error, 2)
			var wg sync.WaitGroup
			run := func(v attempt) {
				wg.Go(func() {
					_, _, err := x.decide(v.h, report, 1, v.action, v.note, "", onboardingID(t))
					results <- err
				})
			}
			run(order[0])
			waitBlocked(t, x.syncFixtureState, pid, 1)
			run(order[1])
			waitBlocked(t, x.syncFixtureState, pid, 2)
			free()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			wg.Wait()
			close(results)
			ok, lost := 0, 0
			for err := range results {
				switch {
				case err == nil:
					ok++
				case errors.Is(err, maintapp.ErrConflict):
					lost++
				default:
					t.Fatal("unexpected race outcome", err)
				}
			}
			want := "ACKNOWLEDGE:ACKNOWLEDGED:2"
			if order[0].action == incident.Dismiss {
				want = "DISMISS:DISMISSED:2"
			}
			if steps, audits, _ := x.history(t, report); ok != 1 || lost != 1 || steps != want || audits != 1 {
				t.Fatal("two concurrent steps did not leave exactly the first", ok, lost, steps, audits)
			}
		})
	}
}

// EDR-238/239: forced RLS, append-only history and the database guards
// refuse an impossible, self-approved, unaudited or cross-bound row even
// when the application is bypassed; the report row changes only by a step.
func TestIncidentReportSchemaIsForcedRLSAppendOnlyAndGuarded(t *testing.T) {
	x := handlingFixture(t)
	org := x.scope.OrgID().String()
	blocking := damagePayload(x.vehicle.AssetID, nil)
	blocking["blocks_use"] = true
	report := x.reported(t, blocking)
	owner := x.handlerFor(t, "OWNER", "ORG", "")
	if _, _, err := x.decide(owner, report, 1, incident.Acknowledge, nil, "", onboardingID(t)); err != nil {
		t.Fatal(err)
	}
	reporter := reporterHandler(x.syncFixtureState)
	refused := func(name string, err error, code string) {
		t.Helper()
		if err == nil {
			t.Fatal(name, "accepted")
		}
		unitSchemaCode(t, err, code)
	}
	step := func(revision int64, action, from, to string, note any, duplicate any, h incidentHandler) error {
		return unitSchemaExec(t, x.legalFixtureState, `INSERT INTO org_incident_report_transitions(org_id,report_id,revision,action,from_state,to_state,note,duplicate_of,actor_account_id,actor_membership_id,actor_employee_id,operation_id,request_hash)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,repeat('a',64))`, org, report, revision, action, from, to, note, duplicate, h.account, h.member, h.employee, onboardingID(t))
	}
	refused("stale step", step(2, "TREAT", "REPORTED", "IN_TREATMENT", "x", nil, owner), "23514")
	refused("skipped revision", step(4, "TREAT", "ACKNOWLEDGED", "IN_TREATMENT", "x", nil, owner), "23514")
	refused("acknowledge with note", step(3, "ACKNOWLEDGE", "REPORTED", "ACKNOWLEDGED", "x", nil, owner), "23514")
	refused("treat without note", step(3, "TREAT", "ACKNOWLEDGED", "IN_TREATMENT", nil, nil, owner), "23514")
	refused("self resolution of a blocking report", step(3, "RESOLVE", "ACKNOWLEDGED", "RESOLVED", "x", nil, reporter), "23514")
	refused("duplicate of itself", step(3, "DISMISS", "ACKNOWLEDGED", "DISMISSED", "x", report, owner), "23514")
	refused("duplicate on resolve", step(3, "RESOLVE", "ACKNOWLEDGED", "RESOLVED", "x", x.reported(t, damagePayload(x.vehicle.AssetID, nil)), owner), "23514")
	refused("unaudited step", step(3, "TREAT", "ACKNOWLEDGED", "IN_TREATMENT", "x", nil, owner), "23503")
	refused("direct state change", unitSchemaExec(t, x.legalFixtureState, `UPDATE org_incident_reports SET state='RESOLVED',revision=3 WHERE org_id=$1 AND id=$2`, org, report), "55000")
	refused("description rewritten", unitSchemaExec(t, x.legalFixtureState, `UPDATE org_incident_reports SET description='Outro' WHERE org_id=$1 AND id=$2`, org, report), "42501")
	refused("report deleted", unitSchemaExec(t, x.legalFixtureState, `DELETE FROM org_incident_reports WHERE org_id=$1 AND id=$2`, org, report), "42501")
	for name, query := range map[string]string{
		"owner update report":     `UPDATE org_incident_reports SET description='Outro' WHERE id=$1`,
		"owner delete report":     `DELETE FROM org_incident_reports WHERE id=$1`,
		"owner update transition": `UPDATE org_incident_report_transitions SET note='x' WHERE report_id=$1`,
		"owner delete transition": `DELETE FROM org_incident_report_transitions WHERE report_id=$1`,
	} {
		_, err := x.f.Admin.Exec(t.Context(), query, report)
		refused(name, err, "55000")
	}
	_, err := x.f.Admin.Exec(t.Context(), `TRUNCATE org_incident_report_transitions`)
	refused("owner truncate", err, "55000")
	// Forced RLS: another scope sees nothing.
	var visible int
	if err := database.WithOrgTx(t.Context(), x.f.Runtime, x.otherScope(t), func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM org_incident_reports)+(SELECT count(*) FROM org_incident_report_transitions)`).Scan(&visible)
	}); err != nil || visible != 0 {
		t.Fatal("forced RLS leaked a report", visible, err)
	}
	var forced bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT bool_and(relrowsecurity AND relforcerowsecurity) FROM pg_class WHERE oid IN('org_incident_reports'::regclass,'org_incident_report_transitions'::regclass)`).Scan(&forced); err != nil || !forced {
		t.Fatal("RLS not forced", err)
	}
	// The report's own insert guard: a DRAFT asset, a wrong snapshot or an
	// operation of another type is refused even when the owner is bypassed.
	var op string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT operation_id::text FROM org_incident_reports WHERE org_id=$1 AND id=$2`, org, report).Scan(&op); err != nil {
		t.Fatal(err)
	}
	refused("second report on the same operation", unitSchemaExec(t, x.legalFixtureState, `INSERT INTO org_incident_reports(org_id,id,operation_id,payload_digest,report_type,asset_id,unit_id,description,blocks_use,reporter_account_id,reporter_membership_id,
 reporter_employee_id,enrollment_id,occurred_at,occurred_submicro,entry_zone,device_offset_seconds,state,revision)
SELECT org_id,$3,operation_id,payload_digest,report_type,asset_id,unit_id,description,blocks_use,reporter_account_id,reporter_membership_id,reporter_employee_id,enrollment_id,occurred_at,occurred_submicro,
 entry_zone,device_offset_seconds,'REPORTED',1 FROM org_incident_reports WHERE org_id=$1 AND id=$2`, org, report, onboardingID(t)), "23514")
	// Document links: the INCIDENT kind is bound to exactly its own typed
	// same-organization reference (EDR-239).
	var binding bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT pg_get_constraintdef(b.oid) LIKE '%''INCIDENT''%incident_report_id%' AND pg_get_constraintdef(k.oid) LIKE '%INCIDENT%'
 AND EXISTS(SELECT 1 FROM pg_constraint f WHERE f.conname='org_document_links_incident_fkey' AND f.contype='f' AND f.confrelid='org_incident_reports'::regclass)
 FROM pg_constraint b,pg_constraint k WHERE b.conname='org_document_links_binding' AND k.conname='org_document_links_target_kind_check'`).Scan(&binding); err != nil || !binding {
		t.Fatal("INCIDENT link binding", err)
	}
	// The asset, unit and cost-centre usage sources still answer with the report key.
	var usage bool
	if err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT org_asset_current_usage($1,$2) IS NOT NULL AND org_unit_current_usage($1,$3) IS NOT NULL`, org, x.vehicle.AssetID, x.unit).Scan(&usage)
	}); err != nil || !usage {
		t.Fatal("a usage source refused the report key", err)
	}
	if _, _, err := x.decide(owner, report, 2, incident.Treat, text("Oficina"), "", onboardingID(t)); err != nil {
		t.Fatal("guarded step through the owner", err)
	}
}

// otherScope is the scope of a new, unrelated organization.
func (x handlingState) otherScope(t *testing.T) kernel.OrgScope {
	t.Helper()
	other, _, err := x.onboarding.Create(t.Context(), x.actor, onboardingCommand(t, onboardingID(t)))
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(other.OrgID)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}
