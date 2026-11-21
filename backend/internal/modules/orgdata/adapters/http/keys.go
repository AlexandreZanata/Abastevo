package http

import (
	"bytes"
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
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const (
	ListKeysPath  = "/v1/orgs/{org_id}/data/keys/list"
	IssueKeyPath  = "/v1/orgs/{org_id}/data/keys/issue"
	RotateKeyPath = "/v1/orgs/{org_id}/data/keys/{key_id}/rotate"
	RevokeKeyPath = "/v1/orgs/{org_id}/data/keys/{key_id}/revoke"
)

type keyRecords struct{ policy access.Policy }

func (k keyRecords) Resolve(_ context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String()}, nil
}

func (k keyRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, target permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || scope != subject.Scope() || policy != k.policy ||
		target.OrgID != scope.OrgID() || target.ID != scope.OrgID().String() || len(rules) == 0 {
		return access.ErrUnavailable
	}
	return nil
}

type listRequest struct{ orgapi.Page }

func (listRequest) Credentials() orgapi.Session { return orgapi.Session{} }
func (r *listRequest) UnmarshalJSON(raw []byte) error {
	type plain listRequest
	return keyRequestObject(raw, (*plain)(r))
}

type issueRequest struct {
	orgapi.Command
	Label     string         `json:"label"`
	Scopes    []domain.Scope `json:"scopes"`
	ExpiresAt time.Time      `json:"expires_at"`
}

func (r *issueRequest) UnmarshalJSON(raw []byte) error {
	type plain issueRequest
	return keyRequestObject(raw, (*plain)(r))
}

type rotateRequest struct {
	issueRequest
	OverlapUntil *time.Time `json:"overlap_until,omitempty"`
}

func (r *rotateRequest) UnmarshalJSON(raw []byte) error {
	type issueFields issueRequest
	var dto struct {
		issueFields
		OverlapUntil *time.Time `json:"overlap_until,omitempty"`
	}
	if err := keyRequestObject(raw, &dto); err != nil {
		return err
	}
	r.issueRequest = issueRequest(dto.issueFields)
	r.OverlapUntil = dto.OverlapUntil
	return nil
}

type revokeRequest struct{ orgapi.Command }

func (r *revokeRequest) UnmarshalJSON(raw []byte) error {
	type plain revokeRequest
	return keyRequestObject(raw, (*plain)(r))
}

func keyRequestObject(raw []byte, target any) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return orgapi.Malformed
	}
	return httpapi.DecodeJSON(raw, target)
}

type keyResponse struct {
	ID           string          `json:"id"`
	Label        string          `json:"label"`
	Scopes       []domain.Scope  `json:"scopes"`
	State        domain.KeyState `json:"state"`
	Revision     int64           `json:"revision"`
	IssuedAt     time.Time       `json:"issued_at"`
	ExpiresAt    time.Time       `json:"expires_at"`
	OverlapUntil *time.Time      `json:"overlap_until,omitempty"`
}

func keyDTO(key domain.Key) keyResponse {
	out := keyResponse{ID: key.ID, Label: key.Label, Scopes: key.Scopes, State: key.State, Revision: key.Revision,
		IssuedAt: key.IssuedAt, ExpiresAt: key.ExpiresAt}
	if !key.OverlapUntil.IsZero() {
		out.OverlapUntil = &key.OverlapUntil
	}
	return out
}

type listResponse struct {
	Keys       []keyResponse `json:"keys"`
	NextCursor string        `json:"next_cursor"`
}

type commandResponse struct {
	Key      keyResponse `json:"key"`
	Bearer   string      `json:"bearer,omitempty"`
	Replayed bool        `json:"replayed"`
}

