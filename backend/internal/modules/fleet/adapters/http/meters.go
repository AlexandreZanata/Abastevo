package http

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const MeterInstallPath = AssetPrefix + "/{asset_id}/meters/install"
const MeterListPath = AssetPrefix + "/{asset_id}/meters/list"
const MeterLatestPath = AssetPrefix + "/{asset_id}/meters/{meter_id}/latest"

func meterMapped(err error) error {
	for _, pair := range []struct {
		source error
		code   orgapi.Code
	}{{meter.ErrAlreadyInService, orgapi.MeterAlreadyInService}, {meter.ErrInvalid, orgapi.ValidationFailed}, {meter.ErrScope, orgapi.NotFound}, {meter.ErrOperationReused, orgapi.OperationReused}} {
		if errors.Is(err, pair.source) {
			return pair.code
		}
	}
	return assetMapped(err)
}
func meterPathID(ctx context.Context) (string, error) {
	raw := chi.URLParamFromCtx(ctx, "meter_id")
	id, e := kernel.ParseOrgID(raw)
	if e != nil || id.String() != raw {
		return "", orgapi.NotFound
	}
	return raw, nil
}

func projectMeterReading(scope kernel.OrgScope, value meter.Reading) (map[string]any, error) {
	in := value.Snapshot()
	if _, e := meter.NewReading(scope, in); e != nil {
		return nil, orgapi.Unavailable
	}
	capture := value.CaptureSource()
	return map[string]any{"id": in.Reference.ID(), "value": strconv.FormatInt(in.Value.Count(), 10), "state": in.State, "occurred_at": orgtime.FormatInstant(in.Declared.OccurredAt), "entry_zone": in.Declared.EntryZone, "device_offset_seconds": in.Declared.DeviceOffsetSeconds, "received_at": orgtime.FormatInstant(in.ReceivedAt), "author_membership_id": in.Author.ID(), "operation_id": in.Operation.ID(), "origin": in.Origin, "method": in.Method, "capture": map[string]any{"kind": capture.Kind(), "id": capture.ID()}}, nil
}
func projectMeter(scope kernel.OrgScope, value app.MeterView) (map[string]any, error) {
	if value.Reference.Asset().Validate(scope) != nil || !validLabel(value.Label) || value.Segment.Meter() != value.Reference {
		return nil, orgapi.Unavailable
	}
	if _, e := meter.NewMeterRef(scope, value.Reference.Asset(), value.Reference.ID(), value.Reference.Kind(), value.Reference.Modulus()); e != nil {
		return nil, orgapi.Unavailable
	}
	if _, e := meter.NewSegmentRef(scope, value.Reference, value.Segment.ID()); e != nil {
		return nil, orgapi.Unavailable
	}
	installed := orgtime.FormatInstant(value.InstalledAt)
	if _, e := orgtime.ParseInstant(installed); e != nil {
		return nil, orgapi.Unavailable
	}
	out := map[string]any{"id": value.Reference.ID(), "asset_id": value.Reference.Asset().ID().Value(), "segment_id": value.Segment.ID(), "kind": value.Reference.Kind(), "unit": value.Reference.Kind().Unit(), "label": value.Label, "installed_at": installed, "data_mode": "SYNTHETIC"}
	if modulus := value.Reference.Modulus(); modulus != nil {
		out["modulus"] = strconv.FormatInt(*modulus, 10)
	}
	if value.RemovedAt != nil {
		if !value.RemovedAt.After(value.InstalledAt) {
			return nil, orgapi.Unavailable
		}
		removed := orgtime.FormatInstant(*value.RemovedAt)
		if _, e := orgtime.ParseInstant(removed); e != nil {
			return nil, orgapi.Unavailable
		}
		out["removed_at"] = removed
	}
	if value.Latest != nil {
		if value.Latest.Snapshot().Reference.Segment() != value.Segment || value.Latest.Snapshot().State != meter.Valid {
			return nil, orgapi.Unavailable
		}
		reading, e := projectMeterReading(scope, *value.Latest)
		if e != nil {
			return nil, e
		}
		out["latest"] = reading
	}
	return out, nil
}

func RegisterMeters(router chi.Router, service app.MeterService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
	if !service.Enabled {
		return nil
	}
	if router == nil || missing(service.Repository) || service.NewID == nil || missing(authorization.Source) || missing(authorization.Capabilities) || missing(authorization.Records) || authorization.Now == nil {
		return app.ErrUnavailable
	}
	owned, gateway, e := organization.CompanyEntry(router, cookies, gateway)
	if e != nil {
		return e
	}
	resource := assetResource{}
	if e := guarded.Register(owned, MeterInstallPath, gateway, assetRule(true, true, 201), authorization, app.Policy(true), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input meterInstallRequest) (any, error) {
		actor, e := currentActor(ctx)
		if e != nil {
			return nil, e
		}
		assetID, e := assetPathID(ctx)
		if e != nil {
			return nil, orgapi.NotFound
		}
		command, e := input.command(assetID)
		if e != nil {
			return nil, meterMapped(e)
		}
		v, replay, e := service.Install(ctx, p.Scope, actor, decision, command)
		if e != nil {
			return nil, meterMapped(e)
		}
		return map[string]any{"asset_id": v.AssetID, "meter_id": v.MeterID, "segment_id": v.SegmentID, "opening_reading_id": v.OpeningReadingID, "revision": v.AssetRevision, "replayed": replay, "data_mode": "SYNTHETIC"}, nil
	}); e != nil {
		return e
	}
	if e := guarded.Register(owned, MeterLatestPath, gateway, assetRule(false, false, 200), authorization, app.Policy(false), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ meterReadRequest) (any, error) {
		actor, e := currentActor(ctx)
		if e != nil {
			return nil, e
		}
		assetID, e := assetPathID(ctx)
		if e != nil {
			return nil, orgapi.NotFound
		}
		meterID, e := meterPathID(ctx)
		if e != nil {
			return nil, e
		}
		v, e := service.Latest(ctx, p.Scope, actor, decision, assetID, meterID)
		if e != nil {
			return nil, meterMapped(e)
		}
		return projectMeter(p.Scope, v)
	}); e != nil {
		return e
	}
	return guarded.Register(owned, MeterListPath, gateway, assetRule(false, false, 200), authorization, app.Policy(false), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input meterListRequest) (any, error) {
		actor, e := currentActor(ctx)
		if e != nil {
			return nil, e
		}
		assetID, e := assetPathID(ctx)
		if e != nil {
			return nil, orgapi.NotFound
		}
		page, e := input.Page.Normalize()
		if e != nil {
			return nil, orgapi.Malformed
		}
		binding := fmt.Sprintf("fleet.meters.v1:%s:%d", assetID, page.Limit)
		after, e := gateway.OpenCursor(p, MeterListPath, binding, page.Cursor)
		if e != nil {
			return nil, orgapi.Malformed
		}
		v, e := service.List(ctx, p.Scope, actor, decision, assetID, app.MeterFilter{AfterID: after, Limit: page.Limit})
		if e != nil {
			return nil, meterMapped(e)
		}
		items := make([]map[string]any, 0, len(v.Meters))
		for _, value := range v.Meters {
			item, e := projectMeter(p.Scope, value)
			if e != nil {
				return nil, e
			}
			items = append(items, item)
		}
		next := ""
		if v.NextID != "" {
			next, e = gateway.SealCursor(p, MeterListPath, binding, v.NextID)
			if e != nil {
				return nil, orgapi.Unavailable
			}
		}
		return map[string]any{"items": items, "next_cursor": next}, nil
	})
}
