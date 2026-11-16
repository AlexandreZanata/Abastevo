//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	maintpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/adapters"
	maintapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/checklist"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// runState is the EMP-CHECK-003 handling fixture (SELF intake member with an
// ACTIVE allocated vehicle, company handlers) plus the real checklist
// template store, the check.run.complete owner and the company run owner
// (EMP-CHECK-002).
type runState struct {
	handlingState
	templates *maintpg.ChecklistStore
	runs      *maintpg.RunStore
	run       syncapp.Handler
}

// runGrants are the runtime privileges of the EMP-CHECK-002 owners (README
// proposal in the evidence): runs, answers and releases, the columns the
// release trigger updates and the template rows the recheck reads.
func runGrants(t *testing.T, f *orgtest.Fixture) {
	t.Helper()
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_checklist_runs,org_checklist_run_answers,org_checklist_run_releases TO "+role+
		";GRANT UPDATE(outcome,revision,updated_at) ON org_checklist_runs TO "+role+
		";GRANT SELECT,INSERT,UPDATE ON org_checklist_templates TO "+role+
		";GRANT SELECT,INSERT ON org_checklist_template_commands,org_checklist_template_versions,org_checklist_template_items,org_checklist_template_targets,org_checklist_policy_revisions TO "+role); err != nil {
		t.Fatal(err)
	}
}

func runIntakeConfig(fx syncFixtureState) syncpg.Config {
	config := incidentIntakeConfig(fx)
	config.NoAsyncPlans[syncapp.OperationKey{Type: maintapp.RunOperationType, Schema: 1}] = map[string]syncapp.FollowupMode{protocol.Accepted: syncapp.NoAsyncFollowup, protocol.Rejected: syncapp.NoAsyncFollowup}
	return config
}

func runFixtureOn(t *testing.T, h handlingState) runState {
	t.Helper()
	runGrants(t, h.f)
	x := runState{handlingState: h}
	catalog, err := audit.NewCatalog(append(maintpg.ChecklistAuditDefinitions(), maintpg.RunAuditDefinitions()...))
	if err != nil {
		t.Fatal(err)
	}
	fx := x.legalFixtureState
	assetStructure := &orgpg.ResourceStructure{Kind: orgpg.AssetStructure, Pool: x.f.Runtime, Allocations: x.assets}
	assignee := &orgpg.ResourceAssignee{Pool: x.f.Runtime, Assignments: x.assets}
	x.templates = &maintpg.ChecklistStore{Pool: x.f.Runtime, Authority: &orgpg.ChecklistAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags,
		Structure: assetStructure, Assignee: assignee}, Audit: auditpg.NewStore(x.f.Runtime, catalog)}
	x.runs = &maintpg.RunStore{Pool: x.f.Runtime, Authority: &orgpg.RunCompanyAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags,
		AssetStructure: assetStructure, Assignee: assignee, RunStructure: &orgpg.ResourceStructure{Kind: orgpg.ChecklistRunStructure, Pool: x.f.Runtime}}, Audit: auditpg.NewStore(x.f.Runtime, catalog)}
	meterAuthority, ok := x.meters.Authority.(*orgpg.MeterAuthority)
	if !ok {
		t.Fatal("fixture meter authority is not the real one")
	}
	if x.run, err = maintpg.NewRunHandler((*orgpg.RunAuthority)(meterAuthority), x.meters, x.assets, x.runs); err != nil {
		t.Fatal(err)
	}
	x.trips = syncpg.NewStore(runIntakeConfig(x.syncFixtureState))
	return x
}

func runFixture(t *testing.T) runState { return runFixtureOn(t, handlingFixture(t)) }

func (x runState) process(t *testing.T, d syncFixtureState, op protocol.Operation) (protocol.Acknowledgement, error) {
	t.Helper()
	if op.Snapshot().Type == maintapp.RunOperationType {
		return x.trips.Process(t.Context(), d.source, op, protocol.DefaultLimits(), x.run)
	}
	return x.incidentState.process(t, d, op)
}

// runItems: 1 lights (optional), 2 brakes (critical), 3 tyres.
func runItems() []checklist.Item {
	return []checklist.Item{{Label: "Luzes", Optional: true}, {Label: "Freios", Critical: true}, {Label: "Pneus"}}
}

// template creates an ACTIVE template of the context for the target with the
// three run items and returns its acknowledgement.
func (x runState) template(t *testing.T, context checklist.Context, target checklist.Target) maintapp.TemplateAck {
	t.Helper()
	ack, _, err := templateRun(t, x.legalFixtureState, x.templates, templateCommand(t, maintapp.TemplateCommandInput{Kind: maintapp.CommandCreate, Context: context, Name: "Checklist de saída sintético",
		Target: target, Items: runItems(), Policy: checklist.DefaultPolicy()}))
	if err != nil {
		t.Fatal("template fixture", err)
	}
	return ack
}

func (x runState) policy(t *testing.T, template maintapp.TemplateAck, revision int64, mode checklist.Mode) maintapp.TemplateAck {
	t.Helper()
	ack, _, err := templateRun(t, x.legalFixtureState, x.templates, templateCommand(t, maintapp.TemplateCommandInput{Kind: maintapp.CommandPolicy, TemplateID: template.TemplateID,
		ExpectedRevision: revision, Policy: checklist.Policy{Mode: mode, ValiditySeconds: 43200}}))
	if err != nil {
		t.Fatal("policy fixture", err)
	}
	return ack
}

