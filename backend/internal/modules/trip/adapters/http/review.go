// Package http exposes the EMP-TRIP-004 reviewer read and decision routes
// through the guarded company pipeline (EDR-222).
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
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const (
	TripPrefix       = "/v1/orgs/{org_id}/trips"
	ReviewListPath   = TripPrefix + "/review/list"
	ReviewReadPath   = TripPrefix + "/{trip_id}/review/read"
	ReviewDecidePath = TripPrefix + "/{trip_id}/review/decide"
)

// ReviewRecords accepts only this module's two review policies.
type ReviewRecords struct{}

func (ReviewRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.Validate(scope) != nil || facts.OrgID != scope.OrgID() || len(rules) == 0 ||
		policy != app.ReviewPolicy(app.ReviewReadAction) && policy != app.ReviewPolicy(app.ReviewDecideAction) {
		return access.ErrUnavailable
	}
	return nil
}

// queueRoot resolves the organization itself for the review queue: an
// organization-level read that only ORG grants cover.
type queueRoot struct{}

func (queueRoot) Resolve(_ context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String()}, nil
}

// tripResource resolves the path trip; structure comes from the server-side
// TripStructure source, never from the request (EDR-161).
type tripResource struct{}

func (tripResource) Resolve(ctx context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	id, err := tripPathID(ctx)
	if err != nil {
		return permissions.ResourceFacts{}, err
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: id}, nil
}

func tripPathID(ctx context.Context) (string, error) {
	raw := chi.URLParamFromCtx(ctx, "trip_id")
	id, err := kernel.ParseOrgID(raw)
	if err != nil || id.String() != raw {
		return "", orgapi.NotFound
	}
	return raw, nil
}

type companyRequest struct{}

func (companyRequest) Credentials() orgapi.Session { return orgapi.Session{} }

type reviewReadRequest struct{ companyRequest }
type reviewListRequest struct {
	companyRequest
	orgapi.Page
}

type reviewDecideRequest struct {
	companyRequest
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	Revision      *int64 `json:"revision"`
	Decision      string `json:"decision"`
	Reason        string `json:"reason"`
}

func (v reviewDecideRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}

func (v *reviewReadRequest) UnmarshalJSON(raw []byte) error {
	type plain reviewReadRequest
	var in plain
	if err := decodeStrict(raw, &in); err != nil {
		return err
	}
	*v = reviewReadRequest(in)
	return nil
}

func (v *reviewListRequest) UnmarshalJSON(raw []byte) error {
	type plain reviewListRequest
	var in plain
	if err := decodeStrict(raw, &in); err != nil {
		return err
	}
	*v = reviewListRequest(in)
	return nil
}

func (v *reviewDecideRequest) UnmarshalJSON(raw []byte) error {
	type plain reviewDecideRequest
	var in plain
	if err := decodeStrict(raw, &in); err != nil {
		return err
	}
	*v = reviewDecideRequest(in)
	return nil
}

// losslessStrings refuses invalid UTF-8 and lone escaped surrogates, which
// encoding/json would otherwise replace before the reason profile is checked
// (the APPR-006 and fleet route precedent).
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

