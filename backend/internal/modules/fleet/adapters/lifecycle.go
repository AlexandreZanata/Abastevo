package adapters

import (
	"context"
	"errors"
	"math"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/fleet"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ app.AssetLifecycleRepository = (*AssetStore)(nil)

// NoOpenWork is the explicit "none" open-work adapter of B-BR-EMP-205/EDR-144.
// No trip or maintenance work order can exist before the trip and maintenance
// epics deliver them; each epic replaces this adapter with its actual source.
type NoOpenWork struct{}

var _ app.AssetOpenWork = NoOpenWork{}

func (NoOpenWork) OpenAssetWorkTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, asset string) ([]app.OpenWorkKind, error) {
	if tx == nil || scope.Validate() != nil || !validID(asset) {
		return nil, app.ErrUnavailable
	}
	return nil, nil
}

// lifecycleActions are the exact audit actions of EDR-144; deactivated,
// disposed and archived are the fleet.md §9 lifecycle event names.
var lifecycleActions = map[fleet.LifecycleCommand]string{
	fleet.RecordAcquisition: "fleet.asset.acquisition_recorded",
	fleet.Deactivate:        "fleet.asset.deactivated",
	fleet.Reactivate:        "fleet.asset.reactivated",
	fleet.Dispose:           "fleet.asset.disposed",
	fleet.RevertDisposal:    "fleet.asset.disposal_reverted",
	fleet.Archive:           "fleet.asset.archived",
}

// The audit record carries the disposal kind only as a registered code.
var disposalReasonCodes = map[fleet.DisposalKind]string{fleet.Sale: "sale", fleet.WriteOff: "write_off", fleet.ReturnToHolder: "return_to_holder"}

func lifecycleAuditDefinitions() []audit.Definition {
	out := make([]audit.Definition, 0, len(lifecycleActions))
	for _, command := range fleet.LifecycleCommands() {
		definition := audit.Definition{Action: lifecycleActions[command], TargetType: "asset"}
		if command == fleet.Dispose {
			definition.Reasons, definition.RequireReason = []string{"sale", "write_off", "return_to_holder"}, true
		}
		out = append(out, definition)
	}
	return out
}

func optionalTime(at *time.Time) (pgtype.Timestamptz, int16) {
	if at == nil {
		return pgtype.Timestamptz{}, 0
	}
	return meterTime(*at)
}

// validReceiptState checks a stored lifecycle receipt against its command.
func validReceiptState(command fleet.LifecycleCommand, state string) bool {
	for _, from := range []fleet.State{fleet.Draft, fleet.Active, fleet.Inactive, fleet.Disposed} {
		if to, err := fleet.Transition(from, command); err == nil && string(to) == state {
			return true
		}
	}
	return false
}

func (s *AssetStore) openWork(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, asset string) error {
	if meterIntakeAbsent(s.OpenWork) {
		return app.ErrUnavailable
	}
	kinds, err := s.OpenWork.OpenAssetWorkTx(ctx, tx, scope, asset)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return app.ErrUnavailable
	}
	if len(kinds) == 0 {
		return nil
	}
	return app.NewOpenOperations(kinds)
}

