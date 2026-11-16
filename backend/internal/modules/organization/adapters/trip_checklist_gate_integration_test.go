//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"net/url"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	maintpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/adapters"
	maintapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/checklist"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

// checklistGateImage reports whether the schema has the EMP-TRIP-019 links
// (migration 000255); an upgrade rehearsal may stop earlier.
func checklistGateImage(t *testing.T, fx legalFixtureState) bool {
	t.Helper()
	var installed bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_trip_checklist_links') IS NOT NULL`).Scan(&installed); err != nil {
		t.Fatal(err)
	}
	return installed
}

// checklistGateSource is the real EMP-TRIP-019 gate with the runtime grants
// it needs (README proposal in the evidence): the trip owner reads the
// CHECK-001 templates and CHECK-002 runs and writes links and gate reasons.
// A rehearsal image before 000255 gets the flagged gate, which requires
// nothing and finds no run.
func checklistGateSource(t *testing.T, fx legalFixtureState) tripapp.ChecklistGate {
	t.Helper()
	if !checklistGateImage(t, fx) {
		return maintpg.TripChecklistGate{PreGateImage: true}
	}
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_trip_checklist_links,org_trip_checklist_holds TO "+role+
		";GRANT SELECT ON org_checklist_runs,org_checklist_run_releases,org_checklist_templates,org_checklist_template_versions,org_checklist_template_targets,org_checklist_policy_revisions,org_assets TO "+role); err != nil {
		t.Fatal(err)
	}
	return maintpg.TripChecklistGate{}
}

// gateState is the EMP-CHECK-002 run fixture (SELF member with an assigned,
// allocated vehicle, the real template store, run owner and company
// handlers) whose trip owners carry the real EMP-TRIP-019 gate.
type gateState struct{ runState }

func gateFixture(t *testing.T) gateState { return gateState{runFixture(t)} }

func runReference(run string) []any {
	return []any{map[string]any{"type": tripapp.ChecklistRunReferenceType, "id": run}}
}

// namedStart is member d's start of a new trip on the vehicle naming the run
// ("" none) as its departure run.
func (x gateState) namedStart(t *testing.T, d syncFixtureState, run string, declared time.Time) protocol.Operation {
	t.Helper()
	return x.startOp(t, d, declared, func(e, _ map[string]any) {
		if run != "" {
			e["soft_references"] = runReference(run)
		}
	})
}

// completedRun stores member d's accepted run of the context on the asset,
// completed at declared; blocking answers the critical item NOT_OK.
func (x gateState) completedRun(t *testing.T, d syncFixtureState, template maintapp.TemplateAck, context, asset string, declared time.Time, blocking bool) string {
	t.Helper()
	device, brakes := "COMPLETED_OK", answer("OK", nil)
	if blocking {
		device, brakes = "BLOCKING", answer("NOT_OK", "Freios rangendo")
	}
	payload := x.runPayload(template.TemplateID, template.Version, template.PolicyRevision, declared.Add(-5*time.Minute), device, answer("OK", nil), brakes, answer("OK", nil))
	payload["context"], payload["asset_id"] = context, asset
	op := x.runOp(t, d, payload, declared, nil)
	ack, err := x.process(t, d, op)
	x.runAccepted(t, op, ack, err, 3)
	return runOf(op)
}

// link answers the run linked to the trip for the context ("" none).
func (x gateState) link(t *testing.T, trip, context string) string {
	t.Helper()
	var run string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT COALESCE((SELECT run_id::text FROM org_trip_checklist_links WHERE org_id=$1 AND trip_id=$2 AND context=$3),'')`,
		x.scope.OrgID().String(), trip, context).Scan(&run); err != nil {
		t.Fatal(err)
	}
	return run
}

// reason answers the stored gate reason of a held start and its run.
func (x gateState) reason(t *testing.T, op protocol.Operation) (string, string) {
	t.Helper()
	var reason, run string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT COALESCE((SELECT reason||'|'||COALESCE(run_id::text,'')||'|'||(review_item_id=(SELECT review_item_id FROM org_trip_held_starts h WHERE h.org_id=c.org_id AND h.operation_id=c.operation_id))::text
 FROM org_trip_checklist_holds c WHERE org_id=$1 AND operation_id=$2),'||')`, x.scope.OrgID().String(), op.ID()).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(reason, "|")
	if len(parts) != 3 || parts[0] != "" && parts[2] != "true" {
		t.Fatal("gate reason row", reason)
	}
	reason, run = parts[0], parts[1]
	return reason, run
}

