package application

import (
	"context"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	"github.com/jackc/pgx/v5"
)

func UsagePolicy() access.Policy {
	return access.Policy{Permission: "data.usage.read", Capability: "data", Effect: access.Read}
}

type DatasetAfter struct {
	Key      string
	Revision int64
}

type DatasetView struct {
	Key             string
	Revision        int64
	Name            string
	Region          string
	Method          string
	MinimumStations int32
}

type TermUsage struct {
	Start, End    time.Time
	Used, Ceiling int64
}

type RateView struct {
	WindowSeconds int32
	Calls         int64
}

type UsageView struct {
	ObservedAt time.Time
	Term       *TermUsage
	Rate       *RateView
	Enabled    bool
	Datasets   []DatasetView
	After      DatasetAfter
}

// UsageViewAuthority holds the organization entitlement lock before the root
// through the usage projection, including the current operator switch.
type UsageViewAuthority interface {
	CheckUsageReadTx(context.Context, pgx.Tx, kernel.OrgScope, organization.CompanyActor, string) error
}

type UsageReader interface {
	ReadUsage(context.Context, kernel.OrgScope, organization.CompanyActor, string, DatasetAfter, int) (UsageView, error)
}
