package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"net/url"
	"strings"
	"time"
)

const InvitationPrefix = "/v1/orgs/{org_id}/invitations"
const InvitationIssuePath = InvitationPrefix + "/issue"
const InvitationResendPath = InvitationPrefix + "/{invitation_id}/resend"
const InvitationRevokePath = InvitationPrefix + "/{invitation_id}/revoke"
const InvitationReadPath = InvitationPrefix + "/{invitation_id}/read"
const InvitationListPath = InvitationPrefix + "/list"

type invitationIssueRequest struct {
	companyRequest
	SchemaVersion int                 `json:"schema_version"`
	OperationID   string              `json:"operation_id"`
	EmployeeID    string              `json:"employee_id"`
	Grants        []d.InvitationGrant `json:"grants"`
}

func (v invitationIssueRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID}
}

type invitationRevisionRequest struct {
	companyRequest
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	Revision      *int64 `json:"revision"`
}

func (v invitationRevisionRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}

type invitationReadRequest struct{ companyRequest }
type invitationListRequest struct {
	companyRequest
	orgapi.Page
	EmployeeID string            `json:"employee_id,omitempty"`
	State      d.InvitationState `json:"state,omitempty"`
}

func invitationCanonical(v any) bool {
	switch value := v.(type) {
	case map[string]any:
		for k, child := range value {
			if !fieldPattern.MatchString(k) || !invitationCanonical(child) {
				return false
			}
		}
	case []any:
		for _, child := range value {
			if !invitationCanonical(child) {
				return false
			}
		}
	case nil:
		return false
	}
	return true
}
func decodeInvitation(raw []byte, target any) error {
	var fields map[string]any
	if json.Unmarshal(raw, &fields) != nil || fields == nil || !invitationCanonical(fields) {
		return orgapi.Malformed
	}
	return decode(raw, target)
}
func (v *invitationIssueRequest) UnmarshalJSON(raw []byte) error {
	type plain invitationIssueRequest
	return decodeInvitation(raw, (*plain)(v))
}
func (v *invitationRevisionRequest) UnmarshalJSON(raw []byte) error {
	type plain invitationRevisionRequest
	return decodeInvitation(raw, (*plain)(v))
}
func (v *invitationReadRequest) UnmarshalJSON(raw []byte) error {
	type plain invitationReadRequest
	return decodeInvitation(raw, (*plain)(v))
}
func (v *invitationListRequest) UnmarshalJSON(raw []byte) error {
	type plain invitationListRequest
	if e := decodeInvitation(raw, (*plain)(v)); e != nil {
		return e
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return orgapi.Malformed
	}
	if _, has := fields["limit"]; has && (v.Limit < 1 || v.Limit > 100) {
		return orgapi.Malformed
	}
	return nil
}

type InvitationRecords struct{}

