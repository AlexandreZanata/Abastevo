// Package adapters persists organization trips and owns the signed
// trip.start and trip.finish intake handlers (EMP-TRIP-002).
package adapters

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/trip"
	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Store is the trip persistence owner. Allocations is the FLEET-005
// snapshot port read on the borrowed intake transaction; NewID names review
// items of held facts.
type Store struct {
	Allocations fleetapp.AllocationSnapshotPort
	NewID       func() (string, error)
}

var _ app.Store = (*Store)(nil)

func absent(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}

func canonicalID(raw string) bool {
	v, e := kernel.ParseOrgID(raw)
	return e == nil && v.String() == raw
}

func (s *Store) port(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope) (*q.Queries, syncapp.DBTX, error) {
	db = syncapp.Borrow(db)
	if s == nil || db == nil || database.ValidateOrgDBTX(ctx, db, scope) != nil {
		return nil, nil, app.ErrUnavailable
	}
	return q.New(db), db, nil
}

func storeError(err error) error {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return app.ErrNotFound
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	}
	return app.ErrUnavailable
}

func instant(at time.Time) (pgtype.Timestamptz, int16) {
	at = at.UTC()
	return pgtype.Timestamptz{Time: at.Truncate(time.Microsecond), Valid: true}, int16(at.Nanosecond() % 1000)
}

func exact(at pgtype.Timestamptz, sub int16) (time.Time, bool) {
	if !at.Valid || at.InfinityModifier != pgtype.Finite || at.Time.Nanosecond()%1000 != 0 || sub < 0 || sub > 999 {
		return time.Time{}, false
	}
	return at.Time.UTC().Add(time.Duration(sub)), true
}

func text(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *v, Valid: true}
}

func id(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func optionalText(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}

func clockColumns(c *trip.RunClock) (pgtype.Text, pgtype.Int8) {
	if c == nil {
		return pgtype.Text{}, pgtype.Int8{}
	}
	return pgtype.Text{String: c.BootID(), Valid: true}, pgtype.Int8{Int64: c.ElapsedMillis(), Valid: true}
}

func clockOf(boot pgtype.Text, elapsed pgtype.Int8) (*trip.RunClock, bool) {
	if boot.Valid != elapsed.Valid {
		return nil, false
	}
	if !boot.Valid {
		return nil, true
	}
	c, err := trip.NewRunClock(boot.String, elapsed.Int64)
	return &c, err == nil
}

func day(raw string) (pgtype.Date, bool) {
	at, err := time.Parse("2006-01-02", raw)
	return pgtype.Date{Time: at, Valid: true}, err == nil
}

// AuthorEmployeeTx returns the author membership's employee record.
func (s *Store) AuthorEmployeeTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, membership string) (string, error) {
	queries, _, err := s.port(ctx, db, scope)
	if err != nil {
		return "", err
	}
	employee, err := queries.TripAuthorEmployee(ctx, q.TripAuthorEmployeeParams{OrgID: scope.OrgID().String(), MembershipID: membership})
	if err != nil {
		return "", storeError(err)
	}
	return employee, nil
}

