package adapters

import (
	"context"
	"errors"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/fleet"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// EMP-METER-005 audit actions and stored kinds (B-BR-EMP-486, EDR-313).
const (
	meterReplacedAction = "meter.replaced"
	meterResetAction    = "meter.reset"
	meterRolloverAction = "meter.rollover.confirmed"
	meterBoundaryTarget = "meter"
	closingRemoval      = "REMOVAL"
	closingReset        = "RESET"
	closingRollover     = "ROLLOVER"
	causeRollover       = "ROLLOVER"
)

func optionalInt8(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

// boundaryResult rebuilds the boundary an operation recorded from its rows
// only, so a replay returns the first answer (EDR-313).
func boundaryResult(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, operation string, change app.MeterChange) (app.MeterBoundaryResult, error) {
	queries := q.New(db)
	row, err := queries.MeterBoundaryByOperation(ctx, q.MeterBoundaryByOperationParams{OrgID: scope.OrgID().String(), OperationID: operation})
	if err != nil {
		return app.MeterBoundaryResult{}, err
	}
	want := map[app.MeterChange]string{app.MeterReplace: closingRemoval, app.MeterReset: closingReset, app.MeterRollover: closingRollover}[change]
	at, err := exactMeterTime(row.ClosedAt, row.ClosedSubmicro)
	if err != nil || row.ClosingKind != want {
		return app.MeterBoundaryResult{}, app.ErrUnavailable
	}
	r := app.MeterBoundaryResult{Change: change, MeterID: row.MeterID, ClosedSegmentID: row.ClosedSegmentID, At: at, ClosingReadingID: row.ClosingReadingID, PreviousReadingID: row.PreviousReadingID,
		NewMeterID: row.NewMeterID, OpenedSegmentID: row.OpenedSegmentID, OpeningReadingID: row.OpeningReadingID, Open: row.OpeningValue, Segments: row.Segments}
	if row.ClosingValue.Valid {
		v := row.ClosingValue.Int64
		r.Close = &v
	}
	if row.ClosingImpliedUsage.Valid {
		v := row.ClosingImpliedUsage.Int64
		r.Implied = &v
	}
	states, err := queries.MeterStatesByOperation(ctx, q.MeterStatesByOperationParams{OrgID: scope.OrgID().String(), MeterID: row.MeterID, OperationID: operation})
	if err != nil {
		return app.MeterBoundaryResult{}, err
	}
	for _, s := range states {
		if s.Cause == causeReevaluation {
			r.Reevaluated = append(r.Reevaluated, app.MeterReadingState{ReadingID: s.ReadingID, State: meter.State(s.State), Flags: flagsOf(s.Flags), Revision: int64(s.Revision)})
		}
	}
	return r, nil
}

// rolloverBoundary attaches the boundary of a CONFIRM_ROLLOVER decision.
func rolloverBoundary(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, v app.MeterDecisionInput, result *app.MeterDecisionResult) error {
	if v.Decision != app.MeterConfirmRollover {
		return nil
	}
	b, err := boundaryResult(ctx, tx, scope, v.OperationID, app.MeterRollover)
	if err != nil {
		return err
	}
	result.Boundary = &b
	return nil
}

// segmentEntries reads the open segment's members (in the segment
// B-BR-EMP-481 derives).
func segmentEntries(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, view app.MeterView) ([]meter.Entry, error) {
	members, err := meterSeries(ctx, db, scope, view.Reference.ID(), view.Segment.ID())
	if err != nil {
		return nil, err
	}
	return seriesEntries(members), nil
}

// openedView is the view of a meter whose open segment is segment.
func openedView(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, asset, meterID, segment string) (app.MeterView, error) {
	view, err := loadMeter(ctx, db, scope, asset, meterID, false)
	if err != nil {
		return app.MeterView{}, err
	}
	if view.Segment.ID() != segment {
		return app.MeterView{}, app.ErrUnavailable
	}
	return view, nil
}

// confirmRollover closes the open segment at the reading's instant with the
// modulus, opens the ROLLOVER segment on it, makes it VALID and re-evaluates
// the new segment (B-BR-EMP-486). newID is the new segment.
func (s *MeterStore) confirmRollover(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, view app.MeterView, member, newID string, v app.MeterDecisionInput, target q.MeterReadingCurrentRow) error {
	entries, err := segmentEntries(ctx, tx, scope, view)
	if err != nil {
		return err
	}
	b, err := meter.PlanRollover(view.Reference.Kind(), view.Reference.Modulus(), entries, v.ReadingID)
	if err != nil {
		return meter.ErrInvalid
	}
	queries := q.New(tx)
	org := scope.OrgID().String()
	at, sub := meterTime(b.At)
	n, err := queries.CloseMeterSegment(ctx, q.CloseMeterSegmentParams{OrgID: org, MeterID: v.MeterID, ID: view.Segment.ID(), ClosedAt: at, ClosedSubmicro: sub, ClosingKind: closingRollover,
		ClosingValue: optionalInt8(b.Close), OperationID: v.OperationID, PreviousReadingID: b.Previous, ImpliedUsage: optionalInt8(&b.Implied)})
	if err != nil {
		return err
	}
	if n != 1 {
		return app.ErrConflict
	}
	if err := queries.InsertBoundarySegment(ctx, q.InsertBoundarySegmentParams{OrgID: org, ID: newID, MeterID: v.MeterID, Kind: closingRollover, OpenedAt: at, OpenedSubmicro: sub, OpeningReadingID: v.ReadingID, OperationID: v.OperationID}); err != nil {
		return err
	}
	if err := queries.InsertMeterReadingState(ctx, q.InsertMeterReadingStateParams{OrgID: org, MeterID: v.MeterID, ReadingID: v.ReadingID, Revision: target.Revision + 1, State: string(meter.Valid), Flags: []string{},
		Cause: causeRollover, OperationID: v.OperationID, ActorMembershipID: member, Reason: v.Reason}); err != nil {
		return err
	}
	if s.Plausibility == nil {
		return nil
	}
	opened, err := openedView(ctx, tx, scope, v.AssetID, v.MeterID, newID)
	if err != nil {
		return err
	}
	return reevaluateSegment(ctx, tx, scope, *s.Plausibility, opened, v.OperationID)
}

// ChangeMeter replaces or resets the meter in service (B-BR-EMP-484/485)
// under the root, asset and meter locks with one audit event.
func (s *MeterStore) ChangeMeter(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member string, ids app.MeterChangeIDs, command app.MeterChangeCommand) (app.MeterBoundaryResult, bool, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.MeterBoundaryResult{}, false, err
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.MeterBoundaryResult{}, false, err
	}
	for _, id := range []string{ids.Segment, ids.Meter, ids.OpeningReading, ids.RemovalReading} {
		if !validID(id) {
			return app.MeterBoundaryResult{}, false, meter.ErrInvalid
		}
	}
	if !s.ready() || s.Audit == nil {
		return app.MeterBoundaryResult{}, false, app.ErrUnavailable
	}
	action := map[app.MeterChange]string{app.MeterReplace: meterReplacedAction, app.MeterReset: meterResetAction}[v.Change]
	digest := command.Digest(scope)
	bound := audit.Event{OperationID: v.OperationID, ActorID: actor.AccountID, Action: action, TargetType: meterBoundaryTarget, TargetID: v.MeterID, RequestHash: digest}
	source := app.MeterSource{Web: &actor}
	org := scope.OrgID().String()
	var result app.MeterBoundaryResult
	replay := false
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.check(ctx, tx, scope, source, member, v.AssetID, app.MeterCorrectAction); err != nil {
			return err
		}
		queries := q.New(tx)
		if _, err := queries.AssetRootLock(ctx, org); err != nil {
			return err
		}
		if _, err := queries.LockMeterAsset(ctx, q.LockMeterAssetParams{OrgID: org, AssetID: v.AssetID}); err != nil {
			return err
		}
		if _, err := queries.MeterBoundaryByOperation(ctx, q.MeterBoundaryByOperationParams{OrgID: org, OperationID: v.OperationID}); err == nil {
			repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, bound, func(context.Context, pgx.Tx) error { return app.ErrUnavailable })
			if err != nil {
				return err
			}
			if !repeated {
				return app.ErrUnavailable
			}
			if result, err = boundaryResult(ctx, tx, scope, v.OperationID, v.Change); err != nil {
				return err
			}
			replay = true
			return s.check(ctx, tx, scope, source, member, v.AssetID, app.MeterCorrectAction)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err := queries.LockMeter(ctx, q.LockMeterParams{OrgID: org, AssetID: v.AssetID, ID: v.MeterID}); err != nil {
			return err
		}
		view, err := loadMeter(ctx, tx, scope, v.AssetID, v.MeterID, false)
		if err != nil {
			return err
		}
		chain, err := queries.MeterSegmentChain(ctx, q.MeterSegmentChainParams{OrgID: org, MeterID: v.MeterID})
		if err != nil {
			return err
		}
		if int64(len(chain)) != v.ExpectedSegments {
			return app.ErrConflict
		}
		now, err := meterDBNow(ctx, queries)
		if err != nil {
			return err
		}
		if v.Kind() != view.Reference.Kind() || v.Declared.OccurredAt.After(now) {
			return meter.ErrInvalid
		}
		entries, err := segmentEntries(ctx, tx, scope, view)
		if err != nil {
			return err
		}
		var plan meter.Boundary
		if v.Change == app.MeterReplace {
			var final *int64
			if v.Final != nil {
				n := v.Final.Count()
				final = &n
			}
			plan, err = meter.PlanReplacement(view.Reference.Kind(), view.Reference.Modulus(), entries, v.Declared.OccurredAt, final, v.Modulus, v.Opening.Count())
		} else {
			plan, err = meter.PlanReset(view.Reference.Kind(), view.Reference.Modulus(), entries, v.Declared.OccurredAt, v.Before.Count(), v.After.Count())
		}
		if err != nil {
			return meter.ErrInvalid
		}
		repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, bound, func(ctx context.Context, tx pgx.Tx) error {
			if err := s.applyChange(ctx, tx, scope, view, source, member, ids, v, digest, plan); err != nil {
				return err
			}
			var err error
			result, err = boundaryResult(ctx, tx, scope, v.OperationID, v.Change)
			return err
		})
		if err != nil {
			return err
		}
		if repeated {
			return app.ErrUnavailable
		}
		return s.check(ctx, tx, scope, source, member, v.AssetID, app.MeterCorrectAction)
	})
	if err != nil {
		if errors.Is(err, audit.ErrReused) {
			return app.MeterBoundaryResult{}, false, app.ErrOperationReused
		}
		return app.MeterBoundaryResult{}, false, meterError(ctx, err)
	}
	return result.Clone(), replay, nil
}

