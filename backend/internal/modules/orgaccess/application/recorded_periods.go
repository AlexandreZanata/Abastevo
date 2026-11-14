package application

import (
	"context"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

// RecordedRolePeriod is persistence evidence, not authorization at device time.
type RecordedRolePeriod struct {
	privateAuthority
	Assignment domain.Assignment
	CreatedAt  time.Time
}

type RecordedRolePeriods interface {
	RecordedPeriodsDBTX(context.Context, AuthorityQueries, kernel.OrgScope, string) ([]RecordedRolePeriod, error)
}
