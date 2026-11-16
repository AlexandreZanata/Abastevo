package adapters

import (
	"context"
	"errors"
	"slices"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/fleet"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ app.ReservationRepository = (*AssetStore)(nil)
var _ app.ReservationOutcomes = (*AssetStore)(nil)
var _ app.StartAvailabilityPort = (*AssetStore)(nil)

// The fleet.md §9 reservation and block facts are the exact audit actions;
// expired and missed are the time-driven outcomes (EDR-193).
const (
	reservationRequestedAction = "fleet.reservation.requested"
	reservationConfirmedAction = "fleet.reservation.confirmed"
	reservationCancelledAction = "fleet.reservation.cancelled"
	reservationExpiredAction   = "fleet.reservation.expired"
	reservationMissedAction    = "fleet.reservation.missed"
	blockCreatedAction         = "fleet.block.created"
	blockRemovedAction         = "fleet.block.removed"
)

func reservationAuditDefinitions() []audit.Definition {
	out := []audit.Definition{}
	for _, action := range []string{reservationRequestedAction, reservationConfirmedAction, reservationCancelledAction, reservationExpiredAction, reservationMissedAction, blockCreatedAction, blockRemovedAction} {
		out = append(out, audit.Definition{Action: action, TargetType: "asset"})
	}
	return out
}

// NoActiveUse is the explicit "no active use source" adapter (EDR-194): no
// trip can exist before EMP-TRIP-002, which replaces it with its source.
type NoActiveUse struct{}

var _ app.ActiveUsePort = NoActiveUse{}

func (NoActiveUse) ActiveUseDBTX(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, asset string, at time.Time) (app.UseAnswer, error) {
	if syncapp.Borrow(db) == nil || scope.Validate() != nil || !validID(asset) || at.IsZero() {
		return app.UseAnswer{}, app.ErrUnavailable
	}
	return app.UseAnswer{}, nil
}

// NoMaintenanceHolds is the explicit "no maintenance-hold source" adapter
// (EDR-194) until the maintenance epic delivers holds.
type NoMaintenanceHolds struct{}

var _ app.MaintenanceHoldPort = NoMaintenanceHolds{}

func (NoMaintenanceHolds) MaintenanceHoldsDBTX(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, asset string, at time.Time) (app.HoldAnswer, error) {
	if syncapp.Borrow(db) == nil || scope.Validate() != nil || !validID(asset) || at.IsZero() {
		return app.HoldAnswer{}, app.ErrUnavailable
	}
	return app.HoldAnswer{}, nil
}

// NoChecklistBlocks is the explicit "no checklist-block source" adapter
// (EDR-285); the answer reports the missing source CHECKLIST_RUN.
type NoChecklistBlocks struct{}

var _ app.ChecklistBlockPort = NoChecklistBlocks{}

func (NoChecklistBlocks) ChecklistBlocksDBTX(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, asset string, at time.Time) (app.ChecklistBlockAnswer, error) {
	if syncapp.Borrow(db) == nil || scope.Validate() != nil || !validID(asset) || at.IsZero() {
		return app.ChecklistBlockAnswer{}, app.ErrUnavailable
	}
	return app.ChecklistBlockAnswer{}, nil
}

// reservationError maps the B-BR-EMP-295 exclusion refusals (a backstop
// behind the locked pre-check, which reports the window) and the ended
// refusals before the shared store mapping.
func reservationError(ctx context.Context, err error) error {
	var pe *pgconn.PgError
	if ctx.Err() == nil && errors.As(err, &pe) && pe.Code == "23P01" && (pe.ConstraintName == "org_asset_reservations_confirmed_no_overlap" || pe.ConstraintName == "org_asset_reservations_block_overlap") {
		return app.ErrReservationConflict
	}
	for _, known := range []error{app.ErrReservationConflict, app.ErrNotDue, fleet.ErrReservationEnded, fleet.ErrBlockEnded} {
		if errors.Is(err, known) {
			return err
		}
	}
	return storeError(ctx, err)
}

func txNow(ctx context.Context, queries *q.Queries) (time.Time, error) {
	stamp, err := queries.ReservationTxTime(ctx)
	if err != nil {
		return time.Time{}, err
	}
	at, err := exactMeterTime(stamp, 0)
	if err != nil || at.IsZero() {
		return time.Time{}, app.ErrUnavailable
	}
	return at, nil
}

// reservationCommand is one write of EMP-FLEET-007 bound to its receipt.
type reservationCommand struct {
	asset, operation, kind, action, digest string
	// effect returns the target identifier and the result state ("" for
	// blocks) at the transaction instant.
	effect func(pgx.Tx, *q.Queries, time.Time) (string, string, error)
}

// reservationTx runs one command under the organization root lock: authority,
// receipt replay, exactly one audit event, the effect, the receipt and
// authority again, all in one transaction (the FLEET-006 pattern).
func (s *AssetStore) reservationTx(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member string, c reservationCommand) (string, string, bool, error) {
	if s == nil || s.Pool == nil || s.Authority == nil || s.Audit == nil || c.digest == "" {
		return "", "", false, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	var target, state string
	replayed := false
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, c.asset, app.FleetReserve); err != nil {
			return err
		}
		queries := q.New(tx)
		if _, err := queries.AssetRootLock(ctx, org); err != nil {
			return err
		}
		prior, err := queries.ReservationCommandReceipt(ctx, q.ReservationCommandReceiptParams{OrgID: org, OperationID: c.operation})
		if err == nil {
			if prior.ActorID != actor.AccountID || prior.Kind != c.kind || prior.RequestHash != c.digest || prior.AssetID != c.asset {
				return app.ErrOperationReused
			}
			target, state, replayed = prior.TargetID, prior.ResultState, true
			return s.authorize(ctx, tx, scope, actor, member, c.asset, app.FleetReserve)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		event := audit.Event{OperationID: c.operation, ActorID: actor.AccountID, Action: c.action, TargetType: "asset", TargetID: c.asset, RequestHash: c.digest}
		repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			queries := q.New(tx)
			if _, err := queries.AssetLifecycleTarget(ctx, q.AssetLifecycleTargetParams{OrgID: org, ID: c.asset}); err != nil {
				return err
			}
			now, err := txNow(ctx, queries)
			if err != nil {
				return err
			}
			if target, state, err = c.effect(tx, queries, now); err != nil {
				return err
			}
			return queries.InsertReservationCommand(ctx, q.InsertReservationCommandParams{OrgID: org, OperationID: c.operation, ActorID: actor.AccountID, AssetID: c.asset, Kind: c.kind, RequestHash: c.digest, TargetID: target, ResultState: state})
		})
		if err != nil {
			return err
		}
		if repeated {
			return app.ErrUnavailable
		}
		return s.authorize(ctx, tx, scope, actor, member, c.asset, app.FleetReserve)
	})
	if err != nil {
		return "", "", false, reservationError(ctx, err)
	}
	return target, state, replayed, nil
}

