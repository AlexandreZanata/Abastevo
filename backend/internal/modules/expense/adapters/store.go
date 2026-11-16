// Package adapters persists organization expenses, projects each stored
// fueling into its one expense and owns the signed expense.submit intake
// handler (EMP-EXP-002).
package adapters

import (
	"context"
	"errors"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/expense"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/domain"
	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Store is the expense persistence owner. Allocations is the FLEET-005
// snapshot port read on the borrowed intake transaction; NewID mints the
// identity of a projected FUELING expense.
type Store struct {
	Allocations fleetapp.AllocationSnapshotPort
	NewID       func() (string, error)
}

var _ app.Store = (*Store)(nil)

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

func instant(at pgtype.Timestamptz, submicro int16) (time.Time, bool) {
	if !at.Valid || at.InfinityModifier != pgtype.Finite || at.Time.Nanosecond()%1000 != 0 || submicro < 0 || submicro > 999 {
		return time.Time{}, false
	}
	return at.Time.UTC().Add(time.Duration(submicro)), true
}

func optional(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

// zones reads the organization zone and, when the record names a unit, the
// unit zone (BR-TIME-004); an unknown unit is unavailable, never a fallback.
func zones(ctx context.Context, queries *q.Queries, scope kernel.OrgScope, unit string) (orgtime.ReportingZones, error) {
	row, err := queries.ExpenseReportingZones(ctx, q.ExpenseReportingZonesParams{OrgID: scope.OrgID().String(), UnitID: optional(unit)})
	if err != nil {
		return orgtime.ReportingZones{}, storeError(err)
	}
	if !row.OrganizationZone.Valid {
		return orgtime.ReportingZones{}, app.ErrUnavailable
	}
	z := orgtime.ReportingZones{Organization: row.OrganizationZone.String}
	if unit != "" {
		if row.UnitZone == "" {
			return orgtime.ReportingZones{}, app.ErrUnavailable
		}
		z.Unit = &row.UnitZone
	}
	return z, nil
}

func (s *Store) insert(ctx context.Context, queries *q.Queries, scope kernel.OrgScope, r app.Record) (bool, error) {
	day, err := time.Parse("2006-01-02", r.Reporting.Day)
	if err != nil {
		return false, app.ErrUnavailable
	}
	occurred := r.Declared.OccurredAt.UTC()
	rows, err := queries.InsertExpense(ctx, q.InsertExpenseParams{OrgID: scope.OrgID().String(), ID: r.ID, OriginKind: r.OriginKind, SourceID: r.SourceID,
		FuelingID: optional(r.FuelingID), SourceRevision: r.SourceRevision, OperationID: r.OperationID, PayloadDigest: r.PayloadDigest, AssetID: r.AssetID,
		AuthorMembershipID: r.AuthorMembershipID, AuthorEmployeeID: r.AuthorEmployeeID, EnrollmentID: r.EnrollmentID, Category: r.Category,
		PaidCentavos: r.Paid, CompanyCentavos: r.Company, Payer: r.Payer,
		OccurredAt: pgtype.Timestamptz{Time: occurred.Truncate(time.Microsecond), Valid: true}, OccurredSubmicro: int16(occurred.Nanosecond() % 1000),
		EntryZone: r.Declared.EntryZone, DeviceOffsetSeconds: r.Declared.DeviceOffsetSeconds, ReportingDay: pgtype.Date{Time: day, Valid: true},
		ReportingZone: r.Reporting.Zone, ReportingSource: r.Reporting.Source, UnitID: optional(r.UnitID), CostCentreID: optional(r.CostCentreID),
		ReviewState: r.ReviewState, Revision: r.Revision})
	if err != nil {
		return false, storeError(err)
	}
	return rows == 1, nil
}

// byOrigin reads the stored expense of one FUELING origin, if any.
func (s *Store) byOrigin(ctx context.Context, queries *q.Queries, db syncapp.DBTX, scope kernel.OrgScope, fueling string) (*app.Record, error) {
	id, err := queries.ExpenseIDByOrigin(ctx, q.ExpenseIDByOriginParams{OrgID: scope.OrgID().String(), OriginKind: string(domain.KindFueling), SourceID: fueling})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, storeError(err)
	}
	record, err := s.ReadExpenseTx(ctx, db, scope, id)
	if err != nil {
		return nil, app.ErrUnavailable
	}
	return &record, nil
}

// ProjectFuelingTx stores the one FUELING expense of a stored fueling at the
// named fueling revision, or proves the stored one equal (EDR-287/290).
func (s *Store) ProjectFuelingTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, fueling string, revision int64) error {
	queries, db, err := s.port(ctx, db, scope)
	if err != nil || s.NewID == nil {
		return app.ErrUnavailable
	}
	_, err = s.project(ctx, queries, db, scope, fueling, revision)
	return err
}

