package adapters

import (
	"context"
	"errors"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/driver"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ app.ShiftStore = (*Store)(nil)
var _ app.DistanceStore = (*Store)(nil)

func lastClock(s domain.Shift) (pgtype.Text, pgtype.Int8, pgtype.Int8) {
	if s.Last == nil {
		return pgtype.Text{}, pgtype.Int8{}, pgtype.Int8{}
	}
	return pgtype.Text{String: s.Last.BootID, Valid: true}, pgtype.Int8{Int64: s.Last.ElapsedMillis, Valid: true}, pgtype.Int8{Int64: s.Last.UTCMillis, Valid: true}
}

func shiftFromRows(owner string, row q.DriverShift, gaps []q.DriverShiftGap) (domain.Shift, error) {
	s := domain.Shift{
		ID: uuid(row.ID), Owner: owner, Origin: row.Origin, Vehicle: uuid(row.VehicleID), Timezone: row.Timezone,
		State: row.State, Version: row.Version, ActiveMillis: row.ActiveMillis, ManuallyReviewedMillis: row.ManuallyReviewedMillis,
	}
	if row.LastBootID.Valid || row.LastElapsedMillis.Valid || row.LastUtcMillis.Valid {
		if !row.LastBootID.Valid || !row.LastElapsedMillis.Valid || !row.LastUtcMillis.Valid {
			return domain.Shift{}, domain.ErrInvalid
		}
		s.Last = &domain.Clock{BootID: row.LastBootID.String, ElapsedMillis: row.LastElapsedMillis.Int64, UTCMillis: row.LastUtcMillis.Int64}
	}
	s.Gaps = make([]domain.ShiftGap, len(gaps))
	for i, g := range gaps {
		s.Gaps[i] = domain.ShiftGap{
			ID: uuid(g.ID), Reason: g.Reason, Reviewed: g.Reviewed,
			Previous: domain.Clock{BootID: g.PreviousBootID, ElapsedMillis: g.PreviousElapsedMillis, UTCMillis: g.PreviousUtcMillis},
			Next:     domain.Clock{BootID: g.NextBootID, ElapsedMillis: g.NextElapsedMillis, UTCMillis: g.NextUtcMillis},
		}
		if g.ReviewedWorkMillis.Valid {
			n := g.ReviewedWorkMillis.Int64
			s.Gaps[i].ReviewedWorkMillis = &n
		}
	}
	if e := s.Validate(); e != nil {
		return domain.Shift{}, domain.ErrInvalid
	}
	return s, nil
}

func (s *Store) loadShift(ctx context.Context, query *q.Queries, owner string, row q.DriverShift) (domain.Shift, error) {
	gaps, e := query.ListDriverShiftGaps(ctx, q.ListDriverShiftGapsParams{AccountID: uid(owner), ShiftID: row.ID})
	if e != nil {
		return domain.Shift{}, storageError(ctx, e)
	}
	return shiftFromRows(owner, row, gaps)
}

func (s *Store) GetShift(ctx context.Context, owner, id string) (domain.Shift, error) {
	if !domain.ValidID(owner) || !domain.ValidID(id) {
		return domain.Shift{}, domain.ErrInvalid
	}
	tx, query, e := s.begin(ctx, owner, "")
	if e != nil {
		return domain.Shift{}, e
	}
	defer rollback(tx)
	row, e := query.GetDriverShift(ctx, q.GetDriverShiftParams{AccountID: uid(owner), ID: uid(id)})
	if errors.Is(e, pgx.ErrNoRows) {
		return domain.Shift{}, app.ErrNotFound
	}
	if e != nil {
		return domain.Shift{}, storageError(ctx, e)
	}
	shift, e := s.loadShift(ctx, query, owner, row)
	if e != nil {
		return domain.Shift{}, e
	}
	if e = s.authorize(ctx, tx, owner, shift.Vehicle); e != nil {
		return domain.Shift{}, e
	}
	return shift, nil
}

func (s *Store) CurrentShift(ctx context.Context, owner string) (domain.Shift, error) {
	if !domain.ValidID(owner) {
		return domain.Shift{}, domain.ErrInvalid
	}
	tx, query, e := s.begin(ctx, owner, "")
	if e != nil {
		return domain.Shift{}, e
	}
	defer rollback(tx)
	row, e := query.CurrentDriverShift(ctx, uid(owner))
	if errors.Is(e, pgx.ErrNoRows) {
		return domain.Shift{}, app.ErrNotFound
	}
	if e != nil {
		return domain.Shift{}, storageError(ctx, e)
	}
	shift, e := s.loadShift(ctx, query, owner, row)
	if e != nil {
		return domain.Shift{}, e
	}
	if e = s.authorize(ctx, tx, owner, shift.Vehicle); e != nil {
		return domain.Shift{}, e
	}
	return shift, nil
}

func (s *Store) ApplyShift(ctx context.Context, owner string, in app.ApplyShift) (domain.ShiftTransition, error) {
	if !domain.ValidID(owner) || !domain.ValidID(in.ShiftID) || in.Command.Validate() != nil || in.Policy.Validate() != nil {
		return domain.ShiftTransition{}, domain.ErrInvalid
	}
	if in.Create != nil {
		if in.Command.Action != domain.ActionStart || in.Command.ExpectedVersion != 0 || in.Create.ID != in.ShiftID ||
			!domain.ValidID(in.Create.Vehicle) || in.Create.Origin == "" || len(in.Create.Origin) > 128 ||
			in.Create.Timezone == "" || in.Create.Timezone == "Local" {
			return domain.ShiftTransition{}, domain.ErrInvalid
		}
		if _, e := time.LoadLocation(in.Create.Timezone); e != nil {
			return domain.ShiftTransition{}, domain.ErrInvalid
		}
	} else if in.Command.Action == domain.ActionStart {
		return domain.ShiftTransition{}, domain.ErrInvalid
	}
	fingerprint, e := in.Command.Fingerprint()
	if e != nil {
		return domain.ShiftTransition{}, e
	}
	tx, query, e := s.begin(ctx, owner, "")
	if e != nil {
		return domain.ShiftTransition{}, e
	}
	defer rollback(tx)

	receipt, e := query.DriverShiftCommand(ctx, q.DriverShiftCommandParams{AccountID: uid(owner), ShiftID: uid(in.ShiftID), OperationID: uid(in.Command.ID)})
	if e == nil {
		if receipt.Fingerprint != fingerprint {
			return domain.ShiftTransition{}, domain.ErrConflict
		}
		row, e := query.GetDriverShift(ctx, q.GetDriverShiftParams{AccountID: uid(owner), ID: uid(in.ShiftID)})
		if errors.Is(e, pgx.ErrNoRows) {
			return domain.ShiftTransition{}, app.ErrNotFound
		}
		if e != nil {
			return domain.ShiftTransition{}, storageError(ctx, e)
		}
		shift, e := s.loadShift(ctx, query, owner, row)
		if e != nil {
			return domain.ShiftTransition{}, e
		}
		if in.Create != nil && (shift.Origin != in.Create.Origin || shift.Vehicle != in.Create.Vehicle || shift.Timezone != in.Create.Timezone) {
			return domain.ShiftTransition{}, domain.ErrConflict
		}
		if e = s.authorize(ctx, tx, owner, shift.Vehicle); e != nil {
			return domain.ShiftTransition{}, e
		}
		return domain.ShiftTransition{Shift: shift, Receipt: domain.ShiftReceipt{ID: uuid(receipt.OperationID), Fingerprint: receipt.Fingerprint, AcceptedVersion: receipt.AcceptedVersion}, Replayed: true}, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return domain.ShiftTransition{}, storageError(ctx, e)
	}

	var shift domain.Shift
	if in.Create != nil {
		if _, e = query.CurrentDriverShift(ctx, uid(owner)); e == nil {
			return domain.ShiftTransition{}, domain.ErrConflict
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return domain.ShiftTransition{}, storageError(ctx, e)
		}
		if _, e = query.GetDriverShift(ctx, q.GetDriverShiftParams{AccountID: uid(owner), ID: uid(in.ShiftID)}); e == nil {
			return domain.ShiftTransition{}, domain.ErrConflict
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return domain.ShiftTransition{}, storageError(ctx, e)
		}
		shift, e = domain.NewShift(owner, in.ShiftID, in.Create.Origin, in.Create.Vehicle, in.Create.Timezone)
		if e != nil {
			return domain.ShiftTransition{}, e
		}
		if e = s.authorize(ctx, tx, owner, in.Create.Vehicle); e != nil {
			return domain.ShiftTransition{}, e
		}
	} else {
		row, e := query.GetDriverShift(ctx, q.GetDriverShiftParams{AccountID: uid(owner), ID: uid(in.ShiftID)})
		if errors.Is(e, pgx.ErrNoRows) {
			return domain.ShiftTransition{}, app.ErrNotFound
		}
		if e != nil {
			return domain.ShiftTransition{}, storageError(ctx, e)
		}
		shift, e = s.loadShift(ctx, query, owner, row)
		if e != nil {
			return domain.ShiftTransition{}, e
		}
		if e = s.authorize(ctx, tx, owner, shift.Vehicle); e != nil {
			return domain.ShiftTransition{}, e
		}
	}
	transition, e := shift.Apply(in.Command, in.Policy)
	if e != nil {
		return domain.ShiftTransition{}, e
	}
	if in.Create != nil {
		if len(transition.Shift.Gaps) != 0 {
			return domain.ShiftTransition{}, domain.ErrInvalid
		}
		boot, elapsed, utc := lastClock(transition.Shift)
		e = query.InsertDriverShift(ctx, q.InsertDriverShiftParams{
			AccountID: uid(owner), ID: uid(transition.Shift.ID), Origin: transition.Shift.Origin, VehicleID: uid(transition.Shift.Vehicle),
			Timezone: transition.Shift.Timezone, State: transition.Shift.State, Version: transition.Shift.Version,
			LastBootID: boot, LastElapsedMillis: elapsed, LastUtcMillis: utc,
			ActiveMillis: transition.Shift.ActiveMillis, ManuallyReviewedMillis: transition.Shift.ManuallyReviewedMillis,
		})
	} else {
		boot, elapsed, utc := lastClock(transition.Shift)
		rows, updateErr := query.UpdateDriverShift(ctx, q.UpdateDriverShiftParams{
			AccountID: uid(owner), ID: uid(transition.Shift.ID), Version: shift.Version,
			State: transition.Shift.State, Version_2: transition.Shift.Version,
			LastBootID: boot, LastElapsedMillis: elapsed, LastUtcMillis: utc,
			ActiveMillis: transition.Shift.ActiveMillis, ManuallyReviewedMillis: transition.Shift.ManuallyReviewedMillis,
		})
		if updateErr != nil {
			e = updateErr
		} else if rows != 1 {
			return domain.ShiftTransition{}, domain.ErrConflict
		} else {
			e = s.applyGapChanges(ctx, query, owner, shift, transition.Shift)
		}
	}
	if e != nil {
		return domain.ShiftTransition{}, storageError(ctx, e)
	}
	if e = query.SaveDriverShiftCommand(ctx, q.SaveDriverShiftCommandParams{
		AccountID: uid(owner), ShiftID: uid(transition.Shift.ID), OperationID: uid(in.Command.ID),
		Fingerprint: fingerprint, AcceptedVersion: transition.Receipt.AcceptedVersion,
	}); e != nil {
		return domain.ShiftTransition{}, storageError(ctx, e)
	}
	pruneAt := transition.Receipt.AcceptedVersion - domain.MaxShiftReceipts
	if pruneAt > 0 {
		if e = query.PruneDriverShiftCommands(ctx, q.PruneDriverShiftCommandsParams{AccountID: uid(owner), ShiftID: uid(transition.Shift.ID), AcceptedVersion: pruneAt}); e != nil {
			return domain.ShiftTransition{}, storageError(ctx, e)
		}
	}
	if e = s.authorize(ctx, tx, owner, transition.Shift.Vehicle); e != nil {
		return domain.ShiftTransition{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return domain.ShiftTransition{}, storageError(ctx, e)
	}
	return transition, nil
}

func (s *Store) applyGapChanges(ctx context.Context, query *q.Queries, owner string, previous, next domain.Shift) error {
	if len(next.Gaps) == len(previous.Gaps) {
		for i := range next.Gaps {
			if previous.Gaps[i].ID != next.Gaps[i].ID {
				return domain.ErrInvalid
			}
			if previous.Gaps[i].Reviewed == next.Gaps[i].Reviewed {
				continue
			}
			if !next.Gaps[i].Reviewed {
				return domain.ErrInvalid
			}
			var work pgtype.Int8
			if next.Gaps[i].ReviewedWorkMillis != nil {
				work = pgtype.Int8{Int64: *next.Gaps[i].ReviewedWorkMillis, Valid: true}
			}
			rows, e := query.ReviewDriverShiftGap(ctx, q.ReviewDriverShiftGapParams{AccountID: uid(owner), ShiftID: uid(previous.ID), ID: uid(next.Gaps[i].ID), ReviewedWorkMillis: work})
			if e != nil {
				return e
			}
			if rows != 1 {
				return domain.ErrConflict
			}
		}
		return nil
	}
	if len(next.Gaps) != len(previous.Gaps)+1 {
		return domain.ErrInvalid
	}
	g := next.Gaps[len(next.Gaps)-1]
	return query.InsertDriverShiftGap(ctx, q.InsertDriverShiftGapParams{
		AccountID: uid(owner), ShiftID: uid(previous.ID), ID: uid(g.ID), Reason: g.Reason,
		PreviousBootID: g.Previous.BootID, PreviousElapsedMillis: g.Previous.ElapsedMillis, PreviousUtcMillis: g.Previous.UTCMillis,
		NextBootID: g.Next.BootID, NextElapsedMillis: g.Next.ElapsedMillis, NextUtcMillis: g.Next.UTCMillis,
	})
}

func qualityVersion(v *int32) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *v, Valid: true}
}