func answer(a string, note any) map[string]any {
	return map[string]any{"answer": a, "note": note, "photo": false}
}

// runPayload is a completed run of the vehicle on the template's version.
func (x runState) runPayload(template string, version, policy int64, started time.Time, device string, answers ...map[string]any) map[string]any {
	list := make([]any, 0, len(answers))
	for _, a := range answers {
		list = append(list, a)
	}
	return map[string]any{"asset_id": x.vehicle.AssetID, "context": "DEPARTURE", "template_id": template, "template_version": version, "policy_revision": policy,
		"started_at": orgtime.FormatInstant(started.Truncate(time.Microsecond)), "answers": list, "device_outcome": device, "acknowledged": false}
}

// runOp is member d's check.run.complete of a new run completed at declared.
func (x runState) runOp(t *testing.T, d syncFixtureState, payload map[string]any, declared time.Time, edit func(envelope, payload map[string]any)) protocol.Operation {
	t.Helper()
	return syncHTTPOperation(t, d.deviceFixtureState, d.source.EnrollmentID, func(raw map[string]any) {
		raw["type"] = maintapp.RunOperationType
		raw["entity"] = map[string]any{"type": maintapp.RunEntityType, "id": onboardingID(t), "expected_revision": nil}
		raw["occurred_at"] = orgtime.FormatInstant(declared)
		raw["entry_zone"] = "America/Cuiaba"
		raw["device_offset_seconds"] = -14400
		raw["payload"] = payload
		if edit != nil {
			edit(raw, payload)
		}
	})
}

func runOf(op protocol.Operation) string { return op.Snapshot().Entity.ID }

type runCounts struct{ runs, answers, effects, claims, receipts int }

func (x runState) countRun(t *testing.T, op protocol.Operation) runCounts {
	t.Helper()
	var c runCounts
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_checklist_runs WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_checklist_run_answers a JOIN org_checklist_runs r ON(r.org_id,r.id)=(a.org_id,a.run_id) WHERE r.org_id=$1 AND r.operation_id=$2),
(SELECT count(*) FROM org_sync_effects WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_sync_operations WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2)`, x.scope.OrgID().String(), op.ID()).Scan(&c.runs, &c.answers, &c.effects, &c.claims, &c.receipts); err != nil {
		t.Fatal(err)
	}
	return c
}

func (x runState) runAccepted(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error, answers int) {
	t.Helper()
	if err != nil || ack.Result() != protocol.Accepted || ack.Snapshot().EntityRevision == nil || *ack.Snapshot().EntityRevision != 1 || ack.Code() != "" {
		t.Fatal("legitimate run was not accepted", err, ack.Result(), ack.Code())
	}
	if c := x.countRun(t, op); c != (runCounts{runs: 1, answers: answers, effects: 1, claims: 1, receipts: 1}) {
		t.Fatalf("accepted run not stored exactly once: %+v", c)
	}
}

func (x runState) runMalformed(t *testing.T, name string, op protocol.Operation, ack protocol.Acknowledgement, err error) {
	t.Helper()
	if err != nil || ack.Result() != protocol.Rejected || ack.Code() != "MALFORMED" {
		t.Fatal("expected a final MALFORMED receipt", name, err, ack.Result(), ack.Code())
	}
	if c := x.countRun(t, op); c != (runCounts{claims: 1, receipts: 1}) {
		t.Fatalf("malformed run stored a fact %s: %+v", name, c)
	}
}

func (x runState) runNothing(t *testing.T, op protocol.Operation) {
	t.Helper()
	if c := x.countRun(t, op); c != (runCounts{}) {
		t.Fatalf("refused run left state: %+v", c)
	}
}

// stored renders the recheck columns of a stored run.
func (x runState) stored(t *testing.T, run string) string {
	t.Helper()
	var v string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT device_outcome||':'||recheck_outcome||':'||outcome||':'||outcome_differs||':'||template_superseded||':'||policy_revision||':'||server_policy_revision||':'||revision
 FROM org_checklist_runs WHERE org_id=$1 AND id=$2`, x.scope.OrgID().String(), run).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// availability reads the vehicle's derived availability as the owner.
func (x runState) availability(t *testing.T) (fleet.Availability, []fleet.Cause) {
	t.Helper()
	v, err := x.assets.ReadAvailability(t.Context(), x.scope, fleetActor(x.legalFixtureState), x.member, x.vehicle.AssetID)
	if err != nil {
		t.Fatal(err)
	}
	return v.Assessment.Availability, v.Assessment.Causes
}

func (x runState) startDecision(t *testing.T, member string, at time.Time) fleet.StartDecision {
	t.Helper()
	var decision fleet.StartDecision
	if err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
		var err error
		decision, err = x.assets.StartDecisionDBTX(t.Context(), tx, x.scope, x.vehicle.AssetID, member, at)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return decision
}

// release never calls t.Fatal, so it is safe inside a goroutine.
func (x runState) release(h incidentHandler, run string, revision int64, reason, operation string) (maintapp.RunView, bool, error) {
	command, err := maintapp.NewReleaseCommand(maintapp.ReleaseInput{OperationID: operation, RunID: run, ExpectedRevision: revision, Reason: reason})
	if err != nil {
		return maintapp.RunView{}, false, err
	}
	return x.runs.ReleaseRun(context.Background(), x.scope, h.actor, h.member, command)
}

