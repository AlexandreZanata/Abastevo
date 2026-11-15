// Package http exposes organization export routes through orgapi (EMP-RPT-001).
package http

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const (
	ExportPrefix = "/v1/orgs/{org_id}/reports/exports"
	RequestPath  = ExportPrefix + "/request"
	ListPath     = ExportPrefix + "/list"
	ReadPath     = ExportPrefix + "/{export_id}/read"
	AccessPath   = ExportPrefix + "/{export_id}/access"
	ContentPath  = ExportPrefix + "/{export_id}/content"
)

// ExportRecords accepts only the export policies on the organization target;
// requester binding is enforced by the repository (EDR-157).
type ExportRecords struct{}

func (ExportRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.Validate(scope) != nil || facts.OrgID != scope.OrgID() || facts.ID != scope.OrgID().String() || len(rules) == 0 ||
		policy != app.Policy(app.ActionRequest) && policy != app.Policy(app.ActionRead) {
		return access.ErrUnavailable
	}
	return nil
}

// exportRoot resolves the organization itself: no client-selected target.
type exportRoot struct{}

func (exportRoot) Resolve(_ context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String()}, nil
}

type companyRequest struct{}

func (companyRequest) Credentials() orgapi.Session { return orgapi.Session{} }

type exportRequest struct {
	companyRequest
	SchemaVersion int             `json:"schema_version"`
	OperationID   string          `json:"operation_id"`
	Kind          string          `json:"kind"`
	Parameters    json.RawMessage `json:"parameters"`
}

func (v exportRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID}
}

type exportListRequest struct {
	companyRequest
	orgapi.Page
	State string `json:"state,omitempty"`
}

type exportReadRequest struct{ companyRequest }

type exportContentRequest struct {
	companyRequest
	Capability string `json:"capability"`
}

func (v *exportRequest) UnmarshalJSON(raw []byte) error {
	type plain exportRequest
	var in plain
	if err := decodeStrict(raw, &in, "kind", "parameters"); err != nil {
		return err
	}
	*v = exportRequest(in)
	return nil
}
func (v *exportListRequest) UnmarshalJSON(raw []byte) error {
	type plain exportListRequest
	var in plain
	if err := decodeStrict(raw, &in); err != nil {
		return err
	}
	*v = exportListRequest(in)
	return nil
}
func (v *exportReadRequest) UnmarshalJSON(raw []byte) error {
	type plain exportReadRequest
	var in plain
	if err := decodeStrict(raw, &in); err != nil {
		return err
	}
	*v = exportReadRequest(in)
	return nil
}
func (v *exportContentRequest) UnmarshalJSON(raw []byte) error {
	type plain exportContentRequest
	var in plain
	if err := decodeStrict(raw, &in, "capability"); err != nil || in.Capability == "" || len(in.Capability) > app.MaxCapability {
		return orgapi.Malformed
	}
	*v = exportContentRequest(in)
	return nil
}

// decodeStrict decodes one object without unknown or duplicate fields, with
// the named fields required and no top-level null.
func decodeStrict[T any](raw []byte, target *T, required ...string) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return orgapi.Malformed
	}
	for _, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return orgapi.Malformed
		}
	}
	for _, key := range required {
		if fields[key] == nil {
			return orgapi.Malformed
		}
	}
	if httpapi.DecodeJSON(raw, target) != nil {
		return orgapi.Malformed
	}
	return nil
}

func decodeExport[T any](raw []byte, target *T) error {
	if err := json.Unmarshal(raw, target); err != nil {
		return orgapi.Malformed
	}
	return nil
}

