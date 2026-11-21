package http

import (
	"context"
	"errors"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const OrganizationOpenItemsPath = "/v1/orgs/{org_id}/lifecycle/open-items"
const OrganizationLifecyclePath = "/v1/orgs/{org_id}/lifecycle/change"

type LifecycleRecords struct{}

func (LifecycleRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.OrgID != scope.OrgID() || facts.ID != scope.OrgID().String() || len(rules) == 0 ||
		policy != app.LifecyclePolicy(false) && policy != app.LifecyclePolicy(true) {
		return access.ErrUnavailable
	}
	return nil
}

type lifecycleReadRequest struct{ companyRequest }

func (v *lifecycleReadRequest) UnmarshalJSON(raw []byte) error {
	type plain lifecycleReadRequest
	return decodeLegal(raw, (*plain)(v))
}

type lifecycleRequest struct {
	companyRequest
	SchemaVersion int                 `json:"schema_version"`
	OperationID   string              `json:"operation_id"`
	Revision      int64               `json:"revision"`
	Action        app.LifecycleAction `json:"action"`
	Confirmation  string              `json:"confirmation,omitempty"`
	ClosureDays   *int                `json:"closure_days,omitempty"`
}

func (v lifecycleRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: &v.Revision}
}

func (v *lifecycleRequest) UnmarshalJSON(raw []byte) error {
	type plain lifecycleRequest
	return decodeLegal(raw, (*plain)(v))
}

func RegisterLifecycle(router chi.Router, repo app.LifecycleRepository, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization, roots app.Repository) error {
	if repo == nil || roots == nil {
		return app.ErrUnavailable
	}
	owned, gateway, err := companyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	view := app.LifecyclePolicy(false)
	if err := guarded.Register(owned, OrganizationOpenItemsPath, gateway, ownershipTransferRule(view, false), authorization, view, RootFacts{roots},
		func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ lifecycleReadRequest) (any, error) {
			actor, err := companyActor(ctx)
			if err != nil {
				return nil, err
			}
			result, err := repo.LifecycleReport(ctx, p.Scope, actor, decision.Subject().MembershipID())
			return result, lifecycleMapped(err)
		}); err != nil {
		return err
	}
	change := app.LifecyclePolicy(true)
	rule := ownershipTransferRule(change, true)
	rule.RequireRevision = true
	return guarded.Register(owned, OrganizationLifecyclePath, gateway, rule, authorization, change, RootFacts{roots},
		func(ctx context.Context, p orgapi.Principal, decision access.Decision, input lifecycleRequest) (any, error) {
			actor, err := companyActor(ctx)
			if err != nil {
				return nil, err
			}
			command := app.LifecycleCommand{Action: input.Action, OperationID: input.OperationID, ExpectedRevision: input.Revision, Confirmation: input.Confirmation}
			if input.ClosureDays != nil {
				command.ClosureDays = *input.ClosureDays
				if command.ClosureDays < 7 || command.ClosureDays > 90 {
					return nil, orgapi.ValidationFailed
				}
			}
			result, err := repo.ChangeLifecycle(ctx, p.Scope, actor, decision.Subject().MembershipID(), command)
			return result, lifecycleMapped(err)
		})
}

func lifecycleMapped(err error) error {
	if err == nil {
		return nil
	}
	var open app.OrganizationOpenWork
	switch {
	case errors.As(err, &open):
		return orgapi.OrganizationOpenOperations(open.Trips, open.Agreements)
	case errors.Is(err, audit.ErrReused):
		return orgapi.OperationReused
	case errors.Is(err, d.ErrTransition), errors.Is(err, d.ErrConditions), errors.Is(err, d.ErrConflict):
		return orgapi.Conflict
	case errors.Is(err, d.ErrClosed):
		return orgapi.NotFound
	default:
		return legalMapped(err, "")
	}
}
