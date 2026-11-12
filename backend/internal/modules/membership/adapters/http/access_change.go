package http

import (
	"context"
	"errors"
	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	orgweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const MembershipAccessPrefix = "/v1/orgs/{org_id}/memberships/{membership_id}"
const MembershipAccessReadPath = MembershipAccessPrefix + "/read"
const MembershipSuspendPath = MembershipAccessPrefix + "/suspend"
const MembershipReactivatePath = MembershipAccessPrefix + "/reactivate"

type membershipAccessRequest struct {
	companyRequest
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	Revision      *int64 `json:"revision"`
	Reason        string `json:"reason"`
}

func (v membershipAccessRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}
func (v *membershipAccessRequest) UnmarshalJSON(raw []byte) error {
	type plain membershipAccessRequest
	return decodeInvitation(raw, (*plain)(v))
}

type membershipAccessReadRequest struct{ companyRequest }

func (v *membershipAccessReadRequest) UnmarshalJSON(raw []byte) error {
	type plain membershipAccessReadRequest
	return decodeInvitation(raw, (*plain)(v))
}

type MembershipAccessRecords struct{}

func (MembershipAccessRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.OrgID != scope.OrgID() || facts.ID != scope.OrgID().String() || len(rules) == 0 || (policy != app.MembershipAccessPolicy(app.MembershipAccessRead) && policy != app.MembershipAccessPolicy(app.MembershipSuspend) && policy != app.MembershipAccessPolicy(app.MembershipReactivate)) {
		return access.ErrUnavailable
	}
	return nil
}
func membershipAccessMapped(e error) error {
	switch {
	case errors.Is(e, d.ErrSelfSuspension):
		return orgapi.Forbidden
	case errors.Is(e, kernel.ErrLastOrgOwner):
		return orgapi.LastOwner
	case errors.Is(e, app.ErrAccessOperationReused):
		return orgapi.OperationReused
	case errors.Is(e, d.ErrConditions), errors.Is(e, d.ErrEnded):
		return orgapi.Conflict
	}
	return mapped(e)
}
func membershipAccessRule(action app.MembershipAccessAction) orgapi.Rule {
	p := app.MembershipAccessPolicy(action)
	return orgapi.Rule{Permission: string(p.Permission), Write: p.Effect.Write(), Fresh: p.Fresh, RequireRevision: action != app.MembershipAccessRead, CompanyOnly: true, Success: 200}
}
func membershipAccessID(ctx context.Context) (string, error) {
	raw := chi.URLParamFromCtx(ctx, "membership_id")
	p, e := kernel.ParseOrgID(raw)
	if e != nil || p.String() != raw {
		return "", orgapi.NotFound
	}
	return raw, nil
}
func membershipAccessView(v app.MembershipAccessResult, write bool) map[string]any {
	s := v.State
	out := map[string]any{"membership_id": s.ID, "employee_id": s.EmployeeID, "access_model": s.Model, "state": s.State, "revision": s.Revision, "suspension_origin": s.Suspension, "end_reason": s.EndReason, "data_mode": "SYNTHETIC"}
	if write {
		out["operation_id"] = v.OperationID
		out["reason"] = v.Reason
		out["previous_suspension_origin"] = v.PreviousSuspension
		out["occurred_at"] = v.OccurredAt
		out["replayed"] = v.Replayed
		out["notice_state"] = "PENDING_NOTIFICATION_CORE"
	} else {
		out["observed_at"] = v.OccurredAt
	}
	return out
}
func RegisterMembershipAccess(router chi.Router, service app.MembershipAccessService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization, roots org.Repository) error {
	if !service.Enabled {
		return nil
	}
	if service.Repository == nil || roots == nil {
		return app.ErrRegisterUnavailable
	}
	owned, gateway, e := orgweb.CompanyEntry(router, cookies, gateway)
	if e != nil {
		return e
	}
	facts := orgweb.RootFacts{Roots: roots}
	for _, route := range []struct {
		path   string
		action app.MembershipAccessAction
	}{{MembershipSuspendPath, app.MembershipSuspend}, {MembershipReactivatePath, app.MembershipReactivate}} {
		if e := guarded.Register(owned, route.path, gateway, membershipAccessRule(route.action), authorization, app.MembershipAccessPolicy(route.action), facts, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input membershipAccessRequest) (any, error) {
			a, e := actor(ctx)
			if e != nil {
				return nil, e
			}
			id, e := membershipAccessID(ctx)
			if e != nil {
				return nil, e
			}
			result, e := service.Change(ctx, p.Scope, a, decision, app.MembershipAccessWrite{Action: route.action, OperationID: input.OperationID, MembershipID: id, ExpectedRevision: *input.Revision, Reason: input.Reason})
			if e != nil {
				return nil, membershipAccessMapped(e)
			}
			return membershipAccessView(result, true), nil
		}); e != nil {
			return e
		}
	}
	return guarded.Register(owned, MembershipAccessReadPath, gateway, membershipAccessRule(app.MembershipAccessRead), authorization, app.MembershipAccessPolicy(app.MembershipAccessRead), facts, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ membershipAccessReadRequest) (any, error) {
		a, e := actor(ctx)
		if e != nil {
			return nil, e
		}
		id, e := membershipAccessID(ctx)
		if e != nil {
			return nil, e
		}
		result, e := service.Read(ctx, p.Scope, a, decision, id)
		if e != nil {
			return nil, membershipAccessMapped(e)
		}
		return membershipAccessView(result, false), nil
	})
}
