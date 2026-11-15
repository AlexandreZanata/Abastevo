package http

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

// EMP-TRIP-007 (EDR-263): the trip conflict queue beside the meter review
// routes of the OFFL-008 mechanism, under trip.trip.review.
const (
	ConflictPrefix      = "/v1/orgs/{org_id}/sync/reviews/trips"
	ConflictListPath    = ConflictPrefix + "/list"
	ConflictReadPath    = ConflictPrefix + "/{review_item_id}/read"
	ConflictResolvePath = ConflictPrefix + "/{review_item_id}/resolve"
)

// ConflictRecords accepts only the two trip.trip.review policies.
type ConflictRecords struct{}

func (ConflictRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.Validate(scope) != nil || facts.OrgID != scope.OrgID() || len(rules) == 0 ||
		policy != app.ConflictPolicy(false) && policy != app.ConflictPolicy(true) {
		return access.ErrUnavailable
	}
	return nil
}

// conflictResource resolves the path item; its structure comes from the
// server-side HeldTripStructure source (EDR-161, EDR-263).
type conflictResource struct{}

func (conflictResource) Resolve(ctx context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	id, err := conflictPathID(ctx)
	if err != nil {
		return permissions.ResourceFacts{}, err
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: id}, nil
}

func conflictPathID(ctx context.Context) (string, error) {
	raw := chi.URLParamFromCtx(ctx, "review_item_id")
	id, err := kernel.ParseOrgID(raw)
	if err != nil || id.String() != raw {
		return "", orgapi.NotFound
	}
	return raw, nil
}

type conflictListRequest struct {
	companyRequest
	orgapi.Page
	State string `json:"state,omitempty"`
}

type conflictResolveRequest struct {
	companyRequest
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	Revision      *int64 `json:"revision"`
	Decision      string `json:"decision"`
	Reason        string `json:"reason,omitempty"`
}

func (v conflictResolveRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}

func (v *conflictListRequest) UnmarshalJSON(raw []byte) error {
	type plain conflictListRequest
	var in plain
	if err := decodeStrict(raw, &in); err != nil {
		return err
	}
	*v = conflictListRequest(in)
	return nil
}

func (v *conflictResolveRequest) UnmarshalJSON(raw []byte) error {
	type plain conflictResolveRequest
	var in plain
	if err := decodeStrict(raw, &in); err != nil {
		return err
	}
	*v = conflictResolveRequest(in)
	return nil
}

func conflictRule(write bool) orgapi.Rule {
	policy := app.ConflictPolicy(write)
	return orgapi.Rule{Permission: string(policy.Permission), Write: write, Fresh: policy.Fresh, RequireRevision: write, CompanyOnly: true, Success: 200}
}

func optionalString(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}

func projectHeld(f *app.HeldStartFact) (clock, reading any) {
	if f.Clock != nil {
		clock = map[string]any{"boot_id": f.Clock.BootID(), "elapsed_millis": strconv.FormatInt(f.Clock.ElapsedMillis(), 10)}
	}
	if f.ReadingID != "" && f.ReadingValue != nil {
		reading = map[string]any{"meter_id": f.MeterID, "segment_id": f.SegmentID, "reading_id": f.ReadingID, "kind": f.ReadingKind, "value": strconv.FormatInt(*f.ReadingValue, 10)}
	}
	return clock, reading
}