func reservationFacts(state string, from, to pgtype.Timestamptz, tolerance int32, confirmed bool, ended pgtype.Timestamptz) (fleet.Reservation, error) {
	stored, err := fleet.ParseReservationState(state)
	if err != nil {
		return fleet.Reservation{}, app.ErrUnavailable
	}
	start, err := allocationTime(from)
	if err != nil {
		return fleet.Reservation{}, err
	}
	end, err := allocationTime(to)
	if err != nil {
		return fleet.Reservation{}, err
	}
	window, err := fleet.NewReservationWindow(start, end)
	if err != nil {
		return fleet.Reservation{}, app.ErrUnavailable
	}
	r := fleet.Reservation{State: stored, Window: window, Tolerance: time.Duration(tolerance) * time.Second, Confirmed: confirmed}
	if stored == fleet.ReservationCancelled {
		at, err := exactMeterTime(ended, 0)
		if err != nil {
			return fleet.Reservation{}, err
		}
		r.EndedAt = &at
	}
	if r.Validate() != nil {
		return fleet.Reservation{}, app.ErrUnavailable
	}
	return r, nil
}

// settleTx applies one due time-driven outcome of a locked reservation with
// its own audit event (the requesting account is the actor: the audit log
// has no system actor, EDR-193). A reservation not due answers ErrNotDue;
// one already settled or ended answers its state and changes nothing.
func (s *AssetStore) settleTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, id string, expect fleet.ReservationCommand, now time.Time) (fleet.ReservationState, error) {
	queries := q.New(tx)
	org := scope.OrgID().String()
	row, err := queries.ReservationForUpdate(ctx, q.ReservationForUpdateParams{OrgID: org, ID: id})
	if err != nil {
		return "", err
	}
	r, err := reservationFacts(row.State, row.StartsAt, row.EndsAt, row.NoShowTolerance, row.Confirmed, row.EndedAt)
	if err != nil {
		return "", err
	}
	due, command, ok := r.DueAt()
	if !ok || expect != "" && command != expect {
		return r.State, nil
	}
	if now.Before(due) {
		return r.State, app.ErrNotDue
	}
	to, err := fleet.ReservationTransition(r.State, command)
	if err != nil || s.NewID == nil {
		return "", app.ErrUnavailable
	}
	operation, err := s.NewID()
	if err != nil || !validID(operation) || !validID(row.RequestedBy) {
		return "", app.ErrUnavailable
	}
	action := reservationExpiredAction
	if command == fleet.MissReservation {
		action = reservationMissedAction
	}
	event := audit.Event{OperationID: operation, ActorID: row.RequestedBy, Action: action, TargetType: "asset", TargetID: row.AssetID,
		RequestHash: app.SettlementDigest(scope, id, row.Revision, command)}
	if _, err := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
		n, err := q.New(tx).SettleReservationRow(ctx, q.SettleReservationRowParams{State: string(to), OrgID: org, ID: id, FromState: string(r.State), Revision: row.Revision})
		if err != nil {
			return err
		}
		if n != 1 {
			return app.ErrConflict
		}
		return nil
	}); err != nil {
		return "", err
	}
	return to, nil
}

