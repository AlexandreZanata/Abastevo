// Package http binds role management endpoints to company sessions and guarded execution.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const (
	RolePrefix     = "/v1/orgs/{org_id}/roles"
	RoleAssignPath = RolePrefix + "/assign"
	RoleEndPath    = RolePrefix + "/{assignment_id}/end"
	RoleChangePath = RolePrefix + "/{assignment_id}/change"
	RoleReadPath   = RolePrefix + "/{assignment_id}/read"
	RoleListPath   = RolePrefix + "/list"
)

type privateDTO struct{}

func (privateDTO) String() string                 { return "RoleRequest(redacted)" }
func (p privateDTO) Format(f fmt.State, _ rune)   { _, _ = fmt.Fprint(f, p.String()) }
func (p privateDTO) MarshalJSON() ([]byte, error) { return json.Marshal(p.String()) }
func (p privateDTO) LogValue() slog.Value         { return slog.StringValue(p.String()) }

type companyRequest struct{ privateDTO }

func (companyRequest) Credentials() orgapi.Session { return orgapi.Session{} }

type roleAssignRequest struct {
	privateDTO
	companyRequest
	SchemaVersion int        `json:"schema_version"`
	OperationID   string     `json:"operation_id"`
	MembershipID  string     `json:"membership_id"`
	Role          string     `json:"role"`
	Scope         string     `json:"scope"`
	TargetID      string     `json:"target_id"`
	ValidFrom     *time.Time `json:"valid_from"`
	ValidTo       *time.Time `json:"valid_to"`
	Reason        string     `json:"reason"`
}

func (v roleAssignRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID}
}

type roleEndRequest struct {
	privateDTO
	companyRequest
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	Revision      *int64 `json:"revision"`
	Reason        string `json:"reason"`
}

func (v roleEndRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}

type roleChangeRequest struct {
	privateDTO
	companyRequest
	SchemaVersion int        `json:"schema_version"`
	OperationID   string     `json:"operation_id"`
	Revision      *int64     `json:"revision"`
	Role          string     `json:"role"`
	Scope         string     `json:"scope"`
	TargetID      string     `json:"target_id"`
	ValidFrom     *time.Time `json:"valid_from"`
	ValidTo       *time.Time `json:"valid_to"`
	Reason        string     `json:"reason"`
}

func (v roleChangeRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}

type roleReadRequest struct {
	privateDTO
	companyRequest
}

type roleListRequest struct {
	privateDTO
	companyRequest
	Page         orgapi.Page `json:"page"`
	MembershipID string      `json:"membership_id"`
	Role         string      `json:"role"`
	State        string      `json:"state"`
}

type RoleRecords struct{}

func (RoleRecords) Check(_ context.Context, scope kernel.OrgScope, subject app.Subject, policy app.Policy, facts d.ResourceFacts, rules []d.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.Validate(scope) != nil || facts.OrgID != scope.OrgID() || len(rules) == 0 {
		return app.ErrUnavailable
	}
	if policy != app.RolePolicy(app.RoleAssignAction) &&
		policy != app.RolePolicy(app.RoleEndAction) &&
		policy != app.RolePolicy(app.RoleChangeAction) &&
		policy != app.RolePolicy(app.RoleReadAction) &&
		policy != app.RolePolicy(app.RoleListAction) {
		return app.ErrUnavailable
	}
	return nil
}

type assignmentResource struct {
	lookup app.Repository
}

func (r assignmentResource) Resolve(ctx context.Context, scope kernel.OrgScope, _ app.Subject) (d.ResourceFacts, error) {
	if scope.Validate() != nil {
		return d.ResourceFacts{}, app.ErrUnavailable
	}
	raw := chi.URLParamFromCtx(ctx, "assignment_id")
	id, err := kernel.ParseOrgID(raw)
	if err != nil || id.String() != raw {
		return d.ResourceFacts{}, app.ErrNotFound
	}
	v, err := r.lookup.Get(ctx, scope, raw)
	if errors.Is(err, app.ErrNotFound) {
		return d.ResourceFacts{}, app.ErrNotFound
	}
	if err != nil {
		return d.ResourceFacts{}, app.ErrUnavailable
	}
	if _, err = v.Snapshot(scope); err != nil {
		return d.ResourceFacts{}, app.ErrUnavailable
	}
	return d.ResourceFacts{OrgID: scope.OrgID(), ID: raw}, nil
}

type RootResolver interface {
	ResolveRoot(ctx context.Context, scope kernel.OrgScope) error
}

