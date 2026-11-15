// Package http exposes organization period commands and reads through orgapi.
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"unicode/utf8"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/application"
	period "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain/period"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const PeriodPrefix = "/v1/orgs/{org_id}/finance/periods"
const PeriodListPath = PeriodPrefix + "/list"
const PeriodReadPath = PeriodPrefix + "/{period}/read"
const PeriodClosePath = PeriodPrefix + "/{period}/close"
const PeriodReopenPath = PeriodPrefix + "/{period}/reopen"

// PeriodRecords accepts only this module's fixed period policies on the
// organization target; periods are organization-wide (EDR-148).
type PeriodRecords struct{}

func (PeriodRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.Validate(scope) != nil || facts.OrgID != scope.OrgID() || facts.ID != scope.OrgID().String() || len(rules) == 0 ||
		policy != app.PeriodPolicy(app.PeriodRead) && policy != app.PeriodPolicy(app.PeriodClose) && policy != app.PeriodPolicy(app.PeriodReopen) {
		return access.ErrUnavailable
	}
	return nil
}

// periodRoot resolves the organization itself: no client-selected target.
type periodRoot struct{}

func (periodRoot) Resolve(_ context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String()}, nil
}

type companyRequest struct{}

func (companyRequest) Credentials() orgapi.Session { return orgapi.Session{} }

type periodReadRequest struct{ companyRequest }
type periodListRequest struct {
	companyRequest
	orgapi.Page
	State string `json:"state,omitempty"`
}
type periodCommandFields struct {
	companyRequest
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	Revision      *int64 `json:"revision"`
}

func (v periodCommandFields) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}

type periodCloseRequest struct{ periodCommandFields }
type periodReopenRequest struct {
	periodCommandFields
	Reason string `json:"reason"`
}

func (v *periodReadRequest) UnmarshalJSON(raw []byte) error {
	type plain periodReadRequest
	var in plain
	if err := decodePeriod(raw, &in); err != nil {
		return err
	}
	*v = periodReadRequest(in)
	return nil
}
func (v *periodListRequest) UnmarshalJSON(raw []byte) error {
	type plain periodListRequest
	var in plain
	if err := decodePeriod(raw, &in); err != nil {
		return err
	}
	*v = periodListRequest(in)
	return nil
}
func (v *periodCloseRequest) UnmarshalJSON(raw []byte) error {
	type plain periodCloseRequest
	var in plain
	if err := decodePeriod(raw, &in); err != nil {
		return err
	}
	*v = periodCloseRequest(in)
	return nil
}
func (v *periodReopenRequest) UnmarshalJSON(raw []byte) error {
	type plain periodReopenRequest
	var in plain
	if err := decodePeriod(raw, &in); err != nil {
		return err
	}
	*v = periodReopenRequest(in)
	return nil
}

// losslessStrings refuses invalid UTF-8 and lone escaped surrogates, which
// encoding/json would otherwise replace before the reason profile is checked.
func losslessStrings(raw []byte) bool {
	if !utf8.Valid(raw) {
		return false
	}
	unit := func(raw []byte) (int, bool) {
		if len(raw) != 4 {
			return 0, false
		}
		value := 0
		for _, b := range raw {
			value <<= 4
			switch {
			case b >= '0' && b <= '9':
				value += int(b - '0')
			case b >= 'a' && b <= 'f':
				value += int(b-'a') + 10
			case b >= 'A' && b <= 'F':
				value += int(b-'A') + 10
			default:
				return 0, false
			}
		}
		return value, true
	}
	quoted := false
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case '"':
			quoted = !quoted
		case '\\':
			if !quoted || i+1 >= len(raw) {
				return false
			}
			i++
			if raw[i] != 'u' {
				continue
			}
			if i+4 >= len(raw) {
				return false
			}
			high, ok := unit(raw[i+1 : i+5])
			if !ok {
				return false
			}
			i += 4
			switch {
			case high >= 0xd800 && high <= 0xdbff:
				if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
					return false
				}
				low, ok := unit(raw[i+3 : i+7])
				if !ok || low < 0xdc00 || low > 0xdfff {
					return false
				}
				i += 6
			case high >= 0xdc00 && high <= 0xdfff:
				return false
			}
		}
	}
	return !quoted
}

// canonicalKeys admits only lower_snake keys and no JSON null.
func canonicalKeys(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			if key == "" || key[0] < 'a' || key[0] > 'z' {
				return false
			}
			for i := 1; i < len(key); i++ {
				if b := key[i]; !(b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '_') {
					return false
				}
			}
			if !canonicalKeys(item) {
				return false
			}
		}
	case []any:
		for _, item := range v {
			if !canonicalKeys(item) {
				return false
			}
		}
	case nil:
		return false
	}
	return true
}

