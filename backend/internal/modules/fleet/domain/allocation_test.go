package domain_test

import (
	"errors"
	"testing"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
)

var allocationStart = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func period(t *testing.T, from time.Time, to *time.Time) fleet.AllocationPeriod {
	t.Helper()
	p, err := fleet.NewAllocationPeriod(from, to)
	if err != nil {
		t.Fatal("valid allocation period refused", from, to, err)
	}
	return p
}

func at(v time.Time) *time.Time { return &v }

// B-BR-EMP-226: the instant is floored to the microsecond, which preserves the
// half-open rule exactly because every stored boundary is a whole microsecond.
func TestAllocationInstantFloorsToTheMicrosecondAndRefusesInvalidInstants(t *testing.T) {
	for name, tc := range map[string]struct{ in, want time.Time }{
		"exact microsecond":  {allocationStart.Add(7 * time.Microsecond), allocationStart.Add(7 * time.Microsecond)},
		"nanoseconds floor":  {allocationStart.Add(999 * time.Nanosecond), allocationStart},
		"just before":        {allocationStart.Add(-1 * time.Nanosecond), allocationStart.Add(-1 * time.Microsecond)},
		"offset normalised":  {time.Date(2026, 3, 1, 9, 0, 0, 1500, time.FixedZone("BRT", -3*3600)), allocationStart.Add(time.Microsecond)},
		"pre-1970 floors":    {time.Date(1969, 12, 31, 23, 59, 59, 999999999, time.UTC), time.Date(1969, 12, 31, 23, 59, 59, 999999000, time.UTC)},
		"first valid year":   {time.Date(1, 1, 1, 0, 0, 0, 1000, time.UTC), time.Date(1, 1, 1, 0, 0, 0, 1000, time.UTC)},
		"last valid instant": {time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC), time.Date(9999, 12, 31, 23, 59, 59, 999999000, time.UTC)},
	} {
		got, err := fleet.AllocationInstant(tc.in)
		if err != nil || !got.Equal(tc.want) || got.Location() != time.UTC {
			t.Fatal("allocation instant floor", name, got, err)
		}
	}
	for name, bad := range map[string]time.Time{
		"unset":      {},
		"year 10000": time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC),
		"year 0":     time.Date(0, 12, 31, 23, 0, 0, 0, time.UTC),
	} {
		if _, err := fleet.AllocationInstant(bad); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("invalid allocation instant accepted", name)
		}
	}
}

// B-BR-EMP-223/AC4: [From, To) — the instant of change belongs to the new period.
func TestAllocationPeriodIsHalfOpenAndExact(t *testing.T) {
	change := allocationStart.Add(31 * 24 * time.Hour)
	march, april := period(t, allocationStart, at(change)), period(t, change, nil)
	if march.Open() || !april.Open() {
		t.Fatal("open-ended period not recognised")
	}
	for name, tc := range map[string]struct {
		at           time.Time
		march, april bool
	}{
		"before first":        {allocationStart.Add(-time.Microsecond), false, false},
		"first start":         {allocationStart, true, false},
		"inside first":        {allocationStart.Add(10 * 24 * time.Hour), true, false},
		"last microsecond":    {change.Add(-time.Microsecond), true, false},
		"change instant":      {change, false, true},
		"after change":        {change.Add(time.Microsecond), false, true},
		"far future":          {time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), false, true},
		"sub-microsecond end": {change.Add(-time.Nanosecond), true, false},
	} {
		if march.Contains(tc.at) != tc.march || april.Contains(tc.at) != tc.april {
			t.Fatal("half-open containment", name)
		}
	}
	for name, tc := range map[string]struct {
		from time.Time
		to   *time.Time
	}{
		"unset start":           {time.Time{}, nil},
		"sub-microsecond start": {allocationStart.Add(time.Nanosecond), nil},
		"sub-microsecond end":   {allocationStart, at(change.Add(500 * time.Nanosecond))},
		"empty":                 {allocationStart, at(allocationStart)},
		"reversed":              {change, at(allocationStart)},
		"year 10000 start":      {time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC), nil},
		"year 10000 end":        {allocationStart, at(time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC))},
	} {
		if _, err := fleet.NewAllocationPeriod(tc.from, tc.to); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("invalid allocation period accepted", name)
		}
	}
	original := change
	held := period(t, allocationStart, &original)
	original = original.Add(time.Hour)
	if !held.To.Equal(change) {
		t.Fatal("period end aliases the caller's value")
	}
	if p := period(t, time.Date(2026, 3, 1, 9, 0, 0, 0, time.FixedZone("BRT", -3*3600)), nil); p.From.Location() != time.UTC || !p.From.Equal(allocationStart) {
		t.Fatal("period start not normalised to UTC")
	}
}

// One history page: start order, no overlap, an open period only last; the
// unit history is gap-free (B-BR-EMP-224), the cost-centre one may have gaps.
func TestAllocationPageOrderOverlapGapAndOpenEnd(t *testing.T) {
	t1, t2, t3 := allocationStart.Add(time.Hour), allocationStart.Add(2*time.Hour), allocationStart.Add(3*time.Hour)
	a, b, c := period(t, allocationStart, at(t1)), period(t, t1, at(t2)), period(t, t2, nil)
	gapped := period(t, t3, nil)
	for name, contiguous := range map[string]bool{"unit": true, "cost centre": false} {
		if err := fleet.ValidAllocationPage(nil, contiguous); err != nil {
			t.Fatal("empty page refused", name)
		}
		if err := fleet.ValidAllocationPage([]fleet.AllocationPeriod{a, b, c}, contiguous); err != nil {
			t.Fatal("contiguous history refused", name, err)
		}
	}
	if err := fleet.ValidAllocationPage([]fleet.AllocationPeriod{a, gapped}, false); err != nil {
		t.Fatal("optional cost-centre gap refused", err)
	}
	if err := fleet.ValidAllocationPage([]fleet.AllocationPeriod{a, gapped}, true); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("unit gap accepted")
	}
	for name, bad := range map[string][]fleet.AllocationPeriod{
		"descending":       {b, a},
		"duplicate start":  {a, a},
		"overlap":          {period(t, allocationStart, at(t2)), b},
		"open not last":    {period(t, allocationStart, nil), b},
		"two open":         {c, period(t, t3, nil)},
		"unvalidated zero": {{}},
		"unaligned":        {{From: allocationStart.Add(time.Nanosecond)}},
	} {
		for _, contiguous := range []bool{true, false} {
			if err := fleet.ValidAllocationPage(bad, contiguous); !errors.Is(err, fleet.ErrInvalid) {
				t.Fatal("invalid allocation page accepted", name, contiguous)
			}
		}
	}
}
