package application_test

import (
	"errors"
	"testing"
	"time"

	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

var p0 = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

func ps(n int) time.Time { return p0.Add(time.Duration(n) * time.Second) }

// endedSession opens at 10 s, records precisely and ends at the given
// instant with the given cause and actor.
func endedSession(end int, cause domain.Cause, actor domain.Actor) app.SessionRecord {
	return app.SessionRecord{ID: policyID(70), TripID: policyID(71), OwnerMembershipID: policyID(72), OperationID: policyID(73), Policy: domain.PolicyExpected, PolicyRevision: 1, OpenedAt: ps(10),
		Transitions: []app.StoredTransition{
			{Seq: 1, State: domain.NotStarted, Cause: domain.CauseNotStarted, Actor: domain.ActorDriver, At: ps(10), OperationID: policyID(73)},
			{Seq: 2, State: domain.Recording, Actor: domain.ActorDriver, Permission: domain.PermissionPrecise, At: ps(10), OperationID: policyID(73)},
			{Seq: 3, State: domain.Ended, Cause: cause, Actor: actor, At: ps(end), OperationID: policyID(74)},
		}}
}

func finishedTrip(finish int) tripapp.Record {
	r := tripapp.Record{ID: policyID(71), AuthorMembershipID: policyID(72), Declared: orgtime.Declaration{OccurredAt: ps(0)}, State: trip.Completed}
	if finish >= 0 {
		r.Finish = &tripapp.FinishRecord{Declared: orgtime.Declaration{OccurredAt: ps(finish)}}
	}
	return r
}

func segment(seq int64, boot *string, elapsed *int64, base time.Time, from, to int) app.ProcessingSegment {
	g := app.ProcessingSegment{Seq: seq, BootID: boot, BaseElapsed: elapsed, Base: base, Received: to - from + 1, Dropped: app.Dropped{Accuracy: 1, Stale: 2}}
	for s := from; s <= to; s++ {
		g.Fixes = append(g.Fixes, app.StoredFix{At: ps(10 + s*10), Lat: -100000000, Lon: int32(-500000000 - s*12670), Accuracy: 8})
	}
	return g
}

func int64p(v int64) *int64 { return &v }

// B-BR-EMP-461..465: the summary is derived from the replayed history, the
// trip's running time and the session's own segments in sequence order.
func TestSummarizeUsesTheStoredRowsOnly(t *testing.T) {
	s := policyScope(t, 1)
	boot := "boot-a1"
	// Two segments delivered out of order, one boot, consistent bases.
	segments := []app.ProcessingSegment{segment(2, &boot, int64p(1000+100_000), ps(110), 10, 19), segment(1, &boot, int64p(1000), ps(10), 0, 9)}
	sum, err := app.Summarize(s, endedSession(210, domain.CauseTripCompleted, domain.ActorTrip), finishedTrip(210), segments)
	if err != nil {
		t.Fatal(err)
	}
	r := sum.Result
	if sum.Watermark != (app.Watermark{Segments: 2, Transitions: 3}) || r.Accepted != 20 || sum.Received != 20 || sum.DroppedAccuracy != 2 || sum.DroppedStale != 4 || sum.ClockUncertain != 0 {
		t.Fatalf("summary counters %+v", sum.Watermark)
	}
	if r.ExpectedSeconds != 210 || len(r.Gaps) != 1 || r.Gaps[0].Reason != domain.GapReason(domain.CauseNotStarted) || r.Gaps[0].Detector != domain.DetectedByServer ||
		!r.Gaps[0].From.Equal(ps(0)) || !r.Gaps[0].To.Equal(ps(10)) || r.Classification.Quality() != domain.QualityPrecise || r.DistanceMetres == nil {
		t.Fatal("summary basis", r.ExpectedSeconds, len(r.Gaps), r.Classification.Quality())
	}
	// A segment whose wall base disagrees with its boot by more than 2 s, and
	// one without a boot identity, are CLOCK_UNCERTAIN; instants stay as stored.
	drift := segment(3, &boot, int64p(1000+200_000), ps(213), 20, 19)
	none := segment(4, nil, nil, ps(214), 20, 19)
	sum, err = app.Summarize(s, endedSession(210, domain.CauseTripCompleted, domain.ActorTrip), finishedTrip(210), append(append([]app.ProcessingSegment{}, segments...), drift, none))
	if err != nil || sum.ClockUncertain != 2 || sum.Watermark.Segments != 4 {
		t.Fatal("clock uncertain", sum.ClockUncertain, err)
	}
	exact := segment(3, &boot, int64p(1000+200_000), ps(212), 20, 19)
	if sum, _ := app.Summarize(s, endedSession(210, domain.CauseTripCompleted, domain.ActorTrip), finishedTrip(210), append(append([]app.ProcessingSegment{}, segments...), exact)); sum.ClockUncertain != 0 {
		t.Fatal("a 2 s disagreement is uncertain", sum.ClockUncertain)
	}
}

// The trip's end: its finish, its supervisor closure, its cancellation or,
// with none stored, the session's end; pauses are not expected time.
func TestSummarizeWindowFollowsTheTrip(t *testing.T) {
	s := policyScope(t, 1)
	session := endedSession(210, domain.CauseMembershipEnded, domain.ActorServer)
	open := finishedTrip(-1)
	open.State = trip.InProgress
	sum, err := app.Summarize(s, session, open, nil)
	if err != nil || sum.Result.ExpectedSeconds != 210 || len(sum.Result.Gaps) != 2 || sum.Result.Gaps[1].Reason != domain.GapSignalLost {
		t.Fatal("open trip window", err, sum.Result.ExpectedSeconds, len(sum.Result.Gaps))
	}
	closed := finishedTrip(-1)
	closed.Closure = &tripapp.ClosureRecord{ClosedAt: ps(300)}
	sum, err = app.Summarize(s, session, closed, nil)
	if err != nil || sum.Result.ExpectedSeconds != 300 || sum.Result.Gaps[len(sum.Result.Gaps)-1].Reason != domain.GapReason(domain.CauseMembershipEnded) ||
		!sum.Result.Gaps[len(sum.Result.Gaps)-1].To.Equal(ps(300)) {
		t.Fatal("closed trip window", err, sum.Result.ExpectedSeconds)
	}
	paused := finishedTrip(400)
	paused.Transitions = []tripapp.TransitionRecord{{From: trip.InProgress, To: trip.Paused, Declared: orgtime.Declaration{OccurredAt: ps(100)}},
		{From: trip.Paused, To: trip.InProgress, Declared: orgtime.Declaration{OccurredAt: ps(250)}}}
	sum, err = app.Summarize(s, endedSession(400, domain.CauseTripCompleted, domain.ActorTrip), paused, nil)
	if err != nil || sum.Result.ExpectedSeconds != 250 {
		t.Fatal("paused trip window", err, sum.Result.ExpectedSeconds)
	}
	cancelled := finishedTrip(-1)
	cancelled.State = trip.Cancelled
	cancelled.Transitions = []tripapp.TransitionRecord{{From: trip.InProgress, To: trip.Cancelled, Declared: orgtime.Declaration{OccurredAt: ps(150)}}}
	sum, err = app.Summarize(s, endedSession(150, domain.CauseTripCancelled, domain.ActorTrip), cancelled, nil)
	if err != nil || sum.Result.ExpectedSeconds != 150 {
		t.Fatal("cancelled trip window", err, sum.Result.ExpectedSeconds)
	}
}

// Summaries are derived only from an ended session of the same trip with
// consistent rows.
func TestSummarizeRefusesInconsistentRows(t *testing.T) {
	s := policyScope(t, 1)
	boot := "boot-a1"
	open := endedSession(210, domain.CauseTripCompleted, domain.ActorTrip)
	open.Transitions = open.Transitions[:2]
	other := finishedTrip(210)
	other.ID = policyID(79)
	backwards := finishedTrip(210)
	backwards.Transitions = []tripapp.TransitionRecord{{From: trip.InProgress, To: trip.Paused, Declared: orgtime.Declaration{OccurredAt: ps(100)}},
		{From: trip.Paused, To: trip.InProgress, Declared: orgtime.Declaration{OccurredAt: ps(50)}}}
	for name, c := range map[string]struct {
		session  app.SessionRecord
		trip     tripapp.Record
		segments []app.ProcessingSegment
	}{
		"session not ended": {open, finishedTrip(210), nil},
		"another trip":      {endedSession(210, domain.CauseTripCompleted, domain.ActorTrip), other, nil},
		"backwards trip":    {endedSession(210, domain.CauseTripCompleted, domain.ActorTrip), backwards, nil},
		"repeated sequence": {endedSession(210, domain.CauseTripCompleted, domain.ActorTrip), finishedTrip(210), []app.ProcessingSegment{segment(1, &boot, int64p(1), ps(10), 0, 1), segment(1, &boot, int64p(1), ps(10), 2, 3)}},
		"repeated instant":  {endedSession(210, domain.CauseTripCompleted, domain.ActorTrip), finishedTrip(210), []app.ProcessingSegment{segment(1, &boot, int64p(1), ps(10), 0, 1), segment(2, &boot, int64p(1), ps(10), 1, 2)}},
	} {
		if _, err := app.Summarize(s, c.session, c.trip, c.segments); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("inconsistent rows summarized", name, err)
		}
	}
	if !(app.Watermark{Segments: 2, Transitions: 3}).Newer(app.Watermark{Segments: 1, Transitions: 3}) || (app.Watermark{Segments: 2, Transitions: 3}).Newer(app.Watermark{Segments: 2, Transitions: 3}) ||
		(app.Watermark{Segments: 3, Transitions: 2}).Newer(app.Watermark{Segments: 2, Transitions: 3}) {
		t.Fatal("watermark order")
	}
}