type RootFacts struct {
	Resolver RootResolver
}

func (p RootFacts) Resolve(ctx context.Context, scope kernel.OrgScope, _ app.Subject) (d.ResourceFacts, error) {
	if scope.Validate() != nil {
		return d.ResourceFacts{}, app.ErrUnavailable
	}
	if p.Resolver != nil {
		if err := p.Resolver.ResolveRoot(ctx, scope); err != nil {
			if errors.Is(err, app.ErrNotFound) {
				return d.ResourceFacts{}, app.ErrNotFound
			}
			return d.ResourceFacts{}, app.ErrUnavailable
		}
	}
	return d.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String()}, nil
}

type companyProofKey struct{}
type companyProof struct{ actor app.CompanyActor }

func companyEntry(router chi.Router, cookies company.Handler, gateway orgapi.Gateway) (chi.Router, orgapi.Gateway, error) {
	if cookies.Service == nil || !cookies.Service.Enabled || gateway.Sessions != nil || gateway.SignedOwner != nil || gateway.WebSession != nil || gateway.WebSessionConfirm != nil {
		return nil, orgapi.Gateway{}, app.ErrUnavailable
	}
	gateway.WebSession = cookies.WebSession
	gateway.WebSessionConfirm = func(r *http.Request, fresh bool) (string, error) {
		proof, _ := r.Context().Value(companyProofKey{}).(*companyProof)
		if proof == nil {
			return "", orgapi.Unavailable
		}
		row, err := cookies.ConfirmSession(r, fresh)
		if err != nil {
			return "", err
		}
		proof.actor = app.CompanyActor{AccountID: row.AccountID, SessionID: row.ID}
		return row.AccountID, nil
	}
	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), companyProofKey{}, &companyProof{})))
		})
	}
	return router.With(middleware), gateway, nil
}

func pathAssignmentID(ctx context.Context) (string, error) {
	raw := chi.URLParamFromCtx(ctx, "assignment_id")
	id, err := kernel.ParseOrgID(raw)
	if err != nil || id.String() != raw {
		return "", app.ErrNotFound
	}
	return raw, nil
}

func currentCompanyActor(ctx context.Context) (app.CompanyActor, error) {
	proof, _ := ctx.Value(companyProofKey{}).(*companyProof)
	if proof == nil || proof.actor.Validate() != nil {
		return app.CompanyActor{}, orgapi.Unauthenticated
	}
	return proof.actor, nil
}

func roleRule(action app.RoleAction, revision bool, status int) orgapi.Rule {
	p := app.RolePolicy(action)
	return orgapi.Rule{
		Permission:      string(p.Permission),
		Write:           p.Effect.Write(),
		Fresh:           p.Fresh,
		RequireRevision: revision,
		CompanyOnly:     true,
		Success:         status,
	}
}

func roleMapError(err error) error {
	var denial *d.PermissionDenial
	if errors.As(err, &denial) {
		return orgapi.MissingPermission(string(denial.Permission()))
	}
	if errors.Is(err, kernel.ErrLastOrgOwner) {
		return orgapi.LastOwner
	}
	if errors.Is(err, kernel.ErrOwnerRequiresPersonal) {
		return orgapi.OwnerRequiresPersonal
	}
	for _, pair := range []struct {
		source error
		code   orgapi.Code
	}{
		{app.ErrRoleDisabled, orgapi.NotFound},
		{app.ErrRoleTargetNotFound, orgapi.NotFound},
		{app.ErrNotFound, orgapi.NotFound},
		{app.ErrRoleUnauthenticated, orgapi.Unauthenticated},
		{app.ErrRoleOperationReused, orgapi.OperationReused},
		{app.ErrRoleTargetNotActive, orgapi.ValidationFailed},
		{d.ErrCannotAssignOwner, orgapi.Forbidden},
		{d.ErrSelfModificationForbidden, orgapi.Forbidden},
		{d.ErrSelfAssignmentForbidden, orgapi.Forbidden},
		{d.ErrEscalation, orgapi.Forbidden},
		{d.ErrSelfAssignment, orgapi.Forbidden},
		{d.ErrAssignmentConflict, orgapi.Conflict},
		{d.ErrAssignmentEnded, orgapi.Conflict},
		{d.ErrAssignmentInvalid, orgapi.ValidationFailed},
		{app.ErrRoleInvalid, orgapi.ValidationFailed},
		{d.ErrAssignmentScope, orgapi.Forbidden},
		{app.ErrNoMembership, orgapi.NotFound},
		{app.ErrOrgSuspended, orgapi.OrgSuspended},
		{app.ErrOrgArchived, orgapi.OrgArchived},
		{app.ErrSetupIncomplete, orgapi.OrgSetupIncomplete},
		{app.ErrEntitlementRequired, orgapi.EntitlementRequired},
	} {
		if errors.Is(err, pair.source) {
			return pair.code
		}
	}
	return orgapi.Unavailable
}

