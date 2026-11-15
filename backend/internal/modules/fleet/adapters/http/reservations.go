package http

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const ReservationRequestPath = AssetPrefix + "/{asset_id}/reservations/request"
const ReservationConfirmPath = AssetPrefix + "/{asset_id}/reservations/{reservation_id}/confirm"
const ReservationCancelPath = AssetPrefix + "/{asset_id}/reservations/{reservation_id}/cancel"
const ReservationListPath = AssetPrefix + "/{asset_id}/reservations/list"
const BlockCreatePath = AssetPrefix + "/{asset_id}/blocks/create"
const BlockRemovePath = AssetPrefix + "/{asset_id}/blocks/{block_id}/remove"
const BlockListPath = AssetPrefix + "/{asset_id}/blocks/list"
const AvailabilityReadPath = AssetPrefix + "/{asset_id}/availability/read"

// reservationRequest admits exactly the BUC-EMP-148 intent: no state,
// requester, tolerance or caller authority is chosen by the request.
type reservationRequest struct {
	companyRequest
	SchemaVersion int     `json:"schema_version"`
	OperationID   string  `json:"operation_id"`
	StartsAt      string  `json:"starts_at"`
	EndsAt        string  `json:"ends_at"`
	EmployeeID    *string `json:"employee_id"`
	Purpose       string  `json:"purpose"`
	Confirm       *bool   `json:"confirm"`
}

func (v reservationRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID}
}
func (v *reservationRequest) UnmarshalJSON(raw []byte) error {
	type plain reservationRequest
	var input plain
	if err := decodeAsset(raw, &input); err != nil {
		return err
	}
	*v = reservationRequest(input)
	return nil
}

func (v reservationRequest) command(asset string) (app.ReservationRequest, error) {
	in := app.ReservationRequestInput{OperationID: v.OperationID, AssetID: asset, Purpose: v.Purpose}
	start, err := wireInstant(&v.StartsAt)
	if err != nil {
		return app.ReservationRequest{}, err
	}
	end, err := wireInstant(&v.EndsAt)
	if err != nil {
		return app.ReservationRequest{}, err
	}
	in.Start, in.End = *start, *end
	if v.EmployeeID != nil {
		if *v.EmployeeID == "" {
			return app.ReservationRequest{}, d.ErrInvalid
		}
		in.EmployeeID = *v.EmployeeID
	}
	if v.Confirm != nil {
		in.Confirm = *v.Confirm
	}
	return app.NewReservationRequest(in)
}

// reservationActRequest confirms (no reason) or cancels (optional reason).
type reservationActRequest struct {
	companyRequest
	SchemaVersion int     `json:"schema_version"`
	OperationID   string  `json:"operation_id"`
	Reason        *string `json:"reason"`
}

func (v reservationActRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID}
}
func (v *reservationActRequest) UnmarshalJSON(raw []byte) error {
	type plain reservationActRequest
	var input plain
	if err := decodeAsset(raw, &input); err != nil {
		return err
	}
	*v = reservationActRequest(input)
	return nil
}

type blockCreateRequest struct {
	companyRequest
	SchemaVersion int     `json:"schema_version"`
	OperationID   string  `json:"operation_id"`
	ReasonKind    string  `json:"reason_kind"`
	Note          *string `json:"note"`
	ValidFrom     *string `json:"valid_from"`
	ValidTo       *string `json:"valid_to"`
}

func (v blockCreateRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID}
}
func (v *blockCreateRequest) UnmarshalJSON(raw []byte) error {
	type plain blockCreateRequest
	var input plain
	if err := decodeAsset(raw, &input); err != nil {
		return err
	}
	*v = blockCreateRequest(input)
	return nil
}

func (v blockCreateRequest) command(asset string) (app.BlockCreate, error) {
	reason, err := d.ParseBlockReason(v.ReasonKind)
	if err != nil {
		return app.BlockCreate{}, d.ErrInvalid
	}
	in := app.BlockCreateInput{OperationID: v.OperationID, AssetID: asset, Reason: reason}
	if in.Start, err = wireInstant(v.ValidFrom); err != nil {
		return app.BlockCreate{}, err
	}
	if in.End, err = wireInstant(v.ValidTo); err != nil {
		return app.BlockCreate{}, err
	}
	if v.Note != nil {
		if *v.Note == "" {
			return app.BlockCreate{}, d.ErrInvalid
		}
		in.Note = *v.Note
	}
	return app.NewBlockCreate(in)
}

