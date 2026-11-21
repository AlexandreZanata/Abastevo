package domain

import (
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

func ClosureDeadline(name, confirmation string, now time.Time, days int) (time.Time, error) {
	if days == 0 {
		days = 30
	}
	if name == "" || name != confirmation || now.IsZero() || days < 7 || days > 90 {
		return time.Time{}, ErrInvalid
	}
	return now.UTC().Add(time.Duration(days) * 24 * time.Hour), nil
}

func ClosureDue(now, deadline time.Time) bool {
	return !now.IsZero() && !deadline.IsZero() && !now.Before(deadline)
}

// ReviseArchivedClosure advances the root revision for a closure schedule or
// cancellation when the organization's archived state itself is unchanged.
func (o Organization) ReviseArchivedClosure(scope kernel.OrgScope, expected int64) (Organization, error) {
	if err := o.changing(scope, expected); err != nil {
		return Organization{}, err
	}
	if o.value.State != Archived {
		return Organization{}, ErrTransition
	}
	o.value.Revision++
	return o, nil
}