// settleDueTx settles, in identifier order, every due outcome of the asset
// before a command checks windows (a stopped job never leaves a stale
// CONFIRMED window in the exclusion constraint, B-BR-EMP-296).
func (s *AssetStore) settleDueTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, asset string, now time.Time) error {
	ids, err := q.New(tx).DueAssetReservations(ctx, q.DueAssetReservationsParams{OrgID: scope.OrgID().String(), AssetID: asset, At: timestamp(now)})
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := s.settleTx(ctx, tx, scope, id, "", now); err != nil {
			return err
		}
	}
	return nil
}

// windowConflict reports the earliest CONFIRMED window or block overlapping
// the window, as a ReservationConflict that carries only that window.
func windowConflict(ctx context.Context, queries *q.Queries, org, asset, id string, window fleet.ReservationWindow) error {
	row, err := queries.ConfirmedConflict(ctx, q.ConfirmedConflictParams{OrgID: org, AssetID: asset, ID: id, StartsAt: timestamp(window.From), EndsAt: timestamp(window.To)})
	if err == nil {
		from, ferr := allocationTime(row.StartsAt)
		to, terr := allocationTime(row.EndsAt)
		if ferr != nil || terr != nil {
			return app.ErrUnavailable
		}
		return app.NewReservationConflict(from, &to)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	block, err := queries.BlockConflict(ctx, q.BlockConflictParams{OrgID: org, AssetID: asset, StartsAt: timestamp(window.From), EndsAt: timestamp(window.To)})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	period, err := assignmentPeriod(block.ValidFrom, block.ValidTo)
	if err != nil {
		return err
	}
	return app.NewReservationConflict(period.From, period.To)
}

// RequestReservation creates one reservation REQUESTED, or CONFIRMED when the
// intent confirms, after settling due outcomes and checking windows.
func (s *AssetStore) RequestReservation(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member, id string, command app.ReservationRequest) (app.ReservationResult, bool, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.ReservationResult{}, false, err
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.ReservationResult{}, false, err
	}
	if !validID(id) {
		return app.ReservationResult{}, false, fleet.ErrInvalid
	}
	org := scope.OrgID().String()
	target, state, replayed, err := s.reservationTx(ctx, scope, actor, member, reservationCommand{asset: v.AssetID, operation: v.OperationID, kind: "REQUEST", action: reservationRequestedAction, digest: command.Digest(scope), effect: func(tx pgx.Tx, queries *q.Queries, now time.Time) (string, string, error) {
		window := fleet.ReservationWindow{From: v.Start, To: v.End}
		if err := fleet.ValidNewReservation(window, fleet.DefaultNoShowTolerance, now); err != nil {
			return "", "", err
		}
		if err := s.settleDueTx(ctx, tx, scope, v.AssetID, now); err != nil {
			return "", "", err
		}
		// UC-FLEET-06: a request is also checked against confirmed windows
		// and blocks; REQUESTED windows may overlap each other.
		if err := windowConflict(ctx, queries, org, v.AssetID, id, window); err != nil {
			return "", "", err
		}
		r := fleet.Reservation{State: fleet.ReservationRequested, Window: window, Tolerance: fleet.DefaultNoShowTolerance}
		params := q.InsertReservationParams{OrgID: org, ID: id, AssetID: v.AssetID, EmployeeID: v.EmployeeID, Purpose: v.Purpose, StartsAt: timestamp(window.From), EndsAt: timestamp(window.To),
			NoShowTolerance: int32(fleet.DefaultNoShowTolerance / time.Second), State: string(fleet.ReservationRequested), OperationID: v.OperationID, RequestedBy: actor.AccountID}
		if v.Confirm {
			r.State, r.Confirmed = fleet.ReservationConfirmed, true
			params.State, params.ConfirmOperationID, params.ConfirmedBy = string(fleet.ReservationConfirmed), v.OperationID, actor.AccountID
		}
		if err := queries.InsertReservation(ctx, params); err != nil {
			return "", "", err
		}
		if err := enqueueOutcome(ctx, tx, scope, id, r); err != nil {
			return "", "", err
		}
		return id, params.State, nil
	}})
	if err != nil {
		return app.ReservationResult{}, false, err
	}
	return app.ReservationResult{AssetID: v.AssetID, ReservationID: target, State: fleet.ReservationState(state)}, replayed, nil
}