func decodePeriod[T any](raw []byte, target *T) error {
	if !losslessStrings(raw) || httpapi.DecodeJSON(raw, target) != nil {
		return orgapi.Malformed
	}
	var fields map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&fields) != nil || fields == nil || !canonicalKeys(fields) {
		return orgapi.Malformed
	}
	return nil
}

func periodMapped(err error) error {
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
		{app.ErrSurfaceDisabled, orgapi.NotFound}, {app.ErrNotFound, orgapi.NotFound}, {app.ErrUnauthenticated, orgapi.Unauthenticated},
		{app.ErrOperationReused, orgapi.OperationReused}, {period.ErrConflict, orgapi.Conflict}, {period.ErrTransition, orgapi.Conflict},
		{period.ErrReasonRequired, orgapi.ValidationFailed}, {period.ErrReasonInvalid, orgapi.ValidationFailed}, {period.ErrInvalid, orgapi.ValidationFailed},
		{access.ErrNoMembership, orgapi.NotFound}, {access.ErrNotFound, orgapi.NotFound}, {access.ErrUnauthenticated, orgapi.Unauthenticated},
		{access.ErrFreshAuthRequired, orgapi.FreshAuthRequired}, {access.ErrMembershipSuspended, orgapi.MembershipSuspended}, {access.ErrOrgSuspended, orgapi.OrgSuspended}, {access.ErrOrgArchived, orgapi.OrgArchived}, {access.ErrSetupIncomplete, orgapi.OrgSetupIncomplete}, {access.ErrEntitlementRequired, orgapi.EntitlementRequired},
	} {
		if errors.Is(err, pair.source) {
			return pair.code
		}
	}
	return orgapi.Unavailable
}

func validID(raw string) bool { id, e := kernel.ParseOrgID(raw); return e == nil && id.String() == raw }

func validStamp(raw string) bool {
	_, err := orgtime.ParseInstant(raw)
	return err == nil
}

func projectPeriod(v app.PeriodView, history bool) (map[string]any, error) {
	month, err := period.ParseMonth(v.Month)
	if err != nil || v.Start != month.Start() || v.End != month.End() || (period.Head{State: v.State, Revision: v.Revision}).Validate() != nil ||
		(v.State == period.Open) != (v.PeriodID == "") || v.PeriodID != "" && !validID(v.PeriodID) {
		return nil, orgapi.Unavailable
	}
	out := map[string]any{"period": v.Month, "start_day": v.Start, "end_day": v.End, "period_id": nil, "state": v.State, "revision": v.Revision, "time_basis": "REPORTING_DAY", "data_mode": "SYNTHETIC"}
	if v.PeriodID != "" {
		out["period_id"] = v.PeriodID
	}
	if !history {
		return out, nil
	}
	entries := make([]map[string]any, 0, len(v.History))
	for _, h := range v.History {
		stamp := orgtime.FormatInstant(h.RecordedAt)
		t := period.Transition{Kind: h.Kind, From: h.From, To: h.To, Revision: h.Revision, Authority: h.Authority, Reason: h.Reason}
		if t.Validate() != nil || !validID(h.ActorAccountID) || !validID(h.ActorMembershipID) || !validID(h.OperationID) || h.RecordedAt.IsZero() || !validStamp(stamp) {
			return nil, orgapi.Unavailable
		}
		entries = append(entries, map[string]any{"revision": h.Revision, "kind": h.Kind, "from_state": h.From, "to_state": h.To, "authority": h.Authority, "reason": h.Reason, "actor_account_id": h.ActorAccountID, "actor_membership_id": h.ActorMembershipID, "operation_id": h.OperationID, "recorded_at": stamp})
	}
	out["history"] = entries
	return out, nil
}

func projectAck(v app.PeriodAck, replayed bool) (map[string]any, error) {
	stamp := orgtime.FormatInstant(v.RecordedAt)
	if _, err := period.ParseMonth(v.Month); err != nil || !validID(v.PeriodID) || v.State == period.Open || (period.Head{State: v.State, Revision: v.Revision}).Validate() != nil || v.RecordedAt.IsZero() || !validStamp(stamp) {
		return nil, orgapi.Unavailable
	}
	switch v.Kind {
	case period.Lock, period.Reopen, period.Relock:
	default:
		return nil, orgapi.Unavailable
	}
	return map[string]any{"period": v.Month, "period_id": v.PeriodID, "kind": v.Kind, "state": v.State, "revision": v.Revision, "recorded_at": stamp, "replayed": replayed, "data_mode": "SYNTHETIC"}, nil
}

