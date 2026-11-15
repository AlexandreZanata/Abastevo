package meter

import (
	"math/big"
	"time"
)

// Flag is a reason for SUSPECT (fleet §10.5). It is never a refusal.
type Flag string

const (
	ReadingBelowPrevious Flag = "READING_BELOW_PREVIOUS"
	ReadingAboveNext     Flag = "READING_ABOVE_NEXT"
	ImplausibleRate      Flag = "IMPLAUSIBLE_RATE"
	FutureInstant        Flag = "FUTURE_INSTANT"
)

// flagOrder is the canonical order of stored and returned flags.
var flagOrder = [...]Flag{ReadingBelowPrevious, ReadingAboveNext, ImplausibleRate, FutureInstant}

// NoteRetroactive marks a reading placed before a later valid reading.
const NoteRetroactive = "RETROACTIVE"

// Plausibility limits of fleet §10.2 (B-BR-EMP-282/283). The tolerances of
// 1 000 m and 300 s are constants of the rule, not configuration.
const (
	DefaultDistancePerHour  int64 = 150_000
	MinDistancePerHour      int64 = 20_000
	MaxDistancePerHour      int64 = 200_000
	DefaultClockSkewSeconds int64 = 300
)

var (
	nanosPerSecond   = big.NewInt(int64(time.Second))
	secondsPerHour   = big.NewInt(3_600)
	displayTolerance = new(big.Int).Mul(big.NewInt(3_600_000), nanosPerSecond)
	hourTolerance    = new(big.Int).Mul(big.NewInt(300), nanosPerSecond)
)

// Plausibility is one validated configuration. The zero value is unusable.
type Plausibility struct {
	redacted
	distance int64
	skew     int64
	set      bool
}

func NewPlausibility(distancePerHour, clockSkewSeconds int64) (Plausibility, error) {
	if distancePerHour < MinDistancePerHour || distancePerHour > MaxDistancePerHour || clockSkewSeconds < 0 {
		return Plausibility{}, ErrInvalid
	}
	return Plausibility{distance: distancePerHour, skew: clockSkewSeconds, set: true}, nil
}
func DefaultPlausibility() Plausibility {
	return Plausibility{distance: DefaultDistancePerHour, skew: DefaultClockSkewSeconds, set: true}
}
func (p Plausibility) DistancePerHour() int64  { return p.distance }
func (p Plausibility) ClockSkewSeconds() int64 { return p.skew }

// Point is one observation of a segment by its ordering key and value.
type Point struct {
	ID         string
	Value      int64
	OccurredAt time.Time
	ReceivedAt time.Time
}

// Entry is a series member with its current classification.
type Entry struct {
	Point
	State State
	Flags []Flag
}

// Verdict is the classification of one reading against its valid neighbours.
// Previous and Next name those neighbours ("" when absent).
type Verdict struct {
	State       State
	Flags       []Flag
	Retroactive bool
	Previous    string
	Next        string
}

// Change is the final re-evaluated verdict of one SUSPECT series member.
type Change struct {
	ID      string
	Verdict Verdict
}

func before(a, b Point) bool {
	if !a.OccurredAt.Equal(b.OccurredAt) {
		return a.OccurredAt.Before(b.OccurredAt)
	}
	if !a.ReceivedAt.Equal(b.ReceivedAt) {
		return a.ReceivedAt.Before(b.ReceivedAt)
	}
	return a.ID < b.ID
}

func (p Plausibility) validPoint(kind Kind, v Point) bool {
	return v.ID != "" && v.Value >= 0 && v.Value <= bound(kind) && validInstant(v.OccurredAt) && validInstant(v.ReceivedAt)
}