// Mapped converts module refusals into registered organization codes.
func Mapped(err error) error {
	var denied *permissions.PermissionDenial
	if errors.As(err, &denied) {
		return orgapi.MissingPermission(string(denied.Permission()))
	}
	if code, ok := orgapi.Registered(err); ok {
		return code
	}
	for _, pair := range []struct {
		source error
		code   orgapi.Code
	}{
		{app.ErrFormatUnavailable, orgapi.ExportFormatUnavailable}, {app.ErrQueueFull, orgapi.ExportQueueFull}, {app.ErrNotReady, orgapi.ExportNotReady}, {app.ErrExpired, orgapi.ExportExpired},
		{app.ErrSurfaceDisabled, orgapi.NotFound}, {app.ErrNotFound, orgapi.NotFound}, {app.ErrUnauthenticated, orgapi.Unauthenticated}, {app.ErrOperationReused, orgapi.OperationReused},
		{domain.ErrInvalid, orgapi.ValidationFailed},
		{access.ErrNoMembership, orgapi.NotFound}, {access.ErrNotFound, orgapi.NotFound}, {access.ErrUnauthenticated, orgapi.Unauthenticated},
		{access.ErrFreshAuthRequired, orgapi.FreshAuthRequired}, {access.ErrMembershipSuspended, orgapi.MembershipSuspended}, {access.ErrOrgSuspended, orgapi.OrgSuspended},
		{access.ErrOrgArchived, orgapi.OrgArchived}, {access.ErrSetupIncomplete, orgapi.OrgSetupIncomplete}, {access.ErrEntitlementRequired, orgapi.EntitlementRequired},
	} {
		if errors.Is(err, pair.source) {
			return pair.code
		}
	}
	return orgapi.Unavailable
}

// optional formats a present instant and leaves an absent one null.
func optional(at time.Time) any {
	if at.IsZero() {
		return nil
	}
	return orgtime.FormatInstant(at)
}

func projectView(v app.View) (map[string]any, error) {
	if v.Validate() != nil {
		return nil, orgapi.Unavailable
	}
	var parameters map[string]any
	decoder := json.NewDecoder(bytes.NewReader(v.Parameters))
	decoder.UseNumber()
	if decoder.Decode(&parameters) != nil {
		return nil, orgapi.Unavailable
	}
	out := map[string]any{"export_id": v.ExportID, "kind": v.Kind, "parameters": parameters, "parameters_digest": v.ParametersDigest, "state": v.State,
		"attempts": v.Attempts, "max_attempts": v.MaxAttempts, "failure_code": nil, "row_count": nil, "byte_count": nil, "sha256": nil, "media_type": nil,
		"requested_at": orgtime.FormatInstant(v.RequestedAt), "as_of": orgtime.FormatInstant(v.AsOf),
		"ready_at": optional(v.ReadyAt), "expires_at": optional(v.ExpiresAt), "failed_at": optional(v.FailedAt), "expired_at": optional(v.ExpiredAt),
		"data_mode": "SYNTHETIC"}
	if v.FailureCode != "" {
		out["failure_code"] = v.FailureCode
	}
	if v.SHA256 != "" {
		out["row_count"], out["byte_count"], out["sha256"], out["media_type"] = v.Rows, v.Bytes, v.SHA256, v.MediaType
	}
	return out, nil
}

func projectCapability(exportID string, c app.Capability) (map[string]any, error) {
	if !domain.ValidID(exportID) || c.Token == "" || len(c.Token) > app.MaxCapability || c.ExpiresAt.IsZero() {
		return nil, orgapi.Unavailable
	}
	return map[string]any{"export_id": exportID, "capability": c.Token, "expires_at": orgtime.FormatInstant(c.ExpiresAt)}, nil
}

func projectContent(c app.Content) (map[string]any, error) {
	if !domain.ValidID(c.ExportID) || !domain.ValidMediaType(c.MediaType) || !domain.ValidSHA256(c.SHA256) || len(c.Bytes) == 0 || int64(len(c.Bytes)) > domain.MaxBytes || c.Rows < 0 {
		return nil, orgapi.Unavailable
	}
	return map[string]any{"export_id": c.ExportID, "media_type": c.MediaType, "sha256": c.SHA256, "row_count": c.Rows, "byte_count": int64(len(c.Bytes)),
		"content_base64": base64.StdEncoding.EncodeToString(c.Bytes)}, nil
}

func pathExport(ctx context.Context) (string, error) {
	raw := chi.URLParamFromCtx(ctx, "export_id")
	if !domain.ValidID(raw) {
		return "", orgapi.NotFound
	}
	return raw, nil
}

func currentActor(ctx context.Context) (app.Actor, error) {
	actor, err := organization.CurrentCompanyActor(ctx)
	if err != nil {
		return app.Actor{}, err
	}
	return app.Actor{AccountID: actor.AccountID, SessionID: actor.SessionID}, nil
}

