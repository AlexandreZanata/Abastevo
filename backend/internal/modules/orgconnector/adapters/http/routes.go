package http

import (
	"context"
	"errors"
	"time"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	orgapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const (
	ListPath      = "/v1/orgs/{org_id}/integrations/connections/list"
	ConfigurePath = "/v1/orgs/{org_id}/integrations/connections/configure"
	ActivatePath  = "/v1/orgs/{org_id}/integrations/connections/{connection_id}/activate"
	DisablePath   = "/v1/orgs/{org_id}/integrations/connections/{connection_id}/disable"
)

type records struct{ policy access.Policy }

func (records) Resolve(_ context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String()}, nil
}

func (r records) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, target permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || scope != subject.Scope() || policy != r.policy ||
		target.OrgID != scope.OrgID() || target.ID != scope.OrgID().String() || len(rules) == 0 {
		return access.ErrUnavailable
	}
	return nil
}

type listRequest struct{ orgapi.Page }

func (listRequest) Credentials() orgapi.Session { return orgapi.Session{} }

type configureRequest struct {
	orgapi.Command
	ApprovalID       string `json:"approval_id"`
	ApprovalRevision int64  `json:"approval_revision"`
	Label            string `json:"label"`
}

type connectionView struct {
	ID              string       `json:"id"`
	Label           string       `json:"label"`
	Kind            domain.Kind  `json:"kind"`
	Provider        string       `json:"provider"`
	State           domain.State `json:"state"`
	Revision        int64        `json:"revision"`
	ChangedAt       time.Time    `json:"changed_at"`
	ApprovalCurrent bool         `json:"approval_current"`
}

func view(connection domain.Connection) connectionView {
	return connectionView{ID: connection.ID, Label: connection.Label, Kind: connection.Kind,
		Provider: connection.Provider, State: connection.State, Revision: connection.Revision,
		ChangedAt: connection.ChangedAt, ApprovalCurrent: connection.ApprovalCurrent}
}

type connectionsPage struct {
	Connections []connectionView `json:"connections"`
	NextCursor  string           `json:"next_cursor"`
}

type changeResponse struct {
	Connection connectionView `json:"connection"`
	Replayed   bool           `json:"replayed"`
}

func Register(router chi.Router, service app.Commands, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
	if service == nil {
		return app.ErrUnavailable
	}
	owned, gateway, err := organization.CompanyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	if err := register(owned, ListPath, gateway, authorization, app.List, func(ctx context.Context, p orgapi.Principal, actor orgapp.CompanyActor, in listRequest) (any, error) {
		input, err := in.Page.Normalize()
		if err != nil {
			return nil, err
		}
		after, err := gateway.OpenCursor(p, ListPath, "", input.Cursor)
		if err != nil {
			return nil, err
		}
		page, err := service.List(ctx, p.Scope, actor, p.MembershipID, after, input.Limit)
		if err != nil {
			return nil, connectorError(err)
		}
		out := connectionsPage{Connections: make([]connectionView, 0, len(page.Connections))}
		for _, row := range page.Connections {
			out.Connections = append(out.Connections, view(row))
		}
		if page.After != "" {
			out.NextCursor, err = gateway.SealCursor(p, ListPath, "", page.After)
		}
		return out, err
	}); err != nil {
		return err
	}
	if err := register(owned, ConfigurePath, gateway, authorization, app.Configure, func(ctx context.Context, p orgapi.Principal, actor orgapp.CompanyActor, in configureRequest) (any, error) {
		if in.Revision != nil {
			return nil, orgapi.Malformed
		}
		return write(ctx, service, p, actor, app.Change{Action: app.Configure, OperationID: in.OperationID,
			ApprovalID: in.ApprovalID, ApprovalRevision: in.ApprovalRevision, Label: in.Label})
	}); err != nil {
		return err
	}
	for _, route := range []struct {
		path   string
		action app.Action
	}{{ActivatePath, app.Activate}, {DisablePath, app.Disable}} {
		if err := register(owned, route.path, gateway, authorization, route.action, func(ctx context.Context, p orgapi.Principal, actor orgapp.CompanyActor, in orgapi.Command) (any, error) {
			id := chi.RouteContext(ctx).URLParam("connection_id")
			if !audit.ValidID(id) {
				return nil, orgapi.NotFound
			}
			return write(ctx, service, p, actor, app.Change{
				Action: route.action, OperationID: in.OperationID, TargetID: id, ExpectedRevision: *in.Revision,
			})
		}); err != nil {
			return err
		}
	}
	return nil
}

func register[T orgapi.Request](router chi.Router, path string, gateway orgapi.Gateway, authorization access.Authorization,
	action app.Action, serve func(context.Context, orgapi.Principal, orgapp.CompanyActor, T) (any, error),
) error {
	policy := app.Policy(action)
	r := records{policy: policy}
	authorization.Records = r
	rule := orgapi.Rule{Permission: string(policy.Permission), Fresh: policy.Fresh, CompanyOnly: true, Write: policy.Effect.Write(),
		RequireRevision: action == app.Activate || action == app.Disable, Success: 200}
	return guarded.Register(router, path, gateway, rule, authorization, policy, r, func(ctx context.Context, p orgapi.Principal, _ access.Decision, in T) (any, error) {
		actor, err := organization.CurrentCompanyActor(ctx)
		if err != nil {
			return nil, orgapi.Unauthenticated
		}
		return serve(ctx, p, actor, in)
	})
}

func write(ctx context.Context, service app.Commands, p orgapi.Principal, actor orgapp.CompanyActor, input app.Change) (any, error) {
	out, err := service.Change(ctx, p.Scope, actor, p.MembershipID, input)
	if err != nil {
		return nil, connectorError(err)
	}
	return changeResponse{Connection: view(out.Connection), Replayed: out.Replayed}, nil
}

func connectorError(err error) error {
	switch {
	case errors.Is(err, permissions.ErrPermissionDenied):
		return orgapi.MissingPermission("integ.connection.manage")
	case errors.Is(err, domain.ErrDenied), errors.Is(err, access.ErrNoMembership), errors.Is(err, orgapp.ErrNotFound):
		return orgapi.NotFound
	case errors.Is(err, access.ErrUnauthenticated), errors.Is(err, orgapp.ErrUnauthenticated):
		return orgapi.Unauthenticated
	case errors.Is(err, access.ErrOrgSuspended):
		return orgapi.OrgSuspended
	case errors.Is(err, access.ErrOrgArchived):
		return orgapi.OrgArchived
	case errors.Is(err, capability.ErrEntitlementRequired):
		return orgapi.EntitlementRequired
	case errors.Is(err, domain.ErrInvalid):
		return orgapi.ValidationFailed
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrNotReady):
		return orgapi.Conflict
	case errors.Is(err, audit.ErrReused):
		return orgapi.OperationReused
	default:
		return orgapi.Unavailable
	}
}
