// Package adapters persists organization fuelings and owns the signed
// fueling.submit intake handler (EMP-FUEL-002).
package adapters

import (
	"context"
	"errors"
	"slices"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/fueling"
	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/application"
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ProductNotAccepted is the BR-FUEL-016 warning code (B-BR-EMP-264).
const ProductNotAccepted = "PRODUCT_NOT_ACCEPTED"

// Store is the fueling persistence owner. Allocations is the FLEET-005
// snapshot port read on the borrowed intake transaction.
type Store struct {
	Allocations fleetapp.AllocationSnapshotPort
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

// AuthorEmployeeTx returns the author membership's employee record.
func (s *Store) AuthorEmployeeTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, membership string) (string, error) {
	queries, _, err := s.port(ctx, db, scope)
	if err != nil {
		return "", err
	}
	employee, err := queries.FuelingAuthorEmployee(ctx, q.FuelingAuthorEmployeeParams{OrgID: scope.OrgID().String(), MembershipID: membership})
	if err != nil {
		return "", storeError(err)
	}
	return employee, nil
}

// PrepareFuelingTx reads the admission facts. The handler holds the root and
// asset locks (the fleet disposal lookup) before calling it.
func (s *Store) PrepareFuelingTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, command app.Command) (app.Prepared, error) {
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
	org := scope.OrgID().String()
	facts, err := queries.FuelingReferenceFacts(ctx, q.FuelingReferenceFactsParams{OrgID: org, AssetID: v.AssetID, ResponsibleID: v.ResponsibleID, Product: string(v.Candidate.Snapshot().Quantity.Product().Code()), FuelingID: v.FuelingID})
	if err != nil {
		return app.Prepared{}, storeError(err)
	}
	prepared := app.Prepared{AuthorEmployeeID: employee, AssetState: facts.AssetState, ExistingOperation: facts.ExistingOperation, ResponsibleActive: facts.ResponsibleActive, ProductAccepted: facts.ProductAccepted}
	// BR-AUTH-005 (B-BR-EMP-263): the allocation at the declared instant.
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
	return prepared, nil
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
func number(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

// InsertFuelingTx writes the original fact, its warnings and the first
// APPR-001 review entry in the borrowed intake transaction.
func (s *Store) InsertFuelingTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, command app.Command, prepared app.Prepared, arithmetic fuel.Arithmetic) error {
	queries, _, err := s.port(ctx, db, scope)
	if err != nil {
		return err
	}
	v, err := command.Snapshot()
	if err != nil || source.MembershipID != v.AuthorMembershipID || prepared.AuthorEmployeeID == "" {
		return app.ErrUnavailable
	}
	c := v.Candidate.Snapshot()
	amounts := c.Amounts.Snapshot()
	product := c.Quantity.Product()
	org := scope.OrgID().String()
	occurred := v.Declared.OccurredAt.UTC()
	params := q.InsertFuelingParams{OrgID: org, ID: v.FuelingID, OperationID: v.OperationID, PayloadDigest: v.PayloadDigest, AssetID: v.AssetID,
		ResponsibleEmployeeID: v.ResponsibleID, AuthorMembershipID: source.MembershipID, AuthorEmployeeID: prepared.AuthorEmployeeID, EnrollmentID: source.EnrollmentID,
		OccurredAt: pgtype.Timestamptz{Time: occurred.Truncate(time.Microsecond), Valid: true}, OccurredSubmicro: int16(occurred.Nanosecond() % 1000),
		EntryZone: v.Declared.EntryZone, DeviceOffsetSeconds: v.Declared.DeviceOffsetSeconds, Product: string(product.Code()), Unit: string(product.Unit()),
		Quantity: c.Quantity.Amount(), PaidCentavos: amounts.Paid, DiscountCentavos: number(amounts.Discount), UnitPriceMilliBrl: number(amounts.UnitPrice), UnitPriceText: text(amounts.SourcePriceText),
		Fill: string(c.Fill), Payer: string(c.Payer), PaymentMethod: string(c.Payment), Origin: string(fuel.App), MissingReadingReason: text(v.MissingReason),
		UnitID: id(prepared.UnitID), CostCentreID: id(prepared.CostCentreID), GrossCentavos: arithmetic.Gross, ExpectedCentavos: number(arithmetic.Expected),
		DifferenceCentavos: number(arithmetic.Difference), ReviewState: string(review.Submitted), Revision: 1}
	if arithmetic.Derived != "" {
		params.DerivedPriceMilliBrl = pgtype.Text{String: arithmetic.Derived, Valid: true}
	}
	if v.PlaceName != nil {
		params.PlaceKind, params.PlaceName = pgtype.Text{String: string(fuel.NotFound), Valid: true}, text(v.PlaceName)
	}
	if v.Reading != nil {
		params.MeterID, params.SegmentID, params.ReadingID = id(v.Reading.MeterID), id(v.Reading.SegmentID), id(v.Reading.ReadingID)
	}
	// APPR-001: server acceptance is the author's SUBMIT of revision 1.
	author := review.Identity{Accounts: []string{source.Actor.AccountID}, Memberships: []string{source.MembershipID}, Employees: []string{prepared.AuthorEmployeeID}}
	submitted, err := review.NewDraft().Apply(review.Command{Kind: review.Submit, Revision: 1}, review.Facts{Actor: author, Author: author, Permitted: true})
	history := submitted.History()
	if err != nil || submitted.State() != review.Submitted || submitted.Revision() != 1 || len(history) != 1 {
		return app.ErrUnavailable
	}
	if err := queries.InsertFueling(ctx, params); err != nil {
		return storeError(err)
	}
	var warnings []string
	if arithmetic.Warning != "" {
		warnings = append(warnings, arithmetic.Warning)
	}
	if !prepared.ProductAccepted {
		warnings = append(warnings, ProductNotAccepted)
	}
	slices.Sort(warnings)
	for _, code := range warnings {
		if err := queries.InsertFuelingWarning(ctx, q.InsertFuelingWarningParams{OrgID: org, FuelingID: v.FuelingID, Revision: 1, Code: code}); err != nil {
			return storeError(err)
		}
	}
	entry := history[0]
	if err := queries.InsertFuelingReviewEntry(ctx, q.InsertFuelingReviewEntryParams{OrgID: org, FuelingID: v.FuelingID, Sequence: 1, Kind: string(entry.Kind), Revision: entry.Revision, State: string(entry.State), Reason: entry.Reason, ActorMembershipID: source.MembershipID, OperationID: v.OperationID}); err != nil {
		return storeError(err)
	}
	return nil
}

func optionalText(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}
func optionalNumber(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	n := v.Int64
	return &n
}

// ReadFuelingTx reads one stored fueling of the scope.
func (s *Store) ReadFuelingTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, fueling string) (app.Record, error) {
	queries, _, err := s.port(ctx, db, scope)
	if err != nil {
		return app.Record{}, err
	}
	row, err := queries.GetFueling(ctx, q.GetFuelingParams{OrgID: scope.OrgID().String(), ID: fueling})
	if err != nil {
		return app.Record{}, storeError(err)
	}
	if !row.OccurredAt.Valid || row.OccurredAt.InfinityModifier != pgtype.Finite || row.OccurredAt.Time.Nanosecond()%1000 != 0 || row.OccurredSubmicro < 0 || row.OccurredSubmicro > 999 || !row.ReceivedAt.Valid || row.ReviewEntryState != row.ReviewState {
		return app.Record{}, app.ErrUnavailable
	}
	derived := ""
	if row.DerivedPriceMilliBrl.Valid {
		derived = row.DerivedPriceMilliBrl.String
	}
	return app.Record{ID: row.ID, OperationID: row.OperationID, PayloadDigest: row.PayloadDigest, AssetID: row.AssetID, ResponsibleID: row.ResponsibleEmployeeID,
		AuthorMembershipID: row.AuthorMembershipID, AuthorEmployeeID: row.AuthorEmployeeID, EnrollmentID: row.EnrollmentID,
		Declared:   orgtime.Declaration{OccurredAt: row.OccurredAt.Time.UTC().Add(time.Duration(row.OccurredSubmicro)), EntryZone: row.EntryZone, DeviceOffsetSeconds: row.DeviceOffsetSeconds},
		ReceivedAt: row.ReceivedAt.Time.UTC(), Product: row.Product, Unit: row.Unit, Quantity: row.Quantity, Paid: row.PaidCentavos, Gross: row.GrossCentavos, Revision: row.Revision,
		Discount: optionalNumber(row.DiscountCentavos), UnitPrice: optionalNumber(row.UnitPriceMilliBrl), Expected: optionalNumber(row.ExpectedCentavos), Difference: optionalNumber(row.DifferenceCentavos),
		UnitPriceText: optionalText(row.UnitPriceText), PlaceName: optionalText(row.PlaceName), MissingReason: optionalText(row.MissingReadingReason),
		Fill: row.Fill, Payer: row.Payer, Payment: row.PaymentMethod, Origin: row.Origin, Derived: derived, ReviewState: row.ReviewState,
		MeterID: row.MeterID, SegmentID: row.SegmentID, ReadingID: row.ReadingID, UnitID: row.UnitID, CostCentreID: row.CostCentreID, Warnings: append([]string(nil), row.Warnings...)}, nil
}
