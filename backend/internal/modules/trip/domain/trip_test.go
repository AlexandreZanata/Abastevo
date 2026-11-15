package domain_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

func scope(t *testing.T, n int) kernel.OrgScope {
	t.Helper()
	id, e := kernel.ParseOrgID(uuid(n))
	if e != nil {
		t.Fatal(e)
	}
	s, e := kernel.NewOrgScope(id)
	if e != nil {
		t.Fatal(e)
	}
	return s
}

// must panics on a fixture construction error; every test that uses it
// defers guard, which turns that panic into an ordinary test failure.
func must[T any](v T, e error) T {
	if e != nil {
		panic(e)
	}
	return v
}

func guard(t *testing.T) {
	t.Helper()
	if r := recover(); r != nil {
		t.Fatalf("fixture construction failed: %v", r)
	}
}

func assetRef(t *testing.T, s kernel.OrgScope, n int) fleet.AssetRef {
	t.Helper()
	return must(fleet.NewAssetRef(s, must(fleet.ParseAssetID(uuid(n)))))
}

func readingRef(t *testing.T, s kernel.OrgScope, asset fleet.AssetRef, meterN, segmentN, readingN int, kind meter.Kind) meter.ReadingRef {
	t.Helper()
	m := must(meter.NewMeterRef(s, asset, uuid(meterN), kind, nil))
	seg := must(meter.NewSegmentRef(s, m, uuid(segmentN)))
	return must(meter.NewReadingRef(s, seg, uuid(readingN)))
}

func observed(t *testing.T, ref meter.ReadingRef, count int64) trip.MeterObservation {
	t.Helper()
	return must(trip.ObservedReading(ref, must(meter.NewValue(ref.Segment().Meter().Kind(), count, nil))))
}

func at(t *testing.T, raw string) *orgtime.Declaration {
	t.Helper()
	return &orgtime.Declaration{OccurredAt: must(orgtime.ParseInstant(raw)), EntryZone: "America/Cuiaba", DeviceOffsetSeconds: -14400}
}

// startInput is the VAL-01 departure of organization A (aliases of vectors_test.go).
func startInput(t *testing.T, s kernel.OrgScope) trip.StartInput {
	t.Helper()
	asset := assetRef(t, s, 21)
	return trip.StartInput{
		Reference: must(trip.NewTripRef(s, uuid(11))),
		Author:    must(trip.NewMembershipRef(s, uuid(41))),
		Asset:     asset,
		Driver:    must(trip.NewEmployeeRef(s, uuid(31))),
		Departure: observed(t, readingRef(t, s, asset, 51, 61, 71, meter.Odometer), 184_320_000),
		Declared:  at(t, "2026-11-14T12:00:00Z"),
		Zones:     orgtime.ReportingZones{Organization: "America/Cuiaba"},
	}
}

func finishInput(t *testing.T, s kernel.OrgScope, tr trip.Trip) trip.FinishInput {
	t.Helper()
	return trip.FinishInput{
		Author:   must(trip.NewMembershipRef(s, uuid(41))),
		Final:    observed(t, readingRef(t, s, tr.Asset(), 51, 61, 72, meter.Odometer), 184_610_000),
		Declared: at(t, "2026-11-14T15:00:00Z"),
	}
}