// PrepareStartTx reads the admission facts. The handler holds the root and
// asset locks (the fleet disposal lookup) before calling it.
func (s *Store) PrepareStartTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, command app.StartCommand) (app.StartFacts, error) {
	queries, db, err := s.port(ctx, db, scope)
	if err != nil || absent(s.Allocations) {
		return app.StartFacts{}, app.ErrUnavailable
	}
	v, err := command.Snapshot()
	if err != nil || source.MembershipID != v.AuthorMembershipID {
		return app.StartFacts{}, app.ErrUnavailable
	}
	employee, err := s.AuthorEmployeeTx(ctx, db, scope, source.MembershipID)
	if err != nil {
		return app.StartFacts{}, err
	}
	org := scope.OrgID().String()
	row, err := queries.TripStartFacts(ctx, q.TripStartFactsParams{OrgID: org, AssetID: v.AssetID, DriverID: v.DriverID, TripID: v.TripID})
	if err != nil {
		return app.StartFacts{}, storeError(err)
	}
	facts := app.StartFacts{AuthorEmployeeID: employee, AssetState: row.AssetState, ExistingOperation: row.ExistingOperation, HeldOperation: row.HeldOperation,
		ActiveTripID: row.ActiveTrip, DriverActive: row.DriverActive, Zones: orgtime.ReportingZones{Organization: row.OrganizationZone}}
	if row.DisposedAt.Valid {
		at, ok := exact(row.DisposedAt, row.DisposedSubmicro)
		if !ok {
			return app.StartFacts{}, app.ErrUnavailable
		}
		facts.DisposedAt = &at
	}
	// BR-AUTH-005 (B-BR-EMP-312): the allocation at the declared instant.
	snapshot, err := s.Allocations.UnitAndCostCentreAtDBTX(ctx, db, scope, v.AssetID, v.Declared.OccurredAt)
	switch {
	case err == nil:
		if snapshot.AssetID != v.AssetID {
			return app.StartFacts{}, app.ErrUnavailable
		}
		facts.UnitID, facts.CostCentreID = snapshot.UnitID, snapshot.CostCentreID
	case errors.Is(err, fleetapp.ErrUnallocated):
	case errors.Is(err, fleetapp.ErrNotFound):
		return app.StartFacts{}, app.ErrNotFound
	default:
		return app.StartFacts{}, app.ErrUnavailable
	}
	if facts.UnitID != "" {
		zone, err := queries.TripUnitZone(ctx, q.TripUnitZoneParams{OrgID: org, UnitID: facts.UnitID})
		if err != nil {
			return app.StartFacts{}, storeError(err)
		}
		facts.Zones.Unit = &zone
	}
	return facts, nil
}

// InsertTripTx writes the start fact of the validated domain trip.
func (s *Store) InsertTripTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, command app.StartCommand, facts app.StartFacts, started trip.Trip) error {
	queries, _, err := s.port(ctx, db, scope)
	if err != nil {
		return err
	}
	v, err := command.Snapshot()
	rep := started.Reporting()
	reportingDay, ok := day(rep.Day)
	if err != nil || !ok || source.MembershipID != v.AuthorMembershipID || facts.AuthorEmployeeID == "" || started.Reference().ID() != v.TripID || started.State() != trip.InProgress {
		return app.ErrUnavailable
	}
	at, sub := instant(v.Declared.OccurredAt)
	boot, elapsed := clockColumns(v.Clock)
	params := q.InsertTripParams{OrgID: scope.OrgID().String(), ID: v.TripID, OperationID: v.OperationID, PayloadDigest: v.PayloadDigest, AssetID: v.AssetID,
		DriverEmployeeID: v.DriverID, AuthorMembershipID: source.MembershipID, AuthorEmployeeID: facts.AuthorEmployeeID, EnrollmentID: source.EnrollmentID,
		OccurredAt: at, OccurredSubmicro: sub, EntryZone: v.Declared.EntryZone, DeviceOffsetSeconds: v.Declared.DeviceOffsetSeconds, BootID: boot, ElapsedMillis: elapsed,
		MissingReadingReason: text(v.MissingReason), UnitID: id(facts.UnitID), CostCentreID: id(facts.CostCentreID),
		ReportingDay: reportingDay, ReportingZone: rep.Zone, ReportingSource: rep.Source}
	if r := v.Reading; r != nil {
		params.MeterID, params.SegmentID, params.ReadingID = id(r.MeterID), id(r.SegmentID), id(r.ReadingID)
	}
	if err := queries.InsertTrip(ctx, params); err != nil {
		return storeError(err)
	}
	return nil
}