// project reports whether this call inserted the cost.
func (s *Store) project(ctx context.Context, queries *q.Queries, db syncapp.DBTX, scope kernel.OrgScope, fueling string, revision int64) (bool, error) {
	row, err := queries.ExpenseFuelingSource(ctx, q.ExpenseFuelingSourceParams{OrgID: scope.OrgID().String(), ID: fueling})
	if err != nil {
		return false, storeError(err)
	}
	switch {
	case revision < row.Revision:
		return false, app.ErrStale
	case revision > row.Revision:
		return false, app.ErrUnavailable
	}
	at, ok := instant(row.OccurredAt, row.OccurredSubmicro)
	if !ok {
		return false, app.ErrUnavailable
	}
	source := app.FuelingSource{ID: row.ID, OperationID: row.OperationID, PayloadDigest: row.PayloadDigest, Revision: row.Revision, AssetID: row.AssetID,
		AuthorMembershipID: row.AuthorMembershipID, AuthorEmployeeID: row.AuthorEmployeeID, EnrollmentID: row.EnrollmentID,
		Declared: orgtime.Declaration{OccurredAt: at, EntryZone: row.EntryZone, DeviceOffsetSeconds: row.DeviceOffsetSeconds},
		Paid:     row.PaidCentavos, Payer: row.Payer, UnitID: row.UnitID, CostCentreID: row.CostCentreID, ReviewState: row.ReviewState}
	z, err := zones(ctx, queries, scope, row.UnitID)
	if err != nil {
		return false, err
	}
	stored, err := s.byOrigin(ctx, queries, db, scope, fueling)
	if err != nil {
		return false, err
	}
	id := ""
	if stored != nil {
		id = stored.ID
	} else if id, err = s.NewID(); err != nil {
		return false, app.ErrUnavailable
	}
	requested, err := app.ProjectFueling(scope, id, source, z)
	if err != nil {
		return false, err
	}
	decision, err := app.DecideProjection(requested, stored)
	if err != nil || decision == app.Unchanged {
		return false, err
	}
	inserted, err := s.insert(ctx, queries, scope, requested)
	if err != nil || inserted {
		return inserted, err
	}
	// A concurrent projection won the origin key: the stored cost decides.
	if stored, err = s.byOrigin(ctx, queries, db, scope, fueling); err != nil || stored == nil {
		return false, app.ErrUnavailable
	}
	requested.ID = stored.ID
	if decision, err = app.DecideProjection(requested, stored); err != nil || decision != app.Unchanged {
		return false, app.ErrUnavailable
	}
	return false, nil
}

// ProjectPendingFuelingsTx is the idempotent per-organization catch-up for
// fuelings stored before migration 000244 (EDR-287). It returns how many
// expenses it inserted.
func (s *Store) ProjectPendingFuelingsTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope) (int, error) {
	queries, db, err := s.port(ctx, db, scope)
	if err != nil || s.NewID == nil {
		return 0, app.ErrUnavailable
	}
	pending, err := queries.PendingFuelingCosts(ctx, scope.OrgID().String())
	if err != nil {
		return 0, storeError(err)
	}
	count := 0
	for _, row := range pending {
		inserted, err := s.project(ctx, queries, db, scope, row.ID, row.Revision)
		if err != nil {
			return 0, err
		}
		if inserted {
			count++
		}
	}
	return count, nil
}

// AuthorEmployeeTx returns the author membership's employee record.
func (s *Store) AuthorEmployeeTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, membership string) (string, error) {
	queries, _, err := s.port(ctx, db, scope)
	if err != nil {
		return "", err
	}
	employee, err := queries.ExpenseAuthorEmployee(ctx, q.ExpenseAuthorEmployeeParams{OrgID: scope.OrgID().String(), MembershipID: membership})
	if err != nil {
		return "", storeError(err)
	}
	return employee, nil
}