// blockRemoveRequest requires a reason (BR-FLEET-032); its absence is a
// validation refusal, never a guessed text.
type blockRemoveRequest struct {
	companyRequest
	SchemaVersion int     `json:"schema_version"`
	OperationID   string  `json:"operation_id"`
	Reason        *string `json:"reason"`
}

func (v blockRemoveRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID}
}
func (v *blockRemoveRequest) UnmarshalJSON(raw []byte) error {
	type plain blockRemoveRequest
	var input plain
	if err := decodeAsset(raw, &input); err != nil {
		return err
	}
	*v = blockRemoveRequest(input)
	return nil
}

// reservationMapped adds the B-BR-EMP-295/297/298 refusals to the asset
// mapping; a conflict reports only its window.
func reservationMapped(err error) error {
	var conflict app.ReservationConflict
	switch {
	case errors.As(err, &conflict):
		return orgapi.ReservationWindowConflict(conflict.From, conflict.To)
	case errors.Is(err, app.ErrReservationConflict):
		return orgapi.ReservationConflict
	case errors.Is(err, d.ErrReservationEnded), errors.Is(err, d.ErrBlockEnded):
		return orgapi.Conflict
	}
	return assetMapped(err)
}

func reserveRule(status int) orgapi.Rule {
	return orgapi.Rule{Permission: string(app.ReservePolicy().Permission), Write: true, Fresh: true, CompanyOnly: true, Success: status}
}

func pathID(ctx context.Context, name string) (string, error) {
	raw := chi.URLParamFromCtx(ctx, name)
	if !canonical(raw) {
		return "", orgapi.NotFound
	}
	return raw, nil
}

func reservationAck(result app.ReservationResult, replayed bool) (map[string]any, error) {
	if !canonical(result.AssetID) || !canonical(result.ReservationID) {
		return nil, orgapi.Unavailable
	}
	if _, err := d.ParseReservationState(string(result.State)); err != nil {
		return nil, orgapi.Unavailable
	}
	return map[string]any{"asset_id": result.AssetID, "reservation_id": result.ReservationID, "state": result.State, "replayed": replayed, "data_mode": "SYNTHETIC"}, nil
}

func blockAck(result app.BlockResult, replayed bool) (map[string]any, error) {
	if !canonical(result.AssetID) || !canonical(result.BlockID) {
		return nil, orgapi.Unavailable
	}
	return map[string]any{"asset_id": result.AssetID, "block_id": result.BlockID, "replayed": replayed, "data_mode": "SYNTHETIC"}, nil
}

func optionalInstant(item map[string]any, key string, at *time.Time) {
	if at != nil {
		item[key] = orgtime.FormatInstant(*at)
	}
}

// reservationItems projects stored rows with the effective state at the page
// instant; optional fields are omitted while absent.
func reservationItems(page app.ReservationPage) ([]map[string]any, error) {
	if page.At.IsZero() {
		return nil, orgapi.Unavailable
	}
	items := make([]map[string]any, 0, len(page.Reservations))
	for _, v := range page.Reservations {
		if !canonical(v.ID) || !canonical(v.AssetID) || v.EmployeeID != "" && !canonical(v.EmployeeID) || v.RequestedAt.IsZero() {
			return nil, orgapi.Unavailable
		}
		item := map[string]any{"reservation_id": v.ID, "asset_id": v.AssetID, "purpose": v.Purpose, "starts_at": orgtime.FormatInstant(v.Window.From), "ends_at": orgtime.FormatInstant(v.Window.To),
			"no_show_tolerance_minutes": int64(v.Tolerance / time.Minute), "state": v.State, "requested_at": orgtime.FormatInstant(v.RequestedAt)}
		if v.EmployeeID != "" {
			item["employee_id"] = v.EmployeeID
		}
		optionalInstant(item, "confirmed_at", v.ConfirmedAt)
		if v.EndedAt != nil {
			item["end_cause"], item["ended_at"] = v.EndCause, orgtime.FormatInstant(*v.EndedAt)
			if v.EndReason != "" {
				item["end_reason"] = v.EndReason
			}
		}
		items = append(items, item)
	}
	return items, nil
}

