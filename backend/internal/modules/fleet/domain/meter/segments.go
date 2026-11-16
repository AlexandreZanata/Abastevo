package meter

import (
	"math"
	"sort"
	"time"
)

// SegmentReason is why a segment opened (fleet §3, B-BR-EMP-481).
type SegmentReason string

const (
	SegmentInstallation SegmentReason = "INSTALLATION"
	SegmentReset        SegmentReason = "RESET"
	SegmentRollover     SegmentReason = "ROLLOVER"
)

// Insufficient names why a figure is INSUFFICIENT_DATA (fleet §10.5).
type Insufficient string

const (
	NoReadingBefore Insufficient = "NO_READING_BEFORE"
	MeterGap        Insufficient = "METER_GAP"
)

// Span is one segment of an asset's track for one kind. Meter names the
// meter it belongs to; Close is nil while open or when a removal closed it
// with an unknown final value.
type Span struct {
	ID       string
	Meter    string
	Reason   SegmentReason
	OpenedAt time.Time
	Open     int64
	Closed   bool
	ClosedAt time.Time
	Close    *int64
}

// Anchor is one VALID reading of a span.
type Anchor struct {
	ID         string
	Segment    string
	Value      int64
	OccurredAt time.Time
	ReceivedAt time.Time
}

// Track is a validated chain of segments of one kind with its VALID
// readings (B-BR-EMP-482). The zero value is unusable.
type Track struct {
	redacted
	kind  Kind
	spans []Span
	// ordered holds every anchor in span order, then BR-METER-008 order;
	// span is the anchor's span index.
	ordered []trackAnchor
}

type trackAnchor struct {
	Anchor
	span int
}

// Figure is a usage figure with the anchors it used. Reason is empty when
// the figure is known; From is empty for a cumulative figure.
type Figure struct {
	Reason     Insufficient
	Value      int64
	Equivalent int64
	From       string
	To         string
}

func (f Figure) Known() bool { return f.Reason == "" }

// NewTrack validates spans in order (meters in installation order) and
// their VALID anchors (B-BR-EMP-482, EDR-314).
func NewTrack(kind Kind, spans []Span, anchors []Anchor) (Track, error) {
	limit := bound(kind)
	if limit < 0 || len(spans) == 0 {
		return Track{}, ErrInvalid
	}
	index := make(map[string]int, len(spans))
	finished := map[string]bool{}
	out := make([]Span, len(spans))
	for i, s := range spans {
		if s.ID == "" || s.Meter == "" || !validInstant(s.OpenedAt) || s.Open < 0 || s.Open > limit {
			return Track{}, ErrInvalid
		}
		if _, dup := index[s.ID]; dup {
			return Track{}, ErrInvalid
		}
		index[s.ID] = i
		if s.Closed {
			if !validInstant(s.ClosedAt) || !s.ClosedAt.After(s.OpenedAt) || s.Close != nil && (*s.Close < s.Open || *s.Close > limit) {
				return Track{}, ErrInvalid
			}
		} else if s.Close != nil || i < len(spans)-1 {
			return Track{}, ErrInvalid
		}
		if i == 0 || s.Meter != spans[i-1].Meter {
			if s.Reason != SegmentInstallation || finished[s.Meter] || i > 0 && s.OpenedAt.Before(spans[i-1].ClosedAt) {
				return Track{}, ErrInvalid
			}
			if i > 0 {
				finished[spans[i-1].Meter] = true
			}
		} else {
			previous := spans[i-1]
			if s.Reason != SegmentReset && s.Reason != SegmentRollover || previous.Close == nil || !s.OpenedAt.Equal(previous.ClosedAt) || s.Reason == SegmentRollover && s.Open != 0 {
				return Track{}, ErrInvalid
			}
		}
		lastOfMeter := i == len(spans)-1 || spans[i+1].Meter != s.Meter
		if s.Closed && s.Close == nil && !lastOfMeter {
			return Track{}, ErrInvalid
		}
		out[i] = s
		if s.Close != nil {
			v := *s.Close
			out[i].Close = &v
		}
	}
	seen := make(map[string]bool, len(anchors))
	ordered := make([]trackAnchor, 0, len(anchors))
	for _, a := range anchors {
		i, ok := index[a.Segment]
		if !ok || a.ID == "" || seen[a.ID] || !validInstant(a.OccurredAt) || !validInstant(a.ReceivedAt) {
			return Track{}, ErrInvalid
		}
		seen[a.ID] = true
		s := out[i]
		if a.Value < s.Open || a.Value > limit || s.Close != nil && a.Value > *s.Close || a.OccurredAt.Before(s.OpenedAt) || s.Closed && a.OccurredAt.After(s.ClosedAt) {
			return Track{}, ErrInvalid
		}
		ordered = append(ordered, trackAnchor{Anchor: a, span: i})
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].span != ordered[j].span {
			return ordered[i].span < ordered[j].span
		}
		return anchorBefore(ordered[i].Anchor, ordered[j].Anchor)
	})
	for i := 1; i < len(ordered); i++ {
		if ordered[i].span == ordered[i-1].span && ordered[i].Value < ordered[i-1].Value {
			return Track{}, ErrInvalid
		}
	}
	return Track{kind: kind, spans: out, ordered: ordered}, nil
}

