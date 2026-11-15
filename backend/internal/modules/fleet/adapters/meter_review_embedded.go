package adapters

import (
	"context"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/fleet"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
)

var _ app.ReviewedReadingRepository = (*MeterReviewStore)(nil)

// ApplyReviewedEmbeddedTx stores the embedded reading of an accepted held
// field fact of another owner (EMP-TRIP-007): the meter lock serialises the
// series, the candidate is classified at its original receipt, and the
// observation guard re-proves the accepted resolution, the held fact and the
// original source and stamps that receipt.
func (s *MeterReviewStore) ApplyReviewedEmbeddedTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, in app.ReviewedEmbeddedReading) (meter.Reading, error) {
	db = syncapp.Borrow(db)
	if s == nil || db == nil || !validID(in.ResolutionID) || !validID(in.AuthorMembershipID) || !validID(in.EnrollmentID) || in.ReceivedAt.IsZero() || database.ValidateOrgDBTX(ctx, db, scope) != nil {
		return meter.Reading{}, app.ErrUnavailable
	}
	v, err := in.Command.Snapshot()
	if err != nil || v.Method != meter.Typed {
		return meter.Reading{}, app.ErrUnavailable
	}
	queries := q.New(db)
	org := scope.OrgID().String()
	if _, err := queries.LockMeter(ctx, q.LockMeterParams{OrgID: org, AssetID: v.AssetID, ID: v.MeterID}); err != nil {
		return meter.Reading{}, meterError(ctx, err)
	}
	view, err := loadMeter(ctx, db, scope, v.AssetID, v.MeterID, false)
	if err != nil {
		return meter.Reading{}, meterError(ctx, err)
	}
	if view.Segment.ID() != v.SegmentID || view.Reference.Kind() != v.Value.Kind() {
		return meter.Reading{}, app.ErrUnavailable
	}
	value, err := meter.NewValue(view.Reference.Kind(), v.Value.Count(), view.Reference.Modulus())
	if err != nil {
		return meter.Reading{}, app.ErrUnavailable
	}
	verdict, err := classifyCandidate(ctx, db, scope, s.Plausibility, view, candidatePoint(v.ReadingID, value.Count(), v.Declared.OccurredAt, in.ReceivedAt))
	if err != nil {
		return meter.Reading{}, meterError(ctx, err)
	}
	at, sub := meterTime(v.Declared.OccurredAt)
	if err := queries.InsertReviewedEmbeddedReading(ctx, q.InsertReviewedEmbeddedReadingParams{OrgID: org, ID: v.ReadingID, MeterID: v.MeterID, SegmentID: v.SegmentID, Value: value.Count(),
		OccurredAt: at, OccurredSubmicro: sub, EntryZone: v.Declared.EntryZone, DeviceOffsetSeconds: v.Declared.DeviceOffsetSeconds, AuthorMembershipID: in.AuthorMembershipID,
		EnrollmentID: in.EnrollmentID, OperationID: v.OperationID, RequestHash: in.Command.Digest(scope), Origin: string(v.Origin), Method: string(v.Method), State: string(verdict.State),
		ReviewResolutionID: in.ResolutionID}); err != nil {
		return meter.Reading{}, meterError(ctx, err)
	}
	if err := recordClassification(ctx, db, scope, s.Plausibility, view, v.ReadingID, in.ResolutionID, verdict); err != nil {
		return meter.Reading{}, meterError(ctx, err)
	}
	row, err := queries.GetMeterReadingByOperation(ctx, q.GetMeterReadingByOperationParams{OrgID: org, MeterID: v.MeterID, OperationID: v.OperationID})
	if err != nil || row.ID != v.ReadingID || row.CaptureKind != string(meter.CaptureEnrolledDevice) || row.CaptureID != in.EnrollmentID || row.AuthorMembershipID != in.AuthorMembershipID {
		return meter.Reading{}, app.ErrUnavailable
	}
	return meterReading(scope, view, row)
}