func (InvitationRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || subject.MembershipID() == "" || facts.OrgID != scope.OrgID() || facts.ID != scope.OrgID().String() || len(rules) == 0 || policy != app.InvitationPolicy(app.InvitationIssue) && policy != app.InvitationPolicy(app.InvitationRead) && policy != app.InvitationPolicy(app.InvitationRevoke) {
		return access.ErrUnavailable
	}
	return nil
}
func invitationMapped(e error) error {
	if errors.Is(e, app.ErrInvitationOperationReused) {
		return orgapi.OperationReused
	}
	if errors.Is(e, app.ErrEmployeeHasAccess) {
		return orgapi.EmployeeHasAccess
	}
	if errors.Is(e, app.ErrInvitationConflict) || errors.Is(e, d.ErrConditions) {
		return orgapi.Conflict
	}
	if errors.Is(e, app.ErrNotFound) {
		return orgapi.NotFound
	}
	return mapped(e)
}
func invitationView(scope kernel.OrgScope, value d.Invitation) (map[string]any, error) {
	v, e := value.Snapshot(scope)
	if e != nil {
		return nil, orgapi.Unavailable
	}
	grants := make([]map[string]string, 0, len(v.Grants))
	for _, g := range v.Grants {
		grants = append(grants, map[string]string{"role": g.Role, "scope": g.Scope})
	}
	view := map[string]any{"id": v.ID, "employee_id": v.EmployeeID, "membership_id": v.MembershipID, "issuer_membership_id": v.IssuerMembershipID, "operation_id": v.OperationID, "state": v.State, "revision": v.Revision, "settings_revision": v.SettingsRevision, "validity_days": v.ValidityDays, "failed_attempts": v.FailedAttempts, "created_at": v.CreatedAt.UTC().Format(time.RFC3339Nano), "expires_at": v.ExpiresAt.UTC().Format(time.RFC3339Nano), "changed_at": v.ChangedAt.UTC().Format(time.RFC3339Nano), "grants": grants, "cause": v.Cause, "notice_state": "PENDING_NOTIFICATION_CORE", "data_mode": "SYNTHETIC"}
	if v.SourceID != "" {
		view["source"] = map[string]any{"type": v.SourceType, "id": v.SourceID, "revision": v.SourceRevision}
	}
	return view, nil
}
func invitationPathID(ctx context.Context) (string, error) {
	raw := chi.URLParamFromCtx(ctx, "invitation_id")
	id, e := kernel.ParseOrgID(raw)
	if e != nil || id.String() != raw {
		return "", orgapi.NotFound
	}
	return raw, nil
}
func invitationRule(action app.InvitationAction) orgapi.Rule {
	p := app.InvitationPolicy(action)
	return orgapi.Rule{Permission: string(p.Permission), Write: p.Effect.Write(), Fresh: p.Fresh, RequireRevision: action == app.InvitationResend || action == app.InvitationRevoke, CompanyOnly: true, Success: 200}
}
func RegisterInvitations(router chi.Router, service app.InvitationService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization, roots org.Repository) error {
	if !service.Enabled {
		return nil
	}
	if service.Repository == nil || service.NewID == nil || service.NewSecret == nil || roots == nil {
		return app.ErrRegisterUnavailable
	}
	owned, gateway, e := orgweb.CompanyEntry(router, cookies, gateway)
	if e != nil {
		return e
	}
	facts := orgweb.RootFacts{Roots: roots}
	ack := func(scope kernel.OrgScope, result app.InvitationResult) (any, error) {
		v, e := invitationView(scope, result.Invitation)
		if e != nil {
			return nil, e
		}
		body := map[string]any{"invitation": v, "replayed": result.Replayed}
		if result.Code != "" {
			link := strings.TrimSuffix(gateway.Origin, "/") + "/empresas/#invitation=" + url.QueryEscape(result.Code)
			body["delivery"] = map[string]string{"code": result.Code, "link": link, "qr_payload": link}
		}
		return body, nil
	}
	if e := guarded.Register(owned, InvitationIssuePath, gateway, invitationRule(app.InvitationIssue), authorization, app.InvitationPolicy(app.InvitationIssue), facts, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input invitationIssueRequest) (any, error) {
		a, e := actor(ctx)
		if e != nil {
			return nil, e
		}
		result, e := service.Write(ctx, p.Scope, a, decision, app.InvitationWrite{Action: app.InvitationIssue, OperationID: input.OperationID, EmployeeID: input.EmployeeID, Grants: input.Grants})
		if e != nil {
			return nil, invitationMapped(e)
		}
		return ack(p.Scope, result)
	}); e != nil {
		return e
	}
	for _, route := range []struct {
		path   string
		action app.InvitationAction
	}{{InvitationResendPath, app.InvitationResend}, {InvitationRevokePath, app.InvitationRevoke}} {
		if e := guarded.Register(owned, route.path, gateway, invitationRule(route.action), authorization, app.InvitationPolicy(route.action), facts, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input invitationRevisionRequest) (any, error) {
			a, e := actor(ctx)
			if e != nil {
				return nil, e
			}
			id, e := invitationPathID(ctx)
			if e != nil {
				return nil, e
			}
			result, e := service.Write(ctx, p.Scope, a, decision, app.InvitationWrite{Action: route.action, OperationID: input.OperationID, InvitationID: id, ExpectedRevision: *input.Revision})
			if e != nil {
				return nil, invitationMapped(e)
			}
			return ack(p.Scope, result)
		}); e != nil {
			return e
		}
	}
	if e := guarded.Register(owned, InvitationReadPath, gateway, invitationRule(app.InvitationRead), authorization, app.InvitationPolicy(app.InvitationRead), facts, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ invitationReadRequest) (any, error) {
		a, e := actor(ctx)
		if e != nil {
			return nil, e
		}
		id, e := invitationPathID(ctx)
		if e != nil {
			return nil, e
		}
		page, e := service.List(ctx, p.Scope, a, decision, app.InvitationFilter{ID: id, Limit: 1})
		if e != nil {
			return nil, invitationMapped(e)
		}
		if len(page.Invitations) != 1 {
			return nil, orgapi.Unavailable
		}
		return invitationView(p.Scope, page.Invitations[0])
	}); e != nil {
		return e
	}
	return guarded.Register(owned, InvitationListPath, gateway, invitationRule(app.InvitationRead), authorization, app.InvitationPolicy(app.InvitationRead), facts, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input invitationListRequest) (any, error) {
		a, e := actor(ctx)
		if e != nil {
			return nil, e
		}
		page, e := input.Page.Normalize()
		if e != nil {
			return nil, orgapi.Malformed
		}
		filters := fmt.Sprintf("invitations.v1:%s:%s:%d", input.EmployeeID, input.State, page.Limit)
		after, e := gateway.OpenCursor(p, InvitationListPath, filters, page.Cursor)
		if e != nil {
			return nil, orgapi.Malformed
		}
		result, e := service.List(ctx, p.Scope, a, decision, app.InvitationFilter{EmployeeID: input.EmployeeID, State: input.State, AfterID: after, Limit: page.Limit})
		if e != nil {
			return nil, invitationMapped(e)
		}
		items := make([]map[string]any, 0, len(result.Invitations))
		for _, v := range result.Invitations {
			view, e := invitationView(p.Scope, v)
			if e != nil {
				return nil, e
			}
			items = append(items, view)
		}
		next := ""
		if result.NextID != "" {
			next, e = gateway.SealCursor(p, InvitationListPath, filters, result.NextID)
			if e != nil {
				return nil, orgapi.Unavailable
			}
		}
		return map[string]any{"items": items, "next_cursor": next}, nil
	})
}