func roleAssignmentView(scope kernel.OrgScope, a d.Assignment) (map[string]any, error) {
	snap, err := a.Snapshot(scope)
	if err != nil {
		return nil, orgapi.Unavailable
	}
	res := map[string]any{
		"id":            snap.ID,
		"membership_id": snap.MembershipID,
		"role":          string(snap.Role),
		"scope_type":    string(snap.Scope),
		"scope_target":  snap.TargetID,
		"valid_from":    snap.ValidFrom.Format(time.RFC3339Nano),
		"assigned_by":   snap.AssignedBy,
		"reason":        snap.Reason,
		"revision":      snap.Revision,
		"data_mode":     "SYNTHETIC",
	}
	if !snap.ValidTo.IsZero() {
		res["valid_to"] = snap.ValidTo.Format(time.RFC3339Nano)
	}
	if !snap.EndedAt.IsZero() {
		res["ended_at"] = snap.EndedAt.Format(time.RFC3339Nano)
	}
	return res, nil
}

func roleAck(scope kernel.OrgScope, r app.RoleResult) (map[string]any, error) {
	view, err := roleAssignmentView(scope, r.Assignment)
	if err != nil {
		return nil, err
	}
	view["replayed"] = r.Replayed
	return view, nil
}

func roleChangeAck(scope kernel.OrgScope, r app.RoleChangeResult) (map[string]any, error) {
	endedView, err := roleAssignmentView(scope, r.EndedAssignment)
	if err != nil {
		return nil, err
	}
	newView, err := roleAssignmentView(scope, r.NewAssignment)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ended_assignment": endedView,
		"new_assignment":   newView,
		"replayed":         r.Replayed,
		"data_mode":        "SYNTHETIC",
	}, nil
}

func rolePage(p orgapi.Principal, gateway orgapi.Gateway, path, filters string, page app.RolePage) (map[string]any, error) {
	items := make([]map[string]any, 0, len(page.Assignments))
	for _, a := range page.Assignments {
		v, err := roleAssignmentView(p.Scope, a)
		if err != nil {
			return nil, err
		}
		items = append(items, v)
	}
	next := ""
	var err error
	if page.NextID != "" {
		next, err = gateway.SealCursor(p, path, filters, page.NextID)
		if err != nil {
			return nil, orgapi.Unavailable
		}
	}
	return map[string]any{"items": items, "next_cursor": next}, nil
}

