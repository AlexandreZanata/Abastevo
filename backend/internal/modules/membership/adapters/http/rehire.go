package http

import (
	"context"
	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	orgweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const EmployeeRehirePath = EmployeePrefix + "/{employee_id}/rehire"

type rehireRequest struct {
	companyRequest
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	Revision      *int64 `json:"revision"`
	Reason        string `json:"reason"`
}

func (v rehireRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}
func (v *rehireRequest) UnmarshalJSON(raw []byte) error {
	type plain rehireRequest
	return decodeRedemption(raw, (*plain)(v), "schema_version", "operation_id", "revision", "reason")
}
func rehireResponse(v app.RehireResult) map[string]any {
	return map[string]any{"employee_id": v.EmployeeID, "state": v.State, "identity_revision": v.Revision, "operation_id": v.OperationID, "reason": v.Reason, "occurred_at": v.OccurredAt, "replayed": v.Replayed, "access_provisioning": app.RehireProvisioning, "data_mode": "SYNTHETIC"}
}
func RegisterRehire(router chi.Router, service app.RehireService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization, roots org.Repository) error {
	if !service.Enabled {
		return nil
	}
	if service.Repository == nil || roots == nil {
		return app.ErrRegisterUnavailable
	}
	owned, g, e := orgweb.CompanyEntry(router, cookies, gateway)
	if e != nil {
		return e
	}
	return guarded.Register(owned, EmployeeRehirePath, g, membershipAccessRule(app.MembershipReactivate), authorization, app.RehirePolicy(), orgweb.RootFacts{Roots: roots}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input rehireRequest) (any, error) {
		actor, e := actor(ctx)
		if e != nil {
			return nil, e
		}
		id, e := pathID(ctx)
		if e != nil {
			return nil, e
		}
		result, e := service.Rehire(ctx, p.Scope, actor, decision, app.RehireCommand{OperationID: input.OperationID, EmployeeID: id, ExpectedRevision: *input.Revision, Reason: input.Reason})
		if e != nil {
			return nil, membershipAccessMapped(e)
		}
		return rehireResponse(result), nil
	})
}
