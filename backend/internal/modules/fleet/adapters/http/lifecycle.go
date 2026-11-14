package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const AssetAcquisitionPath = AssetPrefix + "/{asset_id}/acquisition/record"
const AssetDeactivatePath = AssetPrefix + "/{asset_id}/deactivate"
const AssetReactivatePath = AssetPrefix + "/{asset_id}/reactivate"
const AssetDisposePath = AssetPrefix + "/{asset_id}/dispose"
const AssetRevertDisposalPath = AssetPrefix + "/{asset_id}/disposal/revert"
const AssetArchivePath = AssetPrefix + "/{asset_id}/archive"
const AssetLifecycleListPath = AssetPrefix + "/{asset_id}/lifecycle/list"

// lifecycleRequest is the decoded command union; decodeLifecycle admits only
// the keys that the selected §6.1 command names.
type lifecycleRequest struct {
	companyRequest
	SchemaVersion int     `json:"schema_version"`
	OperationID   string  `json:"operation_id"`
	Revision      *int64  `json:"revision"`
	Reason        *string `json:"reason"`
	Kind          *string `json:"kind"`
	DisposedAt    *string `json:"disposed_at"`
	AcquiredAt    *string `json:"acquired_at"`
}

func (v lifecycleRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}
func (v lifecycleRequest) lifecycle() lifecycleRequest { return v }

// lifecycleKeys are the command-specific request keys (B-BR-EMP-203); the
// common schema_version, operation_id and revision are always required.
var lifecycleKeys = map[d.LifecycleCommand][]string{
	d.RecordAcquisition: {"acquired_at"},
	d.Deactivate:        {"reason"},
	d.Reactivate:        {},
	d.Dispose:           {"kind", "disposed_at", "reason"},
	d.RevertDisposal:    {"reason"},
	d.Archive:           {},
}

func decodeLifecycle(command d.LifecycleCommand, raw []byte) (lifecycleRequest, error) {
	allowed, ok := lifecycleKeys[command]
	if !ok {
		return lifecycleRequest{}, orgapi.Malformed
	}
	var v lifecycleRequest
	if err := decodeAsset(raw, &v); err != nil {
		return lifecycleRequest{}, err
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return lifecycleRequest{}, orgapi.Malformed
	}
	for key := range fields {
		if key != "schema_version" && key != "operation_id" && key != "revision" && !slices.Contains(allowed, key) {
			return lifecycleRequest{}, orgapi.Malformed
		}
	}
	return v, nil
}

// command converts exact wire text: an unknown kind, a non-RFC 3339 instant or
// a missing required field is a validation refusal, never a guessed value.
func (v lifecycleRequest) command(command d.LifecycleCommand, asset string) (app.AssetLifecycle, error) {
	if v.Revision == nil {
		return app.AssetLifecycle{}, d.ErrInvalid
	}
	in := app.AssetLifecycleInput{Command: command, OperationID: v.OperationID, AssetID: asset, ExpectedRevision: *v.Revision}
	if v.Reason != nil {
		in.Reason = *v.Reason
		if in.Reason == "" {
			return app.AssetLifecycle{}, d.ErrInvalid
		}
	}
	if v.Kind != nil {
		kind, err := d.ParseDisposalKind(*v.Kind)
		if err != nil {
			return app.AssetLifecycle{}, d.ErrInvalid
		}
		in.DisposalKind = kind
	}
	for _, raw := range []*string{v.DisposedAt, v.AcquiredAt} {
		if raw == nil {
			continue
		}
		at, err := orgtime.ParseInstant(*raw)
		if err != nil {
			return app.AssetLifecycle{}, d.ErrInvalid
		}
		in.At = at
	}
	return app.NewAssetLifecycle(in)
}

type acquisitionRequest struct{ lifecycleRequest }
type deactivateRequest struct{ lifecycleRequest }
type reactivateRequest struct{ lifecycleRequest }
type disposeRequest struct{ lifecycleRequest }
type revertDisposalRequest struct{ lifecycleRequest }
type archiveRequest struct{ lifecycleRequest }
type lifecycleListRequest struct {
	companyRequest
	orgapi.Page
}

func (v *acquisitionRequest) UnmarshalJSON(raw []byte) (err error) {
	v.lifecycleRequest, err = decodeLifecycle(d.RecordAcquisition, raw)
	return err
}
func (v *deactivateRequest) UnmarshalJSON(raw []byte) (err error) {
	v.lifecycleRequest, err = decodeLifecycle(d.Deactivate, raw)
	return err
}
func (v *reactivateRequest) UnmarshalJSON(raw []byte) (err error) {
	v.lifecycleRequest, err = decodeLifecycle(d.Reactivate, raw)
	return err
}
func (v *disposeRequest) UnmarshalJSON(raw []byte) (err error) {
	v.lifecycleRequest, err = decodeLifecycle(d.Dispose, raw)
	return err
}
func (v *revertDisposalRequest) UnmarshalJSON(raw []byte) (err error) {
	v.lifecycleRequest, err = decodeLifecycle(d.RevertDisposal, raw)
	return err
}
func (v *archiveRequest) UnmarshalJSON(raw []byte) (err error) {
	v.lifecycleRequest, err = decodeLifecycle(d.Archive, raw)
	return err
}
func (v *lifecycleListRequest) UnmarshalJSON(raw []byte) error {
	type plain lifecycleListRequest
	var input plain
	if err := decodeAsset(raw, &input); err != nil {
		return err
	}
	*v = lifecycleListRequest(input)
	return nil
}

