package adapters

import (
	"context"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/fleet"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"time"
)

// PostgreSQL has microsecond precision; preserve the exact accepted nanosecond
// observation as a floored timestamp and a separate 0..999 remainder.
func meterTime(t time.Time) (pgtype.Timestamptz, int16) {
	return pgtype.Timestamptz{Time: t.UTC().Truncate(time.Microsecond), Valid: true}, int16(t.Nanosecond() % 1000)
}
func exactMeterTime(t pgtype.Timestamptz, sub int16) (time.Time, error) {
	if !t.Valid || t.InfinityModifier != pgtype.Finite || sub < 0 || sub > 999 || t.Time.Nanosecond()%1000 != 0 {
		return time.Time{}, app.ErrUnavailable
	}
	v := t.Time.UTC().Add(time.Duration(sub) * time.Nanosecond)
	if v.Year() < 1 || v.Year() > 9999 {
		return time.Time{}, app.ErrUnavailable
	}
	return v, nil
}
func meterDBNow(ctx context.Context, queries *q.Queries) (time.Time, error) {
	stamp, err := queries.MeterCurrentDBTime(ctx)
	if err != nil {
		return time.Time{}, err
	}
	at, err := exactMeterTime(stamp, 0)
	if err != nil || at.IsZero() {
		return time.Time{}, app.ErrUnavailable
	}
	return at, nil
}
func meterView(scope kernel.OrgScope, row q.GetMeterRow) (app.MeterView, error) {
	asset, err := assetRef(scope, row.AssetID)
	if err != nil {
		return app.MeterView{}, app.ErrUnavailable
	}
	var modulus *int64
	if row.Modulus != 0 {
		n := row.Modulus
		modulus = &n
	}
	reference, err := meter.NewMeterRef(scope, asset, row.ID, meter.Kind(row.Kind), modulus)
	if err != nil {
		return app.MeterView{}, app.ErrUnavailable
	}
	segment, err := meter.NewSegmentRef(scope, reference, row.SegmentID)
	if err != nil {
		return app.MeterView{}, app.ErrUnavailable
	}
	installed, err := exactMeterTime(row.InstalledAt, row.InstalledSubmicro)
	if err != nil {
		return app.MeterView{}, err
	}
	view := app.MeterView{Reference: reference, Label: row.Label, InstalledAt: installed, Segment: segment}
	if row.RemovedAt.Valid {
		removed, err := exactMeterTime(row.RemovedAt, row.RemovedSubmicro)
		if err != nil {
			return app.MeterView{}, err
		}
		view.RemovedAt = &removed
	}
	return view, nil
}
func meterReading(scope kernel.OrgScope, view app.MeterView, row q.GetMeterReadingByOperationRow) (meter.Reading, error) {
	if row.MeterID != view.Reference.ID() || row.SegmentID != view.Segment.ID() {
		return meter.Reading{}, app.ErrUnavailable
	}
	ref, err := meter.NewReadingRef(scope, view.Segment, row.ID)
	if err != nil {
		return meter.Reading{}, app.ErrUnavailable
	}
	value, err := meter.NewValue(view.Reference.Kind(), row.Value, view.Reference.Modulus())
	if err != nil {
		return meter.Reading{}, app.ErrUnavailable
	}
	author, err := meter.NewMembershipRef(scope, row.AuthorMembershipID)
	if err != nil {
		return meter.Reading{}, app.ErrUnavailable
	}
	capture, err := meter.NewCaptureSource(scope, meter.CaptureKind(row.CaptureKind), row.CaptureID)
	if err != nil {
		return meter.Reading{}, app.ErrUnavailable
	}
	operation, err := meter.NewOperationRef(scope, row.OperationID)
	if err != nil {
		return meter.Reading{}, app.ErrUnavailable
	}
	occurred, err := exactMeterTime(row.OccurredAt, row.OccurredSubmicro)
	if err != nil {
		return meter.Reading{}, err
	}
	received, err := exactMeterTime(row.ReceivedAt, 0)
	if err != nil {
		return meter.Reading{}, err
	}
	declaration := orgtime.Declaration{OccurredAt: occurred, EntryZone: row.EntryZone, DeviceOffsetSeconds: row.DeviceOffsetSeconds}
	reading, err := meter.NewReading(scope, meter.ReadingInput{Reference: ref, Value: value, Declared: &declaration, ReceivedAt: received, Author: author, Capture: &capture, Operation: operation, Origin: meter.Origin(row.Origin), Method: meter.Method(row.Method), State: meter.State(row.State)})
	if err != nil {
		return meter.Reading{}, app.ErrUnavailable
	}
	return reading, nil
}
func loadMeter(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, asset, id string, latest bool) (app.MeterView, error) {
	queries := q.New(db)
	row, err := queries.GetMeter(ctx, q.GetMeterParams{OrgID: scope.OrgID().String(), AssetID: asset, ID: id})
	if err != nil {
		return app.MeterView{}, err
	}
	view, err := meterView(scope, row)
	if err != nil {
		return app.MeterView{}, err
	}
	if latest {
		last, err := queries.LatestValidMeterReading(ctx, q.LatestValidMeterReadingParams{OrgID: scope.OrgID().String(), MeterID: id, SegmentID: view.Segment.ID()})
		if err == nil {
			reading, err := meterReading(scope, view, q.GetMeterReadingByOperationRow(last))
			if err != nil {
				return app.MeterView{}, err
			}
			view.Latest = &reading
		} else if err != pgx.ErrNoRows {
			return app.MeterView{}, err
		}
	}
	return view, nil
}
func insertObservation(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, view app.MeterView, source app.MeterSource, member string, in app.MeterReadingInput, digest string) (meter.Reading, error) {
	// Existing-operation replay is resolved by the caller before this new
	// observation path. Validate the real service interval before any insert.
	if in.Declared.OccurredAt.Before(view.InstalledAt) || view.RemovedAt != nil && !in.Declared.OccurredAt.Before(*view.RemovedAt) {
		return meter.Reading{}, meter.ErrInvalid
	}
	if view.Reference.ID() != in.MeterID || view.Segment.ID() != in.SegmentID || view.Reference.Asset().ID().Value() != in.AssetID || view.Reference.Kind() != in.Value.Kind() {
		return meter.Reading{}, meter.ErrInvalid
	}
	value, err := meter.NewValue(view.Reference.Kind(), in.Value.Count(), view.Reference.Modulus())
	if err != nil {
		return meter.Reading{}, err
	}
	capture, err := source.Capture(scope)
	if err != nil {
		return meter.Reading{}, err
	}
	author, err := meter.NewMembershipRef(scope, member)
	if err != nil {
		return meter.Reading{}, err
	}
	operation, err := meter.NewOperationRef(scope, in.OperationID)
	if err != nil {
		return meter.Reading{}, err
	}
	ref, err := meter.NewReadingRef(scope, view.Segment, in.ReadingID)
	if err != nil {
		return meter.Reading{}, err
	}
	now, err := meterDBNow(ctx, q.New(db))
	if err != nil {
		return meter.Reading{}, err
	}
	if _, err := meter.NewReading(scope, meter.ReadingInput{Reference: ref, Value: value, Declared: &in.Declared, ReceivedAt: now, Author: author, Capture: &capture, Operation: operation, Origin: in.Origin, Method: in.Method, State: meter.Valid}); err != nil {
		return meter.Reading{}, err
	}
	at, sub := meterTime(in.Declared.OccurredAt)
	params := q.InsertMeterReadingParams{OrgID: scope.OrgID().String(), ID: in.ReadingID, MeterID: in.MeterID, SegmentID: in.SegmentID, Value: value.Count(), OccurredAt: at, OccurredSubmicro: sub, EntryZone: in.Declared.EntryZone, DeviceOffsetSeconds: in.Declared.DeviceOffsetSeconds, AuthorMembershipID: member, CaptureKind: string(capture.Kind()), OperationID: in.OperationID, RequestHash: digest, Origin: string(in.Origin), Method: string(in.Method), State: string(meter.Valid)}
	if capture.Kind() == meter.CaptureWebSession {
		params.WebSessionID = capture.ID()
	} else {
		params.EnrollmentID = capture.ID()
	}
	if err := q.New(db).InsertMeterReading(ctx, params); err != nil {
		return meter.Reading{}, err
	}
	row, err := q.New(db).GetMeterReadingByOperation(ctx, q.GetMeterReadingByOperationParams{OrgID: params.OrgID, MeterID: in.MeterID, OperationID: in.OperationID})
	if err != nil {
		return meter.Reading{}, err
	}
	return meterReading(scope, view, row)
}
