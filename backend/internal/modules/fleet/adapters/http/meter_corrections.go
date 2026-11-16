package http

import (
	"context"
	"errors"
	"fmt"

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

// EMP-METER-004 routes (EDR-187, EDR-188).
const (
	MeterHistoryPath  = AssetPrefix + "/{asset_id}/meters/{meter_id}/readings/list"
	MeterSuspectsPath = AssetPrefix + "/{asset_id}/meters/{meter_id}/readings/suspect/list"
	MeterDecidePath   = AssetPrefix + "/{asset_id}/meters/{meter_id}/readings/{reading_id}/decide"
)

type meterHistoryRequest struct {
	companyRequest
	orgapi.Page
	State string `json:"state,omitempty"`
}
type meterSuspectRequest struct {
	companyRequest
	orgapi.Page
}
type meterDecideRequest struct {
	assetRevisionCommand
	Decision            app.MeterDecision `json:"decision"`
	Reason              string            `json:"reason"`
	Kind                meter.Kind        `json:"kind,omitempty"`
	Value               string            `json:"value,omitempty"`
	OccurredAt          string            `json:"occurred_at,omitempty"`
	EntryZone           string            `json:"entry_zone,omitempty"`
	DeviceOffsetSeconds *int64            `json:"device_offset_seconds,omitempty"`
}

func (v *meterHistoryRequest) UnmarshalJSON(raw []byte) error {
	type plain meterHistoryRequest
	var input plain
	if e := decodeAsset(raw, &input); e != nil {
		return e
	}
	*v = meterHistoryRequest(input)
	return nil
}
func (v *meterSuspectRequest) UnmarshalJSON(raw []byte) error {
	type plain meterSuspectRequest
	var input plain
	if e := decodeAsset(raw, &input); e != nil {
		return e
	}
	*v = meterSuspectRequest(input)
	return nil
}
func (v *meterDecideRequest) UnmarshalJSON(raw []byte) error {
	type plain meterDecideRequest
	var input plain
	if e := decodeAsset(raw, &input); e != nil {
		return e
	}
	*v = meterDecideRequest(input)
	return nil
}

// command reads the corrected value as exact base units of the stated kind;
// the store re-proves the kind against the actual meter.
func (v meterDecideRequest) command(assetID, meterID, readingID string) (app.MeterDecisionCommand, error) {
	if v.SchemaVersion != 1 || v.Revision == nil {
		return app.MeterDecisionCommand{}, orgapi.Malformed
	}
	in := app.MeterDecisionInput{OperationID: v.OperationID, AssetID: assetID, MeterID: meterID, ReadingID: readingID, ExpectedRevision: *v.Revision, Decision: v.Decision, Reason: v.Reason}
	declared := v.OccurredAt != "" || v.EntryZone != "" || v.DeviceOffsetSeconds != nil
	if v.Decision == app.MeterSupersede {
		value, e := meter.ParseBase(v.Kind, v.Value, nil)
		if e != nil {
			return app.MeterDecisionCommand{}, meter.ErrInvalid
		}
		in.Value = value
		if declared {
			at, e := orgtime.ParseInstant(v.OccurredAt)
			if e != nil || v.DeviceOffsetSeconds == nil {
				return app.MeterDecisionCommand{}, meter.ErrInvalid
			}
			in.Declared = &orgtime.Declaration{OccurredAt: at, EntryZone: v.EntryZone, DeviceOffsetSeconds: *v.DeviceOffsetSeconds}
		}
	} else if v.Kind != "" || v.Value != "" || declared {
		return app.MeterDecisionCommand{}, meter.ErrInvalid
	}
	return app.NewMeterDecisionCommand(in)
}

func meterReadingPathID(ctx context.Context) (string, error) {
	raw := chi.URLParamFromCtx(ctx, "reading_id")
	id, e := kernel.ParseOrgID(raw)
	if e != nil || id.String() != raw {
		return "", orgapi.NotFound
	}
	return raw, nil
}

func meterCorrectionMapped(err error) error {
	switch {
	case errors.Is(err, app.ErrReadingAlreadySuperseded):
		return orgapi.ReadingAlreadySuperseded
	case errors.Is(err, app.ErrAmendmentRequired):
		return orgapi.AmendmentRequired
	}
	return meterMapped(err)
}

func meterFlags(flags []meter.Flag) []string {
	out := make([]string, len(flags))
	for i, f := range flags {
		out[i] = string(f)
	}
	return out
}

func projectReadingState(v app.MeterReadingState) map[string]any {
	return map[string]any{"reading_id": v.ReadingID, "state": v.State, "flags": meterFlags(v.Flags), "revision": v.Revision}
}

func projectHistoryItem(scope kernel.OrgScope, v app.MeterHistoryItem, suspects bool) (map[string]any, error) {
	out, e := projectMeterReading(scope, v.Reading)
	if e != nil {
		return nil, e
	}
	out["recorded_state"] = out["state"]
	out["state"] = v.State
	out["flags"] = meterFlags(v.Flags)
	out["retroactive"] = v.Retroactive
	out["revision"] = v.Revision
	out["superseded_by"], out["supersedes"], out["reason"] = nullable(v.SupersededBy), nullable(v.Supersedes), nullable(v.Reason)
	if suspects {
		out["previous_reading_id"], out["next_reading_id"] = nullable(v.Previous), nullable(v.Next)
	}
	return out, nil
}

func nullable(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func meterCorrectionRule(write bool) orgapi.Rule {
	return orgapi.Rule{Permission: string(app.MeterReviewPolicy(write).Permission), Write: write, Fresh: write, RequireRevision: write, CompanyOnly: true, Success: 200}
}

// RegisterMeterCorrections mounts the EMP-METER-004 history, suspect list and
// decision routes on the asset resource (EDR-187, EDR-188).
func RegisterMeterCorrections(router chi.Router, service app.MeterCorrectionService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
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
	list := func(path string, suspects bool) func(context.Context, orgapi.Principal, access.Decision, orgapi.Page, string) (any, error) {
		return func(ctx context.Context, p orgapi.Principal, decision access.Decision, input orgapi.Page, state string) (any, error) {
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
			page, e := input.Normalize()
			if e != nil {
				return nil, orgapi.Malformed
			}
			binding := fmt.Sprintf("fleet.meter-readings.v1:%s:%s:%s:%t:%d", assetID, meterID, state, suspects, page.Limit)
			after, e := gateway.OpenCursor(p, path, binding, page.Cursor)
			if e != nil {
				return nil, orgapi.Malformed
			}
			filter := app.MeterHistoryFilter{AfterID: after, Limit: page.Limit, State: meter.State(state)}
			var v app.MeterHistoryPage
			if suspects {
				v, e = service.Suspects(ctx, p.Scope, actor, decision, assetID, meterID, filter)
			} else {
				v, e = service.History(ctx, p.Scope, actor, decision, assetID, meterID, filter)
			}
			if e != nil {
				return nil, meterCorrectionMapped(e)
			}
			items := make([]map[string]any, 0, len(v.Items))
			for _, value := range v.Items {
				item, e := projectHistoryItem(p.Scope, value, suspects)
				if e != nil {
					return nil, e
				}
				items = append(items, item)
			}
			next := ""
			if v.NextID != "" {
				next, e = gateway.SealCursor(p, path, binding, v.NextID)
				if e != nil {
					return nil, orgapi.Unavailable
				}
			}
			return map[string]any{"items": items, "next_cursor": next, "evaluated_at": orgtime.FormatInstant(v.At), "data_mode": "SYNTHETIC"}, nil
		}
	}
	history := list(MeterHistoryPath, false)
	if e := guarded.Register(owned, MeterHistoryPath, gateway, assetRule(false, false, 200), authorization, app.Policy(false), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input meterHistoryRequest) (any, error) {
		if input.State != "" && input.State != string(meter.Valid) && input.State != string(meter.Suspect) && input.State != string(meter.Voided) {
			return nil, orgapi.ValidationFailed
		}
		return history(ctx, p, decision, input.Page, input.State)
	}); e != nil {
		return e
	}
	suspects := list(MeterSuspectsPath, true)
	if e := guarded.Register(owned, MeterSuspectsPath, gateway, meterCorrectionRule(false), authorization, app.MeterReviewPolicy(false), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input meterSuspectRequest) (any, error) {
		return suspects(ctx, p, decision, input.Page, string(meter.Suspect))
	}); e != nil {
		return e
	}
	if e := registerMeterChange(owned, service, gateway, authorization); e != nil {
		return e
	}
	return guarded.Register(owned, MeterDecidePath, gateway, meterCorrectionRule(true), authorization, app.MeterReviewPolicy(true), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input meterDecideRequest) (any, error) {
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
		readingID, e := meterReadingPathID(ctx)
		if e != nil {
			return nil, e
		}
		command, e := input.command(assetID, meterID, readingID)
		if e != nil {
			return nil, meterCorrectionMapped(e)
		}
		v, replay, e := service.Decide(ctx, p.Scope, actor, decision, command)
		if e != nil {
			return nil, meterCorrectionMapped(e)
		}
		reevaluated := make([]map[string]any, 0, len(v.Reevaluated))
		for _, r := range v.Reevaluated {
			reevaluated = append(reevaluated, projectReadingState(r))
		}
		out := map[string]any{"asset_id": assetID, "meter_id": meterID, "decision": input.Decision, "reading": projectReadingState(v.Target), "replacement": nil, "reevaluated": reevaluated, "replayed": replay, "data_mode": "SYNTHETIC"}
		if v.Replacement != nil {
			out["replacement"] = projectReadingState(*v.Replacement)
		}
		if v.Boundary != nil {
			// EMP-METER-005: a confirmed rollover also returns its boundary.
			out["boundary"] = projectBoundary(*v.Boundary)
		}
		return out, nil
	})
}
