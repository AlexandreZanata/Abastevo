package http

import (
	"context"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const TrackReplayPath = "/v1/orgs/{org_id}/trips/{trip_id}/track/replay"

type trackReplayRequest struct{}

func (trackReplayRequest) Credentials() orgapi.Session { return orgapi.Session{} }

func (v *trackReplayRequest) UnmarshalJSON(raw []byte) error {
	type plain trackReplayRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = trackReplayRequest(in)
	return nil
}

func projectReplay(v app.ReplayView) map[string]any {
	var session any
	if s := v.Session; s != nil {
		transitions := make([]map[string]any, 0, len(s.Transitions))
		for _, t := range s.Transitions {
			transitions = append(transitions, map[string]any{
				"sequence": t.Sequence, "state": t.State, "cause": nullable(t.Cause),
				"actor": t.Actor, "permission": nullable(t.Permission), "occurred_at": orgtime.FormatInstant(t.At),
			})
		}
		session = map[string]any{"session_id": s.ID, "state": s.State, "policy": s.Policy, "opened_at": orgtime.FormatInstant(s.OpenedAt), "transitions": transitions}
	}
	var summary any
	if sm := v.Summary; sm != nil {
		var cov any
		if sm.CoveragePermille != nil {
			cov = *sm.CoveragePermille
		}
		var dist any
		if sm.DistanceMetres != nil {
			dist = *sm.DistanceMetres
		}
		summary = map[string]any{
			"revision": sm.Revision, "quality": sm.Quality, "coverage_permille": cov,
			"distance_m": dist, "expected_seconds": sm.ExpectedSeconds, "gap_seconds": sm.GapSeconds,
			"coarse": sm.Coarse, "processed_at": orgtime.FormatInstant(sm.ProcessedAt),
		}
	}
	points := make([]map[string]any, 0, len(v.Points))
	for _, pt := range v.Points {
		points = append(points, map[string]any{
			"sequence": pt.Sequence, "collected_at": orgtime.FormatInstant(pt.At), "received_at": orgtime.FormatInstant(pt.ReceivedAt),
			"lat_e7": pt.Lat, "lon_e7": pt.Lon, "accuracy_m": pt.Accuracy, "speed_cm_s": optionalInt32(pt.Speed),
			"bearing_deg": optionalInt16(pt.Bearing), "coarse": pt.Coarse,
		})
	}
	lines := make([][][]int32, 0, len(v.SimplifiedLines))
	for _, part := range v.SimplifiedLines {
		pts := make([][]int32, 0, len(part))
		for _, p := range part {
			pts = append(pts, []int32{p.Lat, p.Lon})
		}
		lines = append(lines, pts)
	}
	gaps := make([]map[string]any, 0, len(v.Gaps))
	for _, g := range v.Gaps {
		gaps = append(gaps, map[string]any{"from": orgtime.FormatInstant(g.From), "to": orgtime.FormatInstant(g.To), "reason": g.Reason, "detected_by": g.DetectedBy})
	}
	logbook := make([]map[string]any, 0, len(v.Logbook))
	for _, l := range v.Logbook {
		logbook = append(logbook, map[string]any{"kind": l.Kind, "occurred_at": orgtime.FormatInstant(l.At), "summary": l.Summary})
	}
	return map[string]any{
		"trip_id":          v.TripID,
		"trip_state":       v.TripState,
		"raw_expired":      v.RawExpired,
		"trimmed_count":    v.TrimmedCount,
		"session":          session,
		"summary":          summary,
		"points":           points,
		"simplified_lines": lines,
		"gaps":             gaps,
		"logbook":          logbook,
		"data_mode":        "SYNTHETIC",
	}
}

// RegisterReplay mounts EMP-GPS-009 full trip replay under gps.track.read.
func RegisterReplay(router chi.Router, service app.ReplayService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization, authors TripAuthors) error {
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
	policy := app.TrackPolicy()
	rule := orgapi.Rule{Permission: app.TrackPermission, Write: false, Fresh: policy.Fresh, CompanyOnly: true, Success: 200}
	return guarded.Register(owned, TrackReplayPath, gateway, rule, authorization, policy, trackResource{authors: authors}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ trackReplayRequest) (any, error) {
		actor, err := organization.CurrentCompanyActor(ctx)
		if err != nil {
			return nil, err
		}
		trip, err := tripPathID(ctx)
		if err != nil {
			return nil, err
		}
		view, err := service.ReadReplay(ctx, p.Scope, app.TrackActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, decision, app.ReplayRequest{TripID: trip})
		if err != nil {
			return nil, trackMapped(err)
		}
		return projectReplay(view), nil
	})
}