func RegisterRoles(router chi.Router, service app.RoleService, cookies company.Handler, gateway orgapi.Gateway, authorization app.Authorization, roots RootResolver, lookup app.Repository) error {
	if !service.Enabled {
		return nil
	}
	if service.Repository == nil || service.NewID == nil || roots == nil || lookup == nil {
		return fmt.Errorf("orgaccess: incomplete role service dependencies")
	}
	owned, gateway, err := companyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	resource := assignmentResource{lookup}
	rootResource := RootFacts{Resolver: roots}

	// 1. POST /v1/orgs/{org_id}/roles/assign
	if err := Register(owned, RoleAssignPath, gateway, roleRule(app.RoleAssignAction, false, 201), authorization, app.RolePolicy(app.RoleAssignAction), rootResource, func(ctx context.Context, p orgapi.Principal, decision app.Decision, input roleAssignRequest) (any, error) {
		a, err := currentCompanyActor(ctx)
		if err != nil {
			return nil, err
		}
		var validFrom, validTo time.Time
		if input.ValidFrom != nil {
			validFrom = *input.ValidFrom
		}
		if input.ValidTo != nil {
			validTo = *input.ValidTo
		}
		in := app.RoleAssignInput{
			OperationID:  input.OperationID,
			MembershipID: input.MembershipID,
			Role:         d.Role(input.Role),
			Scope:        d.ScopeType(input.Scope),
			TargetID:     input.TargetID,
			ValidFrom:    validFrom,
			ValidTo:      validTo,
			Reason:       input.Reason,
		}
		result, err := service.Assign(ctx, p.Scope, a, decision, in)
		if err != nil {
			return nil, roleMapError(err)
		}
		return roleAck(p.Scope, result)
	}); err != nil {
		return err
	}

	// 2. POST /v1/orgs/{org_id}/roles/{assignment_id}/end
	if err := Register(owned, RoleEndPath, gateway, roleRule(app.RoleEndAction, true, 200), authorization, app.RolePolicy(app.RoleEndAction), resource, func(ctx context.Context, p orgapi.Principal, decision app.Decision, input roleEndRequest) (any, error) {
		a, err := currentCompanyActor(ctx)
		if err != nil {
			return nil, err
		}
		id, err := pathAssignmentID(ctx)
		if err != nil {
			return nil, orgapi.NotFound
		}
		in := app.RoleEndInput{
			OperationID:      input.OperationID,
			AssignmentID:     id,
			ExpectedRevision: *input.Revision,
			Reason:           input.Reason,
		}
		result, err := service.End(ctx, p.Scope, a, decision, in)
		if err != nil {
			return nil, roleMapError(err)
		}
		return roleAck(p.Scope, result)
	}); err != nil {
		return err
	}

	// 3. POST /v1/orgs/{org_id}/roles/{assignment_id}/change
	if err := Register(owned, RoleChangePath, gateway, roleRule(app.RoleChangeAction, true, 200), authorization, app.RolePolicy(app.RoleChangeAction), resource, func(ctx context.Context, p orgapi.Principal, decision app.Decision, input roleChangeRequest) (any, error) {
		a, err := currentCompanyActor(ctx)
		if err != nil {
			return nil, err
		}
		id, err := pathAssignmentID(ctx)
		if err != nil {
			return nil, orgapi.NotFound
		}
		var validFrom, validTo time.Time
		if input.ValidFrom != nil {
			validFrom = *input.ValidFrom
		}
		if input.ValidTo != nil {
			validTo = *input.ValidTo
		}
		in := app.RoleChangeInput{
			OperationID:      input.OperationID,
			AssignmentID:     id,
			ExpectedRevision: *input.Revision,
			Role:             d.Role(input.Role),
			Scope:            d.ScopeType(input.Scope),
			TargetID:         input.TargetID,
			ValidFrom:        validFrom,
			ValidTo:          validTo,
			Reason:           input.Reason,
		}
		result, err := service.Change(ctx, p.Scope, a, decision, in)
		if err != nil {
			return nil, roleMapError(err)
		}
		return roleChangeAck(p.Scope, result)
	}); err != nil {
		return err
	}

	// 4. POST /v1/orgs/{org_id}/roles/{assignment_id}/read
	if err := Register(owned, RoleReadPath, gateway, roleRule(app.RoleReadAction, false, 200), authorization, app.RolePolicy(app.RoleReadAction), resource, func(ctx context.Context, p orgapi.Principal, decision app.Decision, _ roleReadRequest) (any, error) {
		a, err := currentCompanyActor(ctx)
		if err != nil {
			return nil, err
		}
		id, err := pathAssignmentID(ctx)
		if err != nil {
			return nil, orgapi.NotFound
		}
		assignment, err := service.Read(ctx, p.Scope, a, decision, id)
		if err != nil {
			return nil, roleMapError(err)
		}
		return roleAssignmentView(p.Scope, assignment)
	}); err != nil {
		return err
	}

	// 5. POST /v1/orgs/{org_id}/roles/list
	return Register(owned, RoleListPath, gateway, roleRule(app.RoleListAction, false, 200), authorization, app.RolePolicy(app.RoleListAction), rootResource, func(ctx context.Context, p orgapi.Principal, decision app.Decision, input roleListRequest) (any, error) {
		a, err := currentCompanyActor(ctx)
		if err != nil {
			return nil, err
		}
		page, err := input.Page.Normalize()
		if err != nil {
			return nil, orgapi.Malformed
		}
		f := app.RoleListFilter{
			MembershipID: input.MembershipID,
			Role:         d.Role(input.Role),
			State:        input.State,
			Limit:        page.Limit,
		}
		if f.Validate() != nil {
			return nil, orgapi.Malformed
		}
		filters := fmt.Sprintf("roles.v1:%s:%s:%s:%d", input.MembershipID, input.Role, input.State, page.Limit)
		f.AfterID, err = gateway.OpenCursor(p, RoleListPath, filters, page.Cursor)
		if err != nil {
			return nil, orgapi.Malformed
		}
		result, err := service.List(ctx, p.Scope, a, decision, f)
		if err != nil {
			return nil, roleMapError(err)
		}
		return rolePage(p, gateway, RoleListPath, filters, result)
	})
}