func canonicalFlags(state State, flags []Flag) bool {
	switch state {
	case Valid, Voided:
		return len(flags) == 0
	case Suspect:
		if len(flags) == 0 {
			return false
		}
	default:
		return false
	}
	next := 0
	for _, f := range flags {
		found := false
		for next < len(flagOrder) {
			next++
			if flagOrder[next-1] == f {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// series copies and validates a caller series: non-empty, strictly ordered,
// unique identifiers, bounded values and canonical states and flags.
func (p Plausibility) series(kind Kind, in []Entry) ([]Entry, error) {
	if !p.set || bound(kind) < 0 || len(in) == 0 {
		return nil, ErrInvalid
	}
	out := make([]Entry, len(in))
	seen := make(map[string]bool, len(in))
	for i, e := range in {
		if !p.validPoint(kind, e.Point) || seen[e.ID] || !canonicalFlags(e.State, e.Flags) || i > 0 && !before(in[i-1].Point, e.Point) {
			return nil, ErrInvalid
		}
		seen[e.ID] = true
		out[i] = Entry{Point: e.Point, State: e.State, Flags: append([]Flag(nil), e.Flags...)}
	}
	return out, nil
}

// elapsed is the exact signed number of nanoseconds from a to b.
func elapsed(a, b time.Time) *big.Int {
	d := new(big.Int).Mul(big.NewInt(b.Unix()-a.Unix()), nanosPerSecond)
	return d.Add(d, big.NewInt(int64(b.Nanosecond()-a.Nanosecond())))
}

// plausible applies §10.2 to a non-decreasing ordered pair.
func (p Plausibility) plausible(kind Kind, a, b Point) bool {
	delta := big.NewInt(b.Value - a.Value)
	dt := elapsed(a.OccurredAt, b.OccurredAt)
	if kind == Odometer {
		left := new(big.Int).Mul(delta, secondsPerHour)
		left.Mul(left, nanosPerSecond)
		right := new(big.Int).Mul(big.NewInt(p.distance), dt)
		right.Add(right, displayTolerance)
		return left.Cmp(right) <= 0
	}
	left := new(big.Int).Mul(delta, nanosPerSecond)
	right := new(big.Int).Add(dt, hourTolerance)
	return left.Cmp(right) <= 0
}

// future reports occurred_at strictly later than received_at plus the skew,
// exactly, for every bounded instant and every non-negative skew.
func (p Plausibility) future(v Point) bool {
	limit := new(big.Int).Mul(big.NewInt(p.skew), nanosPerSecond)
	return elapsed(v.ReceivedAt, v.OccurredAt).Cmp(limit) > 0
}

// verdict classifies c against the VALID members of series (which must not
// contain c). keepFuture is nil to decide FUTURE_INSTANT from c's receipt.
func (p Plausibility) verdict(kind Kind, series []Entry, c Point, keepFuture *bool) Verdict {
	var prev, next *Point
	for i := range series {
		e := &series[i]
		if e.State != Valid {
			continue
		}
		if before(e.Point, c) {
			prev = &e.Point
		} else if next == nil {
			next = &e.Point
		}
	}
	raised := map[Flag]bool{}
	if prev != nil {
		if c.Value < prev.Value {
			raised[ReadingBelowPrevious] = true
		} else if !p.plausible(kind, *prev, c) {
			raised[ImplausibleRate] = true
		}
	}
	if next != nil {
		if next.Value < c.Value {
			raised[ReadingAboveNext] = true
		} else if !p.plausible(kind, c, *next) {
			raised[ImplausibleRate] = true
		}
	}
	if keepFuture == nil && p.future(c) || keepFuture != nil && *keepFuture {
		raised[FutureInstant] = true
	}
	v := Verdict{State: Valid, Retroactive: next != nil}
	for _, f := range flagOrder {
		if raised[f] {
			v.Flags = append(v.Flags, f)
		}
	}
	if len(v.Flags) > 0 {
		v.State = Suspect
	}
	if prev != nil {
		v.Previous = prev.ID
	}
	if next != nil {
		v.Next = next.ID
	}
	return v
}

// Check classifies a candidate against the ordered segment series, whose
// first entry is the opening reading (B-BR-EMP-280..284).
func (p Plausibility) Check(kind Kind, series []Entry, candidate Point) (Verdict, error) {
	members, err := p.series(kind, series)
	if err != nil {
		return Verdict{}, err
	}
	if !p.validPoint(kind, candidate) || !before(members[0].Point, candidate) {
		return Verdict{}, ErrInvalid
	}
	for _, e := range members {
		if e.ID == candidate.ID {
			return Verdict{}, ErrInvalid
		}
	}
	return p.verdict(kind, members, candidate, nil), nil
}

func sameFlags(a, b []Flag) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Reevaluate re-checks every SUSPECT member until no promotion happens
// (B-BR-EMP-285) and returns the members whose state or flags changed.
func (p Plausibility) Reevaluate(kind Kind, series []Entry) ([]Change, error) {
	members, err := p.series(kind, series)
	if err != nil {
		return nil, err
	}
	final := map[string]Verdict{}
	for promoted := true; promoted; {
		promoted = false
		for i := range members {
			e := &members[i]
			if e.State != Suspect {
				continue
			}
			future := false
			for _, f := range e.Flags {
				future = future || f == FutureInstant
			}
			others := append(append([]Entry(nil), members[:i]...), members[i+1:]...)
			v := p.verdict(kind, others, e.Point, &future)
			if v.State == e.State && sameFlags(v.Flags, e.Flags) {
				continue
			}
			e.State, e.Flags = v.State, append([]Flag(nil), v.Flags...)
			final[e.ID] = v
			promoted = promoted || v.State == Valid
		}
	}
	var out []Change
	for _, original := range series {
		if v, ok := final[original.ID]; ok && (v.State != original.State || !sameFlags(v.Flags, original.Flags)) {
			out = append(out, Change{ID: original.ID, Verdict: v})
		}
	}
	return out, nil
}
