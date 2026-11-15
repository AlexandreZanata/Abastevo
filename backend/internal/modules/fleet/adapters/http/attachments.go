package http

import (
	"context"
	"errors"

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

const AttachmentAttachPath = AssetPrefix + "/{asset_id}/attachments/attach"
const AttachmentDetachPath = AssetPrefix + "/{asset_id}/attachments/{attachment_id}/detach"
const AttachmentListPath = AssetPrefix + "/{asset_id}/attachments/list"

// attachRequest admits exactly the UC-FLEET-10 intent for the path implement.
type attachRequest struct {
	companyRequest
	SchemaVersion  int     `json:"schema_version"`
	OperationID    string  `json:"operation_id"`
	PoweredAssetID string  `json:"powered_asset_id"`
	ValidFrom      *string `json:"valid_from"`
	ValidTo        *string `json:"valid_to"`
}

func (v attachRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID}
}
func (v *attachRequest) UnmarshalJSON(raw []byte) error {
	type plain attachRequest
	var input plain
	if err := decodeAsset(raw, &input); err != nil {
		return err
	}
	*v = attachRequest(input)
	return nil
}

type detachRequest struct {
	companyRequest
	SchemaVersion int     `json:"schema_version"`
	OperationID   string  `json:"operation_id"`
	ValidTo       *string `json:"valid_to"`
	Reason        *string `json:"reason"`
}

func (v detachRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID}
}
func (v *detachRequest) UnmarshalJSON(raw []byte) error {
	type plain detachRequest
	var input plain
	if err := decodeAsset(raw, &input); err != nil {
		return err
	}
	*v = detachRequest(input)
	return nil
}

// attachmentMapped adds the UC-FLEET-10 refusals: an overlapping attachment
// and an ended one are CONFLICT.
func attachmentMapped(err error) error {
	switch {
	case errors.Is(err, app.ErrAttachmentOverlap), errors.Is(err, d.ErrAttachmentEnded):
		return orgapi.Conflict
	}
	return assetMapped(err)
}

func attachmentAck(result app.AttachmentResult, replayed bool) (map[string]any, error) {
	if !canonical(result.AssetID) || !canonical(result.AttachmentID) {
		return nil, orgapi.Unavailable
	}
	return map[string]any{"asset_id": result.AssetID, "attachment_id": result.AttachmentID, "replayed": replayed, "data_mode": "SYNTHETIC"}, nil
}

func attachmentItems(page app.AttachmentPage) ([]map[string]any, error) {
	if page.At.IsZero() {
		return nil, orgapi.Unavailable
	}
	items := make([]map[string]any, 0, len(page.Attachments))
	for _, v := range page.Attachments {
		if !canonical(v.ID) || !canonical(v.ImplementID) || !canonical(v.PoweredAssetID) || v.RecordedAt.IsZero() || v.State != v.Period.StateAt(page.At) {
			return nil, orgapi.Unavailable
		}
		item := map[string]any{"attachment_id": v.ID, "implement_id": v.ImplementID, "powered_asset_id": v.PoweredAssetID, "state": v.State, "valid_from": orgtime.FormatInstant(v.Period.From), "recorded_at": orgtime.FormatInstant(v.RecordedAt)}
		optionalInstant(item, "valid_to", v.Period.To)
		if v.DetachedAt != nil {
			item["detached_at"] = orgtime.FormatInstant(*v.DetachedAt)
			if v.DetachReason != "" {
				item["detach_reason"] = v.DetachReason
			}
		}
		items = append(items, item)
	}
	return items, nil
}

// RegisterAssetAttachments exposes the EMP-FLEET-011 commands (fresh
// fleet.asset.manage covering both assets) and the list (current
// fleet.asset.read covering the asset).
func RegisterAssetAttachments(router chi.Router, service app.AttachmentService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
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
	if err := guarded.Register(owned, AttachmentAttachPath, gateway, assetRule(true, false, 201), authorization, app.Policy(true), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input attachRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		implement, err := assetPathID(ctx)
		if err != nil {
			return nil, orgapi.NotFound
		}
		in := app.AttachmentCreateInput{OperationID: input.OperationID, ImplementID: implement, PoweredAssetID: input.PoweredAssetID}
		if in.Start, err = wireInstant(input.ValidFrom); err != nil {
			return nil, orgapi.ValidationFailed
		}
		if in.End, err = wireInstant(input.ValidTo); err != nil {
			return nil, orgapi.ValidationFailed
		}
		command, err := app.NewAttachmentCreate(in)
		if err != nil {
			return nil, attachmentMapped(err)
		}
		result, replayed, err := service.Attach(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, attachmentMapped(err)
		}
		return attachmentAck(result, replayed)
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, AttachmentDetachPath, gateway, assetRule(true, false, 200), authorization, app.Policy(true), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input detachRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		asset, err := assetPathID(ctx)
		if err != nil {
			return nil, orgapi.NotFound
		}
		attachment, err := pathID(ctx, "attachment_id")
		if err != nil {
			return nil, err
		}
		in := app.AttachmentDetachInput{OperationID: input.OperationID, AssetID: asset, AttachmentID: attachment}
		if in.End, err = wireInstant(input.ValidTo); err != nil {
			return nil, orgapi.ValidationFailed
		}
		if input.Reason != nil {
			if *input.Reason == "" {
				return nil, orgapi.ValidationFailed
			}
			in.Reason = *input.Reason
		}
		command, err := app.NewAttachmentDetach(in)
		if err != nil {
			return nil, attachmentMapped(err)
		}
		result, replayed, err := service.Detach(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, attachmentMapped(err)
		}
		return attachmentAck(result, replayed)
	}); err != nil {
		return err
	}
	return guarded.Register(owned, AttachmentListPath, gateway, assetRule(false, false, 200), authorization, app.Policy(false), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input assignmentListRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		asset, err := assetPathID(ctx)
		if err != nil {
			return nil, orgapi.NotFound
		}
		filter, binding, err := periodFilter(p, gateway, AttachmentListPath, "fleet.asset.attachments", asset, input)
		if err != nil {
			return nil, err
		}
		v, err := service.List(ctx, p.Scope, actor, decision, asset, filter)
		if err != nil {
			return nil, attachmentMapped(err)
		}
		items, err := attachmentItems(v)
		if err != nil {
			return nil, err
		}
		return pageResponse(p, gateway, AttachmentListPath, binding, items, v.At, v.NextFrom, v.NextID)
	})
}
