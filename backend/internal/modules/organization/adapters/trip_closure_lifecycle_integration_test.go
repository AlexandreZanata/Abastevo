//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"testing"
	"time"

	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	tripdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

// EMP-TRIP-011 × EMP-TRIP-014 on real PostGIS: a paused trip is open and is
// closed at its current revision (a stale one is CONFLICT), the open-trip
// list shows it PAUSED with its revision, a later resume is held
// TRIP_ALREADY_CLOSED; a cancelled trip is closed and a closure is CONFLICT
// with nothing written.
func TestTripClosureTreatsPausedTripsAsOpenAndCancelledAsClosed(t *testing.T) {
	x := tripLifecycleFixture(t)
	d := x.syncFixtureState
	review := tripReviewState{tripState: x.tripState}
	closures := tripClosureState{tripAmendState: tripAmendState{tripPeriodState: tripPeriodState{tripReviewState: review}}, closures: tripClosureStoreOf(t, x.legalFixtureState, x.f.Runtime)}
	supervisor := review.reviewerFor(t, "SUPERVISOR", "ORG", "")
	at := x.declaredNow(t)
	paused := x.started(t, d, at)
	pause := x.transitionOp(t, d, tripapp.PauseOperationType, paused, 1, at.Add(10*time.Minute), nil, nil)
	ack, err := x.process(t, d, pause)
	x.applied(t, pause, ack, err, 2)
	page, err := closures.closures.ListOpenTrips(context.Background(), x.scope, supervisor.actor, supervisor.member, tripapp.QueueFilter{Limit: 10})
	if err != nil || len(page.Items) != 1 || page.Items[0].TripID != paused || page.Items[0].State != tripdomain.Paused || page.Items[0].Revision != 2 {
		t.Fatal("the paused trip is not listed as open at revision 2", err)
	}
	if _, _, err := closures.close(x.scope, supervisor, paused, 1, closureReason, closureMissing, onboardingID(t)); !errors.Is(err, tripapp.ErrClosureConflict) {
		t.Fatal("a closure naming the revision before the pause was not CONFLICT", err)
	}
	result, _, err := closures.close(x.scope, supervisor, paused, 2, closureReason, closureMissing, onboardingID(t))
	if err != nil || result.Operational != tripdomain.Completed || result.Revision != 3 || result.UsageReason != tripdomain.ReasonReadingAbsent {
		t.Fatal("the paused trip was not closed at its next revision", err, result.Revision)
	}
	if state := x.stateOf(t, paused); state != "COMPLETED" {
		t.Fatal("closed paused trip state", state)
	}
	var revision int64
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT org_trip_revision($1,$2)`, x.scope.OrgID().String(), paused).Scan(&revision); err != nil || revision != 3 {
		t.Fatal("the database revision does not count the closure", revision, err)
	}
	resume := x.transitionOp(t, d, tripapp.ResumeOperationType, paused, 2, at.Add(20*time.Minute), nil, nil)
	ack, err = x.process(t, d, resume)
	x.heldCommand(t, resume, ack, err, tripapp.TripAlreadyClosed)
	// A cancelled trip is closed: CONFLICT, nothing written.
	cancelled := x.started(t, d, at.Add(time.Hour))
	cancel := x.transitionOp(t, d, tripapp.CancelOperationType, cancelled, 1, at.Add(time.Hour+time.Minute), "Viagem desmarcada", nil)
	ack, err = x.process(t, d, cancel)
	x.applied(t, cancel, ack, err, 2)
	if _, _, err := closures.close(x.scope, supervisor, cancelled, 2, closureReason, closureMissing, onboardingID(t)); !errors.Is(err, tripapp.ErrClosureConflict) {
		t.Fatal("a cancelled trip was closed", err)
	}
	if c := closures.closureFacts(t, cancelled); c != (tripClosureFacts{state: "CANCELLED"}) {
		t.Fatalf("a refused closure of a cancelled trip changed state: %+v", c)
	}
}