// PrepareExpenseTx reads the admission facts of a direct expense under the
// root and asset locks the handler already holds.
func (s *Store) PrepareExpenseTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, command app.Command) (app.Prepared, error) {
	queries, db, err := s.port(ctx, db, scope)
	if err != nil || absent(s.Allocations) {
		return app.Prepared{}, app.ErrUnavailable
	}
	v, err := command.Snapshot()
	if err != nil || source.MembershipID != v.AuthorMembershipID {
		return app.Prepared{}, app.ErrUnavailable
	}
	employee, err := s.AuthorEmployeeTx(ctx, db, scope, source.MembershipID)
	if err != nil {
		return app.Prepared{}, err
	}
	facts, err := queries.ExpenseReferenceFacts(ctx, q.ExpenseReferenceFactsParams{OrgID: scope.OrgID().String(), AssetID: v.AssetID, ExpenseID: v.ExpenseID})
	if err != nil {
		return app.Prepared{}, storeError(err)
	}
	prepared := app.Prepared{AuthorEmployeeID: employee, AssetState: facts.AssetState, ExistingOperation: facts.ExistingOperation}
	// BR-AUTH-005: the allocation at the competence instant.
	snapshot, err := s.Allocations.UnitAndCostCentreAtDBTX(ctx, db, scope, v.AssetID, v.Declared.OccurredAt)
	switch {
	case err == nil:
		if snapshot.AssetID != v.AssetID {
			return app.Prepared{}, app.ErrUnavailable
		}
		prepared.UnitID, prepared.CostCentreID = snapshot.UnitID, snapshot.CostCentreID
	case errors.Is(err, fleetapp.ErrUnallocated):
	case errors.Is(err, fleetapp.ErrNotFound):
		return app.Prepared{}, app.ErrNotFound
	default:
		return app.Prepared{}, app.ErrUnavailable
	}
	z, err := zones(ctx, queries, scope, prepared.UnitID)
	if err != nil {
		return app.Prepared{}, err
	}
	reporting, err := orgtime.AttributeReportingDay(v.Declared.OccurredAt, z)
	if err != nil {
		return app.Prepared{}, app.ErrUnavailable
	}
	prepared.Reporting = reporting
	return prepared, nil
}

// direct is the stored form of one accepted direct expense.
func direct(source syncapp.Source, v app.CommandInput, prepared app.Prepared) app.Record {
	return app.Record{ID: v.ExpenseID, OriginKind: string(domain.KindDirect), SourceID: v.ExpenseID, SourceRevision: app.SourceRevision,
		OperationID: v.OperationID, PayloadDigest: v.PayloadDigest, AssetID: v.AssetID, AuthorMembershipID: source.MembershipID,
		AuthorEmployeeID: prepared.AuthorEmployeeID, EnrollmentID: source.EnrollmentID, Category: v.Category, Payer: v.Payer,
		Paid: v.Paid, Company: v.Company, Declared: v.Declared, Reporting: prepared.Reporting, UnitID: prepared.UnitID,
		CostCentreID: prepared.CostCentreID, ReviewState: app.Submitted, Revision: 1}
}

// InsertDirectTx writes the original DIRECT expense fact.
func (s *Store) InsertDirectTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, command app.Command, prepared app.Prepared) error {
	queries, _, err := s.port(ctx, db, scope)
	if err != nil {
		return err
	}
	v, err := command.Snapshot()
	if err != nil || source.MembershipID != v.AuthorMembershipID || prepared.AuthorEmployeeID == "" || prepared.Reporting.Day == "" {
		return app.ErrUnavailable
	}
	record := direct(source, v, prepared)
	if _, err := record.Expense(scope); err != nil {
		return app.ErrUnavailable
	}
	inserted, err := s.insert(ctx, queries, scope, record)
	if err != nil {
		return err
	}
	if !inserted {
		return app.ErrUnavailable
	}
	return nil
}

// ReadExpenseTx reads one stored expense of the scope and re-proves it as an
// EMP-EXP-001 value.
func (s *Store) ReadExpenseTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, id string) (app.Record, error) {
	queries, _, err := s.port(ctx, db, scope)
	if err != nil {
		return app.Record{}, err
	}
	row, err := queries.GetExpense(ctx, q.GetExpenseParams{OrgID: scope.OrgID().String(), ID: id})
	if err != nil {
		return app.Record{}, storeError(err)
	}
	at, ok := instant(row.OccurredAt, row.OccurredSubmicro)
	if !ok || !row.ReceivedAt.Valid {
		return app.Record{}, app.ErrUnavailable
	}
	record := app.Record{ID: row.ID, OriginKind: row.OriginKind, SourceID: row.SourceID, FuelingID: row.FuelingID, SourceRevision: row.SourceRevision,
		OperationID: row.OperationID, PayloadDigest: row.PayloadDigest, AssetID: row.AssetID, AuthorMembershipID: row.AuthorMembershipID,
		AuthorEmployeeID: row.AuthorEmployeeID, EnrollmentID: row.EnrollmentID, Category: row.Category, Payer: row.Payer,
		Paid: row.PaidCentavos, Company: row.CompanyCentavos,
		Declared:  orgtime.Declaration{OccurredAt: at, EntryZone: row.EntryZone, DeviceOffsetSeconds: row.DeviceOffsetSeconds},
		Reporting: orgtime.ReportingAttribution{Day: row.ReportingDay, Zone: row.ReportingZone, Source: row.ReportingSource},
		UnitID:    row.UnitID, CostCentreID: row.CostCentreID, ReceivedAt: row.ReceivedAt.Time.UTC(), ReviewState: row.ReviewState, Revision: row.Revision}
	if _, err := record.Expense(scope); err != nil {
		return app.Record{}, app.ErrUnavailable
	}
	return record, nil
}
