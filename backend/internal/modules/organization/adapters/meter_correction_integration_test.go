//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	eventpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/adapters"
	event "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
)

// correctionParents is the parent-record port for store tests: nothing is
// approved unless the test says so.
type correctionParents struct{ approved map[string]bool }

func (p correctionParents) ApprovedParentTx(_ context.Context, _ syncapp.DBTX, _ kernel.OrgScope, reading string) (bool, error) {
	return p.approved[reading], nil
}

// correctionFixture is validationFixture with the decision store composed:
// the web actor of the legal fixture (ORG owner) decides.
func correctionFixture(t *testing.T, opening int64, base time.Time) (syncFixtureState, *fleetpg.MeterStore, app.MeterInstallResult) {
	t.Helper()
	fx, store, installed := validationFixture(t, opening, base)
	catalog, err := audit.NewCatalog(fleetpg.MeterAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	store.Audit = auditpg.NewStore(fx.f.Runtime, catalog)
	store.Parents = correctionParents{}
	return fx, store, installed
}

// decide never calls t.Fatal, so it is safe in a goroutine: an invalid
// synthetic command is returned as its error.
func decide(_ *testing.T, fx syncFixtureState, store *fleetpg.MeterStore, in app.MeterDecisionInput) (app.MeterDecisionResult, bool, error) {
	command, err := app.NewMeterDecisionCommand(in)
	if err != nil {
		return app.MeterDecisionResult{}, false, err
	}
	id, err := jobs.NewUUIDv4()
	if err != nil {
		return app.MeterDecisionResult{}, false, err
	}
	return store.DecideMeterReading(context.Background(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, id, command)
}

func decision(installed app.MeterInstallResult, reading string, revision int64, kind app.MeterDecision, reason string) app.MeterDecisionInput {
	return app.MeterDecisionInput{OperationID: "", AssetID: installed.AssetID, MeterID: installed.MeterID, ReadingID: reading, ExpectedRevision: revision, Decision: kind, Reason: reason}
}

func withOperation(in app.MeterDecisionInput, op string) app.MeterDecisionInput {
	in.OperationID = op
	return in
}

// AC1/AC2/AC5 and example B on real PostGIS: the correction voids and links
// the original, becomes current, re-evaluates the suspect neighbour and a
// second correction of the same original is READING_ALREADY_SUPERSEDED.
func TestMeterCorrectionSupersedesOnceAndReevaluatesNeighbours(t *testing.T) {
	base := time.Now().UTC().Truncate(time.Second).Add(-7 * 24 * time.Hour)
	fx, store, installed := correctionFixture(t, 183_000_000, base)
	validationRecord(t, fx, store, installed, base.Add(47*time.Hour), 183_400_000, readingStateRow{state: "VALID"})
	// R2 typed 3 000 km too high but still plausible: VALID.
	r2, _ := validationRecord(t, fx, store, installed, base.Add(95*time.Hour), 186_932_000, readingStateRow{state: "VALID"})
	// R4 is then below R2: SUSPECT.
	r4, _ := validationRecord(t, fx, store, installed, base.Add(120*time.Hour), 184_190_000, readingStateRow{state: "SUSPECT", flags: "READING_BELOW_PREVIOUS"})
	in := withOperation(decision(installed, r2, 1, app.MeterSupersede, "Dígitos trocados"), onboardingID(t))
	in.Value, _ = meter.NewValue(meter.Odometer, 183_932_000, nil)
	result, replay, err := decide(t, fx, store, in)
	if err != nil || replay {
		t.Fatal("correction refused", err)
	}
	if result.Target.ReadingID != r2 || result.Target.State != meter.Voided || result.Target.Revision != 2 || result.Replacement == nil || result.Replacement.State != meter.Valid || result.Replacement.Revision != 1 {
		t.Fatalf("correction result %+v", result)
	}
	if len(result.Reevaluated) != 1 || result.Reevaluated[0].ReadingID != r4 || result.Reevaluated[0].State != meter.Valid || result.Reevaluated[0].Revision != 2 {
		t.Fatalf("AC5: consistent suspect neighbour not returned to VALID %+v", result.Reevaluated)
	}
	replacement := result.Replacement.ReadingID
	var rows int
	var linked, kept bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_meter_readings WHERE org_id=$1 AND id IN($2,$3)),
 EXISTS(SELECT 1 FROM org_meter_reading_states WHERE org_id=$1 AND reading_id=$2 AND revision=2 AND state='VOIDED' AND cause='SUPERSEDED' AND superseded_by=$3::uuid AND reason='Dígitos trocados' AND actor_membership_id=$4::uuid),
 EXISTS(SELECT 1 FROM org_meter_readings WHERE org_id=$1 AND id=$2 AND value=186932000 AND state='VALID')`, fx.scope.OrgID().String(), r2, replacement, fx.member).Scan(&rows, &linked, &kept); err != nil || rows != 2 || !linked || !kept {
		t.Fatal("AC1: original not kept, voided and linked", err)
	}
	if latestReadingID(t, fx, store, installed) != r4 {
		t.Fatal("the re-evaluated reading did not become current")
	}
	again := withOperation(decision(installed, r2, 2, app.MeterSupersede, "Second try"), onboardingID(t))
	again.Value, _ = meter.NewValue(meter.Odometer, 183_932_001, nil)
	if _, _, err := decide(t, fx, store, again); !errors.Is(err, app.ErrReadingAlreadySuperseded) {
		t.Fatal("AC2: second correction of the original", err)
	}
	// The same operation and digest returns the first result unchanged.
	first := result
	result, replay, err = decide(t, fx, store, in)
	if err != nil || !replay || result.Replacement == nil || result.Replacement.ReadingID != replacement || len(result.Reevaluated) != 1 || !reflect.DeepEqual(result, first) {
		t.Fatal("replay changed the first result", err)
	}
	reused := in
	reused.Reason = "Another reason"
	if _, _, err := decide(t, fx, store, reused); !errors.Is(err, app.ErrOperationReused) {
		t.Fatal("same operation with another digest", err)
	}
}

// AC3 and the void path: a rate flag is confirmed with a reason, a
// monotonicity flag cannot be, a non-suspect cannot be confirmed, a void is
// terminal, a stale revision is CONFLICT and the installation opening is the
// segment anchor.
func TestMeterCorrectionConfirmVoidAndRefusals(t *testing.T) {
	base := time.Now().UTC().Truncate(time.Second).Add(-24 * time.Hour)
	fx, store, installed := correctionFixture(t, 0, base)
	valid, _ := validationRecord(t, fx, store, installed, base.Add(time.Hour), 100_000, readingStateRow{state: "VALID"})
	below, _ := validationRecord(t, fx, store, installed, base.Add(2*time.Hour), 50_000, readingStateRow{state: "SUSPECT", flags: "READING_BELOW_PREVIOUS"})
	fast, _ := validationRecord(t, fx, store, installed, base.Add(3*time.Hour), 600_000, readingStateRow{state: "SUSPECT", flags: "IMPLAUSIBLE_RATE"})
	for name, c := range map[string]struct {
		in   app.MeterDecisionInput
		want error
	}{
		"monotonicity confirmed": {decision(installed, below, 1, app.MeterConfirm, "Synthetic confirmation"), meter.ErrInvalid},
		"valid confirmed":        {decision(installed, valid, 1, app.MeterConfirm, "Synthetic confirmation"), app.ErrConflict},
		"stale revision":         {decision(installed, fast, 0, app.MeterConfirm, "Synthetic confirmation"), app.ErrConflict},
		"opening anchor":         {decision(installed, installed.OpeningReadingID, 0, app.MeterVoid, "Synthetic void"), meter.ErrInvalid},
		"foreign reading":        {decision(installed, onboardingID(t), 0, app.MeterVoid, "Synthetic void"), app.ErrNotFound},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := decide(t, fx, store, withOperation(c.in, onboardingID(t))); !errors.Is(err, c.want) {
				t.Fatal(err)
			}
		})
	}
	confirmed, _, err := decide(t, fx, store, withOperation(decision(installed, fast, 1, app.MeterConfirm, "Synthetic confirmed haul"), onboardingID(t)))
	if err != nil || confirmed.Target.State != meter.Valid || confirmed.Target.Revision != 2 || latestReadingID(t, fx, store, installed) != fast {
		t.Fatal("AC3: rate flag not confirmable", err)
	}
	// The confirmed reading is now the successor of the suspect one, so its
	// re-evaluation adds IMPLAUSIBLE_RATE (revision 2): voiding needs it.
	if rows := readingStates(t, fx, below); len(rows) != 2 || rows[1].flags != "READING_BELOW_PREVIOUS+IMPLAUSIBLE_RATE" || rows[1].cause != "REEVALUATION" {
		t.Fatalf("confirmation did not re-evaluate the suspect neighbour: %+v", rows)
	}
	voided, _, err := decide(t, fx, store, withOperation(decision(installed, below, 2, app.MeterVoid, "Synthetic wrong asset"), onboardingID(t)))
	if err != nil || voided.Target.State != meter.Voided || voided.Target.Revision != 3 || voided.Replacement != nil {
		t.Fatal("void refused", err)
	}
	if _, _, err := decide(t, fx, store, withOperation(decision(installed, below, 3, app.MeterConfirm, "Synthetic"), onboardingID(t))); !errors.Is(err, app.ErrReadingAlreadySuperseded) {
		t.Fatal("VOIDED is not terminal", err)
	}
	var audits int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action IN('meter.reading.confirmed','meter.reading.voided') AND target_type='meter_reading' AND reason_code=''`, fx.scope.OrgID().String()).Scan(&audits); err != nil || audits != 2 {
		t.Fatal("AC4: one audit event per decision", audits, err)
	}
}