// ActOnReservation confirms or cancels one reservation of the asset that is
// REQUESTED (confirm) or REQUESTED/CONFIRMED (cancel) at the transaction
// instant, after settling due outcomes; anything else is ErrReservationEnded.
func (s *AssetStore) ActOnReservation(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member string, command app.ReservationAct) (app.ReservationResult, bool, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.ReservationResult{}, false, err
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.ReservationResult{}, false, err
	}
	org := scope.OrgID().String()
	kind, action := "CONFIRM", reservationConfirmedAction
	if command.Command() == fleet.CancelReservation {
		kind, action = "CANCEL", reservationCancelledAction
	}
	target, state, replayed, err := s.reservationTx(ctx, scope, actor, member, reservationCommand{asset: v.AssetID, operation: v.OperationID, kind: kind, action: action, digest: command.Digest(scope), effect: func(tx pgx.Tx, queries *q.Queries, now time.Time) (string, string, error) {
		if err := s.settleDueTx(ctx, tx, scope, v.AssetID, now); err != nil {
			return "", "", err
		}
		row, err := queries.ReservationForUpdate(ctx, q.ReservationForUpdateParams{OrgID: org, ID: v.ReservationID})
		if err != nil {
			return "", "", err
		}
		if row.AssetID != v.AssetID {
			return "", "", app.ErrNotFound
		}
		r, err := reservationFacts(row.State, row.StartsAt, row.EndsAt, row.NoShowTolerance, row.Confirmed, row.EndedAt)
		if err != nil {
			return "", "", err
		}
		to, err := fleet.ReservationTransition(r.StateAt(now), command.Command())
		if err != nil {
			return "", "", fleet.ErrReservationEnded
		}
		if command.Command() == fleet.ConfirmReservation {
			if err := windowConflict(ctx, queries, org, v.AssetID, v.ReservationID, r.Window); err != nil {
				return "", "", err
			}
			n, err := queries.ConfirmReservationRow(ctx, q.ConfirmReservationRowParams{ConfirmOperationID: v.OperationID, ConfirmedBy: actor.AccountID, OrgID: org, ID: v.ReservationID, Revision: row.Revision})
			if err != nil {
				return "", "", err
			}
			if n != 1 {
				return "", "", app.ErrConflict
			}
			r.State, r.Confirmed = fleet.ReservationConfirmed, true
			if err := enqueueOutcome(ctx, tx, scope, v.ReservationID, r); err != nil {
				return "", "", err
			}
			return v.ReservationID, string(to), nil
		}
		// B-BR-EMP-297: a reason is required unless the requester cancels.
		if v.Reason == "" && row.RequestedBy != actor.AccountID {
			return "", "", fleet.ErrInvalid
		}
		n, err := queries.CancelReservationRow(ctx, q.CancelReservationRowParams{EndOperationID: v.OperationID, EndedBy: actor.AccountID, EndReason: v.Reason, OrgID: org, ID: v.ReservationID, Revision: row.Revision})
		if err != nil {
			return "", "", err
		}
		if n != 1 {
			return "", "", app.ErrConflict
		}
		return v.ReservationID, string(to), nil
	}})
	if err != nil {
		return app.ReservationResult{}, false, err
	}
	return app.ReservationResult{AssetID: v.AssetID, ReservationID: target, State: fleet.ReservationState(state)}, replayed, nil
}

