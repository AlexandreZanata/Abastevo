package domain

import (
	"math/big"
	"regexp"
	"sort"
	"time"
)

var stableID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

type Weight struct {
	ID    string
	Value int64
}
type Allocation struct {
	Amounts map[string]int64
	Unknown string
}

func Allocate(source *int64, weights []Weight) (Allocation, error) {
	out := Allocation{Amounts: map[string]int64{}}
	if source == nil {
		out.Unknown = "source-amount-missing"
		return out, nil
	}
	if !validAmount(*source) || len(weights) > MaxItems {
		return out, ErrInvalid
	}
	total := new(big.Int)
	seen := map[string]bool{}
	for _, w := range weights {
		if !stableID.MatchString(w.ID) || seen[w.ID] || !validAmount(w.Value) {
			return out, ErrInvalid
		}
		seen[w.ID] = true
		total.Add(total, big.NewInt(w.Value))
	}
	if total.Sign() == 0 {
		out.Unknown = "allocation-basis-missing"
		return out, nil
	}
	type remainder struct {
		id string
		r  *big.Int
	}
	var remainders []remainder
	var allocated int64
	for _, w := range weights {
		n := new(big.Int).Mul(big.NewInt(*source), big.NewInt(w.Value))
		q, r := new(big.Int), new(big.Int)
		q.QuoRem(n, total, r)
		out.Amounts[w.ID] = q.Int64()
		allocated += q.Int64()
		remainders = append(remainders, remainder{w.ID, r})
	}
	sort.Slice(remainders, func(i, j int) bool {
		c := remainders[i].r.Cmp(remainders[j].r)
		if c == 0 {
			return remainders[i].id < remainders[j].id
		}
		return c > 0
	})
	for i := int64(0); i < *source-allocated; i++ {
		out.Amounts[remainders[i].id]++
	}
	return out, nil
}

type AllocationSpan struct {
	ID   string
	Span TimeSpan
}

func Prorate(source *int64, period TimeSpan, parts []AllocationSpan) (Allocation, error) {
	if period.From < 0 || period.To <= period.From || len(parts) >= MaxItems {
		return Allocation{}, ErrInvalid
	}
	ordered := append([]AllocationSpan(nil), parts...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Span.From < ordered[j].Span.From })
	previous := period.From
	covered := int64(0)
	weights := make([]Weight, 0, len(parts)+1)
	for _, p := range ordered {
		s := p.Span
		if p.ID == "_unallocated" || s.From < previous || s.To <= s.From || s.To > period.To {
			return Allocation{}, ErrInvalid
		}
		previous = s.To
		covered += s.To - s.From
		weights = append(weights, Weight{p.ID, s.To - s.From})
	}
	weights = append(weights, Weight{"_unallocated", period.To - period.From - covered})
	return Allocate(source, weights)
}

type CalendarSpan struct{ ID, From, To string }

func ProrateDays(source *int64, from, to, timezone string, parts []CalendarSpan) (Allocation, error) {
	if timezone == "" || timezone == "Local" {
		return Allocation{}, ErrInvalid
	}
	if _, e := time.LoadLocation(timezone); e != nil {
		return Allocation{}, ErrInvalid
	}
	day := func(s string) (int64, error) {
		d, e := time.Parse("2006-01-02", s)
		if e != nil || d.Year() < 1 || d.Format("2006-01-02") != s {
			return 0, ErrInvalid
		}
		return d.Unix()/86400 + 719162, nil
	}
	a, e := day(from)
	if e != nil {
		return Allocation{}, e
	}
	b, e := day(to)
	if e != nil {
		return Allocation{}, e
	}
	var spans []AllocationSpan
	for _, p := range parts {
		x, e := day(p.From)
		if e != nil {
			return Allocation{}, e
		}
		y, e := day(p.To)
		if e != nil {
			return Allocation{}, e
		}
		spans = append(spans, AllocationSpan{p.ID, TimeSpan{x, y}})
	}
	return Prorate(source, TimeSpan{a, b}, spans)
}