// ReadTripTx reads one stored trip of the scope with its finish.
func (s *Store) ReadTripTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, tripID string) (app.Record, error) {
	queries, _, err := s.port(ctx, db, scope)
	if err != nil {
		return app.Record{}, err
	}
	row, err := queries.GetTrip(ctx, q.GetTripParams{OrgID: scope.OrgID().String(), ID: tripID})
	if err != nil {
		return app.Record{}, storeError(err)
	}
	at, ok := exact(row.OccurredAt, row.OccurredSubmicro)
	clock, okClock := clockOf(row.BootID, row.ElapsedMillis)
	state, err := trip.ParseState(row.State)
	if !ok || !okClock || err != nil || !row.ReceivedAt.Valid || (row.ReadingID != "") != row.ReadingValue.Valid {
		return app.Record{}, app.ErrUnavailable
	}
	record := app.Record{ID: row.ID, OperationID: row.OperationID, PayloadDigest: row.PayloadDigest, AssetID: row.AssetID, DriverID: row.DriverEmployeeID,
		AuthorMembershipID: row.AuthorMembershipID, AuthorEmployeeID: row.AuthorEmployeeID, EnrollmentID: row.EnrollmentID,
		Declared: orgtime.Declaration{OccurredAt: at, EntryZone: row.EntryZone, DeviceOffsetSeconds: row.DeviceOffsetSeconds}, Clock: clock,
		ReceivedAt: row.ReceivedAt.Time.UTC(), MeterID: row.MeterID, SegmentID: row.SegmentID, ReadingID: row.ReadingID, ReadingKind: row.ReadingKind,
		MissingReason: optionalText(row.MissingReadingReason), UnitID: row.UnitID, CostCentreID: row.CostCentreID,
		Reporting: orgtime.ReportingAttribution{Day: row.ReportingDay, Zone: row.ReportingZone, Source: row.ReportingSource}, State: state}
	if row.ReadingValue.Valid {
		n := row.ReadingValue.Int64
		record.ReadingValue = &n
	}
	if row.FinishOperationID == "" {
		if state != trip.InProgress {
			return app.Record{}, app.ErrUnavailable
		}
		return record, nil
	}
	finishAt, ok := exact(row.FinishOccurredAt, row.FinishOccurredSubmicro.Int16)
	finishClock, okClock := clockOf(row.FinishBootID, row.FinishElapsedMillis)
	if !ok || !okClock || state != trip.Completed || !row.FinishOccurredSubmicro.Valid || !row.FinishEntryZone.Valid || !row.FinishDeviceOffsetSeconds.Valid || !row.FinishReceivedAt.Valid {
		return app.Record{}, app.ErrUnavailable
	}
	record.Finish = &app.FinishRecord{OperationID: row.FinishOperationID, PayloadDigest: row.FinishPayloadDigest, AuthorMembershipID: row.FinishAuthorMembershipID,
		EnrollmentID: row.FinishEnrollmentID, Declared: orgtime.Declaration{OccurredAt: finishAt, EntryZone: row.FinishEntryZone.String, DeviceOffsetSeconds: row.FinishDeviceOffsetSeconds.Int64},
		Clock: finishClock, ReceivedAt: row.FinishReceivedAt.Time.UTC(), MeterID: row.FinishMeterID, SegmentID: row.FinishSegmentID, ReadingID: row.FinishReadingID,
		MissingReason: optionalText(row.FinishMissingReadingReason), EndDay: row.FinishEndDay}
	return record, nil
}

// InsertFinishTx writes the finish fact; its insert completes the trip.
func (s *Store) InsertFinishTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, command app.FinishCommand, endDay string) error {
	queries, _, err := s.port(ctx, db, scope)
	if err != nil {
		return err
	}
	v, err := command.Snapshot()
	end, ok := day(endDay)
	if err != nil || !ok || source.MembershipID != v.AuthorMembershipID {
		return app.ErrUnavailable
	}
	at, sub := instant(v.Declared.OccurredAt)
	boot, elapsed := clockColumns(v.Clock)
	params := q.InsertTripFinishParams{OrgID: scope.OrgID().String(), TripID: v.TripID, OperationID: v.OperationID, PayloadDigest: v.PayloadDigest,
		AuthorMembershipID: source.MembershipID, EnrollmentID: source.EnrollmentID, OccurredAt: at, OccurredSubmicro: sub, EntryZone: v.Declared.EntryZone,
		DeviceOffsetSeconds: v.Declared.DeviceOffsetSeconds, BootID: boot, ElapsedMillis: elapsed, MissingReadingReason: text(v.MissingReason), EndDay: end}
	if r := v.Reading; r != nil {
		params.MeterID, params.SegmentID, params.ReadingID = id(r.MeterID), id(r.SegmentID), id(r.ReadingID)
	}
	if err := queries.InsertTripFinish(ctx, params); err != nil {
		return storeError(err)
	}
	return nil
}

// heldBinding proves that an operation is exactly the source's own.
func heldBinding(source syncapp.Source, op protocol.Operation) bool {
	v := op.Snapshot()
	return source.Validate() == nil && v.OwnerAccountID == source.Actor.AccountID && v.APIOrigin == source.Origin && v.OrgID == source.Scope.OrgID().String() &&
		v.MembershipID == source.MembershipID && v.EnrollmentID == source.EnrollmentID
}

