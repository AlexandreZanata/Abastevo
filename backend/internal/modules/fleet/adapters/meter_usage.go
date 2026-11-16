package adapters

import (
	"context"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/fleet"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ app.MeterUsagePort = (*MeterStore)(nil)

// UsageBetween answers usage between two instants for one asset and kind
// (B-BR-EMP-489/490, EDR-316) under a shared lock of the asset's meters of
// that kind, which every reading change locks for update. It authorizes
// nothing: callers prove their own read first.
func (s *MeterStore) UsageBetween(ctx context.Context, scope kernel.OrgScope, asset string, kind meter.Kind, from, to time.Time) (app.MeterUsage, error) {
	if scope.Validate() != nil {
		return app.MeterUsage{}, app.ErrNotFound
	}
	if !validID(asset) || kind != meter.Odometer && kind != meter.HourMeter || from.IsZero() || to.IsZero() || from.After(to) {
		return app.MeterUsage{}, meter.ErrInvalid
	}
	if s == nil || s.Pool == nil {
		return app.MeterUsage{}, app.ErrUnavailable
	}
	answer := app.MeterUsage{AssetID: asset, Kind: kind, From: from.UTC(), To: to.UTC(), Method: app.MeterUsageMethod}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		queries := q.New(tx)
		org := scope.OrgID().String()
		locked, err := queries.LockAssetKindMeters(ctx, q.LockAssetKindMetersParams{OrgID: org, AssetID: asset, Kind: string(kind)})
		if err != nil {
			return err
		}
		if len(locked) == 0 {
			return app.ErrNotFound
		}
		// Every read below is a new statement, so it sees what committed
		// before the share lock was granted (READ COMMITTED).
		meters, err := queries.AssetKindMeterRevisions(ctx, q.AssetKindMeterRevisionsParams{OrgID: org, AssetID: asset, Kind: string(kind)})
		if err != nil {
			return err
		}
		if len(meters) != len(locked) {
			return app.ErrUnavailable
		}
		for _, m := range meters {
			if !m.Projected {
				// The projection of this meter was never built (EDR-317).
				return app.ErrUnavailable
			}
			answer.Revision += m.Revision
		}
		chain, err := queries.AssetKindSegmentChain(ctx, q.AssetKindSegmentChainParams{OrgID: org, AssetID: asset, Kind: string(kind)})
		if err != nil {
			return err
		}
		spans := make([]meter.Span, 0, len(chain))
		for _, row := range chain {
			opened, err := exactMeterTime(row.OpenedAt, row.OpenedSubmicro)
			if err != nil {
				return err
			}
			span := meter.Span{ID: row.ID, Meter: row.MeterID, Reason: meter.SegmentReason(row.Kind), OpenedAt: opened, Open: row.OpenValue}
			if row.ClosedAt.Valid {
				if span.ClosedAt, err = exactMeterTime(row.ClosedAt, row.ClosedSubmicro); err != nil {
					return err
				}
				span.Closed = true
				if row.ClosingValue.Valid {
					v := row.ClosingValue.Int64
					span.Close = &v
				}
			}
			spans = append(spans, span)
		}
		rows, err := queries.AssetKindValidReadings(ctx, q.AssetKindValidReadingsParams{OrgID: org, AssetID: asset, Kind: string(kind)})
		if err != nil {
			return err
		}
		anchors := make([]meter.Anchor, 0, len(rows))
		byID := make(map[string]meter.Anchor, len(rows))
		for _, row := range rows {
			occurred, err := exactMeterTime(row.OccurredAt, row.OccurredSubmicro)
			if err != nil {
				return err
			}
			received, err := exactMeterTime(row.ReceivedAt, 0)
			if err != nil {
				return err
			}
			a := meter.Anchor{ID: row.ID, Segment: row.SegmentID, Value: row.Value, OccurredAt: occurred, ReceivedAt: received}
			anchors = append(anchors, a)
			byID[a.ID] = a
		}
		track, err := meter.NewTrack(kind, spans, anchors)
		if err != nil {
			// Stored evidence that no longer forms a valid chain is an outage,
			// never a number (BR-TRUTH-003).
			return app.ErrUnavailable
		}
		figure, err := track.Between(answer.From, answer.To)
		if err != nil {
			return meter.ErrInvalid
		}
		answer.Reason, answer.Usage = figure.Reason, figure.Value
		if !figure.Known() {
			answer.Usage = 0
		}
		for _, p := range []struct {
			id     string
			anchor **app.MeterUsageAnchor
		}{{figure.From, &answer.Start}, {figure.To, &answer.End}} {
			if a, ok := byID[p.id]; ok {
				*p.anchor = &app.MeterUsageAnchor{ReadingID: a.ID, Value: a.Value, OccurredAt: a.OccurredAt}
			}
		}
		return nil
	})
	if err != nil {
		return app.MeterUsage{}, meterError(ctx, err)
	}
	return answer, nil
}

// RebuildMeterProjection deletes and recomputes one organization's
// current-value projection (B-BR-EMP-488, EDR-317) and returns the number
// of meters it recomputed.
func RebuildMeterProjection(ctx context.Context, pool *pgxpool.Pool, scope kernel.OrgScope) (int64, error) {
	var n int64
	err := database.WithOrgTx(ctx, pool, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT org_meter_current_rebuild($1::uuid)`, scope.OrgID().String()).Scan(&n)
	})
	return n, err
}