func (x runState) blockingRun(t *testing.T, template maintapp.TemplateAck) string {
	t.Helper()
	d := x.syncFixtureState
	at := x.declaredNow(t)
	op := x.runOp(t, d, x.runPayload(template.TemplateID, template.Version, template.PolicyRevision, at.Add(-10*time.Minute), "BLOCKING", answer("OK", nil), answer("NOT_OK", "Freios rangendo"), answer("OK", nil)), at, nil)
	ack, err := x.process(t, d, op)
	x.runAccepted(t, op, ack, err, 3)
	return runOf(op)
}

// BUC-EMP-222/223, B-BR-EMP-439..442 on real PostGIS through the real owner:
// the run is stored as declared with the server's recheck, replays byte for
// byte, refuses another digest and a second operation for its identity, and
// a BLOCKING run blocks the vehicle for every member's new start.
func TestChecklistRunIntakeRechecksStoresAndBlocks(t *testing.T) {
	x := runFixture(t)
	d := x.syncFixtureState
	template := x.template(t, checklist.Departure, organizationTarget())
	before := x.costSnapshot(t)
	at := x.declaredNow(t)
	op := x.runOp(t, d, x.runPayload(template.TemplateID, 1, 1, at.Add(-10*time.Minute), "BLOCKING", answer("OK", nil), answer("NOT_OK", "Freios rangendo"), answer("OK", nil)), at, nil)
	ack, err := x.process(t, d, op)
	x.runAccepted(t, op, ack, err, 3)
	org := x.scope.OrgID().String()
	var exact bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT r.asset_id=$3 AND r.unit_id=$4 AND r.cost_centre_id IS NULL AND r.context='DEPARTURE' AND r.template_id=$5 AND r.template_version=1
 AND r.policy_revision=1 AND r.server_policy_revision=1 AND r.device_outcome='BLOCKING' AND r.recheck_outcome='BLOCKING' AND r.outcome='BLOCKING' AND NOT r.outcome_differs
 AND NOT r.template_superseded AND NOT r.acknowledged AND r.author_membership_id=$6 AND r.author_employee_id=$7 AND r.author_account_id=$8 AND r.enrollment_id=$9
 AND r.payload_digest=$10 AND r.occurred_submicro=123 AND r.entry_zone='America/Cuiaba' AND r.device_offset_seconds=-14400 AND r.revision=1 AND r.updated_at=r.received_at
 AND r.started_at=$11 AND(SELECT string_agg(position||answer||COALESCE(note,'-'),',' ORDER BY position) FROM org_checklist_run_answers a WHERE a.org_id=r.org_id AND a.run_id=r.id)='1OK-,2NOT_OKFreios rangendo,3OK-'
 FROM org_checklist_runs r WHERE r.org_id=$1 AND r.id=$2`, org, runOf(op), x.vehicle.AssetID, x.unit, template.TemplateID, d.source.MembershipID, d.person.employee,
		d.source.Actor.AccountID, d.source.EnrollmentID, op.PayloadDigest(), at.Add(-10*time.Minute).Truncate(time.Microsecond)).Scan(&exact); err != nil || !exact {
		t.Fatal("stored run is not the exact declared fact with its recheck", err)
	}
	for range 2 {
		replay, err := x.process(t, d, op)
		if err != nil || !bytes.Equal(replay.CanonicalBytes(), ack.CanonicalBytes()) {
			t.Fatal("resend did not return the stored ACK", err)
		}
	}
	x.runAccepted(t, op, ack, nil, 3)
	reused := x.runOp(t, d, x.runPayload(template.TemplateID, 1, 1, at.Add(-10*time.Minute), "COMPLETED_OK", answer("OK", nil), answer("OK", nil), answer("OK", nil)), at, func(raw, _ map[string]any) {
		raw["operation_id"], raw["entity"] = op.ID(), map[string]any{"type": maintapp.RunEntityType, "id": runOf(op), "expected_revision": nil}
	})
	if refusal, err := x.process(t, d, reused); err != nil || refusal.Result() != protocol.Rejected || refusal.Code() != "OPERATION_ID_REUSED" {
		t.Fatal("mismatched digest under the same operation was not refused", err, refusal.Code())
	}
	second := x.runOp(t, d, x.runPayload(template.TemplateID, 1, 1, at.Add(-10*time.Minute), "COMPLETED_OK", answer("OK", nil), answer("OK", nil), answer("OK", nil)), at, func(raw, _ map[string]any) {
		raw["entity"] = map[string]any{"type": maintapp.RunEntityType, "id": runOf(op), "expected_revision": nil}
	})
	ack2, err := x.process(t, d, second)
	x.runMalformed(t, "second operation for a stored run", second, ack2, err)
	// BR-CHECK-010 (EDR-285): the vehicle is BLOCKED by the run for every
	// member from its declared completion; a new start is held.
	availability, causes := x.availability(t)
	if availability != fleet.Blocked || len(causes) != 1 || causes[0].Kind != fleet.ChecklistCause || causes[0].ID != runOf(op) || !causes[0].From.Equal(at.Truncate(time.Microsecond)) || causes[0].To != nil {
		t.Fatal("blocking run does not block the vehicle", availability, causes)
	}
	if decision := x.startDecision(t, d.source.MembershipID, x.declaredNow(t)); decision.Admitted || decision.Code != fleet.AssetUnavailable {
		t.Fatal("a new start on the blocked vehicle was admitted", decision)
	}
	// A critical block ignored by the device is stored BLOCKING (B-BR-EMP-440).
	ignored := x.runOp(t, d, x.runPayload(template.TemplateID, 1, 1, at, "COMPLETED_OK", answer("NOT_APPLICABLE", nil), answer("NOT_OK", nil), answer("OK", nil)), at.Add(time.Minute), nil)
	ack, err = x.process(t, d, ignored)
	x.runAccepted(t, ignored, ack, err, 3)
	if got := x.stored(t, runOf(ignored)); got != "COMPLETED_OK:BLOCKING:BLOCKING:true:false:1:1:1" {
		t.Fatal("ignored critical block", got)
	}
	ok := x.runOp(t, d, x.runPayload(template.TemplateID, 1, 1, at, "COMPLETED_OK", answer("OK", nil), answer("OK", nil), answer("OK", nil)), at.Add(2*time.Minute), nil)
	ack, err = x.process(t, d, ok)
	x.runAccepted(t, ok, ack, err, 3)
	if got := x.stored(t, runOf(ok)); got != "COMPLETED_OK:COMPLETED_OK:COMPLETED_OK:false:false:1:1:1" {
		t.Fatal("agreeing run", got)
	}
	if after := x.costSnapshot(t); !sameCounts(before, after) {
		t.Fatal("a run changed cost or document rows", before, after)
	}
}

// BR-CHECK-007/008 (B-BR-EMP-440/441): the server's policy history decides;
// a disagreement is stored with the field fact and held for review, and a
// superseded template is flagged, never refused.
func TestChecklistRunDisagreementKeepsTheFieldFact(t *testing.T) {
	x := runFixture(t)
	d := x.syncFixtureState
	template := x.template(t, checklist.Departure, organizationTarget())
	warn := x.policy(t, template, template.Revision, checklist.Warn)
	block := x.policy(t, warn, warn.Revision, checklist.Block)
	org := x.scope.OrgID().String()
	var warnAt, blockAt time.Time
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT effective_at FROM org_checklist_policy_revisions WHERE org_id=$1 AND template_id=$2 AND revision=2),
 (SELECT effective_at FROM org_checklist_policy_revisions WHERE org_id=$1 AND template_id=$2 AND revision=3)`, org, template.TemplateID).Scan(&warnAt, &blockAt); err != nil {
		t.Fatal(err)
	}
	now := x.declaredNow(t)
	// Device on the stale WARN revision after BLOCK was in force: BLOCKING.
	stale := x.runPayload(template.TemplateID, 1, 2, now.Add(-time.Minute), "COMPLETED_WITH_ISSUES", answer("OK", nil), answer("NOT_OK", nil), answer("OK", nil))
	stale["acknowledged"] = true
	op := x.runOp(t, d, stale, now, nil)
	ack, err := x.process(t, d, op)
	x.runAccepted(t, op, ack, err, 3)
	if got := x.stored(t, runOf(op)); got != "COMPLETED_WITH_ISSUES:BLOCKING:BLOCKING:true:false:2:3:1" {
		t.Fatal("stale WARN run", got)
	}
	if availability, _ := x.availability(t); availability != fleet.Blocked {
		t.Fatal("the server's BLOCKING outcome did not block the vehicle", availability)
	}
	// Device on a stale BLOCK revision while WARN was in force: the server
	// is less strict and still governs; nothing blocks.
	middle := warnAt.Add(blockAt.Sub(warnAt) / 2)
	lenient := x.runOp(t, d, x.runPayload(template.TemplateID, 1, 1, middle.Add(-time.Microsecond), "BLOCKING", answer("OK", nil), answer("NOT_OK", nil), answer("OK", nil)), middle, nil)
	ack, err = x.process(t, d, lenient)
	x.runAccepted(t, lenient, ack, err, 3)
	if got := x.stored(t, runOf(lenient)); got != "BLOCKING:COMPLETED_WITH_ISSUES:COMPLETED_WITH_ISSUES:true:false:1:2:1" {
		t.Fatal("stale BLOCK run", got)
	}
	// The author reads both outcomes (B-BR-EMP-441); no note is shown.
	author := reporterHandler(d)
	view, err := x.runs.ReadRun(t.Context(), x.scope, author.actor, author.member, x.vehicle.AssetID, runOf(op))
	if err != nil || view.DeviceOutcome != checklist.CompletedWithIssues || view.RecheckOutcome != checklist.Blocking || !view.Differs || view.ServerPolicyRevision != 3 || view.PolicyRevision != 2 ||
		len(view.Issues) != 1 || view.Issues[0] != 2 || len(view.Critical) != 1 || !view.Acknowledged {
		t.Fatal("author's read of the disagreement", view, err)
	}
	if _, err := x.runs.ReadRun(t.Context(), x.scope, author.actor, author.member, onboardingID(t), runOf(op)); !errors.Is(err, maintapp.ErrNotFound) && !errors.Is(err, maintapp.ErrUnavailable) {
		t.Fatal("a run read through another asset", err)
	}
	// A later version supersedes the named one (TEMPLATE_SUPERSEDED flag).
	published, _, err := templateRun(t, x.legalFixtureState, x.templates, templateCommand(t, maintapp.TemplateCommandInput{Kind: maintapp.CommandPublish, TemplateID: template.TemplateID,
		ExpectedRevision: block.Revision, Name: "Versão 2", Target: organizationTarget(), Items: runItems()}))
	if err != nil || published.Version != 2 {
		t.Fatal("publish fixture", err)
	}
	later := x.declaredNow(t)
	superseded := x.runOp(t, d, x.runPayload(template.TemplateID, 1, 3, later.Add(-time.Minute), "COMPLETED_OK", answer("OK", nil), answer("OK", nil), answer("OK", nil)), later, nil)
	ack, err = x.process(t, d, superseded)
	x.runAccepted(t, superseded, ack, err, 3)
	if got := x.stored(t, runOf(superseded)); got != "COMPLETED_OK:COMPLETED_OK:COMPLETED_OK:false:true:3:3:1" {
		t.Fatal("superseded run", got)
	}
}

