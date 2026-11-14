package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
	"strconv"
)

const MeterReviewPrefix = "/v1/orgs/{org_id}/sync/reviews/meter-readings"
const MeterReviewListPath = MeterReviewPrefix + "/list"
const MeterReviewReadPath = MeterReviewPrefix + "/{review_item_id}/read"
const MeterReviewResolvePath = MeterReviewPrefix + "/{review_item_id}/resolve"

type meterReviewListRequest struct {
	companyRequest
	orgapi.Page
	State string `json:"state,omitempty"`
}
type meterReviewResolveRequest struct {
	assetRevisionCommand
	Decision app.MeterReviewDecisionKind `json:"decision"`
	Reason   string                      `json:"reason,omitempty"`
}

func (v *meterReviewListRequest) UnmarshalJSON(raw []byte) error {
	type plain meterReviewListRequest
	var input plain
	if e := decodeAsset(raw, &input); e != nil {
		return e
	}
	*v = meterReviewListRequest(input)
	return nil
}
func (v *meterReviewResolveRequest) UnmarshalJSON(raw []byte) error {
	type plain meterReviewResolveRequest
	var input plain
	if e := decodeAsset(raw, &input); e != nil {
		return e
	}
	*v = meterReviewResolveRequest(input)
	return nil
}
func (v meterReviewResolveRequest) command(id string) (app.MeterReviewDecision, error) {
	if v.SchemaVersion != 1 || v.Revision == nil {
		return app.MeterReviewDecision{}, orgapi.Malformed
	}
	return app.NewMeterReviewDecision(app.MeterReviewDecisionInput{OperationID: v.OperationID, ReviewItemID: id, ExpectedRevision: *v.Revision, Decision: v.Decision, Reason: v.Reason})
}
func projectMeterReview(v app.MeterReviewView) (map[string]any, error) {
	if v.Validate() != nil {
		return nil, orgapi.Unavailable
	}
	fact := v.Fact
	out := map[string]any{"review_item_id": v.ReviewItemID, "operation_id": v.OperationID, "payload_digest": v.PayloadDigest, "code": v.Code, "state": v.State, "revision": v.Revision, "author_account_id": v.AuthorAccountID, "author_membership_id": v.AuthorMembershipID, "author_employee_id": v.AuthorEmployeeID, "enrollment_id": v.EnrollmentID, "received_at": orgtime.FormatInstant(v.ReceivedAt), "expires_at": orgtime.FormatInstant(v.ExpiresAt), "resolution": nil, "fact": map[string]any{"operation_id": fact.OperationID, "asset_id": fact.AssetID, "meter_id": fact.MeterID, "segment_id": fact.SegmentID, "reading_id": fact.ReadingID, "kind": fact.Value.Kind(), "value": strconv.FormatInt(fact.Value.Count(), 10), "occurred_at": orgtime.FormatInstant(fact.Declared.OccurredAt), "entry_zone": fact.Declared.EntryZone, "device_offset_seconds": fact.Declared.DeviceOffsetSeconds, "origin": fact.Origin, "method": fact.Method}}
	if v.Resolution != nil {
		out["resolution"] = json.RawMessage(v.Resolution.CanonicalBytes())
	}
	return out, nil
}

type MeterReviewRecords struct{}

func (MeterReviewRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.Validate(scope) != nil || facts.OrgID != scope.OrgID() || len(rules) == 0 || policy != app.MeterReviewPolicy(false) && policy != app.MeterReviewPolicy(true) {
		return access.ErrUnavailable
	}
	return nil
}

type meterReviewResource struct{ root bool }