// applyChange writes a planned replacement or reset in the command's
// transaction: the closing reading, the closure, the removal and the new
// meter; or the RESET segment with its opening reading and the
// re-evaluation of what now counts in it.
func (s *MeterStore) applyChange(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, view app.MeterView, source app.MeterSource, member string, ids app.MeterChangeIDs, v app.MeterChangeInput, digest string, plan meter.Boundary) error {
	queries := q.New(tx)
	org := scope.OrgID().String()
	at, sub := meterTime(plan.At)
	closure := q.CloseMeterSegmentParams{OrgID: org, MeterID: v.MeterID, ID: view.Segment.ID(), ClosedAt: at, ClosedSubmicro: sub, ClosingValue: optionalInt8(plan.Close), OperationID: v.OperationID, PreviousReadingID: plan.Previous}
	if plan.Close != nil {
		closure.ImpliedUsage = optionalInt8(&plan.Implied)
	}
	if v.Change == app.MeterReset {
		closure.ClosingKind = closingReset
	} else {
		closure.ClosingKind = closingRemoval
		if v.Final != nil {
			removal := app.MeterReadingInput{OperationID: v.OperationID, AssetID: v.AssetID, MeterID: v.MeterID, SegmentID: view.Segment.ID(), ReadingID: ids.RemovalReading, Value: *v.Final, Declared: v.Declared, Origin: meter.Removal, Method: meter.Typed}
			if _, err := insertObservation(ctx, tx, scope, view, source, member, removal, digest, nil, false); err != nil {
				return err
			}
			closure.ClosingReadingID = ids.RemovalReading
		}
	}
	if n, err := queries.CloseMeterSegment(ctx, closure); err != nil {
		return err
	} else if n != 1 {
		return app.ErrConflict
	}
	meterID := v.MeterID
	opening := app.MeterReadingInput{OperationID: v.OperationID, AssetID: v.AssetID, SegmentID: ids.Segment, ReadingID: ids.OpeningReading, Declared: v.Declared, Method: meter.Typed}
	if v.Change == app.MeterReset {
		if err := queries.InsertBoundarySegment(ctx, q.InsertBoundarySegmentParams{OrgID: org, ID: ids.Segment, MeterID: meterID, Kind: closingReset, OpenedAt: at, OpenedSubmicro: sub, OpeningReadingID: ids.OpeningReading, OperationID: v.OperationID}); err != nil {
			return err
		}
		opening.Value, opening.Origin = v.After, meter.Standalone
	} else {
		if n, err := queries.RemoveMeter(ctx, q.RemoveMeterParams{OrgID: org, AssetID: v.AssetID, ID: v.MeterID, RemovedAt: at, RemovedSubmicro: sub, OperationID: v.OperationID}); err != nil {
			return err
		} else if n != 1 {
			return app.ErrConflict
		}
		meterID = ids.Meter
		modulus := int64(0)
		if v.Modulus != nil {
			modulus = *v.Modulus
		}
		if err := queries.InsertMeter(ctx, q.InsertMeterParams{OrgID: org, ID: meterID, AssetID: v.AssetID, Kind: string(view.Reference.Kind()), Label: v.Label, Modulus: modulus, InstalledAt: at, InstalledSubmicro: sub, OperationID: v.OperationID}); err != nil {
			return err
		}
		if err := queries.InsertMeterSegment(ctx, q.InsertMeterSegmentParams{OrgID: org, ID: ids.Segment, MeterID: meterID, OpenedAt: at, OpenedSubmicro: sub, OpeningReadingID: ids.OpeningReading}); err != nil {
			return err
		}
		opening.Value, opening.Origin = v.Opening, meter.Installation
	}
	opening.MeterID = meterID
	opened, err := openedView(ctx, tx, scope, v.AssetID, meterID, ids.Segment)
	if err != nil {
		return err
	}
	if _, err := insertObservation(ctx, tx, scope, opened, source, member, opening, digest, nil, false); err != nil {
		return err
	}
	if v.Change != app.MeterReset || s.Plausibility == nil {
		return nil
	}
	return reevaluateSegment(ctx, tx, scope, *s.Plausibility, opened, v.OperationID)
}