func decodeStrict[T any](raw []byte, target *T) error {
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

// reviewMapped maps the APPR-001, authority and store refusals (EDR-222).
func reviewMapped(err error) error {
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
		{review.ErrSelfApproval, orgapi.SelfApproval}, {review.ErrConflict, orgapi.Conflict}, {review.ErrInvalidTransition, orgapi.Conflict},
		{review.ErrReasonRequired, orgapi.ValidationFailed}, {review.ErrInvalid, orgapi.ValidationFailed}, {review.ErrForbidden, orgapi.Forbidden},
		{app.ErrReviewDisabled, orgapi.NotFound}, {app.ErrNotFound, orgapi.NotFound}, {app.ErrReviewUnauthorized, orgapi.Unauthenticated},
		{app.ErrOperationReused, orgapi.OperationReused},
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

func currentActor(ctx context.Context) (app.ReviewActor, error) {
	actor, err := organization.CurrentCompanyActor(ctx)
	if err != nil {
		return app.ReviewActor{}, err
	}
	return app.ReviewActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, nil
}

func reviewRule(action app.ReviewAction) orgapi.Rule {
	policy := app.ReviewPolicy(action)
	write := action == app.ReviewDecideAction
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

func projectEntry(e app.ReviewEntry) map[string]any {
	return map[string]any{"sequence": e.Sequence, "kind": e.Kind, "revision": e.Revision, "state": e.State, "reason": e.Reason,
		"actor_membership_id": e.ActorMembershipID, "recorded_at": orgtime.FormatInstant(e.RecordedAt)}
}

func projectView(v app.ReviewView) map[string]any {
	history := make([]map[string]any, 0, len(v.History))
	for _, e := range v.History {
		history = append(history, projectEntry(e))
	}
	var finished any
	if v.FinishedAt != nil {
		finished = orgtime.FormatInstant(*v.FinishedAt)
	}
	return map[string]any{"trip_id": v.TripID, "asset_id": v.AssetID, "driver_employee_id": v.DriverEmployeeID, "operational_state": v.Operational,
		"started_at": orgtime.FormatInstant(v.StartedAt), "finished_at": finished, "reporting_day": v.ReportingDay, "review_state": v.Review,
		"revision": v.Revision, "history": history, "data_mode": "SYNTHETIC"}
}

// RegisterReview mounts the review queue, the reviewer read and the decision.
func RegisterReview(router chi.Router, service app.ReviewService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
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
	if err := guarded.Register(owned, ReviewListPath, gateway, reviewRule(app.ReviewReadAction), authorization, app.ReviewPolicy(app.ReviewReadAction), queueRoot{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input reviewListRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		page, err := input.Page.Normalize()
		if err != nil {
			return nil, orgapi.Malformed
		}
		binding := fmt.Sprintf("trip.review-queue.v1:%d", page.Limit)
		filter := app.QueueFilter{Limit: page.Limit}
		if filter.AfterID, err = gateway.OpenCursor(p, ReviewListPath, binding, page.Cursor); err != nil || filter.Validate() != nil {
			return nil, orgapi.Malformed
		}
		result, err := service.List(ctx, p.Scope, actor, decision, filter)
		if err != nil {
			return nil, reviewMapped(err)
		}
		items := make([]map[string]any, 0, len(result.Items))
		for _, v := range result.Items {
			items = append(items, map[string]any{"trip_id": v.TripID, "asset_id": v.AssetID, "driver_employee_id": v.DriverEmployeeID, "reporting_day": v.ReportingDay,
				"review_state": v.Review, "revision": v.Revision, "submitted_at": orgtime.FormatInstant(v.SubmittedAt)})
		}
		next := ""
		if result.NextID != "" {
			if next, err = gateway.SealCursor(p, ReviewListPath, binding, result.NextID); err != nil {
				return nil, orgapi.Unavailable
			}
		}
		return map[string]any{"items": items, "next_cursor": next, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}
	resource := tripResource{}
	if err := guarded.Register(owned, ReviewReadPath, gateway, reviewRule(app.ReviewReadAction), authorization, app.ReviewPolicy(app.ReviewReadAction), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ reviewReadRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		tripID, err := tripPathID(ctx)
		if err != nil {
			return nil, err
		}
		view, err := service.Read(ctx, p.Scope, actor, decision, tripID)
		if err != nil {
			return nil, reviewMapped(err)
		}
		return projectView(view), nil
	}); err != nil {
		return err
	}
	return guarded.Register(owned, ReviewDecidePath, gateway, reviewRule(app.ReviewDecideAction), authorization, app.ReviewPolicy(app.ReviewDecideAction), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input reviewDecideRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		tripID, err := tripPathID(ctx)
		if err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.Revision == nil {
			return nil, orgapi.Malformed
		}
		command, err := app.NewDecisionCommand(app.DecisionInput{OperationID: input.OperationID, TripID: tripID, ExpectedRevision: *input.Revision, Decision: review.Kind(input.Decision), Reason: input.Reason})
		if err != nil {
			return nil, reviewMapped(err)
		}
		result, replayed, err := service.Decide(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, reviewMapped(err)
		}
		return map[string]any{"trip_id": result.TripID, "decision": result.Decision, "review_state": result.State, "revision": result.Revision, "reason": result.Reason,
			"recorded_at": orgtime.FormatInstant(result.RecordedAt), "replayed": replayed, "data_mode": "SYNTHETIC"}, nil
	})
}