func distanceParams(owner string, r domain.DistanceRecord, content string) q.SaveDriverDistanceParams {
	out := q.SaveDriverDistanceParams{
		AccountID: uid(owner), ID: uid(r.ID), Origin: r.Origin, VehicleID: uid(r.VehicleID), Segment: r.Segment, Basis: r.Basis,
		FromAt: stamp(r.From), ToAt: stamp(r.To), Timezone: r.Timezone, Metres: money(r.Metres), Complete: r.Complete,
		HasGap: r.HasGap, Quality: r.Quality, QualityVersion: qualityVersion(r.QualityVersion),
		StartOdometerMetres: money(r.StartOdometerMetres), EndOdometerMetres: money(r.EndOdometerMetres), ContentSha256: content,
	}
	if r.QualityNote != "" {
		out.QualityNote = pgtype.Text{String: r.QualityNote, Valid: true}
	}
	return out
}

func distanceFromRow(owner string, row q.DriverDistanceObservation) (domain.DistanceRecord, error) {
	r := domain.DistanceRecord{
		ID: uuid(row.ID), Origin: row.Origin, VehicleID: uuid(row.VehicleID), Segment: row.Segment, Basis: row.Basis,
		From: row.FromAt.Time.UTC(), To: row.ToAt.Time.UTC(), Timezone: row.Timezone, Metres: optional(row.Metres),
		Complete: row.Complete, HasGap: row.HasGap, Quality: row.Quality,
		StartOdometerMetres: optional(row.StartOdometerMetres), EndOdometerMetres: optional(row.EndOdometerMetres),
	}
	if row.QualityVersion.Valid {
		v := row.QualityVersion.Int32
		r.QualityVersion = &v
	}
	if row.QualityNote.Valid {
		r.QualityNote = row.QualityNote.String
	}
	if _, e := r.Normalize(); e != nil {
		return domain.DistanceRecord{}, e
	}
	hash, e := r.ContentHash()
	if e != nil {
		return domain.DistanceRecord{}, e
	}
	if hash != row.ContentSha256 {
		return domain.DistanceRecord{}, domain.ErrInvalid
	}
	return r, nil
}