func anchorBefore(a, b Anchor) bool {
	return before(Point{ID: a.ID, OccurredAt: a.OccurredAt, ReceivedAt: a.ReceivedAt}, Point{ID: b.ID, OccurredAt: b.OccurredAt, ReceivedAt: b.ReceivedAt})
}

func (t Track) valid() bool { return bound(t.kind) >= 0 && len(t.spans) > 0 }

// anchor is the latest anchor at or before at, or nil.
func (t Track) anchor(at time.Time) *trackAnchor {
	var best *trackAnchor
	for i := range t.ordered {
		if !t.ordered[i].OccurredAt.After(at) {
			best = &t.ordered[i]
		}
	}
	return best
}

// add sums without overflow; false when the exact sum leaves int64.
func add(a, b int64) (int64, bool) {
	if b > 0 && a > math.MaxInt64-b || b < 0 && a < math.MinInt64-b {
		return 0, false
	}
	return a + b, true
}

// closedUsage sums close − open over spans [from, to); false with a gap.
func (t Track) closedUsage(from, to int) (int64, bool, error) {
	var total int64
	for k := from; k < to; k++ {
		s := t.spans[k]
		if s.Close == nil {
			return 0, false, nil
		}
		var ok bool
		if total, ok = add(total, *s.Close-s.Open); !ok {
			return 0, false, ErrInvalid
		}
	}
	return total, true, nil
}

// Cumulative is U(t) with its anchor and the equivalent display E(t).
func (t Track) Cumulative(at time.Time) (Figure, error) {
	if !t.valid() || !validInstant(at) {
		return Figure{}, ErrInvalid
	}
	a := t.anchor(at)
	if a == nil {
		return Figure{Reason: NoReadingBefore}, nil
	}
	sum, known, err := t.closedUsage(0, a.span)
	if err != nil {
		return Figure{}, err
	}
	if !known {
		return Figure{Reason: MeterGap, To: a.ID}, nil
	}
	value, ok := add(sum, a.Value-t.spans[a.span].Open)
	if !ok {
		return Figure{}, ErrInvalid
	}
	equivalent, ok := add(t.spans[0].Open, value)
	if !ok {
		return Figure{}, ErrInvalid
	}
	return Figure{Value: value, Equivalent: equivalent, To: a.ID}, nil
}

// Between is usage(t1, t2) with both anchors; nothing is interpolated.
func (t Track) Between(from, to time.Time) (Figure, error) {
	if !t.valid() || !validInstant(from) || !validInstant(to) || from.After(to) {
		return Figure{}, ErrInvalid
	}
	a, b := t.anchor(from), t.anchor(to)
	f := Figure{}
	if a != nil {
		f.From = a.ID
	}
	if b != nil {
		f.To = b.ID
	}
	if a == nil || b == nil {
		f.Reason = NoReadingBefore
		return f, nil
	}
	if a.span == b.span {
		f.Value = b.Value - a.Value
		return f, nil
	}
	if t.spans[a.span].Close == nil {
		f.Reason = MeterGap
		return f, nil
	}
	middle, known, err := t.closedUsage(a.span+1, b.span)
	if err != nil {
		return Figure{}, err
	}
	if !known {
		f.Reason = MeterGap
		return f, nil
	}
	value, ok := add(*t.spans[a.span].Close-a.Value, middle)
	if ok {
		value, ok = add(value, b.Value-t.spans[b.span].Open)
	}
	if !ok {
		return Figure{}, ErrInvalid
	}
	f.Value = value
	return f, nil
}

