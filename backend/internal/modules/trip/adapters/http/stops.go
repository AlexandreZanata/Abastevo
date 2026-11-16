package http

import (
	"context"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

// TripStopsPath is the member's planned-versus-observed read of an own trip
// (EMP-TRIP-010, EDR-309).
const TripStopsPath = TripPrefix + "/mine/{trip_id}/stops/read"

type stopsReadRequest struct{ companyRequest }

func (v *stopsReadRequest) UnmarshalJSON(raw []byte) error {
	type plain stopsReadRequest
	var in plain
	if err := decodeStrict(raw, &in); err != nil {
		return err
	}
	*v = stopsReadRequest(in)
	return nil
}

// projectObserved is one observed boundary with its recorded times; a
// missing boundary is null.
func projectObserved(f *app.ObservedFact) any {
	if f == nil {
		return nil
	}
	return map[string]any{"kind": string(f.Mark), "fact_id": f.FactID, "occurred_at": orgtime.FormatInstant(f.Declared.OccurredAt), "entry_zone": f.Declared.EntryZone,
		"device_offset_seconds": f.Declared.DeviceOffsetSeconds, "received_at": orgtime.FormatInstant(f.ReceivedAt), "after_receipt": f.AfterReceipt()}
}

func projectIssue(v trip.StopIssue) any {
	if v == "" {
		return nil
	}
	return string(v)
}

func projectPoint(p trip.PlannedPoint) map[string]any {
	if p.Kind == trip.PointStop {
		return map[string]any{"kind": string(p.Kind), "sequence": p.Stop}
	}
	return map[string]any{"kind": string(p.Kind)}
}

func projectPlanned(p *app.PlannedView) any {
	if p == nil {
		return nil
	}
	destination := map[string]any{"kind": "TEXT"}
	if p.DestinationText != nil {
		destination["text"] = *p.DestinationText
	} else {
		destination["kind"], destination["unit_id"] = "UNIT", p.DestinationUnitID
	}
	legs := make([]map[string]any, 0, len(p.Legs))
	for _, leg := range p.Legs {
		legs = append(legs, map[string]any{"sequence": leg.Sequence, "from": projectPoint(leg.From), "to": projectPoint(leg.To)})
	}
	return map[string]any{"mission_id": p.MissionID, "revision": p.Revision, "planned_at": orgtime.FormatInstant(p.PlannedAt), "starts_at": orgtime.FormatInstant(p.StartsAt),
		"ends_at": orgtime.FormatInstant(p.EndsAt), "destination": destination, "stops": projectPlannedStops(p.Stops), "legs": legs}
}

func projectStops(v app.TripStopsView) map[string]any {
	stops := make([]map[string]any, 0, len(v.Stops))
	for _, s := range v.Stops {
		stops = append(stops, map[string]any{"sequence": s.Sequence, "arrival": projectObserved(s.Arrival), "departure": projectObserved(s.Departure), "issue": projectIssue(s.Issue)})
	}
	legs := make([]map[string]any, 0, len(v.Legs))
	for _, l := range v.Legs {
		legs = append(legs, map[string]any{"sequence": l.Sequence, "departure": projectObserved(l.Departure), "arrival": projectObserved(l.Arrival), "issue": projectIssue(l.Issue)})
	}
	held := make([]map[string]any, 0, len(v.Held))
	for _, h := range v.Held {
		held = append(held, map[string]any{"event_id": h.EventID, "type": string(h.Type), "occurred_at": orgtime.FormatInstant(h.Declared.OccurredAt), "entry_zone": h.Declared.EntryZone,
			"device_offset_seconds": h.Declared.DeviceOffsetSeconds, "code": h.Code})
	}
	after := make([]any, 0, len(v.AfterEnd))
	for i := range v.AfterEnd {
		after = append(after, projectObserved(&v.AfterEnd[i]))
	}
	start := v.Start
	return map[string]any{"trip_id": v.TripID, "asset_id": v.AssetID, "operational_state": string(v.State), "start": projectObserved(&start), "end": projectObserved(v.End),
		"planned": projectPlanned(v.Planned), "observed": map[string]any{"stops": stops, "legs": legs, "after_end": after, "held": held}, "data_mode": "SYNTHETIC"}
}

// RegisterStops mounts the member's planned-versus-observed read under the
// EMP-TRIP-012 own-trips policy (trip.trip.start over the requesting
// membership; HandoverRecords accepts it).
func RegisterStops(router chi.Router, service app.StopsService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
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
	return guarded.Register(owned, TripStopsPath, gateway, handoverRule(app.OwnTripsAction), authorization, app.StopsPolicy(), ownMissions{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ stopsReadRequest) (any, error) {
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
		return projectStops(view), nil
	})
}