func (x gateState) countLinks(t *testing.T) int {
	t.Helper()
	var n int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_checklist_links WHERE org_id=$1`, x.scope.OrgID().String()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// heldWithReason proves a held start, its gate reason and that nothing linked.
func (x gateState) heldWithReason(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error, code, reason, run string) {
	t.Helper()
	x.tripState.held(t, op, ack, err, code)
	if gotReason, gotRun := x.reason(t, op); gotReason != reason || gotRun != run {
		t.Fatal("gate reason", gotReason, gotRun, "want", reason, run)
	}
	if x.link(t, tripOf(op), tripapp.ChecklistDeparture) != "" {
		t.Fatal("a held start linked a run")
	}
	for range 2 {
		again, err := x.process(t, x.syncFixtureState, op)
		if err != nil || !bytes.Equal(again.CanonicalBytes(), ack.CanonicalBytes()) {
			t.Fatal("held start replay", err)
		}
	}
}

// finishTrip finishes an accepted trip of member d, optionally naming a run.
func (x gateState) finishTrip(t *testing.T, d syncFixtureState, trip, run string, declared time.Time) (protocol.Operation, protocol.Acknowledgement) {
	t.Helper()
	op := x.finishOp(t, d, trip, declared, func(e, _ map[string]any) {
		if run != "" {
			e["soft_references"] = runReference(run)
		}
	})
	ack, err := x.process(t, d, op)
	x.tripState.accepted(t, op, ack, err, 2, 1)
	return op, ack
}

// otherVehicle is a second ACTIVE vehicle of the unit assigned to employee.
func (x gateState) otherVehicle(t *testing.T, employee string) string {
	t.Helper()
	input := fleetCreateInput()
	id := onboardingID(t)
	input.Profile.Kind, input.Profile.UsageBasis, input.UnitID, input.Profile.SerialNumber = fleet.Vehicle, fleet.Distance, x.unit, "SYNTHETIC-G-"+id[:8]
	tank, err := fleet.NewTank(fuel.Catalog{}, fleet.Main, "100000", []string{"DIESEL_S10"})
	if err != nil {
		t.Fatal(err)
	}
	input.Profile.Tanks = []fleet.Tank{tank}
	created, _, err := fleetWrite(t, x.legalFixtureState, x.assets, fleetCommand(t, input))
	if err != nil {
		t.Fatal(err)
	}
	installed, _, err := meterInstall(t, x.legalFixtureState, x.meters, meterInstallCommand(t, meterInstallInput(t, created.AssetID, lifecycleDBNow(t, x.legalFixtureState).Add(-6*time.Hour))))
	if err != nil {
		t.Fatal(err)
	}
	if active, _, err := fleetWrite(t, x.legalFixtureState, x.assets, fleetCommand(t, fa.AssetWriteInput{Kind: fa.ActivateAsset, AssetID: created.AssetID, ExpectedRevision: installed.AssetRevision})); err != nil || active.State != fleet.Active {
		t.Fatal("second vehicle not active", err)
	}
	x.assign(t, employee, created.AssetID, fleet.PrimaryAssignment, nil, nil)
	return created.AssetID
}

// foreignRun plants a completed DEPARTURE run of another organization in this
// database (triggers and keys off for the planted row only); forced RLS must
// hide it.
func (x gateState) foreignRun(t *testing.T) string {
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
		_, err := tx.Exec(t.Context(), `INSERT INTO org_checklist_runs(org_id,id,operation_id,payload_digest,asset_id,context,template_id,template_version,policy_revision,server_policy_revision,
 started_at,occurred_at,occurred_submicro,entry_zone,device_offset_seconds,device_outcome,recheck_outcome,outcome,outcome_differs,template_superseded,acknowledged,
 author_account_id,author_membership_id,author_employee_id,enrollment_id,revision)
VALUES($1,$2,$3,'sha256:'||repeat('0',64),$4,'DEPARTURE',$5,1,1,1,date_trunc('milliseconds',clock_timestamp())-interval '10 minutes',clock_timestamp()-interval '5 minutes',0,'America/Cuiaba',-14400,
 'COMPLETED_OK','COMPLETED_OK','COMPLETED_OK',false,false,false,$6,$7,$6,$6,1)`,
			other.OrgID.String(), id, onboardingID(t), x.vehicle.AssetID, onboardingID(t), onboardingID(t), other.MembershipID)
		return err
	}); err != nil {
		t.Fatal("foreign run fixture", err)
	}
	return id
}

// B-BR-EMP-512..514, BUC-EMP-261/262: nothing required passes as before and
// an unknown named run is an orphan; once a departure template applies, a
// valid run of the author links its start once (replay byte for byte, another
// digest OPERATION_ID_REUSED, concurrent duplicates one link), and a second
// start naming it, or a start naming nothing, is held with its reason.
func TestTripChecklistGateLinksAValidDepartureRunOnce(t *testing.T) {
	x := gateFixture(t)
	d := x.syncFixtureState
	plain := x.namedStart(t, d, "", x.declaredNow(t))
	ack, err := x.process(t, d, plain)
	x.tripState.accepted(t, plain, ack, err, 1, 1)
	if r, _ := x.reason(t, plain); r != "" || x.link(t, tripOf(plain), tripapp.ChecklistDeparture) != "" {
		t.Fatal("an ungated start stored a gate fact")
	}
	x.finishTrip(t, d, tripOf(plain), "", x.declaredNow(t))
	unknown := x.namedStart(t, d, onboardingID(t), x.declaredNow(t))
	ack, err = x.process(t, d, unknown)
	x.heldWithReason(t, unknown, ack, err, tripapp.OrphanReference, tripapp.OrphanReference, "")

	template := x.template(t, checklist.Departure, organizationTarget())
	run := x.completedRun(t, d, template, "DEPARTURE", x.vehicle.AssetID, x.declaredNow(t), false)
	start := x.namedStart(t, d, run, x.declaredNow(t))
	// V-IDEM: concurrent identical deliveries store one trip and one link.
	acks := make(chan protocol.Acknowledgement, 6)
	errs := make(chan error, 6)
	var wg sync.WaitGroup
	for range 6 {
		wg.Go(func() {
			a, err := x.trips.Process(context.Background(), d.source, start, protocol.DefaultLimits(), x.start)
			acks <- a
			errs <- err
		})
	}
	wg.Wait()
	close(acks)
	close(errs)
	var first []byte
	for a := range acks {
		if err := <-errs; err != nil {
			t.Fatal("concurrent duplicate delivery", err)
		}
		if first == nil {
			first = a.CanonicalBytes()
		} else if !bytes.Equal(first, a.CanonicalBytes()) {
			t.Fatal("concurrent duplicates disagree")
		}
	}
	ack, err = x.process(t, d, start)
	x.tripState.accepted(t, start, ack, err, 1, 1)
	if !bytes.Equal(first, ack.CanonicalBytes()) || x.link(t, tripOf(start), tripapp.ChecklistDeparture) != run || x.countLinks(t) != 1 {
		t.Fatal("the valid departure run was not linked once")
	}
	if r, _ := x.reason(t, start); r != "" {
		t.Fatal("an accepted start stored a gate reason", r)
	}
	other := x.completedRun(t, d, template, "DEPARTURE", x.vehicle.AssetID, x.declaredNow(t), false)
	reused := x.startOp(t, d, x.declaredNow(t), func(e, _ map[string]any) {
		e["operation_id"], e["entity"], e["soft_references"] = start.ID(), map[string]any{"type": tripapp.EntityType, "id": tripOf(start), "expected_revision": nil}, runReference(other)
	})
	if refusal, err := x.process(t, d, reused); err != nil || refusal.Result() != protocol.Rejected || refusal.Code() != "OPERATION_ID_REUSED" {
		t.Fatal("another run under the same operation was not refused", err, refusal.Code())
	}
	if x.countLinks(t) != 1 || x.link(t, tripOf(start), tripapp.ChecklistDeparture) != run {
		t.Fatal("a reused operation changed the link")
	}
	x.finishTrip(t, d, tripOf(start), "", x.declaredNow(t))
	again := x.namedStart(t, d, run, x.declaredNow(t))
	ack, err = x.process(t, d, again)
	x.heldWithReason(t, again, ack, err, tripapp.OrphanReference, tripapp.ChecklistRequired, run)
	none := x.namedStart(t, d, "", x.declaredNow(t))
	ack, err = x.process(t, d, none)
	x.heldWithReason(t, none, ack, err, tripapp.AssetUnavailable, tripapp.ChecklistRequired, "")
	if x.countLinks(t) != 1 {
		t.Fatal("held starts stored links")
	}
}

// B-BR-EMP-512, BR-OFFL-007: a run that does not count — of another
// organization, another vehicle, the return context or another member — is
// an orphan reference with the requirement unmet; unsupported reference
// forms store nothing.
func TestTripChecklistGateHoldsRunsThatDoNotCount(t *testing.T) {
	x := gateFixture(t)
	d := x.syncFixtureState
	departure := x.template(t, checklist.Departure, organizationTarget())
	ret := x.template(t, checklist.Return, organizationTarget())
	second := x.driver(t)
	x.assign(t, second.person.employee, x.vehicle.AssetID, fleet.AdditionalAssignment, nil, nil)
	otherAsset := x.otherVehicle(t, d.person.employee)
	foreign := x.foreignRun(t)
	cases := map[string]struct{ run, stored string }{}
	cases["foreign-run"] = struct{ run, stored string }{foreign, ""}
	r := x.completedRun(t, d, departure, "DEPARTURE", otherAsset, x.declaredNow(t), false)
	cases["another-asset"] = struct{ run, stored string }{r, r}
	r = x.completedRun(t, d, ret, "RETURN", x.vehicle.AssetID, x.declaredNow(t), false)
	cases["return-context"] = struct{ run, stored string }{r, r}
	r = x.completedRun(t, second, departure, "DEPARTURE", x.vehicle.AssetID, x.declaredNow(t), false)
	cases["another-member"] = struct{ run, stored string }{r, r}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			op := x.namedStart(t, d, c.run, x.declaredNow(t))
			ack, err := x.process(t, d, op)
			x.heldWithReason(t, op, ack, err, tripapp.OrphanReference, tripapp.ChecklistRequired, c.stored)
		})
	}
	// The other member's own run counts for that member's start.
	own := x.namedStart(t, second, cases["another-member"].run, x.declaredNow(t))
	ack, err := x.process(t, second, own)
	x.tripState.accepted(t, own, ack, err, 1, 1)
	if x.link(t, tripOf(own), tripapp.ChecklistDeparture) != cases["another-member"].run || x.countLinks(t) != 1 {
		t.Fatal("a member's own run was not linked")
	}
	for name, refs := range map[string][]any{
		"another-type": {map[string]any{"type": "fueling", "id": foreign}},
		"two-runs":     {map[string]any{"type": tripapp.ChecklistRunReferenceType, "id": foreign}, map[string]any{"type": tripapp.ChecklistRunReferenceType, "id": cases["another-asset"].run}},
	} {
		t.Run("unsupported-"+name, func(t *testing.T) {
			op := x.startOp(t, d, x.declaredNow(t), func(e, _ map[string]any) { e["soft_references"] = refs })
			ack, err := x.process(t, d, op)
			if !errors.Is(err, syncapp.ErrUnavailable) || errors.Is(err, syncapp.ErrSourceDenied) || len(ack.CanonicalBytes()) != 0 {
				t.Fatal("unsupported reference form not kept RETRY/UNAVAILABLE", err, ack.Result())
			}
			if c := x.tripState.count(t, op); c != (tripCounts{}) {
				t.Fatalf("unsupported reference stored state: %+v", c)
			}
		})
	}
}

// B-BR-EMP-512/513, BR-CHECK-005/007/012: a BLOCKING departure run holds the
// start (the availability code first, CHECKLIST_BLOCKING stored); once
// released, a later start passes and links it; the server's current policy
// decides the validity whatever the device cached (CHECKLIST_EXPIRED), and a
// run completed after the declared start is outside its validity.
func TestTripChecklistGateBlockingExpiredAndTheServerPolicy(t *testing.T) {
	x := gateFixture(t)
	d := x.syncFixtureState
	template := x.template(t, checklist.Departure, organizationTarget())
	blocking := x.completedRun(t, d, template, "DEPARTURE", x.vehicle.AssetID, x.declaredNow(t), true)
	op := x.namedStart(t, d, blocking, x.declaredNow(t))
	ack, err := x.process(t, d, op)
	x.heldWithReason(t, op, ack, err, tripapp.AssetUnavailable, tripapp.ChecklistBlocking, blocking)
	if _, _, err := x.release(x.handlerFor(t, "FLEET_MANAGER", "ORG", ""), blocking, 1, "Freios revisados no pátio", onboardingID(t)); err != nil {
		t.Fatal(err)
	}
	released := x.namedStart(t, d, blocking, x.declaredNow(t))
	ack, err = x.process(t, d, released)
	x.tripState.accepted(t, released, ack, err, 1, 1)
	if x.link(t, tripOf(released), tripapp.ChecklistDeparture) != blocking {
		t.Fatal("a released run declared before the start was not linked")
	}
	x.finishTrip(t, d, tripOf(released), "", x.declaredNow(t))
	// The device cached revision 1 (12 hours); the server's revision 2 says
	// one hour and decides.
	completed := x.declaredNow(t)
	cached := x.completedRun(t, d, template, "DEPARTURE", x.vehicle.AssetID, completed, false)
	if _, _, err := templateRun(t, x.legalFixtureState, x.templates, templateCommand(t, maintapp.TemplateCommandInput{Kind: maintapp.CommandPolicy, TemplateID: template.TemplateID,
		ExpectedRevision: template.Revision, Policy: checklist.Policy{Mode: checklist.Block, ValiditySeconds: 3600}})); err != nil {
		t.Fatal(err)
	}
	late := x.namedStart(t, d, cached, completed.Add(time.Hour))
	ack, err = x.process(t, d, late)
	x.heldWithReason(t, late, ack, err, tripapp.AssetUnavailable, tripapp.ChecklistExpired, cached)
	early := x.namedStart(t, d, cached, completed.Add(-time.Nanosecond))
	ack, err = x.process(t, d, early)
	x.heldWithReason(t, early, ack, err, tripapp.AssetUnavailable, tripapp.ChecklistExpired, cached)
	inside := x.namedStart(t, d, cached, completed.Add(time.Hour-time.Microsecond))
	ack, err = x.process(t, d, inside)
	x.tripState.accepted(t, inside, ack, err, 1, 1)
	if x.link(t, tripOf(inside), tripapp.ChecklistDeparture) != cached || x.countLinks(t) != 2 {
		t.Fatal("a run valid by the server's policy was not linked")
	}
}

// B-BR-EMP-516: a release and a start naming the BLOCKING run, queued behind
// a held organization root in both orders, never deadlock and are decided by
// history: the start declared before the release's recording is held
// CHECKLIST_BLOCKING with nothing linked, the release is stored once, and a
// start declared after it passes.
func TestTripChecklistGateReleaseRacesStartInBothOrders(t *testing.T) {
	x := gateFixture(t)
	d := x.syncFixtureState
	template := x.template(t, checklist.Departure, organizationTarget())
	manager := x.handlerFor(t, "FLEET_MANAGER", "ORG", "")
	for _, startFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "start-first", false: "release-first"}[startFirst], func(t *testing.T) {
			run := x.completedRun(t, d, template, "DEPARTURE", x.vehicle.AssetID, x.declaredNow(t), true)
			op := x.namedStart(t, d, run, x.declaredNow(t))
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
			var wg sync.WaitGroup
			var startAck protocol.Acknowledgement
			var startErr, releaseErr error
			starting := func() {
				wg.Go(func() {
					startAck, startErr = x.trips.Process(context.Background(), d.source, op, protocol.DefaultLimits(), x.start)
				})
			}
			releasing := func() {
				wg.Go(func() { _, _, releaseErr = x.release(manager, run, 1, "Liberado na corrida", onboardingID(t)) })
			}
			if startFirst {
				starting()
				waitBlocked(t, x.syncFixtureState, pid, 1)
				releasing()
			} else {
				releasing()
				waitBlocked(t, x.syncFixtureState, pid, 1)
				starting()
			}
			waitBlocked(t, x.syncFixtureState, pid, 2)
			free()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			wg.Wait()
			if releaseErr != nil {
				t.Fatal("the release lost the race", releaseErr)
			}
			x.heldWithReason(t, op, startAck, startErr, tripapp.AssetUnavailable, tripapp.ChecklistBlocking, run)
			var releases int
			if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_checklist_run_releases WHERE org_id=$1 AND run_id=$2`, x.scope.OrgID().String(), run).Scan(&releases); err != nil || releases != 1 {
				t.Fatal("release rows", releases, err)
			}
			after := x.namedStart(t, d, run, x.declaredNow(t))
			ack, err := x.process(t, d, after)
			x.tripState.accepted(t, after, ack, err, 1, 1)
			if x.link(t, tripOf(after), tripapp.ChecklistDeparture) != run {
				t.Fatal("a start declared after the release was not linked")
			}
			x.finishTrip(t, d, tripOf(after), "", x.declaredNow(t))
		})
	}
}