// TransitionAsset runs one EMP-FLEET-003 command under the organization root
// lock with exactly one audit event, revision snapshot, receipt and, for a
// state change, history row. A same-intent replay returns the first result.
func (s *AssetStore) TransitionAsset(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member string, command app.AssetLifecycle) (app.AssetResult, bool, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.AssetResult{}, false, err
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.AssetResult{}, false, err
	}
	if s == nil || s.Pool == nil || s.Authority == nil || s.Audit == nil {
		return app.AssetResult{}, false, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	digest := command.Digest(scope)
	var result app.AssetResult
	replayed := false
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, v.AssetID, app.FleetManage); err != nil {
			return err
		}
		queries := q.New(tx)
		if _, err := queries.AssetRootLock(ctx, org); err != nil {
			return err
		}
		prior, err := queries.AssetCommandReceipt(ctx, q.AssetCommandReceiptParams{OrgID: org, OperationID: v.OperationID})
		if err == nil {
			if prior.ActorID != actor.AccountID || prior.Kind != string(v.Command) || prior.RequestHash != digest || prior.AssetID != v.AssetID {
				return app.ErrOperationReused
			}
			if prior.Discarded || prior.ResultRevision != v.ExpectedRevision+1 || !validReceiptState(v.Command, prior.ResultState) {
				return app.ErrUnavailable
			}
			result = app.AssetResult{AssetID: prior.AssetID, State: fleet.State(prior.ResultState), Revision: prior.ResultRevision}
			replayed = true
			return s.authorize(ctx, tx, scope, actor, member, v.AssetID, app.FleetManage)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		event := audit.Event{OperationID: v.OperationID, ActorID: actor.AccountID, Action: lifecycleActions[v.Command], TargetType: "asset", TargetID: v.AssetID, ReasonCode: disposalReasonCodes[v.DisposalKind], RequestHash: digest}
		repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			queries = q.New(tx)
			current, err := loadAsset(ctx, tx, scope, v.AssetID)
			if err != nil {
				return err
			}
			if current.Revision != v.ExpectedRevision || current.Revision == math.MaxInt64 {
				return app.ErrConflict
			}
			to, err := fleet.Transition(current.State, v.Command)
			if err != nil {
				return app.ErrConflict
			}
			now, err := meterDBNow(ctx, queries)
			if err != nil {
				return err
			}
			next := current.Clone()
			next.State, next.Revision = to, current.Revision+1
			switch v.Command {
			case fleet.RecordAcquisition:
				if v.At.After(now) {
					return fleet.ErrInvalid
				}
				at := v.At
				next.AcquiredAt = &at
			case fleet.Dispose:
				if v.At.After(now) || current.AcquiredAt != nil && v.At.Before(*current.AcquiredAt) {
					return fleet.ErrInvalid
				}
				next.Disposal = &app.AssetDisposal{Kind: v.DisposalKind, At: v.At, Reason: v.Reason}
			case fleet.RevertDisposal:
				next.Disposal = nil
			case fleet.Reactivate:
				// BR-FLEET-004 still holds: re-prove the actual activation sources.
				if err := checkActualActivation(ctx, tx, scope, current.Ref, current.Profile, current.UnitID); err != nil {
					return err
				}
				if err := s.checkVehicleSlot(ctx, tx, scope, current, next); err != nil {
					return err
				}
			}
			if fleet.ChecksOpenWork(v.Command) {
				if err := s.openWork(ctx, tx, scope, v.AssetID); err != nil {
					return err
				}
			}
			acquired, acquiredSub := optionalTime(next.AcquiredAt)
			params := q.TransitionAssetLifecycleParams{OrgID: org, ID: v.AssetID, ExpectedRevision: v.ExpectedRevision, State: string(to), AcquiredAt: acquired, AcquiredSubmicro: acquiredSub}
			if d := next.Disposal; d != nil {
				params.DisposedAt, params.DisposedSubmicro = meterTime(d.At)
				params.DisposalKind, params.DisposalReason = string(d.Kind), d.Reason
			}
			n, err := queries.TransitionAssetLifecycle(ctx, params)
			if err != nil {
				return err
			}
			if n != 1 {
				return app.ErrConflict
			}
			raw, err := snapshotBytes(next)
			if err != nil {
				return app.ErrUnavailable
			}
			if err := queries.InsertAssetRevision(ctx, q.InsertAssetRevisionParams{OrgID: org, AssetID: v.AssetID, Revision: next.Revision, OperationID: v.OperationID, Snapshot: raw}); err != nil {
				return err
			}
			if v.Command != fleet.RecordAcquisition {
				entry := q.InsertAssetLifecycleEventParams{OrgID: org, AssetID: v.AssetID, Revision: next.Revision, OperationID: v.OperationID, ActorID: actor.AccountID, Command: string(v.Command), FromState: string(current.State), ToState: string(to), Reason: v.Reason}
				if v.Command == fleet.Dispose {
					entry.DisposalKind = string(v.DisposalKind)
					entry.DisposedAt, entry.DisposedSubmicro = meterTime(v.At)
				}
				if err := queries.InsertAssetLifecycleEvent(ctx, entry); err != nil {
					return err
				}
			}
			if err := queries.InsertAssetCommand(ctx, q.InsertAssetCommandParams{OrgID: org, OperationID: v.OperationID, ActorID: actor.AccountID, AssetID: v.AssetID, Kind: string(v.Command), RequestHash: digest, ResultState: string(to), ResultRevision: next.Revision}); err != nil {
				return err
			}
			result = app.AssetResult{AssetID: v.AssetID, State: to, Revision: next.Revision}
			return nil
		})
		if err != nil {
			return err
		}
		if repeated {
			return app.ErrUnavailable
		}
		return s.authorize(ctx, tx, scope, actor, member, v.AssetID, app.FleetManage)
	})
	if err != nil {
		return app.AssetResult{}, false, storeError(ctx, err)
	}
	return result, replayed, nil
}