func blockItems(page app.BlockPage) ([]map[string]any, error) {
	if page.At.IsZero() {
		return nil, orgapi.Unavailable
	}
	items := make([]map[string]any, 0, len(page.Blocks))
	for _, v := range page.Blocks {
		if !canonical(v.ID) || !canonical(v.AssetID) || v.RecordedAt.IsZero() {
			return nil, orgapi.Unavailable
		}
		item := map[string]any{"block_id": v.ID, "asset_id": v.AssetID, "reason_kind": v.Reason, "valid_from": orgtime.FormatInstant(v.Period.From), "recorded_at": orgtime.FormatInstant(v.RecordedAt)}
		if v.Note != "" {
			item["note"] = v.Note
		}
		optionalInstant(item, "valid_to", v.Period.To)
		if v.RemovedAt != nil {
			item["removed_at"], item["removal_reason"] = orgtime.FormatInstant(*v.RemovedAt), v.RemovalReason
		}
		items = append(items, item)
	}
	return items, nil
}

// availabilityBody carries the derived answer, every cause with its source
// kind, record and period, and the sources that could not be consulted.
func availabilityBody(v app.AvailabilityView) (map[string]any, error) {
	if !canonical(v.AssetID) || v.At.IsZero() {
		return nil, orgapi.Unavailable
	}
	causes := make([]map[string]any, 0, len(v.Assessment.Causes))
	for _, c := range v.Assessment.Causes {
		item := map[string]any{"availability": c.Availability, "kind": c.Kind}
		switch c.Kind {
		case d.LifecycleCause:
			item["lifecycle_state"] = c.Lifecycle
		default:
			if !canonical(c.ID) {
				return nil, orgapi.Unavailable
			}
			item["record_id"], item["valid_from"] = c.ID, orgtime.FormatInstant(c.From)
			optionalInstant(item, "valid_to", c.To)
			if c.Kind == d.BlockCause {
				item["reason_kind"] = c.Block
			}
		}
		causes = append(causes, item)
	}
	missing := make([]string, 0, len(v.MissingSources))
	for _, source := range v.MissingSources {
		missing = append(missing, string(source))
	}
	return map[string]any{"asset_id": v.AssetID, "availability": v.Assessment.Availability, "causes": causes, "missing_sources": missing, "evaluated_at": orgtime.FormatInstant(v.At)}, nil
}

// periodFilter opens a keyset cursor bound to the route, asset and limit.
func periodFilter(p orgapi.Principal, gateway orgapi.Gateway, path, name, asset string, input assignmentListRequest) (app.PeriodFilter, string, error) {
	page, err := input.Page.Normalize()
	if err != nil {
		return app.PeriodFilter{}, "", orgapi.Malformed
	}
	binding := fmt.Sprintf("%s.v1:%s:%d", name, asset, page.Limit)
	after, err := gateway.OpenCursor(p, path, binding, page.Cursor)
	if err != nil {
		return app.PeriodFilter{}, "", orgapi.Malformed
	}
	filter := app.PeriodFilter{Limit: page.Limit}
	if after != "" {
		from, key, ok := strings.Cut(after, "|")
		at, err := orgtime.ParseInstant(from)
		if !ok || err != nil || orgtime.FormatInstant(at) != from || !canonical(key) {
			return app.PeriodFilter{}, "", orgapi.Malformed
		}
		filter.AfterFrom, filter.AfterID = &at, key
	}
	return filter, binding, nil
}

func pageResponse(p orgapi.Principal, gateway orgapi.Gateway, path, binding string, items []map[string]any, at time.Time, nextFrom *time.Time, nextID string) (any, error) {
	cursor := ""
	if nextID != "" && nextFrom != nil {
		var err error
		if cursor, err = gateway.SealCursor(p, path, binding, orgtime.FormatInstant(*nextFrom)+"|"+nextID); err != nil {
			return nil, orgapi.Unavailable
		}
	}
	return map[string]any{"items": items, "next_cursor": cursor, "evaluated_at": orgtime.FormatInstant(at)}, nil
}

