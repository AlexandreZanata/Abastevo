package adapters

import (
	"context"
	"errors"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/fleet"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
)

type MeterStore struct {
	Pool      *pgxpool.Pool
	Authority app.MeterAuthority
	Audit     auditing.TransactionRunner
	Reviews   *MeterReviewStore
}

var _ app.MeterRepository = (*MeterStore)(nil)
var _ app.EmbeddedMeterRepository = (*MeterStore)(nil)
var _ app.MeterIntakeLookup = (*MeterStore)(nil)

func MeterAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: "meter.installed", TargetType: "meter"}}
}
func meterError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	for _, known := range []error{meter.ErrInvalid, meter.ErrScope, meter.ErrAlreadyInService} {
		if errors.Is(err, known) {
			return known
		}
	}
	if errors.Is(err, meter.ErrOperationReused) {
		return app.ErrOperationReused
	}
	var pe *pgconn.PgError
	// All supported input validation happens before writes. A deferred SQL
	// integrity/publication failure is an outage, never a client validation.
	if errors.As(err, &pe) && pe.Code == "23514" {
		return app.ErrUnavailable
	}
	if errors.As(err, &pe) && pe.Code == "23505" {
		switch pe.ConstraintName {
		case "org_meters_one_in_service_kind":
			return meter.ErrAlreadyInService
		case "org_meter_readings_pkey", "org_meter_readings_org_id_meter_id_operation_id_key", "org_meter_commands_pkey":
			return app.ErrOperationReused
		}
	}
	return storeError(ctx, err)
}
func (s *MeterStore) ready() bool { return s != nil && s.Pool != nil && s.Authority != nil }
func (s *MeterStore) check(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source app.MeterSource, member, asset string, action app.MeterAction) error {
	if s == nil || s.Authority == nil {
		return app.ErrUnavailable
	}
	// Company commands own their full retained transaction. The device/embedded
	// path always exposes only borrowed queries to the source adapter.
	if source.Web == nil {
		db = syncapp.Borrow(db)
	}
	return s.Authority.CheckMeterTx(ctx, db, scope, source, member, asset, action)
}
func (s *MeterStore) InstallMeter(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member, newID, newSegment string, command app.MeterInstall) (app.MeterInstallResult, bool, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.MeterInstallResult{}, false, err
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.MeterInstallResult{}, false, err
	}
	if !validID(newID) || !validID(newSegment) || newID == newSegment {
		return app.MeterInstallResult{}, false, meter.ErrInvalid
	}
	if !s.ready() || s.Audit == nil {
		return app.MeterInstallResult{}, false, app.ErrUnavailable
	}
	source := app.MeterSource{Web: &actor}
	digest := command.Digest(scope)
	org := scope.OrgID().String()
	var result app.MeterInstallResult
	replay := false
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.check(ctx, tx, scope, source, member, v.AssetID, app.MeterInstallAction); err != nil {
			return err
		}
		queries := q.New(tx)
		if _, err := queries.AssetRootLock(ctx, org); err != nil {
			return err
		}
		receipt, err := queries.MeterCommandReceipt(ctx, q.MeterCommandReceiptParams{OrgID: org, OperationID: v.OperationID})
		if err == nil {
			if receipt.ActorID != actor.AccountID || receipt.RequestHash != digest || receipt.AssetID != v.AssetID {
				return app.ErrOperationReused
			}
			if !validID(receipt.MeterID) || !validID(receipt.SegmentID) || receipt.OpeningReadingID != v.OpeningReadingID || receipt.AssetRevision < 2 {
				return app.ErrUnavailable
			}
			result = app.MeterInstallResult{AssetID: receipt.AssetID, MeterID: receipt.MeterID, SegmentID: receipt.SegmentID, OpeningReadingID: receipt.OpeningReadingID, AssetRevision: receipt.AssetRevision}
			replay = true
			return s.check(ctx, tx, scope, source, member, v.AssetID, app.MeterInstallAction)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if v.Method != meter.Typed {
			return app.ErrUnavailable
		}
		if _, err := queries.LockMeterAsset(ctx, q.LockMeterAssetParams{OrgID: org, AssetID: v.AssetID}); err != nil {
			return err
		}
		current, err := loadAsset(ctx, tx, scope, v.AssetID)
		if err != nil {
			return err
		}
		if current.Revision != v.ExpectedAssetRevision || current.Revision == math.MaxInt64 || current.State != "DRAFT" && current.State != "ACTIVE" {
			return app.ErrConflict
		}
		repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, audit.Event{OperationID: v.OperationID, ActorID: actor.AccountID, Action: "meter.installed", TargetType: "meter", TargetID: newID, RequestHash: digest}, func(ctx context.Context, tx pgx.Tx) error {
			queries = q.New(tx)
			stamp, sub := meterTime(v.Declared.OccurredAt)
			modulus := int64(0)
			if v.Modulus != nil {
				modulus = *v.Modulus
			}
			if err := queries.InsertMeter(ctx, q.InsertMeterParams{OrgID: org, ID: newID, AssetID: v.AssetID, Kind: string(v.Kind), Label: v.Label, Modulus: modulus, InstalledAt: stamp, InstalledSubmicro: sub, OperationID: v.OperationID}); err != nil {
				return err
			}
			if err := queries.InsertMeterSegment(ctx, q.InsertMeterSegmentParams{OrgID: org, ID: newSegment, MeterID: newID, OpenedAt: stamp, OpenedSubmicro: sub, OpeningReadingID: v.OpeningReadingID}); err != nil {
				return err
			}
			view, err := loadMeter(ctx, tx, scope, v.AssetID, newID, false)
			if err != nil {
				return err
			}
			if _, err := insertObservation(ctx, tx, scope, view, source, member, app.MeterReadingInput{OperationID: v.OperationID, AssetID: v.AssetID, MeterID: newID, SegmentID: newSegment, ReadingID: v.OpeningReadingID, Value: v.Value, Declared: v.Declared, Origin: meter.Installation, Method: v.Method}, digest); err != nil {
				return err
			}
			checked, err := validated(scope, v.AssetID, current.Profile)
			if err != nil {
				return err
			}
			n, err := queries.UpdateAsset(ctx, q.UpdateAssetParams{OrgID: org, ID: v.AssetID, Kind: string(current.Profile.Kind), CategoryID: current.Profile.Category, Name: current.Profile.Name, FleetNumber: current.Profile.FleetNumber, Plate: current.Profile.Plate, SerialNumber: current.Profile.SerialNumber, FleetKey: checked.FleetKey, PlateKey: checked.PlateKey, UsageBasis: string(current.Profile.UsageBasis), State: string(current.State), UnitID: current.UnitID, ExpectedRevision: current.Revision})
			if err != nil {
				return err
			}
			if n != 1 {
				return app.ErrConflict
			}
			current.Revision++
			snapshot, err := snapshotBytes(current)
			if err != nil {
				return err
			}
			if err := queries.InsertAssetRevision(ctx, q.InsertAssetRevisionParams{OrgID: org, AssetID: v.AssetID, Revision: current.Revision, OperationID: v.OperationID, Snapshot: snapshot}); err != nil {
				return err
			}
			if err := queries.InsertMeterCommand(ctx, q.InsertMeterCommandParams{OrgID: org, OperationID: v.OperationID, ActorID: actor.AccountID, AssetID: v.AssetID, MeterID: newID, SegmentID: newSegment, OpeningReadingID: v.OpeningReadingID, RequestHash: digest, AssetRevision: current.Revision}); err != nil {
				return err
			}
			result = app.MeterInstallResult{AssetID: v.AssetID, MeterID: newID, SegmentID: newSegment, OpeningReadingID: v.OpeningReadingID, AssetRevision: current.Revision}
			return nil
		})
		if err != nil {
			return err
		}
		if repeated {
			return app.ErrUnavailable
		}
		return s.check(ctx, tx, scope, source, member, v.AssetID, app.MeterInstallAction)
	})
	if err != nil {
		return app.MeterInstallResult{}, false, meterError(ctx, err)
	}
	return result, replay, nil
}
func (s *MeterStore) ReadLatestMeter(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member, asset, id string) (app.MeterView, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.MeterView{}, err
	}
	if !validID(asset) || !validID(id) {
		return app.MeterView{}, meter.ErrInvalid
	}
	if !s.ready() {
		return app.MeterView{}, app.ErrUnavailable
	}
	source := app.MeterSource{Web: &actor}
	var view app.MeterView
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.check(ctx, tx, scope, source, member, asset, app.MeterReadAction); err != nil {
			return err
		}
		var err error
		view, err = loadMeter(ctx, tx, scope, asset, id, true)
		if err != nil {
			return err
		}
		return s.check(ctx, tx, scope, source, member, asset, app.MeterReadAction)
	})
	if err != nil {
		return app.MeterView{}, meterError(ctx, err)
	}
	return view, nil
}
func (s *MeterStore) ListAssetMeters(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member, asset string, filter app.MeterFilter) (app.MeterPage, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.MeterPage{}, err
	}
	if !validID(asset) {
		return app.MeterPage{}, meter.ErrInvalid
	}
	if err := filter.Validate(); err != nil {
		return app.MeterPage{}, err
	}
	if !s.ready() {
		return app.MeterPage{}, app.ErrUnavailable
	}
	source := app.MeterSource{Web: &actor}
	page := app.MeterPage{Meters: []app.MeterView{}}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.check(ctx, tx, scope, source, member, asset, app.MeterReadAction); err != nil {
			return err
		}
		ids, err := q.New(tx).ListAssetMeterIDs(ctx, q.ListAssetMeterIDsParams{OrgID: scope.OrgID().String(), AssetID: asset, AfterID: filter.AfterID, RowLimit: int32(filter.Limit + 1)})
		if err != nil {
			return err
		}
		more := len(ids) > filter.Limit
		if more {
			ids = ids[:filter.Limit]
		}
		for _, id := range ids {
			view, err := loadMeter(ctx, tx, scope, asset, id, true)
			if err != nil {
				return err
			}
			page.Meters = append(page.Meters, view)
		}
		if more {
			page.NextID = ids[len(ids)-1]
		}
		return s.check(ctx, tx, scope, source, member, asset, app.MeterReadAction)
	})
	if err != nil {
		return app.MeterPage{}, meterError(ctx, err)
	}
	return page, nil
}
func (s *MeterStore) RecordEmbedded(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source app.MeterSource, member string, command app.MeterReadingCommand) (app.MeterReadingResult, bool, error) {
	db = syncapp.Borrow(db)
	source = source.Clone()
	if source.Validate(scope) != nil || !validID(member) {
		return app.MeterReadingResult{}, false, app.ErrUnauthenticated
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.MeterReadingResult{}, false, err
	}
	if db == nil || s == nil || s.Authority == nil {
		return app.MeterReadingResult{}, false, app.ErrUnavailable
	}
	if err := database.ValidateOrgDBTX(ctx, db, scope); err != nil {
		return app.MeterReadingResult{}, false, app.ErrUnavailable
	}
	if err := s.check(ctx, db, scope, source, member, v.AssetID, app.MeterRecordAction); err != nil {
		return app.MeterReadingResult{}, false, meterError(ctx, err)
	}
	queries := q.New(db)
	org := scope.OrgID().String()
	if _, err := queries.AssetRootLock(ctx, org); err != nil {
		return app.MeterReadingResult{}, false, meterError(ctx, err)
	}
	if _, err := queries.LockMeterAsset(ctx, q.LockMeterAssetParams{OrgID: org, AssetID: v.AssetID}); err != nil {
		return app.MeterReadingResult{}, false, meterError(ctx, err)
	}
	if _, err := queries.LockMeter(ctx, q.LockMeterParams{OrgID: org, AssetID: v.AssetID, ID: v.MeterID}); err != nil {
		return app.MeterReadingResult{}, false, meterError(ctx, err)
	}
	view, err := loadMeter(ctx, db, scope, v.AssetID, v.MeterID, false)
	if err != nil {
		return app.MeterReadingResult{}, false, meterError(ctx, err)
	}
	if view.Segment.ID() != v.SegmentID {
		return app.MeterReadingResult{}, false, app.ErrNotFound
	}
	digest := command.Digest(scope)
	row, err := queries.GetMeterReadingByOperation(ctx, q.GetMeterReadingByOperationParams{OrgID: org, MeterID: v.MeterID, OperationID: v.OperationID})
	replay := false
	var reading meter.Reading
	if err == nil {
		capture, e := source.Capture(scope)
		if e != nil || row.RequestHash != digest || row.ID != v.ReadingID || row.AuthorMembershipID != member || row.CaptureKind != string(capture.Kind()) || row.CaptureID != capture.ID() {
			return app.MeterReadingResult{}, false, app.ErrOperationReused
		}
		reading, err = meterReading(scope, view, row)
		replay = true
	} else if errors.Is(err, pgx.ErrNoRows) {
		if v.Method != meter.Typed {
			return app.MeterReadingResult{}, false, app.ErrUnavailable
		}
		reading, err = insertObservation(ctx, db, scope, view, source, member, v, digest)
	}
	if err != nil {
		return app.MeterReadingResult{}, false, meterError(ctx, err)
	}
	if err := s.check(ctx, db, scope, source, member, v.AssetID, app.MeterRecordAction); err != nil {
		return app.MeterReadingResult{}, false, meterError(ctx, err)
	}
	return app.MeterReadingResult{Reading: reading}, replay, nil
}
func (s *MeterStore) ReadMeterTargetTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, asset, id, segment string) (app.MeterView, error) {
	db = syncapp.Borrow(db)
	if db == nil || database.ValidateOrgDBTX(ctx, db, scope) != nil {
		return app.MeterView{}, app.ErrUnavailable
	}
	for _, id := range []string{asset, id, segment} {
		if !validID(id) {
			return app.MeterView{}, meter.ErrInvalid
		}
	}
	view, err := loadMeter(ctx, db, scope, asset, id, false)
	if err != nil {
		return app.MeterView{}, meterError(ctx, err)
	}
	if view.Segment.ID() != segment {
		return app.MeterView{}, app.ErrNotFound
	}
	return view, nil
}
func (s *MeterStore) ReadMeterReadingTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, asset, id, segment, reading string) (meter.Reading, error) {
	view, err := s.ReadMeterTargetTx(ctx, db, scope, asset, id, segment)
	if err != nil {
		return meter.Reading{}, err
	}
	if !validID(reading) {
		return meter.Reading{}, meter.ErrInvalid
	}
	row, err := q.New(db).GetMeterReading(ctx, q.GetMeterReadingParams{OrgID: scope.OrgID().String(), MeterID: id, SegmentID: segment, ID: reading})
	if err != nil {
		return meter.Reading{}, meterError(ctx, err)
	}
	return meterReading(scope, view, q.GetMeterReadingByOperationRow(row))
}