func TestClosedVocabulariesHaveNoDefaults(t *testing.T) {
	defer guard(t)
	states := []trip.State{trip.Planned, trip.Assigned, trip.Accepted, trip.InProgress, trip.Paused, trip.Completed, trip.Cancelled}
	for _, s := range states {
		if got, e := trip.ParseState(string(s)); e != nil || got != s {
			t.Fatal("catalog state refused", s, e)
		}
		if s.Terminal() != (s == trip.Completed || s == trip.Cancelled) {
			t.Fatal("terminal states are exactly COMPLETED and CANCELLED", s)
		}
	}
	for _, c := range []trip.Command{trip.CommandStart, trip.CommandFinish, trip.CommandPause, trip.CommandResume, trip.CommandCancel} {
		if got, e := trip.ParseCommand(string(c)); e != nil || got != c {
			t.Fatal("command refused", c, e)
		}
	}
	for _, r := range []trip.ReviewState{trip.ReviewDraft, trip.ReviewSubmitted, trip.ReviewInReview, trip.ReviewApproved,
		trip.ReviewReturned, trip.ReviewRejected, trip.ReviewCancelled, trip.ReviewVoided} {
		if got, e := trip.ParseReviewState(string(r)); e != nil || got != r {
			t.Fatal("review state refused", r, e)
		}
	}
	for _, raw := range []string{"", "unknown", " IN_PROGRESS", "APPROVED_X"} {
		if _, e := trip.ParseState(raw); !errors.Is(e, trip.ErrInvalid) {
			t.Fatal("undeclared state accepted", raw)
		}
		if _, e := trip.ParseCommand(raw); !errors.Is(e, trip.ErrInvalid) {
			t.Fatal("undeclared command accepted", raw)
		}
		if _, e := trip.ParseReviewState(raw); !errors.Is(e, trip.ErrInvalid) {
			t.Fatal("undeclared review state accepted", raw)
		}
	}
	if trip.State("").Terminal() || trip.State("FINISHED").Terminal() {
		t.Fatal("an unknown state cannot be terminal")
	}
	if _, e := trip.Transition(trip.State("FINISHED"), trip.CommandFinish); !errors.Is(e, trip.ErrInvalid) {
		t.Fatal("unknown state reached the table")
	}
	if _, e := trip.Transition(trip.InProgress, trip.Command("APPROVE")); !errors.Is(e, trip.ErrInvalid) {
		t.Fatal("unknown command reached the table")
	}
}

func TestStandingSeparatesCompletionFromApproval(t *testing.T) {
	defer guard(t)
	states := []trip.State{trip.Planned, trip.Assigned, trip.Accepted, trip.InProgress, trip.Paused, trip.Completed, trip.Cancelled}
	reviews := []trip.ReviewState{trip.ReviewDraft, trip.ReviewSubmitted, trip.ReviewInReview, trip.ReviewApproved,
		trip.ReviewReturned, trip.ReviewRejected, trip.ReviewCancelled, trip.ReviewVoided}
	for _, s := range states {
		for _, r := range reviews {
			st, e := trip.NewStanding(s, r)
			if e != nil || st.Operational() != s || st.Review() != r {
				t.Fatal("standing changed its inputs", s, r, e)
			}
			if st.Completed() != (s == trip.Completed) || st.Approved() != (r == trip.ReviewApproved) {
				t.Fatal("completion and approval are independent", s, r)
			}
		}
	}
	if _, e := trip.NewStanding(trip.State("APPROVED"), trip.ReviewApproved); !errors.Is(e, trip.ErrInvalid) {
		t.Fatal("approval accepted as an operational state")
	}
	if _, e := trip.NewStanding(trip.Completed, trip.ReviewState("COMPLETED")); !errors.Is(e, trip.ErrInvalid) {
		t.Fatal("completion accepted as a review state")
	}
	if st := (trip.Standing{}); st.Completed() || st.Approved() {
		t.Fatal("a zero standing claims completion or approval")
	}
}

func TestValueIsExactlyTheWorkedExample(t *testing.T) {
	defer guard(t)
	s := scope(t, 1)
	started := must(trip.Start(s, startInput(t, s)))
	if started.State() != trip.InProgress || started.MeterUsage().Status() != trip.NotMeasured ||
		started.MeterUsage().Reason() != trip.ReasonNotFinished {
		t.Fatal("an in-progress trip has no usage")
	}
	if _, ok := started.MeterUsage().Delta(); ok {
		t.Fatal("unfinished usage invented a delta")
	}
	done := must(started.Finish(s, finishInput(t, s, started)))
	usage := done.MeterUsage()
	delta, ok := usage.Delta()
	if done.State() != trip.Completed || usage.Status() != trip.Measured || !ok || delta != 290_000 || usage.Kind() != meter.Odometer ||
		usage.Reason() != "" {
		t.Fatal("VAL-01 trip must record 290 000 m of meter evidence", delta, ok)
	}
	if done.Reporting() != started.Reporting() || done.Reference() != started.Reference() || done.Asset() != started.Asset() ||
		done.Driver() != started.Driver() || done.OrganizationID() != s.OrgID() {
		t.Fatal("finish changed the trip identity or context")
	}
	if st := must(trip.NewStanding(done.State(), trip.ReviewDraft)); !st.Completed() || st.Approved() {
		t.Fatal("a completed trip was taken as approved")
	}
}

