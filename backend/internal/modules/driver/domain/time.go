package domain

import "sort"

type TimeSpan struct{ From, To int64 }

// OnlineUnion uses the union of half-open intervals, not summed app counters.
func OnlineUnion(spans []TimeSpan) (int64, error) {
	if len(spans) > MaxItems {
		return 0, ErrInvalid
	}
	if len(spans) == 0 {
		return 0, nil
	}
	copySpans := append([]TimeSpan(nil), spans...)
	for _, s := range copySpans {
		if s.From < 0 || s.To <= s.From {
			return 0, ErrInvalid
		}
	}
	sort.Slice(copySpans, func(i, j int) bool { return copySpans[i].From < copySpans[j].From })
	start, end := copySpans[0].From, copySpans[0].To
	var total int64
	for _, s := range copySpans[1:] {
		if s.From > end {
			total += end - start
			start, end = s.From, s.To
		} else if s.To > end {
			end = s.To
		}
	}
	// Nonnegative disjoint spans lie inside [0, MaxInt64], so this sum cannot overflow.
	return total + (end - start), nil
}