// B-BR-EMP-515, BR-CHECK-013, BUC-EMP-263: a finish links its counting
// return run whatever its outcome (a BLOCKING one blocks the next use) and
// is never held for it; a return run that does not count links nothing.
func TestTripChecklistReturnRunLinksAtTheFinishWithoutHolding(t *testing.T) {
	x := gateFixture(t)
	d := x.syncFixtureState
	ret := x.template(t, checklist.Return, organizationTarget())
	start := x.namedStart(t, d, "", x.declaredNow(t))
	ack, err := x.process(t, d, start)
	x.tripState.accepted(t, start, ack, err, 1, 1)
	damaged := x.completedRun(t, d, ret, "RETURN", x.vehicle.AssetID, x.declaredNow(t), true)
	finish, finishAck := x.finishTrip(t, d, tripOf(start), damaged, x.declaredNow(t))
	if x.link(t, tripOf(start), tripapp.ChecklistReturn) != damaged {
		t.Fatal("the blocking return run was not linked to the finish")
	}
	for range 2 {
		again, err := x.process(t, d, finish)
		if err != nil || !bytes.Equal(again.CanonicalBytes(), finishAck.CanonicalBytes()) {
			t.Fatal("finish replay", err)
		}
	}
	if availability, causes := x.availability(t); availability != fleet.Blocked || len(causes) != 1 || causes[0].ID != damaged {
		t.Fatal("a blocking return run does not block the next use", availability, causes)
	}
	if _, _, err := x.release(x.handlerFor(t, "FLEET_MANAGER", "ORG", ""), damaged, 1, "Retrovisor trocado", onboardingID(t)); err != nil {
		t.Fatal(err)
	}
	// A departure run of the vehicle whose template is then archived, so no
	// departure checklist is required for the starts below.
	departure := x.template(t, checklist.Departure, organizationTarget())
	departureRun := x.completedRun(t, d, departure, "DEPARTURE", x.vehicle.AssetID, x.declaredNow(t), false)
	if _, _, err := templateRun(t, x.legalFixtureState, x.templates, templateCommand(t, maintapp.TemplateCommandInput{Kind: maintapp.CommandArchive, TemplateID: departure.TemplateID,
		ExpectedRevision: departure.Revision})); err != nil {
		t.Fatal(err)
	}
	early := x.completedRun(t, d, ret, "RETURN", x.vehicle.AssetID, x.declaredNow(t), false)
	second := x.driver(t)
	x.assign(t, second.person.employee, x.vehicle.AssetID, fleet.AdditionalAssignment, nil, nil)
	others := x.completedRun(t, second, ret, "RETURN", x.vehicle.AssetID, x.declaredNow(t), false)
	for name, named := range map[string]string{
		"not-stored-yet":      onboardingID(t),
		"already-linked":      damaged,
		"before-the-start":    early,
		"departure-context":   departureRun,
		"another-members-run": others,
	} {
		t.Run(name, func(t *testing.T) {
			trip := x.namedStart(t, d, "", x.declaredNow(t))
			ack, err := x.process(t, d, trip)
			x.tripState.accepted(t, trip, ack, err, 1, 1)
			x.finishTrip(t, d, tripOf(trip), named, x.declaredNow(t))
			if x.link(t, tripOf(trip), tripapp.ChecklistReturn) != "" {
				t.Fatal("a return run that does not count was linked")
			}
		})
	}
	// A held finish (TRIP_ALREADY_CLOSED) links nothing.
	fresh := x.completedRun(t, d, ret, "RETURN", x.vehicle.AssetID, x.declaredNow(t), false)
	late := x.finishOp(t, d, tripOf(start), x.declaredNow(t), func(e, _ map[string]any) { e["soft_references"] = runReference(fresh) })
	ack, err = x.process(t, d, late)
	x.tripState.held(t, late, ack, err, tripapp.TripAlreadyClosed)
	if x.link(t, tripOf(start), tripapp.ChecklistReturn) != damaged || x.countLinks(t) != 1 {
		t.Fatal("a held finish linked a run")
	}
}