func (r meterReviewResource) Resolve(ctx context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	id := scope.OrgID().String()
	if !r.root {
		var e error
		id, e = meterReviewPathID(ctx)
		if e != nil {
			return permissions.ResourceFacts{}, e
		}
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: id}, nil
}
func meterReviewPathID(ctx context.Context) (string, error) {
	raw := chi.URLParamFromCtx(ctx, "review_item_id")
	id, e := kernel.ParseOrgID(raw)
	if e != nil || id.String() != raw {
		return "", orgapi.NotFound
	}
	return raw, nil
}
func meterReviewRule(write bool) orgapi.Rule {
	return orgapi.Rule{Permission: string(app.MeterReviewPolicy(write).Permission), Write: write, Fresh: write, RequireRevision: write, CompanyOnly: true, Success: 200}
}
func reviewMapped(err error) error {
	if errors.Is(err, access.ErrSelfApproval) {
		return orgapi.SelfApproval
	}
	return meterMapped(err)
}

func RegisterMeterReviews(router chi.Router, service app.MeterReviewService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
	if !service.Enabled {
		return nil
	}
	if router == nil || missing(service.Repository) || missing(authorization.Source) || missing(authorization.Capabilities) || missing(authorization.Records) || authorization.Now == nil {
		return app.ErrUnavailable
	}
	owned, gateway, e := organization.CompanyEntry(router, cookies, gateway)
	if e != nil {
		return e
	}
	if e := guarded.Register(owned, MeterReviewListPath, gateway, meterReviewRule(false), authorization, app.MeterReviewPolicy(false), meterReviewResource{root: true}, func(ctx context.Context, p orgapi.Principal, d access.Decision, input meterReviewListRequest) (any, error) {
		actor, e := currentActor(ctx)
		if e != nil {
			return nil, e
		}
		page, e := input.Page.Normalize()
		if e != nil {
			return nil, orgapi.Malformed
		}
		binding := fmt.Sprintf("sync.meter-reviews.v1:%s:%d", input.State, page.Limit)
		after, e := gateway.OpenCursor(p, MeterReviewListPath, binding, page.Cursor)
		if e != nil {
			return nil, orgapi.Malformed
		}
		result, e := service.List(ctx, p.Scope, actor, d, app.MeterReviewFilter{AfterID: after, State: input.State, Limit: page.Limit})
		if e != nil {
			return nil, reviewMapped(e)
		}
		items := make([]map[string]any, 0, len(result.Items))
		for _, v := range result.Items {
			item, e := projectMeterReview(v)
			if e != nil {
				return nil, e
			}
			items = append(items, item)
		}
		next := ""
		if result.NextID != "" {
			next, e = gateway.SealCursor(p, MeterReviewListPath, binding, result.NextID)
			if e != nil {
				return nil, orgapi.Unavailable
			}
		}
		return map[string]any{"items": items, "next_cursor": next}, nil
	}); e != nil {
		return e
	}
	if e := guarded.Register(owned, MeterReviewReadPath, gateway, meterReviewRule(false), authorization, app.MeterReviewPolicy(false), meterReviewResource{}, func(ctx context.Context, p orgapi.Principal, d access.Decision, _ meterReadRequest) (any, error) {
		actor, e := currentActor(ctx)
		if e != nil {
			return nil, e
		}
		id, e := meterReviewPathID(ctx)
		if e != nil {
			return nil, e
		}
		result, e := service.Read(ctx, p.Scope, actor, d, id)
		if e != nil {
			return nil, reviewMapped(e)
		}
		return projectMeterReview(result)
	}); e != nil {
		return e
	}
	return guarded.Register(owned, MeterReviewResolvePath, gateway, meterReviewRule(true), authorization, app.MeterReviewPolicy(true), meterReviewResource{}, func(ctx context.Context, p orgapi.Principal, d access.Decision, input meterReviewResolveRequest) (any, error) {
		actor, e := currentActor(ctx)
		if e != nil {
			return nil, e
		}
		id, e := meterReviewPathID(ctx)
		if e != nil {
			return nil, e
		}
		command, e := input.command(id)
		if e != nil {
			return nil, reviewMapped(e)
		}
		result, replay, e := service.Resolve(ctx, p.Scope, actor, d, command)
		if e != nil {
			return nil, reviewMapped(e)
		}
		return map[string]any{"resolution": json.RawMessage(result.CanonicalBytes()), "replayed": replay, "data_mode": "SYNTHETIC"}, nil
	})
}