func currentRevision(t *testing.T, fx syncFixtureState, reading string) int64 {
	t.Helper()
	var revision int64
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT COALESCE(max(revision),0) FROM org_meter_reading_states WHERE org_id=$1 AND reading_id=$2`, fx.scope.OrgID().String(), reading).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	return revision
}

// Two reviewers correct the same reading at once: one wins, the other sees
// READING_ALREADY_SUPERSEDED, in both orders; a correction that still breaks
// monotonicity is stored SUSPECT and reported.
func TestMeterCorrectionConcurrentReviewersOneWins(t *testing.T) {
	for _, order := range []string{"void-first", "supersede-first"} {
		t.Run(order, func(t *testing.T) {
			base := time.Now().UTC().Truncate(time.Second).Add(-24 * time.Hour)
			fx, store, installed := correctionFixture(t, 0, base)
			validationRecord(t, fx, store, installed, base.Add(time.Hour), 100_000, readingStateRow{state: "VALID"})
			target, _ := validationRecord(t, fx, store, installed, base.Add(2*time.Hour), 50_000, readingStateRow{state: "SUSPECT", flags: "READING_BELOW_PREVIOUS"})
			void := withOperation(decision(installed, target, 1, app.MeterVoid, "Synthetic void"), onboardingID(t))
			supersede := withOperation(decision(installed, target, 1, app.MeterSupersede, "Synthetic correction"), onboardingID(t))
			supersede.Value, _ = meter.NewValue(meter.Odometer, 90_000, nil)
			first, second := void, supersede
			if order == "supersede-first" {
				first, second = supersede, void
			}
			holder := make(chan int, 1)
			release := make(chan struct{})
			var once sync.Once
			free := func() { once.Do(func() { close(release) }) }
			defer free()
			done := make(chan error, 2)
			// The first decision holds the root lock until released.
			go func() {
				done <- database.WithOrgTx(context.Background(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
					if _, err := tx.Exec(context.Background(), `SELECT state FROM organizations WHERE org_id=$1 FOR UPDATE`, fx.scope.OrgID().String()); err != nil {
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
			wg.Go(func() { _, _, err := decide(t, fx, store, first); results <- err })
			waitBlocked(t, fx, pid, 1)
			wg.Go(func() { _, _, err := decide(t, fx, store, second); results <- err })
			waitBlocked(t, fx, pid, 2)
			free()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			wg.Wait()
			close(results)
			var ok, lost int
			for err := range results {
				switch {
				case err == nil:
					ok++
				case errors.Is(err, app.ErrReadingAlreadySuperseded):
					lost++
				default:
					t.Fatal("unexpected race outcome", err)
				}
			}
			var voids int
			if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_meter_reading_states WHERE org_id=$1 AND reading_id=$2 AND state='VOIDED'`, fx.scope.OrgID().String(), target).Scan(&voids); err != nil || ok != 1 || lost != 1 || voids != 1 {
				t.Fatal("two concurrent decisions did not leave exactly one", ok, lost, voids, err)
			}
		})
	}
}

