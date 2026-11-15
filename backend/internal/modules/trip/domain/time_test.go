package domain_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

func runClock(boot string, ms int64) *trip.RunClock {
	c := must(trip.NewRunClock(boot, ms))
	return &c
}

func TestRunClockProfileAndZeroValue(t *testing.T) {
	defer guard(t)
	c := must(trip.NewRunClock("boot-a1", 7))
	if c.BootID() != "boot-a1" || c.ElapsedMillis() != 7 {
		t.Fatal("run clock changed its inputs")
	}
	for _, boot := range []string{"", strings.Repeat("b", 65), "boot 1", "bóot", "boot\n"} {
		if _, e := trip.NewRunClock(boot, 0); !errors.Is(e, trip.ErrInvalid) {
			t.Fatal("malformed boot identity accepted", boot)
		}
	}
	if _, e := trip.NewRunClock("boot-a1", -1); !errors.Is(e, trip.ErrInvalid) {
		t.Fatal("negative monotonic count accepted")
	}
	s := scope(t, 1)
	in := startInput(t, s)
	in.Clock = &trip.RunClock{}
	if _, e := trip.Start(s, in); !errors.Is(e, trip.ErrInvalid) {
		t.Fatal("an unconstructed start clock was accepted", e)
	}
	started := must(trip.Start(s, startInput(t, s)))
	fin := finishInput(t, s, started)
	fin.Clock = &trip.RunClock{}
	if _, e := started.Finish(s, fin); !errors.Is(e, trip.ErrInvalid) {
		t.Fatal("an unconstructed finish clock was accepted", e)
	}
}

func TestClocksAreCopiedAndEventsKeepTheirContext(t *testing.T) {
	defer guard(t)
	s := scope(t, 1)
	in := startInput(t, s)
	in.Clock = runClock("boot-a1", 1_000)
	started := must(trip.Start(s, in))
	*in.Clock = must(trip.NewRunClock("boot-zz", 5))
	got := started.Started().Clock()
	if got == nil || got.BootID() != "boot-a1" || got.ElapsedMillis() != 1_000 {
		t.Fatal("start retained the caller's mutable clock")
	}
	*got = trip.RunClock{}
	if started.Started().Clock().BootID() != "boot-a1" {
		t.Fatal("an accessor exposed the stored clock")
	}
	fin := finishInput(t, s, started)
	fin.Declared = &orgtime.Declaration{OccurredAt: must(orgtime.ParseInstant("2026-11-15T04:10:00Z")), EntryZone: "America/Sao_Paulo",
		DeviceOffsetSeconds: -10800}
	done := must(started.Finish(s, fin))
	end, _ := done.Finished()
	if end.Clock() != nil || done.Reporting() != started.Reporting() || end.Declared().EntryZone != "America/Sao_Paulo" ||
		end.Declared().DeviceOffsetSeconds != -10800 || done.Started().Declared().EntryZone != "America/Cuiaba" {
		t.Fatal("events must keep their own source context and the trip its start attribution")
	}
	c := done.Crossing()
	if !c.Finished() || c.Day() != "2026-11-14" || c.EndDay() != "2026-11-15" || !c.CrossesMidnight() || !c.EntryZoneChanged() ||
		!c.OffsetChanged() || done.Reference() != started.Reference() {
		t.Fatal("midnight and zone crossing must be reported on one trip")
	}
	if c := started.Crossing(); c.Finished() || c.EndDay() != "" || c.CrossesMidnight() || c.Day() != "2026-11-14" {
		t.Fatal("an unfinished trip has no end day")
	}
	el := must(done.Elapsed(0))
	if seconds, nanos, ok := el.Duration(); !ok || seconds != 58_200 || nanos != 0 || el.Status() != trip.ElapsedDeclared || el.Gap() != "" {
		t.Fatal("elapsed time must be the instant difference", seconds, nanos, ok)
	}
}

func TestElapsedNeverInventsDuration(t *testing.T) {
	defer guard(t)
	var zero trip.Trip
	if el := must(zero.Elapsed(0)); el.Status() != trip.ElapsedNotFinished {
		t.Fatal("an unconstructed trip has no elapsed time")
	}
	if _, e := zero.Elapsed(-1); !errors.Is(e, trip.ErrInvalid) {
		t.Fatal("a negative tolerance was accepted")
	}
	if zero.Crossing().Finished() {
		t.Fatal("an unconstructed trip has no crossing")
	}
	s := scope(t, 1)
	in := startInput(t, s)
	in.Clock = runClock("boot-a1", 0)
	started := must(trip.Start(s, in))
	fin := finishInput(t, s, started)
	fin.Clock = runClock("boot-b2", 10_800_000)
	el := must(must(started.Finish(s, fin)).Elapsed(2_000))
	if _, _, ok := el.Duration(); ok || el.Status() != trip.ElapsedGap || el.Gap() != trip.GapBootChanged {
		t.Fatal("a reboot must be a gap without a duration")
	}
	fin.Clock = runClock("boot-a1", 21_600_000)
	el = must(must(started.Finish(s, fin)).Elapsed(2_000))
	if _, _, ok := el.Duration(); ok || el.Gap() != trip.GapClockDiscontinuity {
		t.Fatal("a wall-clock jump must be a gap without a duration")
	}
	for _, v := range []any{el, *runClock("boot-a1", 0)} {
		text := fmt.Sprintf("%v %+v", v, v)
		raw, _ := json.Marshal(v)
		if !strings.Contains(text, "redacted") || strings.Contains(text+string(raw), "boot-a1") || strings.Contains(text, "CLOCK") {
			t.Fatal("time value printed without redaction", text, string(raw))
		}
	}
}