// CreateBlock records one block from the given start (or the transaction
// instant); it changes no reservation and no operation in progress.
func (s *AssetStore) CreateBlock(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member, id string, command app.BlockCreate) (app.BlockResult, bool, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.BlockResult{}, false, err
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.BlockResult{}, false, err
	}
	if !validID(id) {
		return app.BlockResult{}, false, fleet.ErrInvalid
	}
	org := scope.OrgID().String()
	target, _, replayed, err := s.reservationTx(ctx, scope, actor, member, reservationCommand{asset: v.AssetID, operation: v.OperationID, kind: "BLOCK", action: blockCreatedAction, digest: command.Digest(scope), effect: func(_ pgx.Tx, queries *q.Queries, now time.Time) (string, string, error) {
		start := now
		if v.Start != nil {
			if !v.Start.After(now) {
				return "", "", fleet.ErrInvalid
			}
			start = *v.Start
		}
		period, err := fleet.NewAssignmentPeriod(start, v.End)
		if err != nil {
			return "", "", err
		}
		if err := fleet.ValidNewBlock(v.Reason, period, now); err != nil {
			return "", "", err
		}
		return id, "", queries.InsertBlock(ctx, q.InsertBlockParams{OrgID: org, ID: id, AssetID: v.AssetID, ReasonKind: string(v.Reason), Note: v.Note, ValidFrom: timestamp(period.From), ValidTo: optionalTimestamp(period.To), OperationID: v.OperationID, BlockedBy: actor.AccountID})
	}})
	if err != nil {
		return app.BlockResult{}, false, err
	}
	return app.BlockResult{AssetID: v.AssetID, BlockID: target}, replayed, nil
}

// RemoveBlock ends one block that is not removed or ended at the transaction
// instant (at its start when it has not started), with its reason.
func (s *AssetStore) RemoveBlock(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member string, command app.BlockRemove) (app.BlockResult, bool, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.BlockResult{}, false, err
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.BlockResult{}, false, err
	}
	org := scope.OrgID().String()
	target, _, replayed, err := s.reservationTx(ctx, scope, actor, member, reservationCommand{asset: v.AssetID, operation: v.OperationID, kind: "UNBLOCK", action: blockRemovedAction, digest: command.Digest(scope), effect: func(_ pgx.Tx, queries *q.Queries, now time.Time) (string, string, error) {
		row, err := queries.BlockForRemoval(ctx, q.BlockForRemovalParams{OrgID: org, AssetID: v.AssetID, ID: v.BlockID})
		if err != nil {
			return "", "", err
		}
		if row.Removed {
			return "", "", fleet.ErrBlockEnded
		}
		period, err := assignmentPeriod(row.ValidFrom, row.ValidTo)
		if err != nil {
			return "", "", err
		}
		ended, err := fleet.RemoveBlockAt(period, now)
		if err != nil {
			return "", "", err
		}
		n, err := queries.RemoveBlockRow(ctx, q.RemoveBlockRowParams{ValidTo: timestamp(*ended.To), RemoveOperationID: v.OperationID, RemovedBy: actor.AccountID, RemovalReason: v.Reason, OrgID: org, ID: v.BlockID})
		if err != nil {
			return "", "", err
		}
		if n != 1 {
			return "", "", app.ErrConflict
		}
		return v.BlockID, "", nil
	}})
	if err != nil {
		return app.BlockResult{}, false, err
	}
	return app.BlockResult{AssetID: v.AssetID, BlockID: target}, replayed, nil
}

// SettleReservation is the job's entry (EDR-193): it settles the expected
// due outcome of one reservation; a repeat or an outcome already reached
// changes nothing. It acts for no session and checks no member authority.
func (s *AssetStore) SettleReservation(ctx context.Context, scope kernel.OrgScope, id string, expect fleet.ReservationCommand) (fleet.ReservationState, error) {
	if s == nil || s.Pool == nil || s.Audit == nil || s.NewID == nil {
		return "", app.ErrUnavailable
	}
	if scope.Validate() != nil || !validID(id) || expect != fleet.ExpireReservation && expect != fleet.MissReservation {
		return "", fleet.ErrInvalid
	}
	var state fleet.ReservationState
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		now, err := txNow(ctx, q.New(tx))
		if err != nil {
			return err
		}
		state, err = s.settleTx(ctx, tx, scope, id, expect, now)
		return err
	})
	if err != nil {
		return "", reservationError(ctx, err)
	}
	return state, nil
}

