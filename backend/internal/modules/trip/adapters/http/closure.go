package http

import (
	"context"
	"errors"
	"fmt"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

// EMP-TRIP-014 supervisor closure routes (EDR-259).
const (
	OpenListPath = TripPrefix + "/open/list"
	ClosePath    = TripPrefix + "/{trip_id}/close"
)

// ClosureRecords accepts only the two trip.trip.close_for_other policies.
type ClosureRecords struct{}

func (ClosureRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.Validate(scope) != nil || facts.OrgID != scope.OrgID() || len(rules) == 0 ||
		policy != app.ClosePolicy(app.ClosureReadAction) && policy != app.ClosePolicy(app.ClosureWriteAction) {
		return access.ErrUnavailable
	}
	return nil
}

type openListRequest struct {
	companyRequest
	orgapi.Page
}

func (v *openListRequest) UnmarshalJSON(raw []byte) error {
	type plain openListRequest
	var in plain
	if err := decodeStrict(raw, &in); err != nil {
		return err
	}
	*v = openListRequest(in)
	return nil
}

// closeRequest has no reading, value or distance field: a body naming one is
// MALFORMED (B-BR-EMP-400).
type closeRequest struct {
	companyRequest
	SchemaVersion        int    `json:"schema_version"`
	OperationID          string `json:"operation_id"`
	Revision             *int64 `json:"revision"`
	Reason               string `json:"reason"`
	MissingReadingReason string `json:"missing_reading_reason"`
}

func (v closeRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}

func (v *closeRequest) UnmarshalJSON(raw []byte) error {
	type plain closeRequest
	var in plain
	if err := decodeStrict(raw, &in); err != nil {
		return err
	}
	*v = closeRequest(in)
	return nil
}

// closureMapped maps the closure refusals, then the shared review mapping.
func closureMapped(err error) error {
	for _, pair := range []struct {
		source error
		code   orgapi.Code
	}{
		{trip.ErrSelfClosure, orgapi.SelfApproval}, {app.ErrClosureConflict, orgapi.Conflict}, {app.ErrInvalidClosure, orgapi.ValidationFailed},
		{trip.ErrInvalidTransition, orgapi.Conflict},
	} {
		if errors.Is(err, pair.source) {
			return pair.code
		}
	}
	return reviewMapped(err)
}

func closureRule(action app.ClosureAction) orgapi.Rule {
	policy := app.ClosePolicy(action)
	write := action == app.ClosureWriteAction
	return orgapi.Rule{Permission: string(policy.Permission), Write: write, Fresh: policy.Fresh, RequireRevision: write, CompanyOnly: true, Success: 200}
}

func projectClosure(c app.ClosureRecord) map[string]any {
	return map[string]any{"closed_at": orgtime.FormatInstant(c.ClosedAt), "closer_membership_id": c.CloserMembershipID, "reason": c.Reason, "missing_reading_reason": c.MissingReason}
}

// RegisterClosure mounts the open-trip list and the supervisor closure.
func RegisterClosure(router chi.Router, service app.ClosureService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
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
	if err := guarded.Register(owned, OpenListPath, gateway, closureRule(app.ClosureReadAction), authorization, app.ClosePolicy(app.ClosureReadAction), queueRoot{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input openListRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		page, err := input.Page.Normalize()
		if err != nil {
			return nil, orgapi.Malformed
		}
		binding := fmt.Sprintf("trip.open-list.v1:%d", page.Limit)
		filter := app.QueueFilter{Limit: page.Limit}
		if filter.AfterID, err = gateway.OpenCursor(p, OpenListPath, binding, page.Cursor); err != nil || filter.Validate() != nil {
			return nil, orgapi.Malformed
		}
		result, err := service.ListOpen(ctx, p.Scope, actor, decision, filter)
		if err != nil {
			return nil, closureMapped(err)
		}
		items := make([]map[string]any, 0, len(result.Items))
		for _, v := range result.Items {
			items = append(items, map[string]any{"trip_id": v.TripID, "asset_id": v.AssetID, "driver_employee_id": v.DriverEmployeeID, "author_membership_id": v.AuthorMembershipID,
				"started_at": orgtime.FormatInstant(v.StartedAt), "reporting_day": v.ReportingDay, "operational_state": trip.InProgress, "revision": v.Revision})
		}
		next := ""
		if result.NextID != "" {
			if next, err = gateway.SealCursor(p, OpenListPath, binding, result.NextID); err != nil {
				return nil, orgapi.Unavailable
			}
		}
		return map[string]any{"items": items, "next_cursor": next, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}
	return guarded.Register(owned, ClosePath, gateway, closureRule(app.ClosureWriteAction), authorization, app.ClosePolicy(app.ClosureWriteAction), tripResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input closeRequest) (any, error) {
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
		command, err := app.NewCloseCommand(app.CloseInput{OperationID: input.OperationID, TripID: tripID, ExpectedRevision: *input.Revision, Reason: input.Reason, MissingReason: input.MissingReadingReason})
		if err != nil {
			return nil, closureMapped(err)
		}
		result, replayed, err := service.Close(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, closureMapped(err)
		}
		return map[string]any{"trip_id": result.TripID, "operational_state": result.Operational, "revision": result.Revision, "closure": projectClosure(result.Closure),
			"final_reading": nil, "usage": map[string]any{"status": result.UsageStatus, "reason": result.UsageReason, "kind": nil, "distance": nil},
			"replayed": replayed, "data_mode": "SYNTHETIC"}, nil
	})
}
