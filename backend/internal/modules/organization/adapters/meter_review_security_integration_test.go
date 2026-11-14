//go:build integration

package adapters_test

import (
	"bytes"
	"errors"
	"sync"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

func TestMeterReviewConcurrentOriginalDecisionsAndCompetingReviewers(t *testing.T) {
	fx, _, installed := meterEvidenceFixture(t)
	store := meterReviewStore(t, fx)
	_, ref, _, err := storeMeterHeldReview(t, fx, store, meterObservedCommand(t, installed, time.Now().UTC().Add(-time.Hour), 7))
	if err != nil {
		t.Fatal(err)
	}
	input := app.MeterReviewDecisionInput{OperationID: onboardingID(t), ReviewItemID: ref.ReviewItemID, ExpectedRevision: 1, Decision: app.MeterReviewAccept}
	command, err := app.NewMeterReviewDecision(input)
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		resolution protocol.ReviewResolution
		replay     bool
		err        error
	}
	results := make(chan result, 12)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for range 12 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			r, replay, e := store.ResolveMeterReview(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, command)
			results <- result{r, replay, e}
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	var original []byte
	applied := 0
	for result := range results {
		if result.err != nil {
			t.Fatal("concurrent original decision failed", result.err)
		}
		if !result.replay {
			applied++
		}
		if original == nil {
			original = result.resolution.CanonicalBytes()
		} else if !bytes.Equal(original, result.resolution.CanonicalBytes()) {
			t.Fatal("concurrent retry changed original decision")
		}
	}
	if applied != 1 {
		t.Fatal("original command applied more than once")
	}
	input.Reason = "Synthetic changed immutable intent"
	changed, _ := app.NewMeterReviewDecision(input)
	if _, _, err := store.ResolveMeterReview(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, changed); !errors.Is(err, app.ErrOperationReused) {
		t.Fatal("same command accepted changed decision body", err)
	}
	// A genuinely distinct current member/cookie is the competing reviewer.
	other := deviceNewPerson(t, fx.deviceFixtureState)
	matrixAssign(t, fx.deviceFixtureState, matrixPerson{member: other.member}, "ADMIN", "ORG", "ACTIVE")
	_, second, _, err := storeMeterHeldReview(t, fx, store, meterObservedCommand(t, installed, time.Now().UTC().Add(-30*time.Minute), 7))
	if err != nil {
		t.Fatal(err)
	}
	results = make(chan result, 12)
	start = make(chan struct{})
	for i := range 12 {
		actor := fleetActor(fx.legalFixtureState)
		member := fx.member
		kind := app.MeterReviewAccept
		reason := ""
		if i%2 == 1 {
			actor = app.FleetActor{AccountID: other.web.AccountID, SessionID: other.web.SessionID}
			member = other.member
			kind = app.MeterReviewRefuse
			reason = "Synthetic competing refusal"
		}
		c, _ := app.NewMeterReviewDecision(app.MeterReviewDecisionInput{OperationID: onboardingID(t), ReviewItemID: second.ReviewItemID, ExpectedRevision: 1, Decision: kind, Reason: reason})
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			r, replay, e := store.ResolveMeterReview(t.Context(), fx.scope, actor, member, c)
			results <- result{r, replay, e}
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	success, conflicts := 0, 0
	accepted := false
	for result := range results {
		if result.err == nil {
			success++
			accepted = result.resolution.Snapshot().Outcome == protocol.ReviewAccepted
		} else if errors.Is(result.err, app.ErrConflict) {
			conflicts++
		} else {
			t.Fatal("competing actual reviewer failed outside CAS", result.err)
		}
	}
	if success != 1 || conflicts != 11 {
		t.Fatal("review CAS did not select exactly one decision")
	}
	var resolutions, audits, effects int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_sync_review_resolutions WHERE org_id=$1),(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action='offl.review.resolved'),(SELECT count(*) FROM org_sync_effects WHERE org_id=$1)`, fx.scope.OrgID().String()).Scan(&resolutions, &audits, &effects); err != nil {
		t.Fatal(err)
	}
	wantEffects := 1
	if accepted {
		wantEffects++
	}
	if resolutions != 2 || audits != 2 || effects != wantEffects {
		t.Fatal("racing decisions duplicated publication/audit/readiness")
	}
}

func TestMeterReviewLatePublicationFailureRollsBackEveryAcceptedEffect(t *testing.T) {
	fx, _, installed := meterEvidenceFixture(t)
	store := meterReviewStore(t, fx)
	op, ref, ack, err := storeMeterHeldReview(t, fx, store, meterObservedCommand(t, installed, time.Now().UTC().Add(-time.Hour), 7))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_review_failure()RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='synthetic review publication failure';END$$;CREATE CONSTRAINT TRIGGER synthetic_review_failure AFTER INSERT ON org_sync_review_resolutions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_review_failure()`); err != nil {
		t.Fatal(err)
	}
	command, _ := app.NewMeterReviewDecision(app.MeterReviewDecisionInput{OperationID: onboardingID(t), ReviewItemID: ref.ReviewItemID, ExpectedRevision: 1, Decision: app.MeterReviewAccept})
	if _, _, err := store.ResolveMeterReview(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, command); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("late commit failure exposed accepted decision", err)
	}
	var exact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM org_sync_review_resolutions WHERE org_id=$1) AND NOT EXISTS(SELECT 1 FROM org_sync_effects WHERE org_id=$1) AND(SELECT count(*) FROM org_meter_readings WHERE org_id=$1)=1 AND(SELECT canonical_ack FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2)=$3::bytea AND NOT EXISTS(SELECT 1 FROM org_audit_events WHERE org_id=$1 AND action='offl.review.resolved')`, fx.scope.OrgID().String(), op.ID(), ack.CanonicalBytes()).Scan(&exact); err != nil || !exact {
		t.Fatal("failed decision leaked partial publication or changed original ACK", err)
	}
	if _, err := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_review_failure ON org_sync_review_resolutions;DROP FUNCTION synthetic_review_failure()`); err != nil {
		t.Fatal(err)
	}
	if _, replay, err := store.ResolveMeterReview(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, command); err != nil || replay {
		t.Fatal("rolled-back command did not recover exactly once", err)
	}
}

func TestMeterReviewOwnResultsWithholdWholeBodyOnForeignOrChangedSelector(t *testing.T) {
	fx, _, installed := meterEvidenceFixture(t)
	store := meterReviewStore(t, fx)
	op, _, _, err := storeMeterHeldReview(t, fx, store, meterObservedCommand(t, installed, time.Now().UTC().Add(-time.Hour), 7))
	if err != nil {
		t.Fatal(err)
	}
	good := protocol.ReviewSelector{OperationID: op.ID(), PayloadDigest: op.PayloadDigest()}
	bad := good
	bad.OperationID = onboardingID(t)
	for _, selectors := range [][]protocol.ReviewSelector{{good, bad}, {{OperationID: op.ID(), PayloadDigest: "sha256:0000000000000000000000000000000000000000000000000000000000000000"}}} {
		rows, err := store.OwnMeterReviewResults(t.Context(), fx.source, selectors)
		if !errors.Is(err, orgapi.NotFound) || len(rows) != 0 {
			t.Fatal("own results exposed buffered prefix or mismatched intent", err)
		}
	}
	foreign := fx.source
	foreign.EnrollmentID = onboardingID(t)
	if rows, err := store.OwnMeterReviewResults(t.Context(), foreign, []protocol.ReviewSelector{good}); err == nil || len(rows) != 0 {
		t.Fatal("foreign enrollment read original result")
	}
}