func optionalExact(v pgtype.Timestamptz) (*time.Time, error) {
	if !v.Valid {
		return nil, nil
	}
	at, err := exactMeterTime(v, 0)
	if err != nil {
		return nil, err
	}
	return &at, nil
}

// reservationView projects one stored row with its effective state at now;
// a time-driven outcome reports the instant at which it happened.
func reservationView(row q.ListAssetReservationsRow, now time.Time) (app.ReservationView, error) {
	confirmedAt, err := optionalExact(row.ConfirmedAt)
	if err != nil {
		return app.ReservationView{}, err
	}
	r, err := reservationFacts(row.State, row.StartsAt, row.EndsAt, row.NoShowTolerance, confirmedAt != nil, row.EndedAt)
	if err != nil {
		return app.ReservationView{}, err
	}
	requested, err := exactMeterTime(row.RequestedAt, 0)
	if err != nil {
		return app.ReservationView{}, err
	}
	v := app.ReservationView{ID: row.ID, AssetID: row.AssetID, EmployeeID: row.EmployeeID, Purpose: row.Purpose, Window: r.Window, Tolerance: r.Tolerance, State: r.StateAt(now), RequestedAt: requested, ConfirmedAt: confirmedAt, EndCause: app.ReservationEndCause(row.EndCause), EndReason: row.EndReason}
	switch v.State {
	case fleet.ReservationExpired:
		v.EndCause, v.EndedAt = app.EndedByTime, &r.Window.From
	case fleet.ReservationNoShow:
		deadline := r.NoShowDeadline()
		v.EndCause, v.EndedAt = app.EndedByTime, &deadline
	case fleet.ReservationCancelled:
		v.EndedAt = r.EndedAt
	}
	return v, nil
}

// ListReservations reads one visible asset's reservations in start order
// with their effective states at one database instant (AC3 at read time).
func (s *AssetStore) ListReservations(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member, asset string, filter app.PeriodFilter) (app.ReservationPage, error) {
	var page app.ReservationPage
	err := s.assetRead(ctx, scope, actor, member, asset, filter, func(_ pgx.Tx, queries *q.Queries, org string, now time.Time) error {
		rows, err := queries.ListAssetReservations(ctx, q.ListAssetReservationsParams{OrgID: org, AssetID: asset, AfterFrom: optionalTimestamp(filter.AfterFrom), AfterID: filter.AfterID, RowLimit: int32(filter.Limit + 1)})
		if err != nil {
			return err
		}
		page = app.ReservationPage{Reservations: []app.ReservationView{}, At: now}
		for i, row := range rows {
			if i == filter.Limit {
				last := page.Reservations[i-1]
				page.NextFrom, page.NextID = &last.Window.From, last.ID
				break
			}
			v, err := reservationView(row, now)
			if err != nil {
				return err
			}
			page.Reservations = append(page.Reservations, v)
		}
		return nil
	})
	return page, err
}

// ListBlocks reads one visible asset's blocks in start order.
func (s *AssetStore) ListBlocks(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member, asset string, filter app.PeriodFilter) (app.BlockPage, error) {
	var page app.BlockPage
	err := s.assetRead(ctx, scope, actor, member, asset, filter, func(_ pgx.Tx, queries *q.Queries, org string, now time.Time) error {
		rows, err := queries.ListAssetBlocks(ctx, q.ListAssetBlocksParams{OrgID: org, AssetID: asset, AfterFrom: optionalTimestamp(filter.AfterFrom), AfterID: filter.AfterID, RowLimit: int32(filter.Limit + 1)})
		if err != nil {
			return err
		}
		page = app.BlockPage{Blocks: []app.BlockView{}, At: now}
		for i, row := range rows {
			if i == filter.Limit {
				last := page.Blocks[i-1]
				page.NextFrom, page.NextID = &last.Period.From, last.ID
				break
			}
			period, err := assignmentPeriod(row.ValidFrom, row.ValidTo)
			if err != nil {
				return err
			}
			recorded, err := exactMeterTime(row.RecordedAt, 0)
			if err != nil {
				return err
			}
			removed, err := optionalExact(row.RemovedAt)
			if err != nil {
				return err
			}
			reason, err := fleet.ParseBlockReason(row.ReasonKind)
			if err != nil {
				return app.ErrUnavailable
			}
			page.Blocks = append(page.Blocks, app.BlockView{ID: row.ID, AssetID: row.AssetID, Reason: reason, Note: row.Note, Period: period, RecordedAt: recorded, RemovedAt: removed, RemovalReason: row.RemovalReason})
		}
		return nil
	})
	return page, err
}