func TestInputsAreCopiedAndResultsAreValues(t *testing.T) {
	defer guard(t)
	s := scope(t, 1)
	in := startInput(t, s)
	started := must(trip.Start(s, in))
	in.Declared.OccurredAt = in.Declared.OccurredAt.Add(time.Hour)
	in.Declared.EntryZone = "UTC"
	if got := started.Started().Declared(); !got.OccurredAt.Equal(must(orgtime.ParseInstant("2026-11-14T12:00:00Z"))) ||
		got.EntryZone != "America/Cuiaba" {
		t.Fatal("start retained the caller's mutable declaration")
	}
	fin := finishInput(t, s, started)
	done := must(started.Finish(s, fin))
	fin.Declared.OccurredAt = fin.Declared.OccurredAt.Add(time.Hour)
	end, ok := done.Finished()
	if !ok || !end.Declared().OccurredAt.Equal(must(orgtime.ParseInstant("2026-11-14T15:00:00Z"))) {
		t.Fatal("finish retained the caller's mutable declaration")
	}
	if _, ok := started.Finished(); ok || started.State() != trip.InProgress {
		t.Fatal("finish mutated the receiver")
	}
	r := end.Evidence().Reading()
	*r = meter.ReadingRef{}
	if end.Evidence().Reading() == nil || *end.Evidence().Reading() == (meter.ReadingRef{}) {
		t.Fatal("an accessor exposed internal evidence")
	}
	absence := must(meter.NewMissing("Painel apagado"))
	o := must(trip.AbsentReading(absence))
	a := o.Absence()
	*a = meter.Missing{}
	if o.Absence().Reason() != "Painel apagado" || o.Reading() != nil {
		t.Fatal("an accessor exposed the absence reason")
	}
	if _, ok := o.Value(); ok {
		t.Fatal("an absence carries no value")
	}
	if e := must(trip.Start(s, startInput(t, s))).Started().Author(); e.ID() != uuid(41) || e.OrganizationID() != s.OrgID() {
		t.Fatal("the author reference changed")
	}
}

func TestObservationsAreExactAndExclusive(t *testing.T) {
	defer guard(t)
	s := scope(t, 1)
	asset := assetRef(t, s, 21)
	ref := readingRef(t, s, asset, 51, 61, 71, meter.Odometer)
	if _, e := trip.ObservedReading(meter.ReadingRef{}, must(meter.NewValue(meter.Odometer, 1, nil))); !errors.Is(e, trip.ErrInvalid) {
		t.Fatal("unconstructed reading accepted")
	}
	if _, e := trip.ObservedReading(ref, meter.Value{}); !errors.Is(e, trip.ErrInvalid) {
		t.Fatal("unconstructed value accepted")
	}
	if _, e := trip.ObservedReading(ref, must(meter.NewValue(meter.HourMeter, 1, nil))); !errors.Is(e, trip.ErrInvalid) {
		t.Fatal("value of another kind accepted")
	}
	modulus := int64(1000)
	bounded := must(meter.NewReadingRef(s, must(meter.NewSegmentRef(s, must(meter.NewMeterRef(s, asset, uuid(52), meter.Odometer, &modulus)), uuid(62))), uuid(72)))
	if _, e := trip.ObservedReading(bounded, must(meter.NewValue(meter.Odometer, 1000, nil))); !errors.Is(e, trip.ErrInvalid) {
		t.Fatal("value at the meter modulus accepted")
	}
	if _, e := trip.AbsentReading(meter.Missing{}); !errors.Is(e, trip.ErrInvalid) {
		t.Fatal("an absence without a reason accepted")
	}
	o := observed(t, ref, 7)
	if v, ok := o.Value(); !ok || v.Count() != 7 || o.Absence() != nil || *o.Reading() != ref {
		t.Fatal("observed reading changed")
	}
	var zero trip.MeterObservation
	if zero.Reading() != nil || zero.Absence() != nil {
		t.Fatal("the zero observation must be 'not supplied'")
	}
}