// ListAssetLifecycle reads the immutable history of one visible asset.
func (s *AssetStore) ListAssetLifecycle(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member, asset string, filter app.LifecycleFilter) (app.LifecyclePage, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.LifecyclePage{}, err
	}
	if !validID(asset) {
		return app.LifecyclePage{}, fleet.ErrInvalid
	}
	if err := filter.Validate(); err != nil {
		return app.LifecyclePage{}, err
	}
	if s == nil || s.Pool == nil || s.Authority == nil {
		return app.LifecyclePage{}, app.ErrUnavailable
	}
	page := app.LifecyclePage{Entries: []app.LifecycleEntry{}}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, asset, app.FleetRead); err != nil {
			return err
		}
		queries := q.New(tx)
		org := scope.OrgID().String()
		if _, err := queries.AssetLifecycleTarget(ctx, q.AssetLifecycleTargetParams{OrgID: org, ID: asset}); err != nil {
			return err
		}
		rows, err := queries.ListAssetLifecycleEvents(ctx, q.ListAssetLifecycleEventsParams{OrgID: org, AssetID: asset, AfterRevision: filter.AfterRevision, RowLimit: int32(filter.Limit + 1)})
		if err != nil {
			return err
		}
		more := len(rows) > filter.Limit
		if more {
			rows = rows[:filter.Limit]
		}
		for _, row := range rows {
			recorded, err := exactMeterTime(row.RecordedAt, 0)
			if err != nil {
				return err
			}
			entry := app.LifecycleEntry{Revision: row.Revision, Command: fleet.LifecycleCommand(row.Command), From: fleet.State(row.FromState), To: fleet.State(row.ToState), Reason: row.Reason, DisposalKind: fleet.DisposalKind(row.DisposalKind), RecordedAt: recorded}
			if row.DisposedAt.Valid {
				at, err := exactMeterTime(row.DisposedAt, row.DisposedSubmicro)
				if err != nil {
					return err
				}
				entry.DisposedAt = &at
			}
			page.Entries = append(page.Entries, entry)
		}
		if more {
			page.NextRevision = rows[len(rows)-1].Revision
		}
		return s.authorize(ctx, tx, scope, actor, member, asset, app.FleetRead)
	})
	if err != nil {
		return app.LifecyclePage{}, storeError(ctx, err)
	}
	return page, nil
}

var _ app.MeterAssetLifecycle = (*MeterStore)(nil)

// MeterAssetDisposalTx takes the organization root lock, then the asset row
// lock, in the borrowed intake transaction (the same order as every asset
// command) and returns the exact current disposal instant, if any.
func (s *MeterStore) MeterAssetDisposalTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, asset string) (*time.Time, error) {
	db = syncapp.Borrow(db)
	if s == nil || db == nil || database.ValidateOrgDBTX(ctx, db, scope) != nil || !validID(asset) {
		return nil, app.ErrUnavailable
	}
	queries := q.New(db)
	org := scope.OrgID().String()
	if _, err := queries.AssetRootLock(ctx, org); err != nil {
		return nil, meterError(ctx, err)
	}
	row, err := queries.MeterAssetDisposal(ctx, q.MeterAssetDisposalParams{OrgID: org, ID: asset})
	if err != nil {
		return nil, meterError(ctx, err)
	}
	if !row.DisposedAt.Valid {
		return nil, nil
	}
	at, err := exactMeterTime(row.DisposedAt, row.DisposedSubmicro)
	if err != nil {
		return nil, err
	}
	return &at, nil
}