// assetRead runs one read between two authority checks at one database
// instant, after the asset is found.
func (s *AssetStore) assetRead(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member, asset string, filter app.PeriodFilter, read func(pgx.Tx, *q.Queries, string, time.Time) error) error {
	if err := validCall(scope, actor, member); err != nil {
		return err
	}
	if !validID(asset) || filter.Validate() != nil {
		return fleet.ErrInvalid
	}
	if s == nil || s.Pool == nil || s.Authority == nil {
		return app.ErrUnavailable
	}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, asset, app.FleetRead); err != nil {
			return err
		}
		queries := q.New(tx)
		org := scope.OrgID().String()
		if _, err := queries.AssetLifecycleTarget(ctx, q.AssetLifecycleTargetParams{OrgID: org, ID: asset}); err != nil {
			return err
		}
		now, err := meterDBNow(ctx, queries)
		if err != nil {
			return err
		}
		if err := read(tx, queries, org, now); err != nil {
			return err
		}
		return s.authorize(ctx, tx, scope, actor, member, asset, app.FleetRead)
	})
	return reservationError(ctx, err)
}

// availabilityFacts collects the stored facts of the asset at instant at and
// the sources that answered "no source" (B-BR-EMP-299).
func (s *AssetStore) availabilityFacts(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, asset string, at time.Time) (fleet.AvailabilityFacts, []app.AvailabilitySource, error) {
	if meterIntakeAbsent(s.ActiveUse) || meterIntakeAbsent(s.MaintenanceHolds) || meterIntakeAbsent(s.ChecklistBlocks) {
		return fleet.AvailabilityFacts{}, nil, app.ErrUnavailable
	}
	queries := q.New(db)
	org := scope.OrgID().String()
	stored, err := queries.AttachmentAssetKind(ctx, q.AttachmentAssetKindParams{OrgID: org, ID: asset})
	if err != nil {
		return fleet.AvailabilityFacts{}, nil, err
	}
	lifecycle, err := fleet.ParseState(stored.State)
	if err != nil {
		return fleet.AvailabilityFacts{}, nil, app.ErrUnavailable
	}
	facts := fleet.AvailabilityFacts{Lifecycle: lifecycle}
	var missing []app.AvailabilitySource
	uses, err := s.ActiveUse.ActiveUseDBTX(ctx, db, scope, asset, at)
	if err != nil {
		return fleet.AvailabilityFacts{}, nil, app.ErrUnavailable
	}
	if !uses.Sourced {
		missing = append(missing, app.ActiveUseSource)
	}
	holds, err := s.MaintenanceHolds.MaintenanceHoldsDBTX(ctx, db, scope, asset, at)
	if err != nil {
		return fleet.AvailabilityFacts{}, nil, app.ErrUnavailable
	}
	if !holds.Sourced {
		missing = append(missing, app.MaintenanceHoldSource)
	}
	facts.Uses, facts.Holds = append([]fleet.UseFact(nil), uses.Uses...), append([]fleet.HoldFact(nil), holds.Holds...)
	// EMP-FLEET-011 (§6.2 row 2): an attached implement is in use through
	// the trips of the powered asset it is attached to at the instant.
	if fleet.Kind(stored.Kind) == fleet.Implement {
		powered, attached, err := s.PoweredAssetDBTX(ctx, db, scope, asset, at)
		if err != nil {
			return fleet.AvailabilityFacts{}, nil, err
		}
		if attached {
			through, err := s.ActiveUse.ActiveUseDBTX(ctx, db, scope, powered, at)
			if err != nil {
				return fleet.AvailabilityFacts{}, nil, app.ErrUnavailable
			}
			if !through.Sourced && uses.Sourced {
				missing = append(missing, app.ActiveUseSource)
			}
			for _, use := range through.Uses {
				use.Via = powered
				facts.Uses = append(facts.Uses, use)
			}
		}
	}
	blocks, err := queries.AvailabilityBlocks(ctx, q.AvailabilityBlocksParams{OrgID: org, AssetID: asset, At: timestamp(at)})
	if err != nil {
		return fleet.AvailabilityFacts{}, nil, err
	}
	for _, row := range blocks {
		period, err := assignmentPeriod(row.ValidFrom, row.ValidTo)
		if err != nil {
			return fleet.AvailabilityFacts{}, nil, err
		}
		facts.Blocks = append(facts.Blocks, fleet.BlockFact{ID: row.ID, Reason: fleet.BlockReason(row.ReasonKind), Period: period})
	}
	checklists, err := s.ChecklistBlocks.ChecklistBlocksDBTX(ctx, db, scope, asset, at)
	if err != nil {
		return fleet.AvailabilityFacts{}, nil, app.ErrUnavailable
	}
	if !checklists.Sourced {
		missing = append(missing, app.ChecklistBlockSource)
	}
	facts.Checklists = append([]fleet.ChecklistBlockFact(nil), checklists.Blocks...)
	reservations, err := queries.AvailabilityReservations(ctx, q.AvailabilityReservationsParams{OrgID: org, AssetID: asset, At: timestamp(at)})
	if err != nil {
		return fleet.AvailabilityFacts{}, nil, err
	}
	for _, row := range reservations {
		r, err := reservationFacts(row.State, row.StartsAt, row.EndsAt, row.NoShowTolerance, row.Confirmed, row.EndedAt)
		if err != nil {
			return fleet.AvailabilityFacts{}, nil, err
		}
		facts.Reservations = append(facts.Reservations, fleet.ReservationFact{ID: row.ID, EmployeeID: row.EmployeeID, Reservation: r})
	}
	slices.Sort(missing)
	return facts, missing, nil
}