// projectConflict is the reviewer's view of one held trip item: the item,
// its exact held fact, the trips it conflicts with and its resolution.
func projectConflict(v app.ConflictView) (map[string]any, error) {
	if v.Validate() != nil {
		return nil, orgapi.Unavailable
	}
	f := v.Fact
	fact := map[string]any{"trip_id": f.TripID, "occurred_at": orgtime.FormatInstant(f.Declared.OccurredAt), "entry_zone": f.Declared.EntryZone, "device_offset_seconds": f.Declared.DeviceOffsetSeconds,
		"asset_id": nil, "driver_employee_id": nil, "run_clock": nil, "reading": nil, "missing_reading_reason": nil, "event": nil, "submitted_revision": nil}
	held := f.Start
	if held == nil {
		held = f.Finish
	}
	if held != nil {
		fact["run_clock"], fact["reading"] = projectHeld(held)
		fact["missing_reading_reason"] = optionalString(held.MissingReason)
	}
	if f.Start != nil {
		fact["asset_id"], fact["driver_employee_id"] = f.Start.AssetID, f.Start.DriverID
	}
	if f.Event != nil {
		fact["event"] = map[string]any{"event_id": f.Event.EventID, "event_type": f.Event.EventType, "text": optionalString(f.Event.Text)}
	}
	if f.SubmittedRevision != nil {
		fact["submitted_revision"] = *f.SubmittedRevision
	}
	trips := make([]map[string]any, 0, len(v.Trips))
	for _, t := range v.Trips {
		var finished any
		if t.FinishedAt != nil {
			finished = orgtime.FormatInstant(*t.FinishedAt)
		}
		trips = append(trips, map[string]any{"trip_id": t.TripID, "operational_state": t.State, "started_at": orgtime.FormatInstant(t.StartedAt), "finished_at": finished})
	}
	out := map[string]any{"review_item_id": v.ReviewItemID, "operation_id": v.OperationID, "operation_type": v.OperationType, "payload_digest": v.PayloadDigest, "code": v.Code,
		"state": v.State, "revision": v.Revision, "author_account_id": v.AuthorAccountID, "author_membership_id": v.AuthorMembershipID, "author_employee_id": v.AuthorEmployeeID,
		"enrollment_id": v.EnrollmentID, "received_at": orgtime.FormatInstant(v.ReceivedAt), "expires_at": orgtime.FormatInstant(v.ExpiresAt), "fact": fact,
		"conflicting_trips": trips, "resolution": nil, "data_mode": "SYNTHETIC"}
	if v.Resolution != nil {
		out["resolution"] = json.RawMessage(v.Resolution.CanonicalBytes())
	}
	return out, nil
}

// RegisterConflicts mounts the queue, the read and the resolution.
func RegisterConflicts(router chi.Router, service app.ConflictService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
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
	if err := guarded.Register(owned, ConflictListPath, gateway, conflictRule(false), authorization, app.ConflictPolicy(false), queueRoot{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input conflictListRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		page, err := input.Page.Normalize()
		if err != nil {
			return nil, orgapi.Malformed
		}
		binding := fmt.Sprintf("trip.conflict-queue.v1:%s:%d", input.State, page.Limit)
		filter := app.ConflictFilter{State: input.State, Limit: page.Limit}
		if filter.AfterID, err = gateway.OpenCursor(p, ConflictListPath, binding, page.Cursor); err != nil || filter.Validate() != nil {
			return nil, orgapi.Malformed
		}
		result, err := service.List(ctx, p.Scope, actor, decision, filter)
		if err != nil {
			return nil, reviewMapped(err)
		}
		items := make([]map[string]any, 0, len(result.Items))
		for _, v := range result.Items {
			item, err := projectConflict(v)
			if err != nil {
				return nil, err
			}
			delete(item, "data_mode")
			items = append(items, item)
		}
		next := ""
		if result.NextID != "" {
			if next, err = gateway.SealCursor(p, ConflictListPath, binding, result.NextID); err != nil {
				return nil, orgapi.Unavailable
			}
		}
		return map[string]any{"items": items, "next_cursor": next, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}
	resource := conflictResource{}
	if err := guarded.Register(owned, ConflictReadPath, gateway, conflictRule(false), authorization, app.ConflictPolicy(false), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ reviewReadRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		id, err := conflictPathID(ctx)
		if err != nil {
			return nil, err
		}
		view, err := service.Read(ctx, p.Scope, actor, decision, id)
		if err != nil {
			return nil, reviewMapped(err)
		}
		return projectConflict(view)
	}); err != nil {
		return err
	}
	return guarded.Register(owned, ConflictResolvePath, gateway, conflictRule(true), authorization, app.ConflictPolicy(true), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input conflictResolveRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		id, err := conflictPathID(ctx)
		if err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.Revision == nil {
			return nil, orgapi.Malformed
		}
		command, err := app.NewConflictDecision(app.ConflictDecisionInput{OperationID: input.OperationID, ReviewItemID: id, ExpectedRevision: *input.Revision, Decision: app.ConflictDecisionKind(input.Decision), Reason: input.Reason})
		if err != nil {
			return nil, reviewMapped(err)
		}
		result, replayed, err := service.Resolve(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, reviewMapped(err)
		}
		return map[string]any{"resolution": json.RawMessage(result.CanonicalBytes()), "replayed": replayed, "data_mode": "SYNTHETIC"}, nil
	})
}