// EDR-334, B-BR-EMP-513/514: forced RLS, insert-only links and reasons, and
// the database's own re-proof of a link and of a BLOCKING/EXPIRED reason.
func TestTripChecklistGateSchemaGuards(t *testing.T) {
	x := gateFixture(t)
	d := x.syncFixtureState
	org := x.scope.OrgID().String()
	for _, table := range []string{"org_trip_checklist_links", "org_trip_checklist_holds"} {
		var forced bool
		if err := x.f.Admin.QueryRow(t.Context(), `SELECT relrowsecurity AND relforcerowsecurity FROM pg_class WHERE oid=$1::regclass`, table).Scan(&forced); err != nil || !forced {
			t.Fatal(table, "without forced RLS", err)
		}
	}
	template := x.template(t, checklist.Departure, organizationTarget())
	ret := x.template(t, checklist.Return, organizationTarget())
	run := x.completedRun(t, d, template, "DEPARTURE", x.vehicle.AssetID, x.declaredNow(t), false)
	start := x.namedStart(t, d, run, x.declaredNow(t))
	ack, err := x.process(t, d, start)
	x.tripState.accepted(t, start, ack, err, 1, 1)
	trip := tripOf(start)
	later := x.completedRun(t, d, template, "DEPARTURE", x.vehicle.AssetID, x.declaredNow(t), false)
	returnRun := x.completedRun(t, d, ret, "RETURN", x.vehicle.AssetID, x.declaredNow(t), false)
	held := x.namedStart(t, d, "", x.declaredNow(t))
	ack, err = x.process(t, d, held)
	x.heldWithReason(t, held, ack, err, tripapp.AssetBusy, tripapp.ChecklistRequired, "")
	var review string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT review_item_id::text FROM org_trip_held_starts WHERE org_id=$1 AND operation_id=$2`, org, held.ID()).Scan(&review); err != nil {
		t.Fatal(err)
	}
	const link = `INSERT INTO org_trip_checklist_links(org_id,trip_id,context,run_id,operation_id) VALUES($1,$2,$3,$4,$5)`
	const hold = `INSERT INTO org_trip_checklist_holds(org_id,operation_id,review_item_id,reason,run_id) VALUES($1,$2,$3,$4,$5)`
	for name, c := range map[string]struct {
		query      string
		args       []any
		code, text string
	}{
		"departure-completed-after-the-start": {link, []any{org, trip, "DEPARTURE", later, start.ID()}, "23514", "checklist link source unavailable"},
		"return-run-as-departure":             {link, []any{org, trip, "DEPARTURE", returnRun, start.ID()}, "23514", "checklist link source unavailable"},
		"departure-by-another-operation":      {link, []any{org, trip, "DEPARTURE", run, held.ID()}, "23514", "checklist link source unavailable"},
		"return-without-the-finish":           {link, []any{org, trip, "RETURN", returnRun, start.ID()}, "23514", "checklist link source unavailable"},
		"unknown-run":                         {link, []any{org, trip, "DEPARTURE", onboardingID(t), start.ID()}, "23503", "private reference not found"},
		"second-link-of-the-trip":             {link, []any{org, trip, "DEPARTURE", run, start.ID()}, "23505", "org_trip_checklist_links_pkey"},
		"blocking-reason-not-proved":          {hold, []any{org, held.ID(), review, "CHECKLIST_BLOCKING", run}, "23514", "checklist hold reason unavailable"},
		"expired-reason-not-proved":           {hold, []any{org, held.ID(), review, "CHECKLIST_EXPIRED", run}, "23514", "checklist hold reason unavailable"},
		"reason-without-a-held-start":         {hold, []any{org, start.ID(), review, "CHECKLIST_REQUIRED", nil}, "23514", "checklist hold source unavailable"},
		"blocking-reason-without-run":         {hold, []any{org, held.ID(), review, "CHECKLIST_BLOCKING", nil}, "23514", ""},
		"second-reason":                       {hold, []any{org, held.ID(), review, "CHECKLIST_REQUIRED", nil}, "23505", "org_trip_checklist_holds_pkey"},
	} {
		t.Run(name, func(t *testing.T) {
			planSchemaCode(t, unitSchemaExec(t, x.legalFixtureState, c.query, c.args...), c.code, c.text)
		})
	}
	role := x.f.RuntimeRole
	if _, err := x.f.Admin.Exec(t.Context(), `GRANT UPDATE,DELETE,TRUNCATE ON org_trip_checklist_links,org_trip_checklist_holds TO "`+role+`"`); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"org_trip_checklist_links", "org_trip_checklist_holds"} {
		for name, query := range map[string]string{
			"update":   `UPDATE ` + table + ` SET recorded_at=recorded_at WHERE org_id=$1`,
			"delete":   `DELETE FROM ` + table + ` WHERE org_id=$1`,
			"truncate": `TRUNCATE ` + table,
		} {
			t.Run(table+"-"+name, func(t *testing.T) {
				args := []any{org}
				if name == "truncate" {
					args = nil
				}
				planSchemaCode(t, unitSchemaExec(t, x.legalFixtureState, query, args...), "55000", "")
			})
		}
		var visible int
		if err := x.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM `+table).Scan(&visible); err != nil || visible != 0 {
			t.Fatal(table, "visible without the organization setting", visible, err)
		}
	}
	if x.countLinks(t) != 1 || x.link(t, trip, tripapp.ChecklistDeparture) != run {
		t.Fatal("refused writes changed the links")
	}
}