func exportRule(action app.Action) orgapi.Rule {
	policy := app.Policy(action)
	if action == app.ActionRequest {
		return orgapi.Rule{Permission: string(policy.Permission), Write: true, Fresh: policy.Fresh, CompanyOnly: true, Success: 202}
	}
	return orgapi.Rule{Permission: string(policy.Permission), Fresh: policy.Fresh, CompanyOnly: true, Success: 200}
}

func missing(port any) bool {
	if port == nil {
		return true
	}
	v := reflect.ValueOf(port)
	switch v.Kind() {
	case reflect.Pointer, reflect.Func, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan:
		return v.IsNil()
	}
	return false
}

// RegisterExports mounts the five guarded export routes.
func RegisterExports(router chi.Router, service app.Service, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
	if !service.Enabled {
		return nil
	}
	if router == nil || missing(service.Repository) || missing(service.Tokens) || missing(service.Recorder) || missing(service.Objects) || service.NewID == nil || service.Now == nil ||
		missing(authorization.Source) || missing(authorization.Capabilities) || missing(authorization.Records) || authorization.Now == nil {
		return app.ErrUnavailable
	}
	owned, gateway, err := organization.CompanyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	root := exportRoot{}
	if err := guarded.Register(owned, RequestPath, gateway, exportRule(app.ActionRequest), authorization, app.Policy(app.ActionRequest), root, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input exportRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		view, replayed, err := service.Request(ctx, p.Scope, actor, decision, app.CommandInput{OperationID: input.OperationID, Kind: input.Kind, Parameters: input.Parameters})
		if err != nil {
			return nil, Mapped(err)
		}
		projected, err := projectView(view)
		if err != nil {
			return nil, err
		}
		return map[string]any{"export": projected, "replayed": replayed}, nil
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, ListPath, gateway, exportRule(app.ActionRead), authorization, app.Policy(app.ActionRead), root, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input exportListRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		page, err := input.Page.Normalize()
		if err != nil {
			return nil, orgapi.Malformed
		}
		filter := app.Filter{State: domain.State(input.State), Limit: page.Limit}
		if filter.Validate() != nil {
			return nil, orgapi.Malformed
		}
		binding := fmt.Sprintf("reports.exports.v1:%s:%d", input.State, page.Limit)
		after, err := gateway.OpenCursor(p, ListPath, binding, page.Cursor)
		if err != nil {
			return nil, orgapi.Malformed
		}
		if after != "" {
			if filter.After, err = app.ParseKey(after); err != nil {
				return nil, orgapi.Malformed
			}
		}
		result, err := service.List(ctx, p.Scope, actor, decision, filter)
		if err != nil {
			return nil, Mapped(err)
		}
		items := make([]map[string]any, 0, len(result.Exports))
		for _, v := range result.Exports {
			item, err := projectView(v)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		next := ""
		if !result.Next.Zero() {
			if next, err = gateway.SealCursor(p, ListPath, binding, result.Next.String()); err != nil {
				return nil, orgapi.Unavailable
			}
		}
		return map[string]any{"items": items, "next_cursor": next}, nil
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, ReadPath, gateway, exportRule(app.ActionRead), authorization, app.Policy(app.ActionRead), root, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ exportReadRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		export, err := pathExport(ctx)
		if err != nil {
			return nil, err
		}
		view, err := service.Read(ctx, p.Scope, actor, decision, export)
		if err != nil {
			return nil, Mapped(err)
		}
		return projectView(view)
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, AccessPath, gateway, exportRule(app.ActionRead), authorization, app.Policy(app.ActionRead), root, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ exportReadRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		export, err := pathExport(ctx)
		if err != nil {
			return nil, err
		}
		capability, err := service.Access(ctx, p.Scope, actor, decision, export)
		if err != nil {
			return nil, Mapped(err)
		}
		return projectCapability(export, capability)
	}); err != nil {
		return err
	}
	return guarded.Register(owned, ContentPath, gateway, exportRule(app.ActionRead), authorization, app.Policy(app.ActionRead), root, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input exportContentRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		export, err := pathExport(ctx)
		if err != nil {
			return nil, err
		}
		content, err := service.Open(ctx, p.Scope, actor, decision, export, input.Capability)
		if err != nil {
			return nil, Mapped(err)
		}
		return projectContent(content)
	})
}
