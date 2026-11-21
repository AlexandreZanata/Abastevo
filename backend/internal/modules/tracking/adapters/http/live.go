package http

import (
	"context"
	"errors"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const (
	LiveReadPath = "/v1/orgs/{org_id}/trips/{trip_id}/location/live/read"
	LiveListPath = "/v1/orgs/{org_id}/tracking/live/list"
)

type LiveRecords struct{}

func (LiveRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.Validate(scope) != nil || facts.OrgID != scope.OrgID() || len(rules) == 0 || policy != app.LivePolicy() {
		return access.ErrUnavailable
	}
	return nil
}

type liveTripResource struct{ authors TripAuthors }

func (r liveTripResource) Resolve(ctx context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil || missing(r.authors) {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	id, err := tripPathID(ctx)
	if err != nil {
		return permissions.ResourceFacts{}, err
	}
	author, err := r.authors.TripAuthor(ctx, scope, id)
	if errors.Is(err, app.ErrNotFound) {
		return permissions.ResourceFacts{}, access.ErrNotFound
	}
	if err != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: id, AuthoredBy: author, TripDriver: author}, nil
}

type liveOrgResource struct{}

func (liveOrgResource) Resolve(_ context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String()}, nil
}

type liveReadRequest struct{}

func (liveReadRequest) Credentials() orgapi.Session { return orgapi.Session{} }

func (v *liveReadRequest) UnmarshalJSON(raw []byte) error {
	type plain liveReadRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = liveReadRequest(in)
	return nil
}

type liveListRequest struct {
	UnitID        *string `json:"unit_id"`
	SinceRevision *int64  `json:"since_revision"`
	Limit         *int    `json:"limit"`
}

func (liveListRequest) Credentials() orgapi.Session { return orgapi.Session{} }

func (v *liveListRequest) UnmarshalJSON(raw []byte) error {
	type plain liveListRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = liveListRequest(in)
	return nil
}

func optionalBool(b *bool) any {
	if b == nil {
		return nil
	}
	return *b
}

func projectLiveItem(v app.LiveTripView) map[string]any {
	return map[string]any{
		"trip_id":               v.TripID,
		"session_id":            v.SessionID,
		"session_state":         v.SessionState,
		"trip_state":            v.TripState,
		"status_note":           nullable(v.StatusNote),
		"unit_id":               nullable(v.UnitID),
		"unit_name":             nullable(v.UnitName),
		"asset_id":              v.AssetID,
		"asset_label":           nullable(v.AssetLabel),
		"driver_name":           nullable(v.DriverName),
		"freshness":             string(v.Freshness),
		"lat_e7":                optionalInt32(v.Lat),
		"lon_e7":                optionalInt32(v.Lon),
		"accuracy_m":            optionalInt32(v.Accuracy),
		"speed_cm_s":            optionalInt32(v.Speed),
		"bearing_deg":           optionalInt16(v.Bearing),
		"coarse":                optionalBool(v.Coarse),
		"collected_at":          optionalInstant(v.CollectedAt),
		"received_at":           optionalInstant(v.ReceivedAt),
		"last_communication_at": optionalInstant(v.LastCommunicationAt),
		"revision":              v.Revision,
	}
}

func projectAlert(a app.AlertView) map[string]any {
	return map[string]any{
		"id":                a.ID,
		"kind":              a.Kind,
		"trip_id":           a.TripID,
		"session_id":        a.SessionID,
		"unit_id":           nullable(a.UnitID),
		"geofence_id":       nullable(a.GeofenceID),
		"event_kind":        nullable(a.EventKind),
		"anchor_at":         orgtime.FormatInstant(a.AnchorAt),
		"observed_at":       orgtime.FormatInstant(a.ObservedAt),
		"duration_seconds":  a.DurationSeconds,
		"threshold_seconds": a.ThresholdSeconds,
		"radius_m":          a.RadiusMetres,
	}
}

// RegisterLive mounts EMP-GPS-007 live location routes under gps.location.read_live.
func RegisterLive(router chi.Router, service app.LiveService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization, authors TripAuthors) error {
	if !service.Enabled {
		return nil
	}
	if router == nil || missing(service.Repository) || missing(authors) || missing(authorization.Source) || missing(authorization.Capabilities) || missing(authorization.Records) || authorization.Now == nil {
		return app.ErrUnavailable
	}
	owned, gateway, err := organization.CompanyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	policy := app.LivePolicy()
	rule := orgapi.Rule{Permission: app.LivePermission, Write: false, Fresh: policy.Fresh, CompanyOnly: true, Success: 200}

	if err := guarded.Register(owned, LiveReadPath, gateway, rule, authorization, policy, liveTripResource{authors: authors}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ liveReadRequest) (any, error) {
		actor, err := organization.CurrentCompanyActor(ctx)
		if err != nil {
			return nil, err
		}
		trip, err := tripPathID(ctx)
		if err != nil {
			return nil, err
		}
		la, err := app.NewLiveActor(actor.AccountID, actor.SessionID)
		if err != nil {
			return nil, trackMapped(err)
		}
		view, err := service.ReadLive(ctx, p.Scope, la, decision, app.LiveReadRequest{TripID: trip})
		if err != nil {
			return nil, trackMapped(err)
		}
		item := projectLiveItem(view)
		item["data_mode"] = "SYNTHETIC"
		return item, nil
	}); err != nil {
		return err
	}

	return guarded.Register(owned, LiveListPath, gateway, rule, authorization, policy, liveOrgResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, in liveListRequest) (any, error) {
		actor, err := organization.CurrentCompanyActor(ctx)
		if err != nil {
			return nil, err
		}
		la, err := app.NewLiveActor(actor.AccountID, actor.SessionID)
		if err != nil {
			return nil, trackMapped(err)
		}
		req := app.LiveListRequest{Limit: app.MaxLivePage}
		if in.UnitID != nil {
			if *in.UnitID != "" {
				uid, err := kernel.ParseOrgID(*in.UnitID)
				if err != nil || uid.String() != *in.UnitID {
					return nil, orgapi.Malformed
				}
				req.UnitID = *in.UnitID
			}
		}
		if in.SinceRevision != nil {
			req.SinceRevision = *in.SinceRevision
		}
		if in.Limit != nil {
			req.Limit = *in.Limit
		}
		view, err := service.ListLive(ctx, p.Scope, la, decision, req)
		if err != nil {
			return nil, trackMapped(err)
		}
		items := make([]map[string]any, 0, len(view.Items))
		for _, it := range view.Items {
			items = append(items, projectLiveItem(it))
		}
		alerts := make([]map[string]any, 0, len(view.Alerts))
		for _, a := range view.Alerts {
			alerts = append(alerts, projectAlert(a))
		}
		return map[string]any{
			"items":           items,
			"alerts":          alerts,
			"max_revision":    view.MaxRevision,
			"current_seconds": view.CurrentSeconds,
			"stale_seconds":   view.StaleSeconds,
			"data_mode":       "SYNTHETIC",
		}, nil
	})
}