const checklistGateMigration = "000255_organization_trip_checklist_gate.sql"

// A populated head before 000255 (completed and in-progress trips through the
// real signed intake, a held ASSET_BUSY start, an ACTIVE departure template
// and an accepted run) upgrades to 000255 by an ordinary non-superuser
// migrator: an injected failure leaves schema, functions and data unchanged,
// recovery applies once, replay is a no-op, the previous embedded image stays
// compatible, earlier receipts replay, and the gate then works on the
// upgraded schema with the pre-upgrade run (EMP-TRIP-019).
func TestTripChecklistGatePopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= checklistGateMigration || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	for _, head := range []string{"000242_organization_checklist_runs.sql", "000244_organization_expenses.sql", missionReservationLinkMigration} {
		if _, ok := previous[head]; !ok {
			t.Fatal("previous image does not contain", head)
		}
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	legal := legalFixtureFor(t, f, onboarding, actor, flags)
	base := tripFixtureOn(t, selfIntakeFixtureOn(t, syncFixtureOn(t, devicesFixtureOn(t, legal))))
	if base.gate.(maintpg.TripChecklistGate) != (maintpg.TripChecklistGate{PreGateImage: true}) {
		t.Fatal("the pre-000255 rehearsal did not get the flagged gate")
	}
	x := gateState{runFixtureOn(t, handlingState{incidentState: incidentFixtureOn(t, eventFixtureOn(t, base))})}
	tripReviewGrants(t, x.f)
	d := x.syncFixtureState
	first := x.startOp(t, d, x.declaredNow(t), nil)
	ack, err := x.process(t, d, first)
	x.tripState.accepted(t, first, ack, err, 1, 1)
	end := x.finishOp(t, d, tripOf(first), x.declaredNow(t).Add(time.Microsecond), nil)
	ack, err = x.process(t, d, end)
	x.tripState.accepted(t, end, ack, err, 2, 1)
	ongoing := x.startOp(t, d, x.declaredNow(t), nil)
	ongoingAck, err := x.process(t, d, ongoing)
	x.tripState.accepted(t, ongoing, ongoingAck, err, 1, 1)
	busy := x.startOp(t, d, x.declaredNow(t), nil)
	busyAck, err := x.process(t, d, busy)
	x.tripState.held(t, busy, busyAck, err, "ASSET_BUSY")
	template := x.template(t, checklist.Departure, organizationTarget())
	run := x.completedRun(t, d, template, "DEPARTURE", x.vehicle.AssetID, x.declaredNow(t), false)
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_trips x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,trip_id)FROM org_trip_finishes x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,review_item_id)FROM org_trip_held_starts x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_checklist_runs x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_checklist_templates x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_sync_review_items x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_acknowledgements x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	functions := func() string {
		t.Helper()
		var v string
		if err := f.Admin.QueryRow(t.Context(), `SELECT string_agg(p.oid::regprocedure::text||'='||md5(pg_get_functiondef(p.oid)),E'\n' ORDER BY p.oid::regprocedure::text)
 FROM pg_proc p WHERE p.pronamespace='public'::regnamespace AND p.prokind='f'`).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	before, beforeFunctions := snapshot(), functions()
	migrator := f.RuntimeRole + "_gate_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+
		";GRANT SELECT,INSERT ON schema_migrations TO "+role+";GRANT REFERENCES ON org_trips,org_checklist_runs,org_sync_operations,org_trip_held_starts,org_sync_review_items TO "+role); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := f.Admin.Exec(ctx, "REASSIGN OWNED BY "+role+" TO CURRENT_USER;DROP OWNED BY "+role+";DROP ROLE "+role); err != nil {
			t.Error(err)
		}
	})
	location, err := url.Parse(f.AdminDSN)
	if err != nil {
		t.Fatal(err)
	}
	query := location.Query()
	query.Set("options", "-c role="+migrator)
	location.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	raw, err := fs.ReadFile(migrations.Files, checklistGateMigration)
	if err != nil {
		t.Fatal(err)
	}
	version := strings.SplitN(checklistGateMigration, "_", 2)[0]
	broken := fstest.MapFS{checklistGateMigration: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected gate migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_trip_checklist_links') IS NULL AND to_regclass('org_trip_checklist_holds') IS NULL
 AND to_regprocedure('org_trip_checklist_validity(uuid,uuid,timestamptz)') IS NULL AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&clean); err != nil || !clean ||
		before != snapshot() || beforeFunctions != functions() {
		t.Fatal("failed gate migration changed the original schema, functions or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[checklistGateMigration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != version {
		t.Fatal("ordinary gate migration recovery", applied, err)
	}
	if before != snapshot() {
		t.Fatal("gate upgrade rewrote existing state")
	}
	// No existing function is redefined; the three new ones are added.
	after := map[string]bool{}
	for _, line := range strings.Split(functions(), "\n") {
		after[line] = true
	}
	for _, line := range strings.Split(beforeFunctions, "\n") {
		if !after[line] {
			t.Fatal("gate upgrade changed an existing function", line)
		}
	}
	if len(after) != len(strings.Split(beforeFunctions, "\n"))+4 {
		t.Fatal("gate upgrade function count", len(after))
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("gate migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	for _, table := range []string{"org_trip_checklist_links", "org_trip_checklist_holds"} {
		if err := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin AND(SELECT relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1)
 FROM pg_class WHERE oid=$2::regclass)FROM pg_roles WHERE rolname=$1`, migrator, table).Scan(&clean); err != nil || !clean {
			t.Fatal(table, "is not owned by the ordinary forced-RLS migrator", err)
		}
	}
	// Earlier receipts replay byte for byte.
	for op, want := range map[*protocol.Operation]protocol.Acknowledgement{&ongoing: ongoingAck, &busy: busyAck} {
		again, err := x.process(t, d, *op)
		if err != nil || !bytes.Equal(again.CanonicalBytes(), want.CanonicalBytes()) {
			t.Fatal("a pre-upgrade receipt does not replay", err)
		}
	}
	// The real gate on the upgraded schema: the pre-upgrade trip completes;
	// the pre-upgrade run gates a new start once; a start naming nothing is
	// held CHECKLIST_REQUIRED.
	x.gate = checklistGateSource(t, x.legalFixtureState)
	x.start, x.finish = x.handlers(t, x.meters, x.store)
	x.finishTrip(t, d, tripOf(ongoing), "", x.declaredNow(t))
	gated := x.namedStart(t, d, run, x.declaredNow(t))
	ack, err = x.process(t, d, gated)
	x.tripState.accepted(t, gated, ack, err, 1, 1)
	if x.link(t, tripOf(gated), tripapp.ChecklistDeparture) != run {
		t.Fatal("the pre-upgrade run did not gate a start after the upgrade")
	}
	x.finishTrip(t, d, tripOf(gated), "", x.declaredNow(t))
	none := x.namedStart(t, d, "", x.declaredNow(t))
	ack, err = x.process(t, d, none)
	x.heldWithReason(t, none, ack, err, tripapp.AssetUnavailable, tripapp.ChecklistRequired, "")
}
