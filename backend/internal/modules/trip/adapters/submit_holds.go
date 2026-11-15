package adapters

import (
	"context"
	"errors"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/trip"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// EMP-TRIP-015 held submissions (B-BR-EMP-364, EDR-235) and the METER-004
// parent answer for trip readings (B-BR-EMP-368).

// heldSubmission compares a stored held submission with the operation.
func heldSubmission(source syncapp.Source, op protocol.Operation, row q.ReadTripSubmitHeldProofRow, v app.SubmitInput) bool {
	e := op.Snapshot()
	at, ok := exact(row.OccurredAt, row.OccurredSubmicro)
	return ok && heldBinding(source, op) && row.ProofVersion == 1 && canonicalID(row.ID) && canonicalID(row.AuthorEmployeeID) &&
		row.OperationType == app.SubmitOperationType && row.OperationSchema == e.OperationSchema && row.PayloadDigest == op.PayloadDigest() &&
		row.OwnerAccountID == source.Actor.AccountID && row.ApiOrigin == source.Origin && row.MembershipID == source.MembershipID &&
		row.EnrollmentID == source.EnrollmentID && row.EntityType == app.ReviewEntityType && row.EntityID == v.TripID && row.TripID == v.TripID &&
		row.Revision == v.Revision && row.Code == app.StaleRevision && at.Equal(v.Declared.OccurredAt) && row.EntryZone == v.Declared.EntryZone &&
		row.DeviceOffsetSeconds == v.Declared.DeviceOffsetSeconds
}

func parsedSubmit(op protocol.Operation) (app.SubmitInput, error) {
	command, err := app.ParseSubmit(op)
	if err != nil {
		return app.SubmitInput{}, app.ErrUnavailable
	}
	return command.Snapshot()
}

// HoldSubmitTx keeps a stale or duplicate submission as one held fact with
// its version-1 review item; the review case is never changed.
func (s *Store) HoldSubmitTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, command app.SubmitCommand, code string) (string, error) {
	queries, _, err := s.port(ctx, db, source.Scope)
	v, errCommand := command.Snapshot()
	if err != nil || errCommand != nil || s.NewID == nil || code != app.StaleRevision || op.Snapshot().Type != app.SubmitOperationType || v.OperationID != op.ID() || !heldBinding(source, op) {
		return "", app.ErrUnavailable
	}
	org := source.Scope.OrgID().String()
	row, err := queries.ReadTripSubmitHeldProof(ctx, q.ReadTripSubmitHeldProofParams{OrgID: org, OperationID: op.ID()})
	if err == nil {
		if !heldSubmission(source, op, row, v) {
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
	at, sub := instant(v.Declared.OccurredAt)
	if err := queries.InsertTripHeldSubmission(ctx, q.InsertTripHeldSubmissionParams{OrgID: org, ReviewItemID: review, OperationID: op.ID(), TripID: v.TripID,
		Revision: v.Revision, OccurredAt: at, OccurredSubmicro: sub, EntryZone: v.Declared.EntryZone, DeviceOffsetSeconds: v.Declared.DeviceOffsetSeconds}); err != nil {
		return "", storeError(err)
	}
	return review, nil
}

// SubmitProof is the OFFL-008 review proof of held submissions.
type SubmitProof struct{}

var _ syncapp.ReviewProof = SubmitProof{}

func submitProofRow(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation) (q.ReadTripSubmitHeldProofRow, error) {
	db = syncapp.Borrow(db)
	if db == nil || database.ValidateOrgDBTX(ctx, db, source.Scope) != nil {
		return q.ReadTripSubmitHeldProofRow{}, app.ErrUnavailable
	}
	return q.New(db).ReadTripSubmitHeldProof(ctx, q.ReadTripSubmitHeldProofParams{OrgID: source.Scope.OrgID().String(), OperationID: op.ID()})
}

func heldSubmissionReceipt(op protocol.Operation, row q.ReadTripSubmitHeldProofRow) error {
	return heldReceipt(op, q.ReadTripHeldProofRow{ID: row.ID, Code: row.Code, AcknowledgementResult: row.AcknowledgementResult, AcknowledgementCode: row.AcknowledgementCode,
		AcknowledgementReviewItemID: row.AcknowledgementReviewItemID, AcknowledgementProofVersion: row.AcknowledgementProofVersion, ReceivedAt: row.ReceivedAt, CanonicalAck: row.CanonicalAck})
}

// CheckHeldTx proves a held submission outcome against the stored held fact
// and, once written, its exact receipt.
func (SubmitProof) CheckHeldTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, outcome syncapp.Outcome) error {
	if !heldBinding(source, op) {
		return app.ErrUnavailable
	}
	v, err := parsedSubmit(op)
	if err != nil {
		return app.ErrUnavailable
	}
	row, err := submitProofRow(ctx, db, source, op)
	if err != nil || !heldSubmission(source, op, row, v) || outcome.Result != protocol.AcceptedForReview || outcome.Code != row.Code || outcome.ReviewItemID != row.ID || outcome.EntityRevision != nil {
		return app.ErrUnavailable
	}
	if len(row.CanonicalAck) != 0 {
		return heldSubmissionReceipt(op, row)
	}
	if row.AcknowledgementResult != "" || row.AcknowledgementProofVersion != 0 || row.ReceivedAt.Valid {
		return app.ErrUnavailable
	}
	return nil
}

// CheckOwnReceiptTx lets the author re-read its own held submission receipt
// without the record read permission (OFFL-008).
func (SubmitProof) CheckOwnReceiptTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation) (bool, error) {
	if source.Validate() != nil {
		return false, app.ErrUnavailable
	}
	row, err := submitProofRow(ctx, db, source, op)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, app.ErrUnavailable
	}
	v, err := parsedSubmit(op)
	if err != nil {
		return false, app.ErrUnavailable
	}
	if !heldSubmission(source, op, row, v) {
		return false, app.ErrNotFound
	}
	if err := heldSubmissionReceipt(op, row); err != nil {
		return false, err
	}
	return true, nil
}

// ApprovedParentTx answers the fleet decision port (EMP-METER-004,
// B-BR-EMP-368): whether the reading is a start, finish or superseding
// amendment reading of a trip with an approved revision.
func (s *Store) ApprovedParentTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, reading string) (bool, error) {
	queries, _, err := s.port(ctx, db, scope)
	if err != nil || !canonicalID(reading) {
		return false, app.ErrUnavailable
	}
	approved, err := queries.TripReadingApprovedParent(ctx, q.TripReadingApprovedParentParams{OrgID: scope.OrgID().String(), ReadingID: reading})
	if err != nil {
		return false, app.ErrUnavailable
	}
	return approved, nil
}
