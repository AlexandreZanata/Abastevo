package adapters

import (
	"context"
	"errors"
	"math"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/driver"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type LedgerCostPort func(context.Context, pgx.Tx, string, string, string, int64) (*int64, string, error)

var _ app.CostProfileStore = (*Store)(nil)
var _ app.MaintenanceStore = (*Store)(nil)

func day(s string) (pgtype.Date, error) {
	t, e := time.Parse("2006-01-02", s)
	if e != nil {
		return pgtype.Date{}, domain.ErrInvalid
	}
	return pgtype.Date{Time: t, Valid: true}, nil
}

func optionalDay(s string) (pgtype.Date, error) {
	if s == "" {
		return pgtype.Date{}, nil
	}
	return day(s)
}

func dayString(v pgtype.Date) string {
	if !v.Valid {
		return ""
	}
	return v.Time.Format("2006-01-02")
}

func costEpoch(ctx context.Context, query *q.Queries, owner string, expected int64) (int64, error) {
	if expected < 0 {
		return 0, domain.ErrInvalid
	}
	if e := query.EnsureDriverCostState(ctx, uid(owner)); e != nil {
		return 0, storageError(ctx, e)
	}
	epoch, e := query.LockDriverCostEpoch(ctx, uid(owner))
	if e != nil {
		return 0, storageError(ctx, e)
	}
	if epoch != expected {
		return epoch, app.ErrErased
	}
	return epoch, nil
}

func maintenanceEpoch(ctx context.Context, query *q.Queries, owner string, expected int64) (int64, error) {
	if expected < 0 {
		return 0, domain.ErrInvalid
	}
	if e := query.EnsureDriverMaintenanceState(ctx, uid(owner)); e != nil {
		return 0, storageError(ctx, e)
	}
	epoch, e := query.LockDriverMaintenanceEpoch(ctx, uid(owner))
	if e != nil {
		return 0, storageError(ctx, e)
	}
	if epoch != expected {
		return epoch, app.ErrErased
	}
	return epoch, nil
}

func sameAmount(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func costProfileFromRows(profile q.DriverCostProfile, items []q.DriverCostProfileItem) (domain.VehicleCostProfile, error) {
	out := domain.VehicleCostProfile{
		Vehicle: uuid(profile.VehicleID), Ownership: profile.Ownership, Timezone: profile.Timezone,
		From: dayString(profile.FromDay), Until: dayString(profile.UntilDay), Version: profile.Version,
	}
	out.Costs = make([]domain.ProfileCost, len(items))
	for i, item := range items {
		c := domain.ProfileCost{ID: item.CostID, Kind: item.Kind, Provenance: item.Provenance, From: dayString(item.FromDay), Until: dayString(item.UntilDay), Amount: optional(item.AmountCentavos)}
		if item.LedgerSourceID.Valid {
			c.LedgerSourceID = uuid(item.LedgerSourceID)
		}
		if item.SourceRevision.Valid {
			c.SourceRevision = item.SourceRevision.Int64
		}
		out.Costs[i] = c
	}
	normalized, e := out.Normalize()
	if e != nil {
		return out, e
	}
	hash, e := domain.Digest(normalized)
	if e != nil || hash != profile.ContentSha256 {
		return out, domain.ErrInvalid
	}
	return normalized, nil
}

func (s *Store) SaveCostProfile(ctx context.Context, owner string, in app.SaveCostProfile) (app.CostProfileReceipt, error) {
	if !domain.ValidID(owner) || !domain.ValidID(in.OperationID) || in.ExpectedVersion < 0 || in.ExpectedVersion == math.MaxInt64 || in.ExpectedEpoch < 0 {
		return app.CostProfileReceipt{}, domain.ErrInvalid
	}
	profile, e := in.Profile.Normalize()
	if e != nil {
		return app.CostProfileReceipt{}, e
	}
	if profile.Version != in.ExpectedVersion+1 {
		return app.CostProfileReceipt{}, domain.ErrInvalid
	}
	in.Profile = profile
	digest, e := domain.Digest(struct {
		Action  string
		Command app.SaveCostProfile
	}{"SAVE_COST_PROFILE", in})
	if e != nil {
		return app.CostProfileReceipt{}, e
	}
	content, e := domain.Digest(profile)
	if e != nil {
		return app.CostProfileReceipt{}, e
	}
	tx, query, e := s.begin(ctx, owner, "")
	if e != nil {
		return app.CostProfileReceipt{}, e
	}
	defer rollback(tx)
	if e = s.authorize(ctx, tx, owner, profile.Vehicle); e != nil {
		return app.CostProfileReceipt{}, e
	}
	if _, e = costEpoch(ctx, query, owner, in.ExpectedEpoch); e != nil {
		return app.CostProfileReceipt{}, e
	}
	if _, e = query.DriverCostTombstone(ctx, q.DriverCostTombstoneParams{AccountID: uid(owner), VehicleID: uid(profile.Vehicle), Version: profile.Version}); e == nil {
		return app.CostProfileReceipt{}, app.ErrErased
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return app.CostProfileReceipt{}, storageError(ctx, e)
	}
	if command, e := query.DriverCostCommand(ctx, q.DriverCostCommandParams{AccountID: uid(owner), OperationID: uid(in.OperationID)}); e == nil {
		if command.RequestSha256 != digest {
			return app.CostProfileReceipt{}, domain.ErrConflict
		}
		if e = s.requirePaid(ctx, tx, owner); e != nil {
			return app.CostProfileReceipt{}, e
		}
		return app.CostProfileReceipt{Vehicle: uuid(command.VehicleID), Version: command.Version, Replayed: true}, nil
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return app.CostProfileReceipt{}, storageError(ctx, e)
	}
	latest, e := query.LatestDriverCostVersion(ctx, q.LatestDriverCostVersionParams{AccountID: uid(owner), VehicleID: uid(profile.Vehicle)})
	if e != nil {
		return app.CostProfileReceipt{}, storageError(ctx, e)
	}
	if latest != in.ExpectedVersion {
		return app.CostProfileReceipt{}, domain.ErrConflict
	}
	for _, item := range profile.Costs {
		if item.Provenance != "ACTUAL" {
			continue
		}
		if s.ledger == nil {
			return app.CostProfileReceipt{}, app.ErrDenied
		}
		amount, status, e := s.ledger(ctx, tx, owner, profile.Vehicle, item.LedgerSourceID, item.SourceRevision)
		if e != nil {
			return app.CostProfileReceipt{}, storageError(ctx, e)
		}
		switch status {
		case "ok":
			if !sameAmount(amount, item.Amount) {
				return app.CostProfileReceipt{}, domain.ErrConflict
			}
		case "conflict":
			return app.CostProfileReceipt{}, domain.ErrConflict
		default:
			return app.CostProfileReceipt{}, app.ErrDenied
		}
	}
	from, e := day(profile.From)
	if e != nil {
		return app.CostProfileReceipt{}, e
	}
	until, e := optionalDay(profile.Until)
	if e != nil {
		return app.CostProfileReceipt{}, e
	}
	if e = query.SaveDriverCostProfile(ctx, q.SaveDriverCostProfileParams{
		AccountID: uid(owner), VehicleID: uid(profile.Vehicle), Version: profile.Version, Ownership: profile.Ownership,
		Timezone: profile.Timezone, FromDay: from, UntilDay: until, ContentSha256: content,
	}); e != nil {
		return app.CostProfileReceipt{}, storageError(ctx, e)
	}
	for _, item := range profile.Costs {
		itemFrom, e := day(item.From)
		if e != nil {
			return app.CostProfileReceipt{}, e
		}
		itemUntil, e := optionalDay(item.Until)
		if e != nil {
			return app.CostProfileReceipt{}, e
		}
		var ledgerID pgtype.UUID
		var sourceRevision pgtype.Int8
		if item.Provenance == "ACTUAL" {
			ledgerID = uid(item.LedgerSourceID)
			sourceRevision = pgtype.Int8{Int64: item.SourceRevision, Valid: true}
		}
		if e = query.SaveDriverCostItem(ctx, q.SaveDriverCostItemParams{
			AccountID: uid(owner), VehicleID: uid(profile.Vehicle), Version: profile.Version, CostID: item.ID, Kind: item.Kind,
			Provenance: item.Provenance, LedgerSourceID: ledgerID, SourceRevision: sourceRevision,
			FromDay: itemFrom, UntilDay: itemUntil, AmountCentavos: money(item.Amount),
		}); e != nil {
			return app.CostProfileReceipt{}, storageError(ctx, e)
		}
	}
	if e = query.SaveDriverCostCommand(ctx, q.SaveDriverCostCommandParams{
		AccountID: uid(owner), OperationID: uid(in.OperationID), RequestSha256: digest, VehicleID: uid(profile.Vehicle), Version: profile.Version,
	}); e != nil {
		return app.CostProfileReceipt{}, storageError(ctx, e)
	}
	if e = s.requirePaid(ctx, tx, owner); e != nil {
		return app.CostProfileReceipt{}, e
	}
	if e = s.authorize(ctx, tx, owner, profile.Vehicle); e != nil {
		return app.CostProfileReceipt{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return app.CostProfileReceipt{}, storageError(ctx, e)
	}
	return app.CostProfileReceipt{Vehicle: profile.Vehicle, Version: profile.Version}, nil
}

func (s *Store) GetCostProfile(ctx context.Context, owner, vehicle string, version int64) (domain.VehicleCostProfile, error) {
	if !domain.ValidID(owner) || !domain.ValidID(vehicle) || version < 1 {
		return domain.VehicleCostProfile{}, domain.ErrInvalid
	}
	tx, query, e := s.begin(ctx, owner, vehicle)
	if e != nil {
		return domain.VehicleCostProfile{}, e
	}
	defer rollback(tx)
	row, e := query.GetDriverCostProfile(ctx, q.GetDriverCostProfileParams{AccountID: uid(owner), VehicleID: uid(vehicle), Version: version})
	if errors.Is(e, pgx.ErrNoRows) {
		return domain.VehicleCostProfile{}, app.ErrNotFound
	}
	if e != nil {
		return domain.VehicleCostProfile{}, storageError(ctx, e)
	}
	rows, e := query.DriverCostItems(ctx, q.DriverCostItemsParams{AccountID: uid(owner), VehicleID: uid(vehicle), Version: version})
	if e != nil {
		return domain.VehicleCostProfile{}, storageError(ctx, e)
	}
	return costProfileFromRows(row, rows)
}

func (s *Store) ListCostProfiles(ctx context.Context, owner, vehicle string, afterVersion int64, limit int) ([]domain.VehicleCostProfile, error) {
	if !domain.ValidID(owner) || !domain.ValidID(vehicle) || afterVersion < 0 || limit < 1 || limit > domain.MaxItems {
		return nil, domain.ErrInvalid
	}
	tx, query, e := s.begin(ctx, owner, vehicle)
	if e != nil {
		return nil, e
	}
	defer rollback(tx)
	rows, e := query.ListDriverCostProfiles(ctx, q.ListDriverCostProfilesParams{AccountID: uid(owner), VehicleID: uid(vehicle), Version: afterVersion, Limit: int32(limit)})
	if e != nil {
		return nil, storageError(ctx, e)
	}
	out := make([]domain.VehicleCostProfile, len(rows))
	for i, row := range rows {
		items, e := query.DriverCostItems(ctx, q.DriverCostItemsParams{AccountID: uid(owner), VehicleID: uid(vehicle), Version: row.Version})
		if e != nil {
			return nil, storageError(ctx, e)
		}
		out[i], e = costProfileFromRows(row, items)
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}

func (s *Store) EraseCostProfile(ctx context.Context, owner, vehicle string, version, expectedEpoch int64) error {
	if !domain.ValidID(owner) || !domain.ValidID(vehicle) || version < 1 || expectedEpoch < 0 {
		return domain.ErrInvalid
	}
	tx, query, e := s.begin(ctx, owner, vehicle)
	if e != nil {
		return e
	}
	defer rollback(tx)
	if _, e = costEpoch(ctx, query, owner, expectedEpoch); e != nil {
		return e
	}
	if _, e = query.DriverCostTombstone(ctx, q.DriverCostTombstoneParams{AccountID: uid(owner), VehicleID: uid(vehicle), Version: version}); e == nil {
		return nil
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return storageError(ctx, e)
	}
	row, e := query.GetDriverCostProfile(ctx, q.GetDriverCostProfileParams{AccountID: uid(owner), VehicleID: uid(vehicle), Version: version})
	if errors.Is(e, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	if e != nil {
		return storageError(ctx, e)
	}
	if e = query.SaveDriverCostTombstone(ctx, q.SaveDriverCostTombstoneParams{AccountID: uid(owner), VehicleID: uid(vehicle), Version: version, ErasedContentSha256: row.ContentSha256}); e != nil {
		return storageError(ctx, e)
	}
	if e = query.DeleteDriverCostProfile(ctx, q.DeleteDriverCostProfileParams{AccountID: uid(owner), VehicleID: uid(vehicle), Version: version}); e != nil {
		return storageError(ctx, e)
	}
	if e = s.authorize(ctx, tx, owner, vehicle); e != nil {
		return e
	}
	return storageError(ctx, tx.Commit(ctx))
}

func (s *Store) EraseCostOwner(ctx context.Context, owner string, expectedEpoch int64) error {
	if !domain.ValidID(owner) || expectedEpoch < 0 {
		return domain.ErrInvalid
	}
	tx, query, e := s.begin(ctx, owner, "")
	if e != nil {
		return e
	}
	defer rollback(tx)
	if _, e = costEpoch(ctx, query, owner, expectedEpoch); e != nil {
		return e
	}
	if _, e = query.AdvanceDriverCostEpoch(ctx, uid(owner)); e != nil {
		return storageError(ctx, e)
	}
	for _, remove := range []func(context.Context, pgtype.UUID) error{query.DeleteDriverCostOwnerProfiles, query.DeleteDriverCostOwnerCommands, query.DeleteDriverCostOwnerTombstones} {
		if e = remove(ctx, uid(owner)); e != nil {
			return storageError(ctx, e)
		}
	}
	if e = s.authorize(ctx, tx, owner, ""); e != nil {
		return e
	}
	return storageError(ctx, tx.Commit(ctx))
}

func maintenanceFromRow(row q.DriverMaintenanceSchedule) (domain.MaintenanceSchedule, error) {
	out := domain.MaintenanceSchedule{
		ID: uuid(row.ID), Vehicle: uuid(row.VehicleID), Version: row.Version, Timezone: row.Timezone, Kind: row.Kind,
		Basis: row.Basis, EffectiveFrom: dayString(row.EffectiveFrom), EffectiveUntil: dayString(row.EffectiveUntil),
		IntervalMetres: optional(row.IntervalMetres), IntervalDays: optional(row.IntervalDays),
		OdometerSegment: row.OdometerSegment, BaselineMetres: optional(row.BaselineMetres), BaselineDay: dayString(row.BaselineDay),
		NextDueMetres: optional(row.NextDueMetres), NextDueDay: dayString(row.NextDueDay),
	}
	normalized, e := out.Normalize()
	if e != nil {
		return out, e
	}
	hash, e := domain.Digest(normalized)
	if e != nil || hash != row.ContentSha256 {
		return out, domain.ErrInvalid
	}
	return normalized, nil
}

func (s *Store) SaveMaintenance(ctx context.Context, owner string, in app.SaveMaintenance) (app.MaintenanceReceipt, error) {
	if !domain.ValidID(owner) || !domain.ValidID(in.OperationID) || in.ExpectedVersion < 0 || in.ExpectedVersion == math.MaxInt64 || in.ExpectedEpoch < 0 {
		return app.MaintenanceReceipt{}, domain.ErrInvalid
	}
	schedule, e := in.Schedule.Normalize()
	if e != nil {
		return app.MaintenanceReceipt{}, e
	}
	if schedule.Version != in.ExpectedVersion+1 {
		return app.MaintenanceReceipt{}, domain.ErrInvalid
	}
	in.Schedule = schedule
	digest, e := domain.Digest(struct {
		Action  string
		Command app.SaveMaintenance
	}{"SAVE_MAINTENANCE", in})
	if e != nil {
		return app.MaintenanceReceipt{}, e
	}
	content, e := domain.Digest(schedule)
	if e != nil {
		return app.MaintenanceReceipt{}, e
	}
	tx, query, e := s.begin(ctx, owner, "")
	if e != nil {
		return app.MaintenanceReceipt{}, e
	}
	defer rollback(tx)
	if e = s.authorize(ctx, tx, owner, schedule.Vehicle); e != nil {
		return app.MaintenanceReceipt{}, e
	}
	if _, e = maintenanceEpoch(ctx, query, owner, in.ExpectedEpoch); e != nil {
		return app.MaintenanceReceipt{}, e
	}
	if _, e = query.DriverMaintenanceTombstone(ctx, q.DriverMaintenanceTombstoneParams{AccountID: uid(owner), ScheduleID: uid(schedule.ID), Version: schedule.Version}); e == nil {
		return app.MaintenanceReceipt{}, app.ErrErased
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return app.MaintenanceReceipt{}, storageError(ctx, e)
	}
	if command, e := query.DriverMaintenanceCommand(ctx, q.DriverMaintenanceCommandParams{AccountID: uid(owner), OperationID: uid(in.OperationID)}); e == nil {
		if command.RequestSha256 != digest {
			return app.MaintenanceReceipt{}, domain.ErrConflict
		}
		if e = s.requirePaid(ctx, tx, owner); e != nil {
			return app.MaintenanceReceipt{}, e
		}
		return app.MaintenanceReceipt{ScheduleID: uuid(command.ScheduleID), Version: command.Version, Replayed: true}, nil
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return app.MaintenanceReceipt{}, storageError(ctx, e)
	}
	latest, e := query.LatestDriverMaintenanceVersion(ctx, q.LatestDriverMaintenanceVersionParams{AccountID: uid(owner), ID: uid(schedule.ID)})
	if e != nil {
		return app.MaintenanceReceipt{}, storageError(ctx, e)
	}
	if latest != in.ExpectedVersion {
		return app.MaintenanceReceipt{}, domain.ErrConflict
	}
	effectiveFrom, e := day(schedule.EffectiveFrom)
	if e != nil {
		return app.MaintenanceReceipt{}, e
	}
	effectiveUntil, e := optionalDay(schedule.EffectiveUntil)
	if e != nil {
		return app.MaintenanceReceipt{}, e
	}
	baselineDay, e := optionalDay(schedule.BaselineDay)
	if e != nil {
		return app.MaintenanceReceipt{}, e
	}
	nextDueDay, e := optionalDay(schedule.NextDueDay)
	if e != nil {
		return app.MaintenanceReceipt{}, e
	}
	if e = query.SaveDriverMaintenance(ctx, q.SaveDriverMaintenanceParams{
		AccountID: uid(owner), ID: uid(schedule.ID), Version: schedule.Version, VehicleID: uid(schedule.Vehicle), Timezone: schedule.Timezone,
		Kind: schedule.Kind, Basis: schedule.Basis, EffectiveFrom: effectiveFrom, EffectiveUntil: effectiveUntil,
		IntervalMetres: money(schedule.IntervalMetres), IntervalDays: money(schedule.IntervalDays), OdometerSegment: schedule.OdometerSegment,
		BaselineMetres: money(schedule.BaselineMetres), BaselineDay: baselineDay, NextDueMetres: money(schedule.NextDueMetres), NextDueDay: nextDueDay,
		ContentSha256: content,
	}); e != nil {
		return app.MaintenanceReceipt{}, storageError(ctx, e)
	}
	if e = query.SaveDriverMaintenanceCommand(ctx, q.SaveDriverMaintenanceCommandParams{
		AccountID: uid(owner), OperationID: uid(in.OperationID), RequestSha256: digest, ScheduleID: uid(schedule.ID), Version: schedule.Version,
	}); e != nil {
		return app.MaintenanceReceipt{}, storageError(ctx, e)
	}
	if e = s.requirePaid(ctx, tx, owner); e != nil {
		return app.MaintenanceReceipt{}, e
	}
	if e = s.authorize(ctx, tx, owner, schedule.Vehicle); e != nil {
		return app.MaintenanceReceipt{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return app.MaintenanceReceipt{}, storageError(ctx, e)
	}
	return app.MaintenanceReceipt{ScheduleID: schedule.ID, Version: schedule.Version}, nil
}

func (s *Store) GetMaintenance(ctx context.Context, owner, id string, version int64) (domain.MaintenanceSchedule, error) {
	if !domain.ValidID(owner) || !domain.ValidID(id) || version < 1 {
		return domain.MaintenanceSchedule{}, domain.ErrInvalid
	}
	tx, query, e := s.begin(ctx, owner, "")
	if e != nil {
		return domain.MaintenanceSchedule{}, e
	}
	defer rollback(tx)
	row, e := query.GetDriverMaintenance(ctx, q.GetDriverMaintenanceParams{AccountID: uid(owner), ID: uid(id), Version: version})
	if errors.Is(e, pgx.ErrNoRows) {
		return domain.MaintenanceSchedule{}, app.ErrNotFound
	}
	if e != nil {
		return domain.MaintenanceSchedule{}, storageError(ctx, e)
	}
	out, e := maintenanceFromRow(row)
	if e != nil {
		return out, e
	}
	if e = s.authorize(ctx, tx, owner, out.Vehicle); e != nil {
		return domain.MaintenanceSchedule{}, e
	}
	return out, nil
}

func (s *Store) ListMaintenance(ctx context.Context, owner, vehicle string, afterVersion int64, limit int) ([]domain.MaintenanceSchedule, error) {
	if !domain.ValidID(owner) || !domain.ValidID(vehicle) || afterVersion < 0 || limit < 1 || limit > domain.MaxItems {
		return nil, domain.ErrInvalid
	}
	tx, query, e := s.begin(ctx, owner, vehicle)
	if e != nil {
		return nil, e
	}
	defer rollback(tx)
	rows, e := query.ListDriverMaintenance(ctx, q.ListDriverMaintenanceParams{AccountID: uid(owner), VehicleID: uid(vehicle), Version: afterVersion, Limit: int32(limit)})
	if e != nil {
		return nil, storageError(ctx, e)
	}
	out := make([]domain.MaintenanceSchedule, len(rows))
	for i, row := range rows {
		out[i], e = maintenanceFromRow(row)
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}

func (s *Store) EraseMaintenance(ctx context.Context, owner, id string, version, expectedEpoch int64) error {
	if !domain.ValidID(owner) || !domain.ValidID(id) || version < 1 || expectedEpoch < 0 {
		return domain.ErrInvalid
	}
	tx, query, e := s.begin(ctx, owner, "")
	if e != nil {
		return e
	}
	defer rollback(tx)
	if _, e = maintenanceEpoch(ctx, query, owner, expectedEpoch); e != nil {
		return e
	}
	if _, e = query.DriverMaintenanceTombstone(ctx, q.DriverMaintenanceTombstoneParams{AccountID: uid(owner), ScheduleID: uid(id), Version: version}); e == nil {
		return nil
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return storageError(ctx, e)
	}
	row, e := query.GetDriverMaintenance(ctx, q.GetDriverMaintenanceParams{AccountID: uid(owner), ID: uid(id), Version: version})
	if errors.Is(e, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	if e != nil {
		return storageError(ctx, e)
	}
	if e = query.SaveDriverMaintenanceTombstone(ctx, q.SaveDriverMaintenanceTombstoneParams{AccountID: uid(owner), ScheduleID: uid(id), Version: version, ErasedContentSha256: row.ContentSha256}); e != nil {
		return storageError(ctx, e)
	}
	if e = query.DeleteDriverMaintenance(ctx, q.DeleteDriverMaintenanceParams{AccountID: uid(owner), ID: uid(id), Version: version}); e != nil {
		return storageError(ctx, e)
	}
	if e = s.authorize(ctx, tx, owner, uuid(row.VehicleID)); e != nil {
		return e
	}
	return storageError(ctx, tx.Commit(ctx))
}

func (s *Store) EraseMaintenanceOwner(ctx context.Context, owner string, expectedEpoch int64) error {
	if !domain.ValidID(owner) || expectedEpoch < 0 {
		return domain.ErrInvalid
	}
	tx, query, e := s.begin(ctx, owner, "")
	if e != nil {
		return e
	}
	defer rollback(tx)
	if _, e = maintenanceEpoch(ctx, query, owner, expectedEpoch); e != nil {
		return e
	}
	if _, e = query.AdvanceDriverMaintenanceEpoch(ctx, uid(owner)); e != nil {
		return storageError(ctx, e)
	}
	for _, remove := range []func(context.Context, pgtype.UUID) error{query.DeleteDriverMaintenanceOwnerSchedules, query.DeleteDriverMaintenanceOwnerCommands, query.DeleteDriverMaintenanceOwnerTombstones} {
		if e = remove(ctx, uid(owner)); e != nil {
			return storageError(ctx, e)
		}
	}
	if e = s.authorize(ctx, tx, owner, ""); e != nil {
		return e
	}
	return storageError(ctx, tx.Commit(ctx))
}
