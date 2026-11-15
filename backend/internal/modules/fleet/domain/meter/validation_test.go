package meter_test

import (
	"errors"
	"math/rand"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
)

func at(raw string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		panic(err)
	}
	return t
}

// Example A and B of fleet §10.4 on the default configuration.
func exampleSeries() []meter.Entry {
	return []meter.Entry{
		{Point: meter.Point{ID: validationID("R0"), Value: 183_000_000, OccurredAt: at("2026-11-01T12:00:00Z"), ReceivedAt: at("2026-11-01T12:00:01Z")}, State: meter.Valid},
		{Point: meter.Point{ID: validationID("R1"), Value: 183_400_000, OccurredAt: at("2026-11-03T11:00:00Z"), ReceivedAt: at("2026-11-03T11:00:05Z")}, State: meter.Valid},
		{Point: meter.Point{ID: validationID("R2"), Value: 183_932_000, OccurredAt: at("2026-11-05T11:00:00Z"), ReceivedAt: at("2026-11-05T11:00:05Z")}, State: meter.Valid},
	}
}

func TestValidationDefaultPolicyIsTheDocumentedConfiguration(t *testing.T) {
	p := meter.DefaultPlausibility()
	if p.DistancePerHour() != 150_000 || p.ClockSkewSeconds() != 300 {
		t.Fatalf("default plausibility %d/%d", p.DistancePerHour(), p.ClockSkewSeconds())
	}
	built, err := meter.NewPlausibility(meter.DefaultDistancePerHour, meter.DefaultClockSkewSeconds)
	if err != nil || built != p {
		t.Fatal("default differs from its constants", err)
	}
	var zero meter.Plausibility
	if _, err := zero.Check(meter.Odometer, exampleSeries(), meter.Point{ID: validationID("R3"), Value: 1, OccurredAt: at("2026-11-06T00:00:00Z"), ReceivedAt: at("2026-11-06T00:00:00Z")}); !errors.Is(err, meter.ErrInvalid) {
		t.Fatal("zero configuration accepted", err)
	}
	if _, err := zero.Reevaluate(meter.Odometer, exampleSeries()); !errors.Is(err, meter.ErrInvalid) {
		t.Fatal("zero configuration re-evaluated", err)
	}
	for _, s := range []string{p.String(), p.LogValue().String()} {
		if s != "EnterpriseMeterValue(redacted)" {
			t.Fatal("configuration diagnostics are not redacted")
		}
	}
}

// AC1: the retroactive reading is VALID with RETROACTIVE and the two new
// intervals sum to the previous one; AC2: the wrong reading is SUSPECT and
// leaves the latest valid reading where it was.
func TestValidationExamplesAAndB(t *testing.T) {
	p := meter.DefaultPlausibility()
	series := exampleSeries()
	r3 := meter.Point{ID: validationID("R3"), Value: 183_610_000, OccurredAt: at("2026-11-04T15:30:00Z"), ReceivedAt: at("2026-11-06T09:00:00Z")}
	v, err := p.Check(meter.Odometer, series, r3)
	if err != nil || v.State != meter.Valid || len(v.Flags) != 0 || !v.Retroactive || v.Previous != series[1].ID || v.Next != series[2].ID {
		t.Fatalf("example A %+v %v", v, err)
	}
	if (r3.Value-series[1].Value)+(series[2].Value-r3.Value) != 532_000 || r3.Value-series[1].Value != 210_000 || series[2].Value-r3.Value != 322_000 {
		t.Fatal("example A intervals")
	}
	r4 := meter.Point{ID: validationID("R4"), Value: 148_190_000, OccurredAt: at("2026-11-06T12:00:00Z"), ReceivedAt: at("2026-11-06T12:00:30Z")}
	v, err = p.Check(meter.Odometer, series, r4)
	if err != nil || v.State != meter.Suspect || !reflect.DeepEqual(v.Flags, []meter.Flag{meter.ReadingBelowPrevious}) || v.Retroactive || v.Previous != series[2].ID || v.Next != "" {
		t.Fatalf("example B %+v %v", v, err)
	}
}