func (s *Store) SaveDistance(ctx context.Context, owner string, in app.SaveDistance) (app.DistanceReceipt, error) {
	if !domain.ValidID(owner) || !domain.ValidID(in.OperationID) {
		return app.DistanceReceipt{}, domain.ErrInvalid
	}
	record, e := in.Record.Normalize()
	if e != nil {
		return app.DistanceReceipt{}, e
	}
	content, e := record.ContentHash()
	if e != nil {
		return app.DistanceReceipt{}, e
	}
	digest, e := domain.Digest(struct {
		Action      string
		OperationID string
		Record      domain.DistanceRecord
	}{"SAVE_DISTANCE", in.OperationID, record})
	if e != nil {
		return app.DistanceReceipt{}, e
	}
	tx, query, e := s.begin(ctx, owner, "")
	if e != nil {
		return app.DistanceReceipt{}, e
	}
	defer rollback(tx)
	if e = s.authorize(ctx, tx, owner, record.VehicleID); e != nil {
		return app.DistanceReceipt{}, e
	}
	if command, e := query.DriverDistanceCommand(ctx, q.DriverDistanceCommandParams{AccountID: uid(owner), OperationID: uid(in.OperationID)}); e == nil {
		if command.RequestSha256 != digest {
			return app.DistanceReceipt{}, domain.ErrConflict
		}
		row, e := query.GetDriverDistance(ctx, q.GetDriverDistanceParams{AccountID: uid(owner), ID: command.RecordID})
		if errors.Is(e, pgx.ErrNoRows) {
			return app.DistanceReceipt{}, app.ErrNotFound
		}
		if e != nil {
			return app.DistanceReceipt{}, storageError(ctx, e)
		}
		if row.ContentSha256 != content {
			return app.DistanceReceipt{}, domain.ErrConflict
		}
		return app.DistanceReceipt{ID: uuid(row.ID), ContentHash: row.ContentSha256, Replayed: true}, nil
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return app.DistanceReceipt{}, storageError(ctx, e)
	}
	existing, e := query.GetDriverDistance(ctx, q.GetDriverDistanceParams{AccountID: uid(owner), ID: uid(record.ID)})
	if e == nil {
		if existing.ContentSha256 != content {
			return app.DistanceReceipt{}, domain.ErrConflict
		}
		if e = query.SaveDriverDistanceCommand(ctx, q.SaveDriverDistanceCommandParams{AccountID: uid(owner), OperationID: uid(in.OperationID), RequestSha256: digest, RecordID: existing.ID}); e != nil {
			return app.DistanceReceipt{}, storageError(ctx, e)
		}
		if e = s.authorize(ctx, tx, owner, record.VehicleID); e != nil {
			return app.DistanceReceipt{}, e
		}
		if e = tx.Commit(ctx); e != nil {
			return app.DistanceReceipt{}, storageError(ctx, e)
		}
		return app.DistanceReceipt{ID: uuid(existing.ID), ContentHash: existing.ContentSha256, Replayed: true}, nil
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return app.DistanceReceipt{}, storageError(ctx, e)
	}
	overlap, e := query.DriverDistanceOverlap(ctx, q.DriverDistanceOverlapParams{
		AccountID: uid(owner), VehicleID: uid(record.VehicleID), Basis: record.Basis, ExcludingID: uid(record.ID),
		ToAt: stamp(record.To), FromAt: stamp(record.From),
	})
	if e != nil {
		return app.DistanceReceipt{}, storageError(ctx, e)
	}
	if overlap {
		return app.DistanceReceipt{}, domain.ErrConflict
	}
	if e = query.SaveDriverDistance(ctx, distanceParams(owner, record, content)); e != nil {
		return app.DistanceReceipt{}, storageError(ctx, e)
	}
	if e = query.SaveDriverDistanceCommand(ctx, q.SaveDriverDistanceCommandParams{AccountID: uid(owner), OperationID: uid(in.OperationID), RequestSha256: digest, RecordID: uid(record.ID)}); e != nil {
		return app.DistanceReceipt{}, storageError(ctx, e)
	}
	if e = s.authorize(ctx, tx, owner, record.VehicleID); e != nil {
		return app.DistanceReceipt{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return app.DistanceReceipt{}, storageError(ctx, e)
	}
	return app.DistanceReceipt{ID: record.ID, ContentHash: content}, nil
}

func (s *Store) GetDistance(ctx context.Context, owner, id string) (domain.DistanceRecord, error) {
	if !domain.ValidID(owner) || !domain.ValidID(id) {
		return domain.DistanceRecord{}, domain.ErrInvalid
	}
	tx, query, e := s.begin(ctx, owner, "")
	if e != nil {
		return domain.DistanceRecord{}, e
	}
	defer rollback(tx)
	row, e := query.GetDriverDistance(ctx, q.GetDriverDistanceParams{AccountID: uid(owner), ID: uid(id)})
	if errors.Is(e, pgx.ErrNoRows) {
		return domain.DistanceRecord{}, app.ErrNotFound
	}
	if e != nil {
		return domain.DistanceRecord{}, storageError(ctx, e)
	}
	record, e := distanceFromRow(owner, row)
	if e != nil {
		return domain.DistanceRecord{}, e
	}
	if e = s.authorize(ctx, tx, owner, record.VehicleID); e != nil {
		return domain.DistanceRecord{}, e
	}
	return record, nil
}

func (s *Store) ListDistances(ctx context.Context, owner, vehicle string, from, to time.Time, limit int) ([]domain.DistanceRecord, error) {
	if !domain.ValidID(owner) || !domain.ValidID(vehicle) || limit < 1 || limit > domain.MaxItems || !to.After(from) {
		return nil, domain.ErrInvalid
	}
	tx, query, e := s.begin(ctx, owner, vehicle)
	if e != nil {
		return nil, e
	}
	defer rollback(tx)
	rows, e := query.ListDriverDistances(ctx, q.ListDriverDistancesParams{
		AccountID: uid(owner), VehicleID: uid(vehicle), FromAt: stamp(from), FromAt_2: stamp(to), Limit: int32(limit),
	})
	if e != nil {
		return nil, storageError(ctx, e)
	}
	out := make([]domain.DistanceRecord, len(rows))
	for i, row := range rows {
		out[i], e = distanceFromRow(owner, row)
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}
