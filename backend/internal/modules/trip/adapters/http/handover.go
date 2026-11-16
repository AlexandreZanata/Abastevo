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

// EMP-TRIP-012 handover routes (EDR-255).
const (
	HandoverPath     = TripPrefix + "/{trip_id}/handover"
	HandoverListPath = TripPrefix + "/handovers/list"
	OwnTripsPath     = TripPrefix + "/mine/list"
)

// HandoverRecords accepts only the three handover policies: the stored trip
// or the organization for trip.trip.close_for_other, the requesting
// membership itself for its own open trips.
type HandoverRecords struct{}

func (HandoverRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.Validate(scope) != nil || facts.OrgID != scope.OrgID() || len(rules) == 0 {
		return access.ErrUnavailable
	}
	switch policy {
	case app.HandoverPolicy(app.HandoverWriteAction), app.HandoverPolicy(app.HandoverReadAction):
		return nil
	case app.HandoverPolicy(app.OwnTripsAction):
		if facts.ID == subject.MembershipID() && facts.AssignedTo == subject.MembershipID() {
			return nil
		}
	}
	return access.ErrUnavailable
}

type handoverListRequest struct {
	companyRequest
	orgapi.Page
}

func (v *handoverListRequest) UnmarshalJSON(raw []byte) error {
	type plain handoverListRequest
	var in plain
	if err := decodeStrict(raw, &in); err != nil {
		return err
	}
	*v = handoverListRequest(in)
	return nil
}

// handoverRequest names the receiving membership and the reason; the instant
// is the server's (BR-TIME-003), so the body has no time field.
type handoverRequest struct {
	companyRequest
	SchemaVersion        int    `json:"schema_version"`
	OperationID          string `json:"operation_id"`
	Revision             *int64 `json:"revision"`
	ReceiverMembershipID string `json:"receiver_membership_id"`
	Reason               string `json:"reason"`
}

func (v handoverRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}

func (v *handoverRequest) UnmarshalJSON(raw []byte) error {
	type plain handoverRequest
	var in plain
	if err := decodeStrict(raw, &in); err != nil {
		return err
	}
	*v = handoverRequest(in)
	return nil
}

// handoverMapped maps the handover refusals, then the shared review mapping.
func handoverMapped(err error) error {
	for _, pair := range []struct {
		source error
		code   orgapi.Code
	}{
		{app.ErrSelfHandover, orgapi.SelfApproval}, {app.ErrHandoverConflict, orgapi.Conflict}, {app.ErrInvalidHandover, orgapi.ValidationFailed},
		{app.ErrReceiverIneligible, orgapi.ValidationFailed}, {trip.ErrInvalidTransition, orgapi.Conflict},
	} {
		if errors.Is(err, pair.source) {
			return pair.code
		}
	}
	return reviewMapped(err)
}

func handoverRule(action app.HandoverAction) orgapi.Rule {
	policy := app.HandoverPolicy(action)
	write := action == app.HandoverWriteAction
	return orgapi.Rule{Permission: string(policy.Permission), Write: write, Fresh: policy.Fresh, RequireRevision: write, CompanyOnly: true, Success: 200}
}

func projectHandover(h app.HandoverRecord) map[string]any {
	return map[string]any{"revision": h.Revision, "handed_over_at": orgtime.FormatInstant(h.HandedOverAt), "from_membership_id": h.FromMembershipID,
		"to_membership_id": h.ToMembershipID, "recorder_membership_id": h.RecorderMembershipID, "reason": h.Reason}
}

// RegisterHandover mounts the handover, the organization's handover list and
// the member's own open trips.
func RegisterHandover(router chi.Router, service app.HandoverService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
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
	list := func(path, tag string, action app.HandoverAction, resolver access.ResourceResolver, read func(context.Context, orgapi.Principal, access.Decision, app.ReviewActor, app.QueueFilter) ([]map[string]any, string, error)) error {
		return guarded.Register(owned, path, gateway, handoverRule(action), authorization, app.HandoverPolicy(action), resolver, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input handoverListRequest) (any, error) {
			actor, err := currentActor(ctx)
			if err != nil {
				return nil, err
			}
			page, err := input.Page.Normalize()
			if err != nil {
				return nil, orgapi.Malformed
			}
			binding := fmt.Sprintf("%s:%d", tag, page.Limit)
			filter := app.QueueFilter{Limit: page.Limit}
			if filter.AfterID, err = gateway.OpenCursor(p, path, binding, page.Cursor); err != nil || filter.Validate() != nil {
				return nil, orgapi.Malformed
			}
			items, nextID, err := read(ctx, p, decision, actor, filter)
			if err != nil {
				return nil, handoverMapped(err)
			}
			next := ""
			if nextID != "" {
				if next, err = gateway.SealCursor(p, path, binding, nextID); err != nil {
					return nil, orgapi.Unavailable
				}
			}
			return map[string]any{"items": items, "next_cursor": next, "data_mode": "SYNTHETIC"}, nil
		})
	}
	if err := list(HandoverListPath, "trip.handover-list.v1", app.HandoverReadAction, queueRoot{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, actor app.ReviewActor, filter app.QueueFilter) ([]map[string]any, string, error) {
		result, err := service.List(ctx, p.Scope, actor, decision, filter)
		if err != nil {
			return nil, "", err
		}
		items := make([]map[string]any, 0, len(result.Items))
		for _, v := range result.Items {
			items = append(items, map[string]any{"trip_id": v.TripID, "asset_id": v.AssetID, "handover": projectHandover(v.Handover)})
		}
		return items, result.NextID, nil
	}); err != nil {
		return err
	}
	if err := list(OwnTripsPath, "trip.own-list.v1", app.OwnTripsAction, ownMissions{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, actor app.ReviewActor, filter app.QueueFilter) ([]map[string]any, string, error) {
		result, err := service.Mine(ctx, p.Scope, actor, decision, filter)
		if err != nil {
			return nil, "", err
		}
		items := make([]map[string]any, 0, len(result.Items))
		for _, v := range result.Items {
			var handover any
			if v.Handover != nil {
				handover = projectHandover(*v.Handover)
			}
			items = append(items, map[string]any{"trip_id": v.TripID, "asset_id": v.AssetID, "operational_state": v.State, "revision": v.Revision,
				"started_at": orgtime.FormatInstant(v.StartedAt), "reporting_day": v.ReportingDay, "handover": handover})
		}
		return items, result.NextID, nil
	}); err != nil {
		return err
	}
	return guarded.Register(owned, HandoverPath, gateway, handoverRule(app.HandoverWriteAction), authorization, app.HandoverPolicy(app.HandoverWriteAction), tripResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input handoverRequest) (any, error) {
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
		command, err := app.NewHandoverCommand(app.HandoverInput{OperationID: input.OperationID, TripID: tripID, ExpectedRevision: *input.Revision,
			ReceiverMembershipID: input.ReceiverMembershipID, Reason: input.Reason})
		if err != nil {
			return nil, handoverMapped(err)
		}
		result, replayed, err := service.HandOver(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, handoverMapped(err)
		}
		return map[string]any{"trip_id": result.TripID, "operational_state": result.Operational, "revision": result.Revision,
			"handover": projectHandover(result.Handover), "replayed": replayed, "data_mode": "SYNTHETIC"}, nil
	})
}