func heldCode(kind, code string) bool {
	if kind == app.StartOperationType {
		return code == app.AssetBusy || code == app.AssetUnavailable || code == app.PermissionChanged
	}
	return kind == app.FinishOperationType && code == app.TripAlreadyClosed
}

// sameHeld compares a stored held fact with the operation's parsed event.
func sameHeld(source syncapp.Source, op protocol.Operation, row q.ReadTripHeldProofRow, e app.Event, asset, driver string) bool {
	v := op.Snapshot()
	at, ok := exact(row.OccurredAt, row.OccurredSubmicro)
	clock, okClock := clockOf(row.BootID, row.ElapsedMillis)
	reading := e.Reading == nil && row.ReadingID == "" && row.MeterID == "" && row.SegmentID == "" && !row.ReadingValue.Valid ||
		e.Reading != nil && row.ReadingID == e.Reading.ReadingID && row.MeterID == e.Reading.MeterID && row.SegmentID == e.Reading.SegmentID && row.ReadingValue.Valid && row.ReadingValue.Int64 == e.Reading.Value.Count()
	reason := e.MissingReason == nil && !row.MissingReadingReason.Valid || e.MissingReason != nil && row.MissingReadingReason.Valid && row.MissingReadingReason.String == *e.MissingReason
	sameClock := clock == nil && e.Clock == nil || clock != nil && e.Clock != nil && *clock == *e.Clock
	return ok && okClock && reading && reason && sameClock && row.ProofVersion == 1 && canonicalID(row.ID) && canonicalID(row.AuthorEmployeeID) &&
		row.OperationType == v.Type && row.OperationSchema == v.OperationSchema && row.PayloadDigest == op.PayloadDigest() && row.OwnerAccountID == source.Actor.AccountID &&
		row.ApiOrigin == source.Origin && row.MembershipID == source.MembershipID && row.EnrollmentID == source.EnrollmentID && row.EntityType == app.EntityType &&
		row.EntityID == e.TripID && row.TripID == e.TripID && row.AssetID == asset && row.DriverEmployeeID == driver && heldCode(v.Type, row.Code) &&
		at.Equal(e.Declared.OccurredAt) && row.EntryZone == e.Declared.EntryZone && row.DeviceOffsetSeconds == e.Declared.DeviceOffsetSeconds
}

// parsedHeld decodes a held operation of either type into its event.
func parsedHeld(op protocol.Operation) (app.Event, string, string, error) {
	switch op.Snapshot().Type {
	case app.StartOperationType:
		command, err := app.ParseStart(op)
		if err != nil {
			return app.Event{}, "", "", app.ErrUnavailable
		}
		v, err := command.Snapshot()
		return v.Event, v.AssetID, v.DriverID, err
	case app.FinishOperationType:
		command, err := app.ParseFinish(op)
		if err != nil {
			return app.Event{}, "", "", app.ErrUnavailable
		}
		v, err := command.Snapshot()
		return v.Event, "", "", err
	}
	return app.Event{}, "", "", app.ErrUnavailable
}