// The validator never mutates or retains the caller's slices.
func TestValidationDoesNotMutateOrRetainInputs(t *testing.T) {
	p := meter.DefaultPlausibility()
	series := exampleSeries()
	series = append(series, meter.Entry{Point: meter.Point{ID: validationID("R4"), Value: 148_190_000, OccurredAt: at("2026-11-06T12:00:00Z"), ReceivedAt: at("2026-11-06T12:00:30Z")}, State: meter.Suspect, Flags: []meter.Flag{meter.ReadingBelowPrevious}})
	before := make([]meter.Entry, len(series))
	for i, e := range series {
		before[i] = e
		before[i].Flags = append([]meter.Flag(nil), e.Flags...)
	}
	changes, err := p.Reevaluate(meter.Odometer, series)
	if err != nil || len(changes) != 0 {
		t.Fatal(changes, err)
	}
	series[1].State = meter.Voided
	changes, err = p.Reevaluate(meter.Odometer, series)
	if err != nil || len(changes) != 0 {
		t.Fatal("voiding the earlier reading cannot clear a reading below R2", changes, err)
	}
	series[1].State = meter.Valid
	if !reflect.DeepEqual(series, before) {
		t.Fatal("re-evaluation mutated its input")
	}
	v, err := p.Check(meter.Odometer, series, meter.Point{ID: validationID("R5"), Value: 184_190_000, OccurredAt: at("2026-11-06T12:00:00Z"), ReceivedAt: at("2026-11-06T14:00:00Z")})
	if err != nil || v.State != meter.Valid {
		t.Fatal(v, err)
	}
	v.Flags = append(v.Flags, meter.FutureInstant)
	if !reflect.DeepEqual(series, before) {
		t.Fatal("verdict aliases the input")
	}
}

// Input order does not decide: any permutation of the same series is refused
// unless it is the canonical order, and the canonical order gives one answer.
func TestValidationRefusesEveryNonCanonicalPermutation(t *testing.T) {
	p := meter.DefaultPlausibility()
	series := exampleSeries()
	series = append(series, meter.Entry{Point: meter.Point{ID: validationID("R3"), Value: 183_932_000, OccurredAt: at("2026-11-05T11:00:00Z"), ReceivedAt: at("2026-11-05T11:00:05Z")}, State: meter.Suspect, Flags: []meter.Flag{meter.ImplausibleRate}})
	candidate := meter.Point{ID: validationID("R9"), Value: 184_000_000, OccurredAt: at("2026-11-06T00:00:00Z"), ReceivedAt: at("2026-11-06T00:00:01Z")}
	want, err := p.Check(meter.Odometer, series, candidate)
	if err != nil {
		t.Fatal(err)
	}
	r := rand.New(rand.NewSource(7))
	for i := 0; i < 64; i++ {
		shuffled := append([]meter.Entry(nil), series...)
		r.Shuffle(len(shuffled), func(a, b int) { shuffled[a], shuffled[b] = shuffled[b], shuffled[a] })
		canonical := sort.SliceIsSorted(shuffled, func(a, b int) bool {
			x, y := shuffled[a], shuffled[b]
			if !x.OccurredAt.Equal(y.OccurredAt) {
				return x.OccurredAt.Before(y.OccurredAt)
			}
			if !x.ReceivedAt.Equal(y.ReceivedAt) {
				return x.ReceivedAt.Before(y.ReceivedAt)
			}
			return x.ID < y.ID
		})
		got, err := p.Check(meter.Odometer, shuffled, candidate)
		if canonical {
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatal("canonical order changed the verdict", got, err)
			}
		} else if !errors.Is(err, meter.ErrInvalid) {
			t.Fatal("non-canonical order accepted", err)
		}
	}
}

// AC5 server side of the shared rule: a suspect verdict is a classification,
// never an error, so the reading is still stored.
func TestValidationSuspectIsAClassificationNotARefusal(t *testing.T) {
	p := meter.DefaultPlausibility()
	series := exampleSeries()
	for _, c := range []meter.Point{
		{ID: validationID("R7"), Value: 1, OccurredAt: at("2026-11-06T00:00:00Z"), ReceivedAt: at("2026-11-06T00:00:01Z")},
		{ID: validationID("R7"), Value: 999_999_999_999, OccurredAt: at("2026-11-06T00:00:00Z"), ReceivedAt: at("2026-11-06T00:00:01Z")},
		{ID: validationID("R7"), Value: 183_932_000, OccurredAt: at("2026-12-06T00:00:00Z"), ReceivedAt: at("2026-11-06T00:00:01Z")},
	} {
		v, err := p.Check(meter.Odometer, series, c)
		if err != nil || v.State != meter.Suspect || len(v.Flags) == 0 {
			t.Fatal("suspect reading refused or unflagged", v, err)
		}
	}
}
