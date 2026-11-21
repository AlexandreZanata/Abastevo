package application

import (
	"context"
	"errors"
	"slices"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/jackc/pgx/v5"
)

var (
	ErrVehicleSelectionInvalid  = errors.New("organization billing: invalid vehicle selection")
	ErrVehicleSelectionReused   = errors.New("organization billing: vehicle selection operation reused")
	ErrVehicleSelectionNotFound = errors.New("organization billing: current vehicle downgrade unavailable")
	ErrVehicleSelectionConflict = errors.New("organization billing: vehicle selection conflicts with current fleet")
)

func SelectVehiclesPolicy() access.Policy {
	return access.Policy{Permission: "subs.plan.manage", Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
}

type VehicleSelection struct {
	OperationID    string
	SubscriptionID string
	Revision       int64
	Kept           []string
}

func (c VehicleSelection) Canonical() (VehicleSelection, error) {
	if !audit.ValidID(c.OperationID) || !audit.ValidID(c.SubscriptionID) || c.Revision < 2 || c.Kept == nil {
		return VehicleSelection{}, ErrVehicleSelectionInvalid
	}
	c.Kept = append([]string{}, c.Kept...)
	slices.Sort(c.Kept)
	for i, id := range c.Kept {
		if !audit.ValidID(id) || i > 0 && id == c.Kept[i-1] {
			return VehicleSelection{}, ErrVehicleSelectionInvalid
		}
	}
	return c, nil
}

type VehicleSelectionResult struct {
	Kept     []string `json:"kept"`
	Archived []string `json:"archived"`
	Replayed bool     `json:"replayed"`
}

type PlanSelectionAuthority interface {
	CheckVehicleSelectionTx(context.Context, pgx.Tx, kernel.OrgScope, organization.CompanyActor, string) error
}

type VehicleSelections interface {
	SelectVehicles(context.Context, kernel.OrgScope, organization.CompanyActor, string, VehicleSelection) (VehicleSelectionResult, error)
}