func pathMonth(ctx context.Context) (string, error) {
	raw := chi.URLParamFromCtx(ctx, "period")
	if _, err := period.ParseMonth(raw); err != nil {
		return "", orgapi.NotFound
	}
	return raw, nil
}

func currentActor(ctx context.Context) (app.PeriodActor, error) {
	actor, err := organization.CurrentCompanyActor(ctx)
	if err != nil {
		return app.PeriodActor{}, err
	}
	return app.PeriodActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, nil
}

func periodRule(action app.PeriodAction) orgapi.Rule {
	policy := app.PeriodPolicy(action)
	write := action != app.PeriodRead
	return orgapi.Rule{Permission: string(policy.Permission), Write: write, Fresh: policy.Fresh, RequireRevision: write, CompanyOnly: true, Success: 200}
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

// RegisterPeriods mounts the four guarded period routes.
func RegisterPeriods(router chi.Router, service app.PeriodService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
	if !service.Enabled {
		return nil
	}
	if router == nil || missing(service.Repository) || service.NewID == nil || missing(authorization.Source) || missing(authorization.Capabilities) || missing(authorization.Records) || authorization.Now == nil {
		return app.ErrUnavailable
	}
	owned, gateway, err := organization.CompanyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	root := periodRoot{}
	if err := guarded.Register(owned, PeriodListPath, gateway, periodRule(app.PeriodRead), authorization, app.PeriodPolicy(app.PeriodRead), root, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input periodListRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		page, err := input.Page.Normalize()
		if err != nil {
			return nil, orgapi.Malformed
		}
		filter := app.PeriodFilter{State: period.State(input.State), Limit: page.Limit}
		if filter.Validate() != nil {
			return nil, orgapi.Malformed
		}
		binding := fmt.Sprintf("finance.periods.v1:%s:%d", input.State, page.Limit)
		if filter.AfterMonth, err = gateway.OpenCursor(p, PeriodListPath, binding, page.Cursor); err != nil || filter.Validate() != nil {
			return nil, orgapi.Malformed
		}
		result, err := service.List(ctx, p.Scope, actor, decision, filter)
		if err != nil {
			return nil, periodMapped(err)
		}
		items := make([]map[string]any, 0, len(result.Periods))
		for _, v := range result.Periods {
			item, err := projectPeriod(v, false)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		next := ""
		if result.NextMonth != "" {
			if next, err = gateway.SealCursor(p, PeriodListPath, binding, result.NextMonth); err != nil {
				return nil, orgapi.Unavailable
			}
		}
		return map[string]any{"items": items, "next_cursor": next}, nil
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, PeriodReadPath, gateway, periodRule(app.PeriodRead), authorization, app.PeriodPolicy(app.PeriodRead), root, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ periodReadRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		month, err := pathMonth(ctx)
		if err != nil {
			return nil, err
		}
		view, err := service.Read(ctx, p.Scope, actor, decision, month)
		if err != nil {
			return nil, periodMapped(err)
		}
		return projectPeriod(view, true)
	}); err != nil {
		return err
	}
	transition := func(ctx context.Context, p orgapi.Principal, decision access.Decision, input periodCommandFields, action period.Action, reason string) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		month, err := pathMonth(ctx)
		if err != nil {
			return nil, err
		}
		if input.Revision == nil {
			return nil, orgapi.Malformed
		}
		command, err := app.NewPeriodCommand(app.PeriodCommandInput{Action: action, OperationID: input.OperationID, Month: month, ExpectedRevision: *input.Revision, Reason: reason})
		if err != nil {
			return nil, periodMapped(err)
		}
		ack, replayed, err := service.Transition(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, periodMapped(err)
		}
		return projectAck(ack, replayed)
	}
	if err := guarded.Register(owned, PeriodClosePath, gateway, periodRule(app.PeriodClose), authorization, app.PeriodPolicy(app.PeriodClose), root, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input periodCloseRequest) (any, error) {
		return transition(ctx, p, decision, input.periodCommandFields, period.ActionClose, "")
	}); err != nil {
		return err
	}
	return guarded.Register(owned, PeriodReopenPath, gateway, periodRule(app.PeriodReopen), authorization, app.PeriodPolicy(app.PeriodReopen), root, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input periodReopenRequest) (any, error) {
		return transition(ctx, p, decision, input.periodCommandFields, period.ActionReopen, input.Reason)
	})
}