// B-BR-EMP-439..441 refusals: nothing is stored or revealed.
func TestChecklistRunIntakeRefusals(t *testing.T) {
	x := runFixture(t)
	d := x.syncFixtureState
	template := x.template(t, checklist.Departure, organizationTarget())
	otherAsset, _ := checklistAsset(t, x.legalFixtureState, x.assets, x.unit, "Outra categoria sintética", fleet.Active)
	assetOnly := x.template(t, checklist.Departure, assetTarget(otherAsset))
	ret := x.template(t, checklist.Return, organizationTarget())
	at := x.declaredNow(t)
	valid := func() map[string]any {
		return x.runPayload(template.TemplateID, 1, 1, at.Add(-time.Minute), "COMPLETED_OK", answer("OK", nil), answer("OK", nil), answer("OK", nil))
	}
	refused := func(t *testing.T, op protocol.Operation, want orgapi.Code) {
		t.Helper()
		ack, err := x.process(t, d, op)
		if code, _ := orgapi.Registered(err); !errors.Is(err, syncapp.ErrSourceDenied) || code != want || len(ack.CanonicalBytes()) != 0 {
			t.Fatalf("expected %s refusal, got %v", want, err)
		}
		x.runNothing(t, op)
	}
	t.Run("foreign-asset-not-found", func(t *testing.T) {
		p := valid()
		p["asset_id"] = x.foreignAsset(t)
		refused(t, x.runOp(t, d, p, at, nil), orgapi.NotFound)
	})
	t.Run("unknown-asset-not-found", func(t *testing.T) {
		p := valid()
		p["asset_id"] = onboardingID(t)
		refused(t, x.runOp(t, d, p, at, nil), orgapi.NotFound)
	})
	t.Run("unassigned-self-driver-forbidden", func(t *testing.T) {
		driver := x.driver(t)
		op := x.runOp(t, driver, valid(), x.declaredNow(t), nil)
		ack, err := x.process(t, driver, op)
		if code, _ := orgapi.Registered(err); !errors.Is(err, syncapp.ErrSourceDenied) || code != orgapi.Forbidden || len(ack.CanonicalBytes()) != 0 {
			t.Fatal("unassigned driver", err)
		}
		x.runNothing(t, op)
	})
	t.Run("assigned-self-driver-accepted", func(t *testing.T) {
		driver := x.driver(t)
		x.assign(t, driver.person.employee, x.vehicle.AssetID, fleet.AdditionalAssignment, nil, nil)
		op := x.runOp(t, driver, valid(), x.declaredNow(t), nil)
		ack, err := x.process(t, driver, op)
		x.runAccepted(t, op, ack, err, 3)
	})
	t.Run("malformed", func(t *testing.T) {
		for name, edit := range map[string]func(raw, p map[string]any){
			"unknown-version":         func(_, p map[string]any) { p["template_version"] = 2 },
			"unknown-policy-revision": func(_, p map[string]any) { p["policy_revision"] = 2 },
			"unknown-template":        func(_, p map[string]any) { p["template_id"] = onboardingID(t) },
			"other-context":           func(_, p map[string]any) { p["context"] = "RETURN" },
			"return-template-as-departure": func(_, p map[string]any) {
				p["template_id"] = ret.TemplateID
			},
			"wrong-template-target": func(_, p map[string]any) { p["template_id"] = assetOnly.TemplateID },
			"fewer-answers":         func(_, p map[string]any) { p["answers"] = []any{answer("OK", nil), answer("OK", nil)} },
			"more-answers": func(_, p map[string]any) {
				p["answers"] = []any{answer("OK", nil), answer("OK", nil), answer("OK", nil), answer("OK", nil)}
			},
			"not-applicable-required": func(_, p map[string]any) {
				p["answers"] = []any{answer("OK", nil), answer("NOT_APPLICABLE", nil), answer("OK", nil)}
			},
			"started-after-completion": func(_, p map[string]any) {
				p["started_at"] = orgtime.FormatInstant(at.Add(time.Second).Truncate(time.Microsecond))
			},
			"acknowledged-ok": func(_, p map[string]any) { p["acknowledged"] = true },
			"health-key":      func(_, p map[string]any) { p["injuries"] = "none" },
			"blank-note": func(_, p map[string]any) {
				p["answers"] = []any{answer("NOT_OK", "   "), answer("OK", nil), answer("OK", nil)}
			},
			"dependency": func(raw, _ map[string]any) { raw["depends_on"] = []any{onboardingID(t)} },
		} {
			op := x.runOp(t, d, valid(), at, edit)
			ack, err := x.process(t, d, op)
			if name == "dependency" {
				// The framework answers an unknown dependency first (the
				// service turns it into RETRY), storing nothing.
				if !errors.Is(err, syncapp.ErrUnavailable) && (err != nil || ack.Result() != protocol.Retry) {
					t.Fatal("dependency", err, ack.Result())
				}
				x.runNothing(t, op)
				continue
			}
			x.runMalformed(t, name, op, ack, err)
		}
	})
	t.Run("photo-declaration-unavailable", func(t *testing.T) {
		p := valid()
		p["answers"] = []any{answer("OK", nil), map[string]any{"answer": "NOT_OK", "note": nil, "photo": true}, answer("OK", nil)}
		p["device_outcome"] = "BLOCKING"
		op := x.runOp(t, d, p, at, nil)
		// The owner keeps it unavailable; the service answers RETRY/UNAVAILABLE.
		if _, err := x.process(t, d, op); !errors.Is(err, syncapp.ErrUnavailable) {
			t.Fatal("a photo claim was not kept unavailable", err)
		}
		x.runNothing(t, op)
	})
	t.Run("draft-asset-malformed", func(t *testing.T) {
		draft, _ := checklistAsset(t, x.legalFixtureState, x.assets, x.unit, "Categoria rascunho", fleet.Draft)
		org := x.template(t, checklist.AdHoc, organizationTarget())
		p := x.runPayload(org.TemplateID, 1, 1, at.Add(-time.Minute), "COMPLETED_OK", answer("OK", nil), answer("OK", nil), answer("OK", nil))
		p["asset_id"], p["context"] = draft, "AD_HOC"
		op := x.runOp(t, d, p, at, nil)
		ack, err := x.process(t, d, op)
		if err == nil && ack.Result() == protocol.Rejected {
			x.runMalformed(t, "draft", op, ack, err)
			return
		}
		if code, _ := orgapi.Registered(err); !errors.Is(err, syncapp.ErrSourceDenied) || code != orgapi.Forbidden && code != orgapi.NotFound {
			t.Fatal("draft asset run", err, ack.Result())
		}
		x.runNothing(t, op)
	})
}