// Boundary is the planned effect of a replacement, reset or rollover: the
// boundary instant, the latest VALID reading before it, the closing value
// (nil when unknown), the opening value of what follows and the implied
// usage since that reading (Reason METER_GAP when unknown).
type Boundary struct {
	At       time.Time
	Previous string
	Close    *int64
	Open     int64
	Implied  int64
	Reason   Insufficient
}

func boundedValue(kind Kind, modulus *int64, v int64) bool {
	return v >= 0 && v <= bound(kind) && (modulus == nil || v < *modulus)
}

// latestValidBefore is the latest VALID member strictly before at.
func latestValidBefore(members []Entry, at time.Time) *Entry {
	var found *Entry
	for i := range members {
		if members[i].State == Valid && members[i].OccurredAt.Before(at) {
			found = &members[i]
		}
	}
	return found
}

// PlanReplacement decides a replacement of the meter whose open segment is
// series (B-BR-EMP-484). final nil is an unknown final value.
func PlanReplacement(kind Kind, modulus *int64, series []Entry, at time.Time, final, newModulus *int64, opening int64) (Boundary, error) {
	members, err := memberSeries(kind, series, modulus)
	if err != nil || !validInstant(at) || !at.After(members[0].OccurredAt) || newModulus != nil && *newModulus <= 0 || !boundedValue(kind, newModulus, opening) {
		return Boundary{}, ErrInvalid
	}
	for _, e := range members {
		if e.State != Voided && !e.OccurredAt.Before(at) {
			return Boundary{}, ErrInvalid
		}
	}
	previous := latestValidBefore(members, at)
	if previous == nil {
		return Boundary{}, ErrInvalid
	}
	b := Boundary{At: at, Previous: previous.ID, Open: opening}
	if final == nil {
		b.Reason = MeterGap
		return b, nil
	}
	if !boundedValue(kind, modulus, *final) || *final < previous.Value {
		return Boundary{}, ErrInvalid
	}
	v := *final
	b.Close, b.Implied = &v, v-previous.Value
	return b, nil
}

// PlanReset decides a reset of the open segment series (B-BR-EMP-485).
func PlanReset(kind Kind, modulus *int64, series []Entry, at time.Time, before, after int64) (Boundary, error) {
	members, err := memberSeries(kind, series, modulus)
	if err != nil || !validInstant(at) || !at.After(members[0].OccurredAt) || !boundedValue(kind, modulus, before) || !boundedValue(kind, modulus, after) {
		return Boundary{}, ErrInvalid
	}
	for _, e := range members {
		if e.State == Valid && !e.OccurredAt.Before(at) || e.State != Voided && e.OccurredAt.Equal(at) {
			return Boundary{}, ErrInvalid
		}
	}
	previous := latestValidBefore(members, at)
	if previous == nil || before < previous.Value {
		return Boundary{}, ErrInvalid
	}
	v := before
	return Boundary{At: at, Previous: previous.ID, Close: &v, Open: after, Implied: before - previous.Value}, nil
}

// PlanRollover decides the confirmation of target as a rollover
// (B-BR-EMP-486).
func PlanRollover(kind Kind, modulus *int64, series []Entry, target string) (Boundary, error) {
	if modulus == nil {
		return Boundary{}, ErrInvalid
	}
	members, err := memberSeries(kind, series, modulus)
	if err != nil {
		return Boundary{}, err
	}
	at := -1
	for i, e := range members {
		if e.ID == target {
			at = i
		}
	}
	if at <= 0 || members[at].State != Suspect {
		return Boundary{}, ErrInvalid
	}
	t := members[at]
	flagged := false
	for _, f := range t.Flags {
		flagged = flagged || f == PossibleRollover
	}
	var previous *Entry
	for i := range members[:at] {
		if members[i].State == Valid {
			previous = &members[i]
		}
	}
	if !flagged || previous == nil || !previous.OccurredAt.Before(t.OccurredAt) || t.Value >= previous.Value {
		return Boundary{}, ErrInvalid
	}
	for i, e := range members {
		if i > at && e.State == Valid || i != at && e.State != Voided && e.OccurredAt.Equal(t.OccurredAt) {
			return Boundary{}, ErrInvalid
		}
	}
	m := *modulus
	return Boundary{At: t.OccurredAt, Previous: previous.ID, Close: &m, Implied: (m - previous.Value) + t.Value}, nil
}
