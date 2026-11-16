package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

// The tracking policy routes (B-BR-EMP-467..469, EDR-302).
const (
	PolicyReadPath   = "/v1/orgs/{org_id}/tracking/policy/read"
	PolicyChangePath = "/v1/orgs/{org_id}/tracking/policy/change"
)

// PolicyRecords accepts only the two tracking policy policies over the
// organization itself.
type PolicyRecords struct{}

func (PolicyRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.OrgID != scope.OrgID() || facts.ID != scope.OrgID().String() || len(rules) == 0 ||
		policy != app.PolicySettingsPolicy(app.PolicyRead) && policy != app.PolicySettingsPolicy(app.PolicyManage) {
		return access.ErrUnavailable
	}
	return nil
}

// policyRoot resolves the organization itself: no client-selected target.
type policyRoot struct{}

func (policyRoot) Resolve(_ context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String()}, nil
}

// strictObject decodes a closed JSON object: unknown fields, duplicate keys
// and non-objects are MALFORMED.
func strictObject[T any](raw []byte, target *T) error {
	if httpapi.DecodeJSON(raw, target) != nil {
		return orgapi.Malformed
	}
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if decoder.Decode(&fields) != nil || fields == nil {
		return orgapi.Malformed
	}
	return nil
}

type policyReadRequest struct{}

func (policyReadRequest) Credentials() orgapi.Session { return orgapi.Session{} }

func (v *policyReadRequest) UnmarshalJSON(raw []byte) error {
	type plain policyReadRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = policyReadRequest(in)
	return nil
}

// policyChangeRequest carries no session, member, coordinate or notice text.
type policyChangeRequest struct {
	SchemaVersion   int     `json:"schema_version"`
	OperationID     string  `json:"operation_id"`
	Revision        *int64  `json:"revision"`
	Policy          string  `json:"policy"`
	NoticeVersion   *string `json:"notice_version"`
	NoticeInForceAt *string `json:"notice_in_force_at"`
}

func (policyChangeRequest) Credentials() orgapi.Session { return orgapi.Session{} }

func (v policyChangeRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}

func (v *policyChangeRequest) UnmarshalJSON(raw []byte) error {
	type plain policyChangeRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = policyChangeRequest(in)
	return nil
}

func policyMapped(err error) error {
	var field *app.PolicyField
	if errors.As(err, &field) {
		return orgapi.TrackingPolicyValidation(field.Field, field.Code)
	}
	for _, pair := range []struct {
		source error
		code   orgapi.Code
	}{
		{app.ErrPolicyConflict, orgapi.Conflict}, {app.ErrPolicyOperationReuse, orgapi.OperationReused}, {app.ErrPolicyDisabled, orgapi.NotFound},
		{app.ErrPolicyUnauthorized, orgapi.Unauthenticated}, {app.ErrMalformed, orgapi.Malformed},
	} {
		if errors.Is(err, pair.source) {
			return pair.code
		}
	}
	return trackMapped(err)
}

func optionalInstant(at *time.Time) any {
	if at == nil {
		return nil
	}
	return orgtime.FormatInstant(*at)
}

func nullableText(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// RegisterPolicy mounts the tracking policy read and change.
func RegisterPolicy(router chi.Router, service app.PolicyService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
	if !service.Enabled {
		return nil
	}
	if router == nil || missing(service.Repository) || missing(authorization.Source) || missing(authorization.Capabilities) || missing(authorization.Records) || authorization.Now == nil {
		return app.ErrUnavailable
	}
	owned, gateway, err := organization.CompanyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	actorOf := func(ctx context.Context) (app.PolicyActor, error) {
		actor, err := organization.CurrentCompanyActor(ctx)
		if err != nil {
			return app.PolicyActor{}, err
		}
		return app.PolicyActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, nil
	}
	read := app.PolicySettingsPolicy(app.PolicyRead)
	readRule := orgapi.Rule{Permission: app.PolicyReadPermission, Write: false, Fresh: read.Fresh, CompanyOnly: true, Success: 200}
	if err := guarded.Register(owned, PolicyReadPath, gateway, readRule, authorization, read, policyRoot{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ policyReadRequest) (any, error) {
		actor, err := actorOf(ctx)
		if err != nil {
			return nil, err
		}
		v, err := service.Read(ctx, p.Scope, actor, decision)
		if err != nil {
			return nil, policyMapped(err)
		}
		return map[string]any{"revision": v.Revision, "policy": v.Policy, "notice_version": nullableText(v.NoticeVersion), "notice_in_force_at": optionalInstant(v.NoticeInForce),
			"effective_at": optionalInstant(v.EffectiveAt), "in_force": map[string]any{"policy": v.InForce, "revision": v.InForceRevision}, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}
	manage := app.PolicySettingsPolicy(app.PolicyManage)
	changeRule := orgapi.Rule{Permission: app.PolicyManagePermission, Write: true, Fresh: manage.Fresh, RequireRevision: false, CompanyOnly: true, Success: 200}
	return guarded.Register(owned, PolicyChangePath, gateway, changeRule, authorization, manage, policyRoot{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input policyChangeRequest) (any, error) {
		actor, err := actorOf(ctx)
		if err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 {
			return nil, orgapi.Malformed
		}
		in := app.PolicyCommandInput{OperationID: input.OperationID, Policy: input.Policy, NoticeVersion: input.NoticeVersion}
		if input.Revision != nil {
			in.ExpectedRevision = *input.Revision
		}
		if input.NoticeInForceAt != nil {
			at, err := orgtime.ParseInstant(*input.NoticeInForceAt)
			if err != nil {
				return nil, orgapi.Malformed
			}
			in.NoticeInForceAt = &at
		}
		command, err := app.NewPolicyCommand(in)
		if err != nil {
			return nil, policyMapped(err)
		}
		ack, replayed, err := service.Change(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, policyMapped(err)
		}
		return map[string]any{"revision": ack.Revision, "policy": ack.Policy, "notice_version": nullableText(ack.NoticeVersion), "notice_in_force_at": optionalInstant(ack.NoticeInForce),
			"effective_at": orgtime.FormatInstant(ack.EffectiveAt), "replayed": replayed, "data_mode": "SYNTHETIC"}, nil
	})
}