// lifecycleMapped adds the bounded BR-FLEET-010 refusal to the asset mapping.
func lifecycleMapped(err error) error {
	var open *app.OpenOperationsError
	if errors.As(err, &open) {
		kinds := make([]string, 0, 2)
		for _, kind := range open.Kinds() {
			kinds = append(kinds, string(kind))
		}
		return orgapi.AssetOpenOperations(kinds)
	}
	return assetMapped(err)
}

func lifecycleItems(scope kernel.OrgScope, entries []app.LifecycleEntry) ([]map[string]any, error) {
	if scope.Validate() != nil {
		return nil, orgapi.Unavailable
	}
	items := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		if to, err := d.Transition(entry.From, entry.Command); err != nil || to != entry.To || entry.Revision < 2 || entry.RecordedAt.IsZero() {
			return nil, orgapi.Unavailable
		}
		item := map[string]any{"revision": entry.Revision, "command": entry.Command, "from_state": entry.From, "to_state": entry.To, "reason": entry.Reason, "recorded_at": orgtime.FormatInstant(entry.RecordedAt)}
		if entry.Command == d.Dispose {
			if entry.DisposedAt == nil || entry.DisposalKind == "" {
				return nil, orgapi.Unavailable
			}
			item["disposal_kind"], item["disposed_at"] = entry.DisposalKind, orgtime.FormatInstant(*entry.DisposedAt)
		} else if entry.DisposedAt != nil || entry.DisposalKind != "" {
			return nil, orgapi.Unavailable
		}
		items = append(items, item)
	}
	return items, nil
}

type lifecycleInput interface {
	orgapi.Request
	lifecycle() lifecycleRequest
}

func registerLifecycleCommand[T lifecycleInput](owned chi.Router, path string, command d.LifecycleCommand, gateway orgapi.Gateway, authorization access.Authorization, service app.AssetLifecycleService) error {
	return guarded.Register(owned, path, gateway, assetRule(true, true, 200), authorization, app.Policy(true), assetResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input T) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		id, err := assetPathID(ctx)
		if err != nil {
			return nil, orgapi.NotFound
		}
		c, err := input.lifecycle().command(command, id)
		if err != nil {
			return nil, lifecycleMapped(err)
		}
		result, replayed, err := service.Transition(ctx, p.Scope, actor, decision, c)
		if err != nil {
			return nil, lifecycleMapped(err)
		}
		return assetAck(result, replayed)
	})
}

// RegisterAssetLifecycle exposes the EMP-FLEET-003 commands and history read
// through the existing company pipeline: fresh fleet.asset.manage for every
// command, current fleet.asset.read for the history.
func RegisterAssetLifecycle(router chi.Router, service app.AssetLifecycleService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
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
	if err := registerLifecycleCommand[acquisitionRequest](owned, AssetAcquisitionPath, d.RecordAcquisition, gateway, authorization, service); err != nil {
		return err
	}
	if err := registerLifecycleCommand[deactivateRequest](owned, AssetDeactivatePath, d.Deactivate, gateway, authorization, service); err != nil {
		return err
	}
	if err := registerLifecycleCommand[reactivateRequest](owned, AssetReactivatePath, d.Reactivate, gateway, authorization, service); err != nil {
		return err
	}
	if err := registerLifecycleCommand[disposeRequest](owned, AssetDisposePath, d.Dispose, gateway, authorization, service); err != nil {
		return err
	}
	if err := registerLifecycleCommand[revertDisposalRequest](owned, AssetRevertDisposalPath, d.RevertDisposal, gateway, authorization, service); err != nil {
		return err
	}
	if err := registerLifecycleCommand[archiveRequest](owned, AssetArchivePath, d.Archive, gateway, authorization, service); err != nil {
		return err
	}
	return guarded.Register(owned, AssetLifecycleListPath, gateway, assetRule(false, false, 200), authorization, app.Policy(false), assetResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input lifecycleListRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		id, err := assetPathID(ctx)
		if err != nil {
			return nil, orgapi.NotFound
		}
		page, err := input.Page.Normalize()
		if err != nil {
			return nil, orgapi.Malformed
		}
		binding := fmt.Sprintf("fleet.asset.lifecycle.v1:%s:%d", id, page.Limit)
		after, err := gateway.OpenCursor(p, AssetLifecycleListPath, binding, page.Cursor)
		if err != nil {
			return nil, orgapi.Malformed
		}
		filter := app.LifecycleFilter{Limit: page.Limit}
		if after != "" {
			revision, err := strconv.ParseInt(after, 10, 64)
			if err != nil || revision < 1 || strconv.FormatInt(revision, 10) != after {
				return nil, orgapi.Malformed
			}
			filter.AfterRevision = revision
		}
		v, err := service.History(ctx, p.Scope, actor, decision, id, filter)
		if err != nil {
			return nil, lifecycleMapped(err)
		}
		items, err := lifecycleItems(p.Scope, v.Entries)
		if err != nil {
			return nil, err
		}
		next := ""
		if v.NextRevision != 0 {
			next, err = gateway.SealCursor(p, AssetLifecycleListPath, binding, strconv.FormatInt(v.NextRevision, 10))
			if err != nil {
				return nil, orgapi.Unavailable
			}
		}
		return map[string]any{"items": items, "next_cursor": next}, nil
	})
}