// RegisterAssetReservations exposes the EMP-FLEET-007 commands (fresh
// fleet.reservation.manage covering the asset) and reads (current
// fleet.asset.read covering the asset).
func RegisterAssetReservations(router chi.Router, service app.ReservationService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
	if !service.Enabled {
		return nil
	}
	if router == nil || missing(service.Repository) || service.NewID == nil || missing(authorization.Source) || missing(authorization.Capabilities) || missing(authorization.Records) || authorization.Now == nil {
		return app.ErrUnavailable
	}
	owned, gateway, err := organization.CompanyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	resource := assetResource{}
	if err := guarded.Register(owned, ReservationRequestPath, gateway, reserveRule(201), authorization, app.ReservePolicy(), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input reservationRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		asset, err := assetPathID(ctx)
		if err != nil {
			return nil, orgapi.NotFound
		}
		command, err := input.command(asset)
		if err != nil {
			return nil, reservationMapped(err)
		}
		result, replayed, err := service.Request(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, reservationMapped(err)
		}
		return reservationAck(result, replayed)
	}); err != nil {
		return err
	}
	for _, act := range []struct {
		path string
		make func(app.ReservationActInput) (app.ReservationAct, error)
	}{{ReservationConfirmPath, app.NewReservationConfirm}, {ReservationCancelPath, app.NewReservationCancel}} {
		if err := guarded.Register(owned, act.path, gateway, reserveRule(200), authorization, app.ReservePolicy(), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input reservationActRequest) (any, error) {
			actor, err := currentActor(ctx)
			if err != nil {
				return nil, err
			}
			asset, err := assetPathID(ctx)
			if err != nil {
				return nil, orgapi.NotFound
			}
			reservation, err := pathID(ctx, "reservation_id")
			if err != nil {
				return nil, err
			}
			in := app.ReservationActInput{OperationID: input.OperationID, AssetID: asset, ReservationID: reservation}
			if input.Reason != nil {
				if *input.Reason == "" {
					return nil, orgapi.ValidationFailed
				}
				in.Reason = *input.Reason
			}
			command, err := act.make(in)
			if err != nil {
				return nil, reservationMapped(err)
			}
			result, replayed, err := service.Act(ctx, p.Scope, actor, decision, command)
			if err != nil {
				return nil, reservationMapped(err)
			}
			return reservationAck(result, replayed)
		}); err != nil {
			return err
		}
	}
	if err := guarded.Register(owned, BlockCreatePath, gateway, reserveRule(201), authorization, app.ReservePolicy(), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input blockCreateRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		asset, err := assetPathID(ctx)
		if err != nil {
			return nil, orgapi.NotFound
		}
		command, err := input.command(asset)
		if err != nil {
			return nil, reservationMapped(err)
		}
		result, replayed, err := service.Block(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, reservationMapped(err)
		}
		return blockAck(result, replayed)
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, BlockRemovePath, gateway, reserveRule(200), authorization, app.ReservePolicy(), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input blockRemoveRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		asset, err := assetPathID(ctx)
		if err != nil {
			return nil, orgapi.NotFound
		}
		block, err := pathID(ctx, "block_id")
		if err != nil {
			return nil, err
		}
		if input.Reason == nil {
			return nil, orgapi.ValidationFailed
		}
		command, err := app.NewBlockRemove(app.BlockRemoveInput{OperationID: input.OperationID, AssetID: asset, BlockID: block, Reason: *input.Reason})
		if err != nil {
			return nil, reservationMapped(err)
		}
		result, replayed, err := service.Unblock(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, reservationMapped(err)
		}
		return blockAck(result, replayed)
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, ReservationListPath, gateway, assetRule(false, false, 200), authorization, app.Policy(false), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input assignmentListRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		asset, err := assetPathID(ctx)
		if err != nil {
			return nil, orgapi.NotFound
		}
		filter, binding, err := periodFilter(p, gateway, ReservationListPath, "fleet.asset.reservations", asset, input)
		if err != nil {
			return nil, err
		}
		v, err := service.List(ctx, p.Scope, actor, decision, asset, filter)
		if err != nil {
			return nil, reservationMapped(err)
		}
		items, err := reservationItems(v)
		if err != nil {
			return nil, err
		}
		return pageResponse(p, gateway, ReservationListPath, binding, items, v.At, v.NextFrom, v.NextID)
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, BlockListPath, gateway, assetRule(false, false, 200), authorization, app.Policy(false), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input assignmentListRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		asset, err := assetPathID(ctx)
		if err != nil {
			return nil, orgapi.NotFound
		}
		filter, binding, err := periodFilter(p, gateway, BlockListPath, "fleet.asset.blocks", asset, input)
		if err != nil {
			return nil, err
		}
		v, err := service.Blocks(ctx, p.Scope, actor, decision, asset, filter)
		if err != nil {
			return nil, reservationMapped(err)
		}
		items, err := blockItems(v)
		if err != nil {
			return nil, err
		}
		return pageResponse(p, gateway, BlockListPath, binding, items, v.At, v.NextFrom, v.NextID)
	}); err != nil {
		return err
	}
	return guarded.Register(owned, AvailabilityReadPath, gateway, assetRule(false, false, 200), authorization, app.Policy(false), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ assetReadRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		asset, err := assetPathID(ctx)
		if err != nil {
			return nil, orgapi.NotFound
		}
		v, err := service.Availability(ctx, p.Scope, actor, decision, asset)
		if err != nil {
			return nil, reservationMapped(err)
		}
		return availabilityBody(v)
	})
}
