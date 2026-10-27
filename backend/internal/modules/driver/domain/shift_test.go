package domain

import (
	"errors"
	"testing"
)

func newTestShift(t *testing.T) Shift {
	t.Helper()
	s, e := NewShift("11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "android", "33333333-3333-4333-8333-333333333333", "America/Cuiaba")
	if e != nil {
		t.Fatal(e)
	}
	return s
}

func clock(boot string, elapsed, utc int64) Clock {
	return Clock{BootID: boot, ElapsedMillis: elapsed, UTCMillis: utc}
}

func command(id, action string, version int64, c Clock) ShiftCommand {
	return ShiftCommand{ID: id, Action: action, ExpectedVersion: version, Clock: c}
}

const (
	opStart   = "44444444-4444-4444-8444-444444444444"
	opPause   = "55555555-5555-4555-8555-555555555555"
	opResume  = "66666666-6666-4666-8666-666666666666"
	opEnd     = "77777777-7777-4777-8777-777777777777"
	opGap     = "88888888-8888-4888-8888-888888888888"
	opReview  = "99999999-9999-4999-8999-999999999999"
	opCheckpt = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	opRecover = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
)

func apply(t *testing.T, s Shift, c ShiftCommand) ShiftTransition {
	t.Helper()
	out, e := s.Apply(c, ClockPolicy{MaximumCheckpointGapMillis: 60_000, WallToleranceMillis: 1_000})
	if e != nil {
		t.Fatal(e)
	}
	return out
}

func TestShiftPauseResumeEndExcludesPausedTime(t *testing.T) {
	s := newTestShift(t)
	s = apply(t, s, command(opStart, ActionStart, 0, clock("boot-a", 1_000, 1_000_000))).Shift
	if s.State != ShiftRunning || s.Version != 1 || s.ActiveMillis != 0 {
		t.Fatalf("start %+v", s)
	}
	s = apply(t, s, command(opPause, ActionPause, 1, clock("boot-a", 11_000, 1_010_000))).Shift
	if s.State != ShiftPaused || s.ActiveMillis != 10_000 {
		t.Fatalf("pause %+v", s)
	}
	s = apply(t, s, command(opResume, ActionResume, 2, clock("boot-a", 31_000, 1_030_000))).Shift
	if s.State != ShiftRunning || s.ActiveMillis != 10_000 {
		t.Fatalf("resume %+v", s)
	}
	s = apply(t, s, command(opEnd, ActionEnd, 3, clock("boot-a", 41_000, 1_040_000))).Shift
	if s.State != ShiftClosed || s.ActiveMillis != 20_000 || s.Version != 4 {
		t.Fatalf("end %+v", s)
	}
	if s.CompleteWorkSeconds() == nil || *s.CompleteWorkSeconds() != 20 {
		t.Fatal("complete work seconds")
	}
}

func TestShiftReplayStaleAndChangedBody(t *testing.T) {
	s := newTestShift(t)
	start := command(opStart, ActionStart, 0, clock("boot-a", 1_000, 1_000_000))
	first := apply(t, s, start)
	replay := apply(t, first.Shift, start)
	if !replay.Replayed || replay.Shift.Version != 1 {
		t.Fatalf("replay %+v", replay)
	}
	changed := start
	changed.Clock.UTCMillis++
	if _, e := first.Shift.Apply(changed, ClockPolicy{MaximumCheckpointGapMillis: 60_000}); !errors.Is(e, ErrConflict) {
		t.Fatal("changed body", e)
	}
	if _, e := first.Shift.Apply(command(opPause, ActionPause, 0, clock("boot-a", 2_000, 1_001_000)), ClockPolicy{MaximumCheckpointGapMillis: 60_000}); !errors.Is(e, ErrConflict) {
		t.Fatal("stale version", e)
	}
}

func TestShiftGapReviewAndBootChange(t *testing.T) {
	s := apply(t, newTestShift(t), command(opStart, ActionStart, 0, clock("boot-a", 1_000, 1_000_000))).Shift
	gap := apply(t, s, command(opGap, ActionCheckpoint, 1, clock("boot-b", 2_000, 1_001_000))).Shift
	if gap.State != ShiftRecoveryRequired || len(gap.Gaps) != 1 || gap.Gaps[0].Reason != GapBootChanged || gap.CompleteWorkSeconds() != nil {
		t.Fatalf("gap %+v", gap)
	}
	work := int64(500)
	review := command(opReview, ActionReviewGap, 2, clock("boot-b", 3_000, 1_002_000))
	review.GapID = opGap
	review.ReviewedWorkMillis = &work
	reviewed := apply(t, gap, review).Shift
	if reviewed.State != ShiftPaused || reviewed.ActiveMillis != 500 || reviewed.ManuallyReviewedMillis != 500 || !reviewed.Gaps[0].Reviewed {
		t.Fatalf("review %+v", reviewed)
	}
	if reviewed.CompleteWorkSeconds() == nil || *reviewed.CompleteWorkSeconds() != 0 {
		t.Fatal("reviewed work seconds")
	}
	again := review
	again.ID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	again.ExpectedVersion = reviewed.Version
	if _, e := reviewed.Apply(again, ClockPolicy{MaximumCheckpointGapMillis: 60_000}); !errors.Is(e, ErrConflict) {
		t.Fatal("already reviewed", e)
	}
}

func TestShiftClockDiscontinuityAndCheckpointGap(t *testing.T) {
	s := apply(t, newTestShift(t), command(opStart, ActionStart, 0, clock("boot-a", 1_000, 1_000_000))).Shift
	if out, e := s.Apply(command(opCheckpt, ActionCheckpoint, 1, clock("boot-a", 900, 999_000)), ClockPolicy{MaximumCheckpointGapMillis: 60_000}); e != nil || out.Shift.Gaps[0].Reason != GapClockDiscontinuity {
		t.Fatal("rollback", out, e)
	}
	if out, e := s.Apply(command(opCheckpt, ActionCheckpoint, 1, clock("boot-a", 100_000, 1_100_000)), ClockPolicy{MaximumCheckpointGapMillis: 60_000, WallToleranceMillis: 1_000}); e != nil || out.Shift.Gaps[0].Reason != GapCheckpoint {
		t.Fatal("checkpoint gap", out, e)
	}
	if out, e := s.Apply(command(opRecover, ActionRecover, 1, clock("boot-a", 2_000, 1_001_000)), ClockPolicy{MaximumCheckpointGapMillis: 60_000}); e != nil || out.Shift.Gaps[0].Reason != GapProcessRecovery {
		t.Fatal("recover gap", out, e)
	}
}

func TestShiftInvalidTransitionsAndInputs(t *testing.T) {
	s := newTestShift(t)
	if _, e := s.Apply(command(opPause, ActionPause, 0, clock("boot-a", 1_000, 1_000_000)), ClockPolicy{MaximumCheckpointGapMillis: 1}); !errors.Is(e, ErrConflict) {
		t.Fatal("pause before start", e)
	}
	if _, e := s.Apply(command(opStart, ActionStart, 0, Clock{BootID: "bad boot", ElapsedMillis: 1}), ClockPolicy{MaximumCheckpointGapMillis: 1}); !errors.Is(e, ErrInvalid) {
		t.Fatal("bad clock", e)
	}
	if _, e := s.Apply(ShiftCommand{ID: "not-a-uuid", Action: ActionStart, Clock: clock("boot-a", 1, 1)}, ClockPolicy{MaximumCheckpointGapMillis: 1}); !errors.Is(e, ErrInvalid) {
		t.Fatal("bad operation", e)
	}
	started := apply(t, s, command(opStart, ActionStart, 0, clock("boot-a", 1_000, 1_000_000))).Shift
	if _, e := started.Apply(command(opStart, ActionStart, 1, clock("boot-a", 2_000, 1_001_000)), ClockPolicy{MaximumCheckpointGapMillis: 60_000}); !errors.Is(e, ErrConflict) {
		t.Fatal("restart running shift", e)
	}
}