// BUC-EMP-224 / B-BR-EMP-442 on real PostGIS through the real company owner.
func TestChecklistRunReleaseSeparationAuditAndAuthority(t *testing.T) {
	x := runFixture(t)
	d := x.syncFixtureState
	template := x.template(t, checklist.Departure, organizationTarget())
	run := x.blockingRun(t, template)
	org := x.scope.OrgID().String()
	second := onboardingID(t)
	if err := unitSchemaExec(t, x.legalFixtureState, `INSERT INTO org_units(org_id,id,name,code,time_zone,state) VALUES($1,$2,'Synthetic second unit','RUN2','UTC','ACTIVE')`, org, second); err != nil {
		t.Fatal(err)
	}
	author := reporterHandler(d)
	manager := x.handlerFor(t, "FLEET_MANAGER", "UNIT", x.unit)
	outOfScope := x.handlerFor(t, "SUPERVISOR", "UNIT", second)
	auditor := x.handlerFor(t, "AUDITOR", "ORG", "")
	// BR-CHECK-011: the author never releases the own run.
	if _, _, err := x.release(author, run, 1, "Liberado por mim", onboardingID(t)); !errors.Is(err, maintapp.ErrSelfApproval) {
		t.Fatal("self release", err)
	}
	for name, h := range map[string]incidentHandler{"out-of-scope": outOfScope, "auditor": auditor} {
		if _, _, err := x.release(h, run, 1, "Liberação indevida", onboardingID(t)); err == nil || errors.Is(err, maintapp.ErrSelfApproval) {
			t.Fatal("release without coverage", name, err)
		}
	}
	if _, err := maintapp.NewReleaseCommand(maintapp.ReleaseInput{OperationID: onboardingID(t), RunID: run, ExpectedRevision: 1, Reason: "  "}); !errors.Is(err, checklist.ErrInvalid) {
		t.Fatal("blank reason", err)
	}
	if _, _, err := x.release(manager, run, 2, "Revisão errada", onboardingID(t)); !errors.Is(err, maintapp.ErrConflict) {
		t.Fatal("stale revision", err)
	}
	if _, _, err := x.release(manager, onboardingID(t), 1, "Inexistente", onboardingID(t)); !errors.Is(err, maintapp.ErrNotFound) {
		t.Fatal("unknown run", err)
	}
	operation := onboardingID(t)
	view, replayed, err := x.release(manager, run, 1, "Freios verificados na oficina", operation)
	if err != nil || replayed || view.Revision != 2 || view.Outcome != checklist.CompletedWithIssues || view.RecheckOutcome != checklist.Blocking || view.ReleasedAt == nil {
		t.Fatal("release", view, err)
	}
	again, replayed, err := x.release(manager, run, 1, "Freios verificados na oficina", operation)
	if err != nil || !replayed || again.Revision != 2 || !again.ReleasedAt.Equal(*view.ReleasedAt) {
		t.Fatal("release replay", again, err)
	}
	if _, _, err := x.release(manager, run, 1, "Outro motivo", operation); !errors.Is(err, maintapp.ErrOperationReused) {
		t.Fatal("reused operation", err)
	}
	if _, _, err := x.release(manager, run, 2, "Segunda liberação", onboardingID(t)); !errors.Is(err, maintapp.ErrConflict) {
		t.Fatal("second release", err)
	}
	var audits, releases int
	var answers string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND target_type='checklist_run' AND target_id=$2 AND action='check.run.released'),
 (SELECT count(*) FROM org_checklist_run_releases WHERE org_id=$1 AND run_id=$2),(SELECT string_agg(position||answer,',' ORDER BY position) FROM org_checklist_run_answers WHERE org_id=$1 AND run_id=$2)`, org, run).Scan(&audits, &releases, &answers); err != nil ||
		audits != 1 || releases != 1 || answers != "1OK,2NOT_OK,3OK" {
		t.Fatal("one audited release with unchanged answers", audits, releases, answers, err)
	}
	if got := x.stored(t, run); got != "BLOCKING:BLOCKING:COMPLETED_WITH_ISSUES:false:false:1:1:2" {
		t.Fatal("released run", got)
	}
	if availability, causes := x.availability(t); availability != fleet.Available || len(causes) != 0 {
		t.Fatal("released vehicle is still blocked", availability, causes)
	}
	// A run that is not BLOCKING is CONFLICT; another organization's run is hidden.
	ok := x.runOp(t, d, x.runPayload(template.TemplateID, 1, 1, x.declaredNow(t).Add(-time.Minute), "COMPLETED_OK", answer("OK", nil), answer("OK", nil), answer("OK", nil)), x.declaredNow(t), nil)
	ack, err := x.process(t, d, ok)
	x.runAccepted(t, ok, ack, err, 3)
	if _, _, err := x.release(manager, runOf(ok), 1, "Nada a liberar", onboardingID(t)); !errors.Is(err, maintapp.ErrConflict) {
		t.Fatal("non-blocking release", err)
	}
	// The AUDITOR reads the run on its asset (fleet.asset.read) and cannot release.
	if read, err := x.runs.ReadRun(t.Context(), x.scope, auditor.actor, auditor.member, x.vehicle.AssetID, run); err != nil || read.Revision != 2 {
		t.Fatal("auditor run read", read, err)
	}
	if _, err := x.runs.ReadRun(t.Context(), x.scope, outOfScope.actor, outOfScope.member, x.vehicle.AssetID, run); err == nil {
		t.Fatal("a supervisor of another unit read the run")
	}
	// The database refuses a self release that bypasses the owner.
	other := x.blockingRun(t, template)
	if err := unitSchemaExec(t, x.legalFixtureState, `INSERT INTO org_checklist_run_releases(org_id,run_id,revision,reason,actor_account_id,actor_membership_id,actor_employee_id,operation_id,request_hash)
 VALUES($1,$2,2,'Auto liberação',$3,$4,$5,$6,repeat('a',64))`, org, other, author.account, author.member, author.employee, onboardingID(t)); !strings.Contains(errText(err), "23514") {
		t.Fatal("database self release", err)
	}
}

func errText(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code + " " + pe.Message
	}
	if err == nil {
		return ""
	}
	return err.Error()
}

// V-IDEM: two releases of one run queued behind a held root lock; the first
// wins, the other is CONFLICT, in both orders, with one row and one audit.
func TestChecklistRunReleaseRaceLeavesOneReleaseInBothOrders(t *testing.T) {
	x := runFixture(t)
	template := x.template(t, checklist.Departure, organizationTarget())
	a, b := x.handlerFor(t, "OWNER", "ORG", ""), x.handlerFor(t, "FLEET_MANAGER", "ORG", "")
	for name, order := range map[string][2]incidentHandler{"owner-first": {a, b}, "manager-first": {b, a}} {
		t.Run(name, func(t *testing.T) {
			run := x.blockingRun(t, template)
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
			attempt := func(h incidentHandler) {
				wg.Go(func() {
					_, _, err := x.release(h, run, 1, "Liberado na corrida", onboardingID(t))
					results <- err
				})
			}
			attempt(order[0])
			waitBlocked(t, x.syncFixtureState, pid, 1)
			attempt(order[1])
			waitBlocked(t, x.syncFixtureState, pid, 2)
			free()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			wg.Wait()
			close(results)
			won, lost := 0, 0
			for err := range results {
				switch {
				case err == nil:
					won++
				case errors.Is(err, maintapp.ErrConflict):
					lost++
				default:
					t.Fatal("unexpected race result", err)
				}
			}
			var rows, audits int
			if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_checklist_run_releases WHERE org_id=$1 AND run_id=$2),(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND target_id=$2 AND action='check.run.released')`,
				x.scope.OrgID().String(), run).Scan(&rows, &audits); err != nil || won != 1 || lost != 1 || rows != 1 || audits != 1 {
				t.Fatal("race did not end in one release", won, lost, rows, audits, err)
			}
		})
	}
}