// hold stores one held fact and its version-1 review item (EDR-204).
func (s *Store) hold(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, e app.Event, asset, driver, code string) (string, error) {
	db = syncapp.Borrow(db)
	if s == nil || s.NewID == nil || db == nil || database.ValidateOrgDBTX(ctx, db, source.Scope) != nil || !heldBinding(source, op) || !heldCode(op.Snapshot().Type, code) {
		return "", app.ErrUnavailable
	}
	queries := q.New(db)
	org := source.Scope.OrgID().String()
	row, err := queries.ReadTripHeldProof(ctx, q.ReadTripHeldProofParams{OrgID: org, OperationID: op.ID()})
	if err == nil {
		if !sameHeld(source, op, row, e, asset, driver) || row.Code != code {
			return "", app.ErrUnavailable
		}
		return row.ID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", app.ErrUnavailable
	}
	review, err := s.NewID()
	if err != nil || !canonicalID(review) {
		return "", app.ErrUnavailable
	}
	if n, err := queries.InsertTripReviewItem(ctx, q.InsertTripReviewItemParams{OrgID: org, ID: review, OperationID: op.ID(), Code: code}); err != nil || n != 1 {
		return "", app.ErrUnavailable
	}
	at, sub := instant(e.Declared.OccurredAt)
	boot, elapsed := clockColumns(e.Clock)
	var meterID, segmentID, readingID pgtype.Text
	var value pgtype.Int8
	if r := e.Reading; r != nil {
		meterID, segmentID, readingID, value = id(r.MeterID), id(r.SegmentID), id(r.ReadingID), pgtype.Int8{Int64: r.Value.Count(), Valid: true}
	}
	if op.Snapshot().Type == app.StartOperationType {
		err = queries.InsertTripHeldStart(ctx, q.InsertTripHeldStartParams{OrgID: org, ReviewItemID: review, OperationID: op.ID(), TripID: e.TripID, AssetID: asset,
			DriverEmployeeID: driver, OccurredAt: at, OccurredSubmicro: sub, EntryZone: e.Declared.EntryZone, DeviceOffsetSeconds: e.Declared.DeviceOffsetSeconds,
			BootID: boot, ElapsedMillis: elapsed, MeterID: meterID, SegmentID: segmentID, ReadingID: readingID, ReadingValue: value, MissingReadingReason: text(e.MissingReason)})
	} else {
		err = queries.InsertTripHeldFinish(ctx, q.InsertTripHeldFinishParams{OrgID: org, ReviewItemID: review, OperationID: op.ID(), TripID: e.TripID,
			OccurredAt: at, OccurredSubmicro: sub, EntryZone: e.Declared.EntryZone, DeviceOffsetSeconds: e.Declared.DeviceOffsetSeconds,
			BootID: boot, ElapsedMillis: elapsed, MeterID: meterID, SegmentID: segmentID, ReadingID: readingID, ReadingValue: value, MissingReadingReason: text(e.MissingReason)})
	}
	if err != nil {
		return "", storeError(err)
	}
	return review, nil
}

// HoldStartTx keeps a second admissible start as a held fact (B-BR-EMP-313).
func (s *Store) HoldStartTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, command app.StartCommand, code string) (string, error) {
	v, err := command.Snapshot()
	if err != nil || op.Snapshot().Type != app.StartOperationType || v.OperationID != op.ID() {
		return "", app.ErrUnavailable
	}
	return s.hold(ctx, db, source, op, v.Event, v.AssetID, v.DriverID, code)
}

// HoldFinishTx keeps the finish of a completed trip (B-BR-EMP-315).
func (s *Store) HoldFinishTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, command app.FinishCommand, code string) (string, error) {
	v, err := command.Snapshot()
	if err != nil || op.Snapshot().Type != app.FinishOperationType || v.OperationID != op.ID() {
		return "", app.ErrUnavailable
	}
	return s.hold(ctx, db, source, op, v.Event, "", "", code)
}

// CheckHeldTx proves a held outcome against the stored held fact and, once
// written, its exact receipt (OFFL-008 ReviewProof).
func (s *Store) CheckHeldTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, outcome syncapp.Outcome) error {
	db = syncapp.Borrow(db)
	if s == nil || db == nil || database.ValidateOrgDBTX(ctx, db, source.Scope) != nil || !heldBinding(source, op) {
		return app.ErrUnavailable
	}
	e, asset, driver, err := parsedHeld(op)
	if err != nil {
		return app.ErrUnavailable
	}
	row, err := q.New(db).ReadTripHeldProof(ctx, q.ReadTripHeldProofParams{OrgID: source.Scope.OrgID().String(), OperationID: op.ID()})
	if err != nil || !sameHeld(source, op, row, e, asset, driver) || outcome.Result != protocol.AcceptedForReview || outcome.Code != row.Code || outcome.ReviewItemID != row.ID || outcome.EntityRevision != nil {
		return app.ErrUnavailable
	}
	if len(row.CanonicalAck) != 0 {
		return heldReceipt(op, row)
	}
	if row.AcknowledgementResult != "" || row.AcknowledgementProofVersion != 0 || row.ReceivedAt.Valid {
		return app.ErrUnavailable
	}
	return nil
}

func heldReceipt(op protocol.Operation, row q.ReadTripHeldProofRow) error {
	if row.AcknowledgementResult != protocol.AcceptedForReview || row.AcknowledgementCode != row.Code || row.AcknowledgementReviewItemID != row.ID || row.AcknowledgementProofVersion != 1 {
		return app.ErrUnavailable
	}
	ack, err := protocol.ParseAcknowledgement(row.CanonicalAck, op, protocol.DefaultLimits())
	if err != nil || !bytes.Equal(ack.CanonicalBytes(), row.CanonicalAck) {
		return app.ErrUnavailable
	}
	v := ack.Snapshot()
	received, ok := exact(row.ReceivedAt, 0)
	if !ok || v.Result != protocol.AcceptedForReview || v.Code != row.Code || v.ReviewItemID != row.ID || v.ReceivedAt != orgtime.FormatInstant(received) {
		return app.ErrUnavailable
	}
	return nil
}