// waitBlocked waits until n runtime sessions other than the holder wait on a lock.
func waitBlocked(t *testing.T, fx syncFixtureState, holder, n int) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		var waiting int
		// Queued decisions wait behind the holder or behind each other.
		if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND usename=$1 AND pid<>$2 AND wait_event_type='Lock' AND cardinality(pg_blocking_pids(pid))>0`, fx.f.RuntimeRole, holder).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("decision never waited behind the held root lock", waiting)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// A correction that still breaks monotonicity is stored SUSPECT and its flag
// is reported; an approved parent record is AMENDMENT_REQUIRED with nothing
// changed; another organization's reading is NOT_FOUND; a rolled-back
// decision leaves nothing; with consumers, one recalculation intent per
// consumer commits with the decision and carries identifiers only.
func TestMeterCorrectionSuspectReplacementAmendmentIsolationAndRecalculation(t *testing.T) {
	base := time.Now().UTC().Truncate(time.Second).Add(-24 * time.Hour)
	fx, store, installed := correctionFixture(t, 0, base)
	validationRecord(t, fx, store, installed, base.Add(time.Hour), 100_000, readingStateRow{state: "VALID"})
	target, _ := validationRecord(t, fx, store, installed, base.Add(2*time.Hour), 50_000, readingStateRow{state: "SUSPECT", flags: "READING_BELOW_PREVIOUS"})
	still := withOperation(decision(installed, target, 1, app.MeterSupersede, "Synthetic still low"), onboardingID(t))
	still.Value, _ = meter.NewValue(meter.Odometer, 90_000, nil)
	store.Parents = correctionParents{approved: map[string]bool{target: true}}
	if _, _, err := decide(t, fx, store, still); !errors.Is(err, app.ErrAmendmentRequired) || currentRevision(t, fx, target) != 1 {
		t.Fatal("approved parent changed in place", err)
	}
	store.Parents = correctionParents{}
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	command, _ := app.NewMeterDecisionCommand(withOperation(decision(installed, target, 1, app.MeterVoid, "Synthetic"), onboardingID(t)))
	if _, _, err := store.DecideMeterReading(t.Context(), other.scope, fleetActor(other), other.member, onboardingID(t), command); !errors.Is(err, app.ErrNotFound) || currentRevision(t, fx, target) != 1 {
		t.Fatal("another organization reached this reading", err)
	}
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_event_intents TO "+role+";GRANT SELECT,INSERT,UPDATE ON job_queue TO "+role); err != nil {
		t.Fatal(err)
	}
	consumers := []string{"meter.synthetic_projection"}
	catalog, err := event.NewCatalog(fleetpg.MeterCorrectionEventDefinitions(consumers))
	if err != nil {
		t.Fatal(err)
	}
	store.Recalculation = &fleetpg.MeterRecalculation{Catalog: catalog, Writer: eventpg.NewStore(fx.f.Runtime), Consumers: consumers}
	result, _, err := decide(t, fx, store, still)
	if err != nil || result.Replacement == nil || result.Replacement.State != meter.Suspect || len(result.Replacement.Flags) != 1 || result.Replacement.Flags[0] != meter.ReadingBelowPrevious {
		t.Fatal("a correction still below its predecessor was not stored SUSPECT and reported", err)
	}
	var intents int
	var exact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*),bool_and(event_name='meter.usage.recalculated' AND target_type='meter_reading' AND target_id=$3::uuid AND revision=2 AND consumer='meter.synthetic_projection') FROM org_event_intents WHERE org_id=$1 AND event_id=$2`, fx.scope.OrgID().String(), still.OperationID, target).Scan(&intents, &exact); err != nil || intents != 1 || !exact {
		t.Fatal("recalculation intent missing or not reference-only", intents, err)
	}
	if _, replay, err := decide(t, fx, store, still); err != nil || !replay {
		t.Fatal("replay with consumers", err)
	}
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_event_intents WHERE org_id=$1 AND event_id=$2`, fx.scope.OrgID().String(), still.OperationID).Scan(&intents); err != nil || intents != 1 {
		t.Fatal("replay published a second intent", intents, err)
	}
}
