package adapters

import (
	"bytes"
	"context"
	"errors"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/fleet"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

func meterReviewCommand(source syncapp.Source, op protocol.Operation, command app.MeterReadingCommand) (app.MeterReadingInput, error) {
	if source.Validate() != nil {
		return app.MeterReadingInput{}, app.ErrUnauthenticated
	}
	v := op.Snapshot()
	if v.OwnerAccountID != source.Actor.AccountID || v.APIOrigin != source.Origin || v.OrgID != source.Scope.OrgID().String() || v.MembershipID != source.MembershipID || v.EnrollmentID != source.EnrollmentID {
		return app.MeterReadingInput{}, app.ErrNotFound
	}
	parsed, err := meterIntakePayload(op)
	if err != nil {
		return app.MeterReadingInput{}, err
	}
	if parsed.Digest(source.Scope) == "" || parsed.Digest(source.Scope) != command.Digest(source.Scope) {
		return app.MeterReadingInput{}, meter.ErrInvalid
	}
	return command.Snapshot()
}

func sameMeterHeld(source syncapp.Source, op protocol.Operation, in app.MeterReadingInput, row q.ReadMeterHeldProofRow) bool {
	v := op.Snapshot()
	at, err := exactMeterTime(row.OccurredAt, row.OccurredSubmicro)
	return err == nil && row.ProofVersion == 1 && validID(row.ID) && validID(row.AuthorEmployeeID) && row.OperationID == op.ID() && row.OperationType == v.Type && row.OperationSchema == v.OperationSchema && row.PayloadDigest == op.PayloadDigest() && row.OwnerAccountID == source.Actor.AccountID && row.ApiOrigin == source.Origin && row.MembershipID == source.MembershipID && row.EnrollmentID == source.EnrollmentID && row.EntityType == "meter_reading" && row.EntityID == in.ReadingID && row.ReadingID == in.ReadingID && row.AssetID == in.AssetID && row.MeterID == in.MeterID && row.SegmentID == in.SegmentID && row.Value == in.Value.Count() && at.Equal(in.Declared.OccurredAt) && row.EntryZone == in.Declared.EntryZone && row.DeviceOffsetSeconds == in.Declared.DeviceOffsetSeconds && row.Origin == string(in.Origin) && row.Method == string(in.Method)
}

func (s *MeterReviewStore) HoldMeterReadingTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, command app.MeterReadingCommand, code string) (app.ReviewReference, error) {
	db = syncapp.Borrow(db)
	if s == nil || s.NewID == nil || db == nil || database.ValidateOrgDBTX(ctx, db, source.Scope) != nil {
		return app.ReviewReference{}, app.ErrUnavailable
	}
	in, err := meterReviewCommand(source, op, command)
	if err != nil {
		return app.ReviewReference{}, err
	}
	if code != "PERMISSION_CHANGED" {
		return app.ReviewReference{}, app.ErrUnavailable
	}
	queries := q.New(db)
	org := source.Scope.OrgID().String()
	row, err := queries.ReadMeterHeldProof(ctx, q.ReadMeterHeldProofParams{OrgID: org, OperationID: op.ID()})
	if err == nil {
		if !sameMeterHeld(source, op, in, row) || row.Code != code || row.RequestHash != command.Digest(source.Scope) {
			return app.ReviewReference{}, app.ErrOperationReused
		}
		return app.ReviewReference{ReviewItemID: row.ID, Revision: 1}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return app.ReviewReference{}, app.ErrUnavailable
	}
	view, err := loadMeter(ctx, db, source.Scope, in.AssetID, in.MeterID, false)
	if err != nil {
		return app.ReviewReference{}, meterError(ctx, err)
	}
	if view.Segment.ID() != in.SegmentID || view.Reference.Kind() != in.Value.Kind() || in.Declared.OccurredAt.Before(view.InstalledAt) || view.RemovedAt != nil && !in.Declared.OccurredAt.Before(*view.RemovedAt) {
		return app.ReviewReference{}, meter.ErrInvalid
	}
	if _, err := meter.NewValue(view.Reference.Kind(), in.Value.Count(), view.Reference.Modulus()); err != nil {
		return app.ReviewReference{}, err
	}
	id, err := s.NewID()
	if err != nil || !validID(id) {
		return app.ReviewReference{}, app.ErrUnavailable
	}
	n, err := queries.InsertMeterReviewItem(ctx, q.InsertMeterReviewItemParams{OrgID: org, ID: id, OperationID: op.ID(), Code: code})
	if err != nil || n != 1 {
		return app.ReviewReference{}, app.ErrUnavailable
	}
	at, sub := meterTime(in.Declared.OccurredAt)
	err = queries.InsertMeterHeldReading(ctx, q.InsertMeterHeldReadingParams{OrgID: org, ReviewItemID: id, OperationID: op.ID(), ReadingID: in.ReadingID, AssetID: in.AssetID, MeterID: in.MeterID, SegmentID: in.SegmentID, Value: in.Value.Count(), OccurredAt: at, OccurredSubmicro: sub, EntryZone: in.Declared.EntryZone, DeviceOffsetSeconds: in.Declared.DeviceOffsetSeconds, Origin: string(in.Origin), Method: string(in.Method), RequestHash: command.Digest(source.Scope)})
	if err != nil {
		return app.ReviewReference{}, meterError(ctx, err)
	}
	return app.ReviewReference{ReviewItemID: id, Revision: 1}, nil
}

