package http

import (
	"context"
	"strconv"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

// MeterChangePath replaces or resets a meter (EMP-METER-005, EDR-313).
const MeterChangePath = AssetPrefix + "/{asset_id}/meters/{meter_id}/change"

type meterChangeRequest struct {
	assetRevisionCommand
	Change              app.MeterChange `json:"change"`
	Kind                meter.Kind      `json:"kind"`
	OccurredAt          string          `json:"occurred_at"`
	EntryZone           string          `json:"entry_zone"`
	DeviceOffsetSeconds *int64          `json:"device_offset_seconds"`
	FinalValue          string          `json:"final_value,omitempty"`
	FinalUnknown        bool            `json:"final_unknown,omitempty"`
	Label               string          `json:"label,omitempty"`
	Modulus             string          `json:"modulus,omitempty"`
	OpeningValue        string          `json:"opening_value,omitempty"`
	BeforeValue         string          `json:"before_value,omitempty"`
	AfterValue          string          `json:"after_value,omitempty"`
}

func (v *meterChangeRequest) UnmarshalJSON(raw []byte) error {
	type plain meterChangeRequest
	var input plain
	if e := decodeAsset(raw, &input); e != nil {
		return e
	}
	*v = meterChangeRequest(input)
	return nil
}

// command reads every value as exact base units of the stated kind; the
// store re-proves the kind, the modulus and the segment count.
func (v meterChangeRequest) command(assetID, meterID string) (app.MeterChangeCommand, error) {
	if v.SchemaVersion != 1 || v.Revision == nil || v.DeviceOffsetSeconds == nil {
		return app.MeterChangeCommand{}, orgapi.Malformed
	}
	at, e := orgtime.ParseInstant(v.OccurredAt)
	if e != nil {
		return app.MeterChangeCommand{}, meter.ErrInvalid
	}
	in := app.MeterChangeInput{OperationID: v.OperationID, AssetID: assetID, MeterID: meterID, ExpectedSegments: *v.Revision, Change: v.Change,
		Declared: orgtime.Declaration{OccurredAt: at, EntryZone: v.EntryZone, DeviceOffsetSeconds: *v.DeviceOffsetSeconds}}
	value := func(raw string) (meter.Value, error) { return meter.ParseBase(v.Kind, raw, nil) }
	switch v.Change {
	case app.MeterReplace:
		if v.BeforeValue != "" || v.AfterValue != "" || (v.FinalValue == "") != v.FinalUnknown {
			return app.MeterChangeCommand{}, meter.ErrInvalid
		}
		if !v.FinalUnknown {
			final, e := value(v.FinalValue)
			if e != nil {
				return app.MeterChangeCommand{}, meter.ErrInvalid
			}
			in.Final = &final
		}
		if in.Opening, e = value(v.OpeningValue); e != nil {
			return app.MeterChangeCommand{}, meter.ErrInvalid
		}
		if v.Modulus != "" {
			// The install request's rule: canonical positive int64 text.
			n, e := strconv.ParseInt(v.Modulus, 10, 64)
			if e != nil || n <= 0 || strconv.FormatInt(n, 10) != v.Modulus {
				return app.MeterChangeCommand{}, meter.ErrInvalid
			}
			in.Modulus = &n
		}
		in.Label = v.Label
	case app.MeterReset:
		if v.FinalValue != "" || v.FinalUnknown || v.Label != "" || v.Modulus != "" || v.OpeningValue != "" {
			return app.MeterChangeCommand{}, meter.ErrInvalid
		}
		if in.Before, e = value(v.BeforeValue); e != nil {
			return app.MeterChangeCommand{}, meter.ErrInvalid
		}
		if in.After, e = value(v.AfterValue); e != nil {
			return app.MeterChangeCommand{}, meter.ErrInvalid
		}
	default:
		return app.MeterChangeCommand{}, meter.ErrInvalid
	}
	return app.NewMeterChangeCommand(in)
}

// optionalCount prints a count as exact decimal text, nil when unknown.
func optionalCount(v *int64) any {
	if v == nil {
		return nil
	}
	return strconv.FormatInt(*v, 10)
}

func projectBoundary(v app.MeterBoundaryResult) map[string]any {
	reevaluated := make([]map[string]any, 0, len(v.Reevaluated))
	for _, r := range v.Reevaluated {
		reevaluated = append(reevaluated, projectReadingState(r))
	}
	implied := map[string]any{"status": "OK", "reason": nil, "value": optionalCount(v.Implied), "from_reading_id": v.PreviousReadingID}
	if v.Implied == nil {
		implied["status"], implied["reason"] = "INSUFFICIENT_DATA", string(meter.MeterGap)
	}
	return map[string]any{"change": v.Change, "meter_id": v.MeterID, "closed_segment_id": v.ClosedSegmentID, "at": orgtime.FormatInstant(v.At),
		"closing_value": optionalCount(v.Close), "closing_reading_id": nullable(v.ClosingReadingID), "implied_usage": implied, "new_meter_id": nullable(v.NewMeterID),
		"opened_segment_id": v.OpenedSegmentID, "opening_reading_id": v.OpeningReadingID, "opening_value": strconv.FormatInt(v.Open, 10), "segments": v.Segments, "reevaluated": reevaluated}
}

func registerMeterChange(owned chi.Router, service app.MeterCorrectionService, gateway orgapi.Gateway, authorization access.Authorization) error {
	return guarded.Register(owned, MeterChangePath, gateway, meterCorrectionRule(true), authorization, app.MeterReviewPolicy(true), assetResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input meterChangeRequest) (any, error) {
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
		command, e := input.command(assetID, meterID)
		if e != nil {
			return nil, meterCorrectionMapped(e)
		}
		v, replay, e := service.Change(ctx, p.Scope, actor, decision, command)
		if e != nil {
			return nil, meterCorrectionMapped(e)
		}
		out := projectBoundary(v)
		out["asset_id"], out["replayed"], out["data_mode"] = assetID, replay, "SYNTHETIC"
		return out, nil
	})
}