// CheckOwnReceiptTx lets the author re-read its own held receipt without the
// record read permission (OFFL-008); anything else is not an own receipt.
func (s *Store) CheckOwnReceiptTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation) (bool, error) {
	db = syncapp.Borrow(db)
	if s == nil || db == nil || source.Validate() != nil || database.ValidateOrgDBTX(ctx, db, source.Scope) != nil {
		return false, app.ErrUnavailable
	}
	row, err := q.New(db).ReadTripHeldProof(ctx, q.ReadTripHeldProofParams{OrgID: source.Scope.OrgID().String(), OperationID: op.ID()})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, app.ErrUnavailable
	}
	e, asset, driver, err := parsedHeld(op)
	if err != nil {
		return false, app.ErrUnavailable
	}
	if !heldBinding(source, op) || !sameHeld(source, op, row, e, asset, driver) {
		return false, app.ErrNotFound
	}
	if err := heldReceipt(op, row); err != nil {
		return false, err
	}
	return true, nil
}

// OpenWork is the trip-backed FLEET-003 open-work source (EDR-206): an
// IN_PROGRESS trip of the asset is TRIP open work.
type OpenWork struct{}

var _ fleetapp.AssetOpenWork = OpenWork{}

func (OpenWork) OpenAssetWorkTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, asset string) ([]fleetapp.OpenWorkKind, error) {
	if tx == nil || scope.Validate() != nil || !canonicalID(asset) || database.ValidateOrgDBTX(ctx, tx, scope) != nil {
		return nil, fleetapp.ErrUnavailable
	}
	open, err := q.New(tx).TripInProgressOnAsset(ctx, q.TripInProgressOnAssetParams{OrgID: scope.OrgID().String(), AssetID: asset})
	if err != nil {
		return nil, fleetapp.ErrUnavailable
	}
	if open {
		return []fleetapp.OpenWorkKind{fleetapp.OpenTrip}, nil
	}
	return nil, nil
}

// ActiveUse is the trip-backed FLEET-007 active-use source (EDR-206): the
// stored trips holding the asset at an instant (started at or before it and
// not finished at or before it).
type ActiveUse struct{}

var _ fleetapp.ActiveUsePort = ActiveUse{}

func (ActiveUse) ActiveUseDBTX(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, asset string, at time.Time) (fleetapp.UseAnswer, error) {
	db = syncapp.Borrow(db)
	if db == nil || scope.Validate() != nil || !canonicalID(asset) || at.IsZero() || database.ValidateOrgDBTX(ctx, db, scope) != nil {
		return fleetapp.UseAnswer{}, fleetapp.ErrUnavailable
	}
	stamp, sub := instant(at)
	rows, err := q.New(db).TripUsesOfAssetAt(ctx, q.TripUsesOfAssetAtParams{OrgID: scope.OrgID().String(), AssetID: asset, At: stamp, AtSubmicro: sub})
	if err != nil {
		return fleetapp.UseAnswer{}, fleetapp.ErrUnavailable
	}
	answer := fleetapp.UseAnswer{Sourced: true, Uses: make([]fleet.UseFact, 0, len(rows))}
	for _, row := range rows {
		from, ok := exact(row.OccurredAt, row.OccurredSubmicro)
		if !ok {
			return fleetapp.UseAnswer{}, fleetapp.ErrUnavailable
		}
		answer.Uses = append(answer.Uses, fleet.UseFact{ID: row.ID, From: from})
	}
	return answer, nil
}

// ActiveTripsOfAssetAtDBTX lists the identities of the trips holding the
// asset at an instant (the ActiveUse answer).
func ActiveTripsOfAssetAtDBTX(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, asset string, at time.Time) ([]string, error) {
	answer, err := ActiveUse{}.ActiveUseDBTX(ctx, db, scope, asset, at)
	if err != nil {
		return nil, app.ErrUnavailable
	}
	ids := make([]string, 0, len(answer.Uses))
	for _, use := range answer.Uses {
		ids = append(ids, use.ID)
	}
	return ids, nil
}
