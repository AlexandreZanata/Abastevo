package application

import (
	"context"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	payment "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/jackc/pgx/v5"
)

var ErrUnavailable = errors.New("orgbilling: plan overview unavailable")

func ReadPlanPolicy() access.Policy {
	return access.Policy{Permission: "subs.plan.read", Capability: "organizations", Effect: access.Read}
}

// ReadAuthority holds the organization entitlement and root read locks through
// the billing projection so the observed clock and subscription agree.
type ReadAuthority interface {
	CheckPlanReadTx(context.Context, pgx.Tx, kernel.OrgScope, organization.CompanyActor, string) error
}

type OverviewReader interface {
	Read(context.Context, kernel.OrgScope, organization.CompanyActor, string) (Overview, error)
}

type Overview struct {
	ObservedAt time.Time
	Plans      []PlanSummary
}

type PlanSummary struct {
	Catalog            domain.Plan
	State              domain.SubscriptionState
	Term               domain.Period
	TermCurrent        bool
	Receipt            *TermReceipt
	Pending            *PendingPlan
	StorageUsedBytes   *int64
	ActiveVehiclesUsed *int64
	ActiveStationsUsed *int64
	VehicleSelection   *VehicleSelectionBasis
}

type VehicleSelectionBasis struct {
	SubscriptionID string
	Revision       int64
	Active         int64
	Ceiling        int64
}

type PendingPlan struct {
	Catalog     domain.Plan
	EffectiveAt time.Time
}

// TermReceipt describes the confirmed financial fact. Fiscal status belongs
// to a separate authorized workflow.
type TermReceipt struct {
	Amount    payment.Money
	SettledAt time.Time
	PaidOn    string
}