// V-IDEM / V-SYNC: twelve concurrent identical deliveries store one run and
// one ACK.
func TestChecklistRunConcurrentDuplicateDeliveries(t *testing.T) {
	x := runFixture(t)
	d := x.syncFixtureState
	template := x.template(t, checklist.Departure, organizationTarget())
	at := x.declaredNow(t)
	op := x.runOp(t, d, x.runPayload(template.TemplateID, 1, 1, at.Add(-time.Minute), "BLOCKING", answer("OK", nil), answer("NOT_OK", nil), answer("OK", nil)), at, nil)
	var wg sync.WaitGroup
	start := make(chan struct{})
	acks := make([]protocol.Acknowledgement, 12)
	errs := make([]error, 12)
	for i := range acks {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			acks[i], errs[i] = x.trips.Process(t.Context(), d.source, op, protocol.DefaultLimits(), x.run)
		}(i)
	}
	close(start)
	wg.Wait()
	var first []byte
	for i := range acks {
		if errs[i] != nil {
			continue
		}
		if first == nil {
			first = acks[i].CanonicalBytes()
		} else if !bytes.Equal(first, acks[i].CanonicalBytes()) {
			t.Fatal("concurrent duplicates returned different ACKs")
		}
	}
	if first == nil {
		t.Fatal("no concurrent delivery succeeded", errs)
	}
	ack, err := x.process(t, d, op)
	x.runAccepted(t, op, ack, err, 3)
}