func (s *MeterReviewStore) CheckHeldTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, outcome syncapp.Outcome) error {
	db = syncapp.Borrow(db)
	if s == nil || db == nil || database.ValidateOrgDBTX(ctx, db, source.Scope) != nil {
		return app.ErrUnavailable
	}
	command, err := meterIntakePayload(op)
	if err != nil {
		return app.ErrUnavailable
	}
	in, err := meterReviewCommand(source, op, command)
	if err != nil {
		return err
	}
	row, err := q.New(db).ReadMeterHeldProof(ctx, q.ReadMeterHeldProofParams{OrgID: source.Scope.OrgID().String(), OperationID: op.ID()})
	if err != nil || !sameMeterHeld(source, op, in, row) || row.RequestHash != command.Digest(source.Scope) || outcome.Result != protocol.AcceptedForReview || outcome.Code != row.Code || outcome.ReviewItemID != row.ID {
		return app.ErrUnavailable
	}
	if len(row.CanonicalAck) != 0 {
		return checkMeterHeldReceipt(op, row)
	}
	if row.AcknowledgementResult != "" || row.AcknowledgementProofVersion != 0 || row.ReceivedAt.Valid {
		return app.ErrUnavailable
	}
	return nil
}

func checkMeterHeldReceipt(op protocol.Operation, row q.ReadMeterHeldProofRow) error {
	if row.AcknowledgementResult != protocol.AcceptedForReview || row.AcknowledgementCode != row.Code || row.AcknowledgementReviewItemID != row.ID || row.AcknowledgementProofVersion != 1 {
		return app.ErrUnavailable
	}
	ack, err := protocol.ParseAcknowledgement(row.CanonicalAck, op, protocol.DefaultLimits())
	if err != nil || !bytes.Equal(ack.CanonicalBytes(), row.CanonicalAck) {
		return app.ErrUnavailable
	}
	v := ack.Snapshot()
	received, err := exactMeterTime(row.ReceivedAt, 0)
	if err != nil || v.Result != protocol.AcceptedForReview || v.Code != row.Code || v.ReviewItemID != row.ID || v.ReceivedAt != received.Format("2006-01-02T15:04:05.999999999Z07:00") {
		return app.ErrUnavailable
	}
	return nil
}

func (s *MeterReviewStore) CheckOwnReceiptTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation) (bool, error) {
	db = syncapp.Borrow(db)
	if s == nil || db == nil || source.Validate() != nil || database.ValidateOrgDBTX(ctx, db, source.Scope) != nil {
		return false, app.ErrUnavailable
	}
	row, err := q.New(db).ReadMeterHeldProof(ctx, q.ReadMeterHeldProofParams{OrgID: source.Scope.OrgID().String(), OperationID: op.ID()})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, app.ErrUnavailable
	}
	command, err := meterIntakePayload(op)
	if err != nil {
		return false, app.ErrUnavailable
	}
	in, err := meterReviewCommand(source, op, command)
	if err != nil {
		return false, err
	}
	if !sameMeterHeld(source, op, in, row) || row.RequestHash != command.Digest(source.Scope) {
		return false, app.ErrNotFound
	}
	if err := checkMeterHeldReceipt(op, row); err != nil {
		return false, err
	}
	return true, nil
}
