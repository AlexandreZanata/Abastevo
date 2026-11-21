package http

import (
	"context"
	"errors"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	orgapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const VehicleSelectionPath = "/v1/orgs/{org_id}/subscriptions/vehicles/select"

type vehicleSelectionRequest struct {
	orgapi.Command
	SubscriptionID string   `json:"subscription_id"`
	Kept           []string `json:"keep_active"`
}

func (r *vehicleSelectionRequest) UnmarshalJSON(raw []byte) error {
	type plain vehicleSelectionRequest
	return httpapi.DecodeJSON(raw, (*plain)(r))
}

func RegisterVehicleSelection(router chi.Router, service billing.VehicleSelections, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
	if service == nil {
		return billing.ErrUnavailable
	}
	owned, gateway, err := organization.CompanyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	policy := billing.SelectVehiclesPolicy()
	authorization.Records = planRecords{}
	rule := orgapi.Rule{Permission: string(policy.Permission), CompanyOnly: true, Write: true, Fresh: true, RequireRevision: true, Success: 200}
	return guarded.Register(owned, VehicleSelectionPath, gateway, rule, authorization, policy, planRecords{},
		func(ctx context.Context, p orgapi.Principal, _ access.Decision, request vehicleSelectionRequest) (any, error) {
			actor, err := organization.CurrentCompanyActor(ctx)
			if err != nil {
				return nil, orgapi.Unauthenticated
			}
			if request.Revision == nil {
				return nil, orgapi.Malformed
			}
			result, err := service.SelectVehicles(ctx, p.Scope, actor, p.MembershipID, billing.VehicleSelection{
				OperationID: request.OperationID, SubscriptionID: request.SubscriptionID, Revision: *request.Revision, Kept: request.Kept,
			})
			if err != nil {
				return nil, vehicleSelectionError(err)
			}
			return result, nil
		})
}

func vehicleSelectionError(err error) error {
	var open *fleet.OpenOperationsError
	switch {
	case errors.Is(err, billing.ErrVehicleSelectionInvalid):
		return orgapi.ValidationFailed
	case errors.Is(err, billing.ErrVehicleSelectionReused):
		return orgapi.OperationReused
	case errors.Is(err, billing.ErrVehicleSelectionConflict), errors.Is(err, fleet.ErrConflict):
		return orgapi.Conflict
	case errors.Is(err, billing.ErrVehicleSelectionNotFound), errors.Is(err, fleet.ErrNotFound), errors.Is(err, access.ErrNoMembership), errors.Is(err, orgapp.ErrNotFound):
		return orgapi.NotFound
	case errors.Is(err, permissions.ErrPermissionDenied):
		return orgapi.MissingPermission(string(billing.SelectVehiclesPolicy().Permission))
	case errors.Is(err, access.ErrUnauthenticated), errors.Is(err, orgapp.ErrUnauthenticated):
		return orgapi.Unauthenticated
	case errors.Is(err, access.ErrEntitlementRequired):
		return orgapi.EntitlementRequired
	case errors.Is(err, access.ErrFreshAuthRequired):
		return orgapi.FreshAuthRequired
	case errors.Is(err, access.ErrSetupIncomplete):
		return orgapi.OrgSetupIncomplete
	case errors.Is(err, access.ErrOrgSuspended):
		return orgapi.OrgSuspended
	case errors.As(err, &open):
		kinds := make([]string, 0, len(open.Kinds()))
		for _, kind := range open.Kinds() {
			kinds = append(kinds, string(kind))
		}
		return orgapi.AssetOpenOperations(kinds)
	}
	return orgapi.Unavailable
}