// Forced RLS, insert-only answers and releases, a guarded run row and the
// database recheck even when the owner is bypassed (B-BR-EMP-442).
func TestChecklistRunSchemaIsForcedRLSAndGuarded(t *testing.T) {
	x := runFixture(t)
	template := x.template(t, checklist.Departure, organizationTarget())
	run := x.blockingRun(t, template)
	org := x.scope.OrgID().String()
	var forced bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*)=3 AND bool_and(relrowsecurity AND relforcerowsecurity) FROM pg_class WHERE oid IN('org_checklist_runs'::regclass,'org_checklist_run_answers'::regclass,'org_checklist_run_releases'::regclass)`).Scan(&forced); err != nil || !forced {
		t.Fatal("forced RLS", err)
	}
	for name, stmt := range map[string]string{
		"answer update":   `UPDATE org_checklist_run_answers SET answer='OK' WHERE org_id=$1 AND run_id=$2`,
		"answer delete":   `DELETE FROM org_checklist_run_answers WHERE org_id=$1 AND run_id=$2`,
		"outcome rewrite": `UPDATE org_checklist_runs SET outcome='COMPLETED_OK' WHERE org_id=$1 AND id=$2`,
		"recheck rewrite": `UPDATE org_checklist_runs SET recheck_outcome='COMPLETED_OK',outcome='COMPLETED_OK' WHERE org_id=$1 AND id=$2`,
		"release w/o row": `UPDATE org_checklist_runs SET outcome='COMPLETED_WITH_ISSUES',revision=2 WHERE org_id=$1 AND id=$2`,
		"run delete":      `DELETE FROM org_checklist_runs WHERE org_id=$1 AND id=$2`,
		"late answer":     `INSERT INTO org_checklist_run_answers(org_id,run_id,template_id,template_version,position,answer) SELECT org_id,id,template_id,template_version,3,'OK' FROM org_checklist_runs WHERE org_id=$1 AND id=$2 AND false`,
	} {
		err := unitSchemaExec(t, x.legalFixtureState, stmt, org, run)
		if name == "late answer" {
			continue // no row selected; the next statement proves the guard
		}
		if err == nil || !strings.Contains(errText(err), "55000") && !strings.Contains(errText(err), "42501") {
			t.Fatal("guard did not refuse", name, err)
		}
	}
	for _, stmt := range []string{`TRUNCATE org_checklist_run_releases`, `TRUNCATE org_checklist_run_answers`, `TRUNCATE org_checklist_runs`} {
		if err := unitSchemaExec(t, x.legalFixtureState, stmt); err == nil {
			t.Fatal("truncate was not refused", stmt)
		}
	}
	if err := unitSchemaExec(t, x.legalFixtureState, `INSERT INTO org_checklist_run_answers(org_id,run_id,template_id,template_version,position,answer) SELECT org_id,id,template_id,template_version,1,'OK' FROM org_checklist_runs WHERE org_id=$1 AND id=$2`, org, run); err == nil {
		t.Fatal("an answer was added after the receipt")
	}
	// Another organization scope sees nothing.
	var visible int
	if err := pgx.BeginFunc(t.Context(), x.f.Runtime, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, onboardingID(t)); err != nil {
			return err
		}
		return tx.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_checklist_runs)+(SELECT count(*) FROM org_checklist_run_answers)+(SELECT count(*) FROM org_checklist_run_releases)`).Scan(&visible)
	}); err != nil || visible != 0 {
		t.Fatal("another scope sees runs", visible, err)
	}
	// The usage sources answer with the run keys as associations.
	var usage bool
	if err := pgx.BeginFunc(t.Context(), x.f.Runtime, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
			return err
		}
		return tx.QueryRow(t.Context(), `SELECT org_asset_current_usage($1,$2) IS NOT NULL AND org_unit_current_usage($1,$3) IS NOT NULL`, org, x.vehicle.AssetID, x.unit).Scan(&usage)
	}); err != nil || !usage {
		t.Fatal("usage sources with run keys", err)
	}
}