func TestZeroTripAndForeignFinish(t *testing.T) {
	defer guard(t)
	s, foreign := scope(t, 1), scope(t, 2)
	var zero trip.Trip
	if _, e := zero.Finish(s, trip.FinishInput{}); !errors.Is(e, trip.ErrInvalid) {
		t.Fatal("an unconstructed trip finished", e)
	}
	if zero.MeterUsage().Status() != trip.NotMeasured || zero.MeterUsage().Reason() != trip.ReasonNotFinished {
		t.Fatal("an unconstructed trip invented usage")
	}
	if _, ok := zero.Finished(); ok {
		t.Fatal("an unconstructed trip claims a finish")
	}
	started := must(trip.Start(s, startInput(t, s)))
	if _, e := started.Finish(kernel.OrgScope{}, finishInput(t, s, started)); !errors.Is(e, trip.ErrScope) {
		t.Fatal("finish without a scope accepted")
	}
	if _, e := started.Finish(foreign, finishInput(t, s, started)); !errors.Is(e, trip.ErrScope) {
		t.Fatal("finish from another organization accepted")
	}
	if _, e := trip.Start(kernel.OrgScope{}, startInput(t, s)); !errors.Is(e, trip.ErrScope) {
		t.Fatal("start without a scope accepted")
	}
	if _, e := trip.NewTripRef(kernel.OrgScope{}, uuid(11)); !errors.Is(e, trip.ErrScope) {
		t.Fatal("reference without a scope accepted")
	}
	if _, e := trip.NewEmployeeRef(s, "not-a-uuid"); !errors.Is(e, trip.ErrInvalid) {
		t.Fatal("malformed reference accepted")
	}
	in := startInput(t, s)
	in.Driver = trip.EmployeeRef{}
	in.Asset = fleet.AssetRef{}
	var incomplete *trip.IncompleteError
	_, e := trip.Start(s, in)
	if !errors.As(e, &incomplete) || !errors.Is(e, trip.ErrIncomplete) || e.Error() != trip.ErrIncomplete.Error() {
		t.Fatal("absent context must be INCOMPLETE", e)
	}
	listed := incomplete.Missing()
	listed[0] = trip.MissingFinal
	if got := incomplete.Missing(); len(got) != 2 || got[0] != trip.MissingAsset || got[1] != trip.MissingDriver {
		t.Fatal("missing list is not an ordered copy", got)
	}
}

func TestConcurrentFinishesShareOneImmutableTrip(t *testing.T) {
	defer guard(t)
	s := scope(t, 1)
	started := must(trip.Start(s, startInput(t, s)))
	good := finishInput(t, s, started)
	other := finishInput(t, s, started)
	other.Author = must(trip.NewMembershipRef(s, uuid(42)))
	var wg sync.WaitGroup
	results := make([]error, 32)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			in := good
			if i%2 == 1 {
				in = other
			}
			done, e := started.Finish(s, in)
			switch {
			case i%2 == 1 && !errors.Is(e, trip.ErrForbidden):
				results[i] = fmt.Errorf("another member finished: %v", e)
			case i%2 == 0 && (e != nil || done.State() != trip.Completed):
				results[i] = fmt.Errorf("the starting member could not finish: %v", e)
			}
		}(i)
	}
	wg.Wait()
	for _, e := range results {
		if e != nil {
			t.Fatal(e)
		}
	}
	if started.State() != trip.InProgress {
		t.Fatal("a shared trip value changed")
	}
}

func TestPrivateValuesAreRedacted(t *testing.T) {
	defer guard(t)
	s := scope(t, 1)
	in := startInput(t, s)
	started := must(trip.Start(s, in))
	done := must(started.Finish(s, finishInput(t, s, started)))
	end, _ := done.Finished()
	absent := must(trip.AbsentReading(must(meter.NewMissing("Painel apagado"))))
	values := []any{in, finishInput(t, s, started), started, done, done.Started(), end, end.Evidence(), absent, done.MeterUsage(),
		in.Reference, in.Author, in.Driver}
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	for _, v := range values {
		for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
			assertRedacted(t, fmt.Sprintf(format, v))
		}
		raw, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		assertRedacted(t, string(raw))
		logger.Info("trip", "value", v)
	}
	assertRedacted(t, logs.String())
}

func assertRedacted(t *testing.T, out string) {
	t.Helper()
	if !strings.Contains(out, "redacted") {
		t.Fatal("value printed without redaction", out)
	}
	for _, secret := range []string{"5e000000", "184320000", "184610000", "290000", "Painel", "Cuiaba"} {
		if strings.Contains(out, secret) {
			t.Fatal("private value leaked", out)
		}
	}
}