// ReadAvailability derives the answer at the database instant between two
// authority checks; it never stores it (BR-FLEET-025).
func (s *AssetStore) ReadAvailability(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member, asset string) (app.AvailabilityView, error) {
	var view app.AvailabilityView
	err := s.assetRead(ctx, scope, actor, member, asset, app.PeriodFilter{Limit: 1}, func(tx pgx.Tx, _ *q.Queries, _ string, now time.Time) error {
		at, err := fleet.AllocationInstant(now)
		if err != nil {
			return app.ErrUnavailable
		}
		facts, missing, err := s.availabilityFacts(ctx, tx, scope, asset, at)
		if err != nil {
			return err
		}
		assessment, err := fleet.Assess(facts, at, at)
		if err != nil {
			return app.ErrUnavailable
		}
		view = app.AvailabilityView{AssetID: asset, At: at, Assessment: assessment, MissingSources: missing}
		return nil
	})
	return view, err
}

// StartDecisionDBTX answers the intake check of a start (B-BR-EMP-300) on
// the borrowed intake transaction: the asset's current lifecycle, the facts
// covering the declared instant and whether the membership's employee record
// is named by an ACTIVE assignment then. It takes no locks.
func (s *AssetStore) StartDecisionDBTX(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, asset, member string, at time.Time) (fleet.StartDecision, error) {
	if s == nil || !validID(asset) {
		return fleet.StartDecision{}, app.ErrUnavailable
	}
	queries, instant, err := portQueries(ctx, db, scope, member, at)
	if err != nil {
		return fleet.StartDecision{}, err
	}
	declared, err := allocationTime(instant)
	if err != nil {
		return fleet.StartDecision{}, err
	}
	now, err := meterDBNow(ctx, queries)
	if err != nil {
		return fleet.StartDecision{}, storeError(ctx, err)
	}
	db = syncapp.Borrow(db)
	facts, _, err := s.availabilityFacts(ctx, db, scope, asset, declared)
	if err != nil {
		return fleet.StartDecision{}, reservationError(ctx, err)
	}
	starter := fleet.Starter{}
	employee, err := queries.MemberEmployee(ctx, q.MemberEmployeeParams{OrgID: scope.OrgID().String(), MembershipID: member, At: instant})
	switch {
	case err == nil:
		starter.EmployeeID = employee
		if starter.Assigned, err = s.AssignedAtDBTX(ctx, db, scope, member, asset, declared); err != nil {
			return fleet.StartDecision{}, err
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return fleet.StartDecision{}, storeError(ctx, err)
	}
	decision, err := fleet.DecideStart(facts, starter, declared, now)
	if err != nil {
		return fleet.StartDecision{}, app.ErrUnavailable
	}
	return decision, nil
}