func RegisterKeys(router chi.Router, service app.KeyCommands, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
	if service == nil {
		return app.ErrUnavailable
	}
	owned, gateway, err := organization.CompanyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	if err := register(owned, ListKeysPath, gateway, authorization, app.ListKeys, func(ctx context.Context, p orgapi.Principal, actor orgapp.CompanyActor, in listRequest) (any, error) {
		input, err := in.Page.Normalize()
		if err != nil {
			return nil, err
		}
		after, err := gateway.OpenCursor(p, ListKeysPath, "", input.Cursor)
		if err != nil {
			return nil, err
		}
		page, err := service.ListKeys(ctx, p.Scope, actor, p.MembershipID, after, input.Limit)
		if err != nil {
			return nil, keyError(err)
		}
		out := listResponse{Keys: make([]keyResponse, 0, len(page.Keys))}
		for _, key := range page.Keys {
			out.Keys = append(out.Keys, keyDTO(key))
		}
		if page.AfterID != "" {
			out.NextCursor, err = gateway.SealCursor(p, ListKeysPath, "", page.AfterID)
		}
		return out, err
	}); err != nil {
		return err
	}
	if err := register(owned, IssueKeyPath, gateway, authorization, app.IssueKey, func(ctx context.Context, p orgapi.Principal, actor orgapp.CompanyActor, in issueRequest) (any, error) {
		if in.Revision != nil {
			return nil, orgapi.Malformed
		}
		return write(ctx, service, p, actor, app.Command{Action: app.IssueKey, OperationID: in.OperationID,
			Label: in.Label, Scopes: in.Scopes, ExpiresAt: in.ExpiresAt})
	}); err != nil {
		return err
	}
	if err := register(owned, RotateKeyPath, gateway, authorization, app.RotateKey, func(ctx context.Context, p orgapi.Principal, actor orgapp.CompanyActor, in rotateRequest) (any, error) {
		input := app.Command{Action: app.RotateKey, OperationID: in.OperationID, ExpectedRevision: *in.Revision,
			Label: in.Label, Scopes: in.Scopes, ExpiresAt: in.ExpiresAt}
		if in.OverlapUntil != nil {
			input.OverlapUntil = *in.OverlapUntil
		}
		return write(ctx, service, p, actor, input)
	}); err != nil {
		return err
	}
	return register(owned, RevokeKeyPath, gateway, authorization, app.RevokeKey, func(ctx context.Context, p orgapi.Principal, actor orgapp.CompanyActor, in revokeRequest) (any, error) {
		return write(ctx, service, p, actor, app.Command{Action: app.RevokeKey, OperationID: in.OperationID, ExpectedRevision: *in.Revision})
	})
}

func register[T orgapi.Request](router chi.Router, path string, gateway orgapi.Gateway, authorization access.Authorization,
	action app.Action, serve func(context.Context, orgapi.Principal, orgapp.CompanyActor, T) (any, error),
) error {
	policy := app.KeyPolicy(action)
	records := keyRecords{policy: policy}
	authorization.Records = records
	rule := orgapi.Rule{Permission: string(policy.Permission), Fresh: policy.Fresh, CompanyOnly: true, Write: policy.Effect.Write(),
		RequireRevision: action == app.RotateKey || action == app.RevokeKey, Success: 200}
	return guarded.Register(router, path, gateway, rule, authorization, policy, records, func(ctx context.Context, p orgapi.Principal, _ access.Decision, in T) (any, error) {
		actor, err := organization.CurrentCompanyActor(ctx)
		if err != nil {
			return nil, orgapi.Unauthenticated
		}
		return serve(ctx, p, actor, in)
	})
}

func write(ctx context.Context, service app.KeyCommands, p orgapi.Principal, actor orgapp.CompanyActor, input app.Command) (any, error) {
	if input.Action != app.IssueKey {
		input.TargetID = chi.RouteContext(ctx).URLParam("key_id")
		if !audit.ValidID(input.TargetID) {
			return nil, orgapi.NotFound
		}
	}
	out, err := service.WriteKey(ctx, p.Scope, actor, p.MembershipID, input)
	if err != nil {
		return nil, keyError(err)
	}
	return commandResponse{Key: keyDTO(out.Key), Bearer: out.Bearer.Reveal(), Replayed: out.Replayed}, nil
}

func keyError(err error) error {
	switch {
	case errors.Is(err, permissions.ErrPermissionDenied):
		return orgapi.MissingPermission("data.key.manage")
	case errors.Is(err, domain.ErrKeyDenied), errors.Is(err, access.ErrNoMembership), errors.Is(err, orgapp.ErrNotFound):
		return orgapi.NotFound
	case errors.Is(err, access.ErrUnauthenticated), errors.Is(err, orgapp.ErrUnauthenticated):
		return orgapi.Unauthenticated
	case errors.Is(err, access.ErrOrgArchived):
		return orgapi.OrgArchived
	case errors.Is(err, access.ErrOrgSuspended):
		return orgapi.OrgSuspended
	case errors.Is(err, capability.ErrEntitlementRequired):
		return orgapi.EntitlementRequired
	case errors.Is(err, domain.ErrInvalidKey):
		return orgapi.ValidationFailed
	case errors.Is(err, domain.ErrKeyConflict):
		return orgapi.Conflict
	case errors.Is(err, audit.ErrReused):
		return orgapi.OperationReused
	default:
		return orgapi.Unavailable
	}
}
