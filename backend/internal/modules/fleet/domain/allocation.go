package domain

import "time"

// AllocationPeriod is one EMP-FLEET-005 unit or cost-centre allocation
// period [From, To) of one asset. To is nil while the period is open-ended.
// Boundaries are whole microseconds (B-BR-EMP-223), as the database stores.
type AllocationPeriod struct {
	From time.Time
	To   *time.Time
}

func allocationYear(at time.Time) bool {
	year := at.UTC().Year()
	return year >= 1 && year <= 9999
}

// AllocationInstant returns the microsecond floor of a valid instant in UTC.
// Every period boundary is a whole microsecond, so the floor keeps the
// half-open rule exact for any nanosecond instant (B-BR-EMP-226).
func AllocationInstant(at time.Time) (time.Time, error) {
	if at.IsZero() || !allocationYear(at) {
		return time.Time{}, ErrInvalid
	}
	// Time.Truncate floors relative to the zero time, which is aligned with
	// every whole microsecond of the Unix epoch, including before 1970.
	return at.UTC().Truncate(time.Microsecond), nil
}

func exactBoundary(at time.Time) bool {
	return !at.IsZero() && allocationYear(at) && at.Equal(at.Truncate(time.Microsecond))
}

// NewAllocationPeriod validates exact microsecond bounds with To after From
// and returns a UTC copy that does not alias the caller's end value.
func NewAllocationPeriod(from time.Time, to *time.Time) (AllocationPeriod, error) {
	if !exactBoundary(from) {
		return AllocationPeriod{}, ErrInvalid
	}
	p := AllocationPeriod{From: from.UTC()}
	if to != nil {
		if !exactBoundary(*to) || !to.After(from) {
			return AllocationPeriod{}, ErrInvalid
		}
		end := to.UTC()
		p.To = &end
	}
	return p, nil
}

func (p AllocationPeriod) valid() bool {
	_, err := NewAllocationPeriod(p.From, p.To)
	return err == nil
}

// Open reports an open-ended period.
func (p AllocationPeriod) Open() bool { return p.To == nil }

// Contains applies the half-open rule: From <= at < To.
func (p AllocationPeriod) Contains(at time.Time) bool {
	return !at.Before(p.From) && (p.To == nil || at.Before(*p.To))
}

// ValidAllocationPage checks one start-ordered page of one history: valid
// periods with strictly increasing starts, no overlap and an open-ended
// period only last. A unit history is gap-free (B-BR-EMP-224); a cost-centre
// history may have gaps.
func ValidAllocationPage(periods []AllocationPeriod, contiguous bool) error {
	for i, p := range periods {
		if !p.valid() {
			return ErrInvalid
		}
		if i == 0 {
			continue
		}
		prior := periods[i-1]
		if prior.To == nil || p.From.Before(*prior.To) || contiguous && !p.From.Equal(*prior.To) {
			return ErrInvalid
		}
	}
	return nil
}
