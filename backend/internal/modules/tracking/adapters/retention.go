package adapters

import (
	"context"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RetentionPGStore enforces raw track and live position retention (EMP-GPS-011)
// across organizations using purge receipts and preserving derived summaries.
type RetentionPGStore struct {
	Pool *pgxpool.Pool
}

var _ app.RetentionStore = (*RetentionPGStore)(nil)

func (s *RetentionPGStore) PurgeExpired(ctx context.Context, orgID *kernel.OrgID, now time.Time) (app.RetentionResult, error) {
	if s == nil || s.Pool == nil {
		return app.RetentionResult{}, app.ErrUnavailable
	}
	var orgs []kernel.OrgID
	if orgID != nil {
		orgs = []kernel.OrgID{*orgID}
	} else {
		rows, err := s.Pool.Query(ctx, `SELECT org_id::text FROM organizations`)
		if err != nil {
			return app.RetentionResult{}, err
		}
		for rows.Next() {
			var raw string
			if err := rows.Scan(&raw); err != nil {
				rows.Close()
				return app.RetentionResult{}, err
			}
			parsed, err := kernel.ParseOrgID(raw)
			if err == nil {
				orgs = append(orgs, parsed)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return app.RetentionResult{}, err
		}
		rows.Close()
	}

	var result app.RetentionResult
	refNow := now.UTC()
	for _, id := range orgs {
		scope, err := kernel.NewOrgScope(id)
		if err != nil {
			continue
		}
		err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
			org := id.String()
			tag, err := tx.Exec(ctx, `DELETE FROM org_tracking_live_positions WHERE org_id=$1 AND expires_at IS NOT NULL AND expires_at<=$2`, org, refNow)
			if err != nil {
				return err
			}
			result.ExpiredLivePositions += int(tag.RowsAffected())

			retentionDays := domain.DefaultRawRetentionDays
			if err := tx.QueryRow(ctx, `SELECT COALESCE((SELECT raw_retention_days FROM org_tracking_settings WHERE org_id=$1),$2)`,
				org, domain.DefaultRawRetentionDays).Scan(&retentionDays); err != nil {
				return err
			}

			// Find ended/completed trips whose raw retention window expired AND either
			// have a summary or had zero segments to preserve summary invariant.
			type sessionTarget struct {
				tripID, sessionID string
				endedAt           time.Time
				segCount          int
			}
			cutoff := refNow.Add(-time.Duration(retentionDays) * 24 * time.Hour)
			rows, err := tx.Query(ctx, `SELECT s.trip_id::text,s.id::text,end_ref.ended_at,(SELECT count(*) FROM org_tracking_segments g WHERE g.org_id=s.org_id AND g.session_id=s.id)
 FROM org_tracking_sessions s
 JOIN LATERAL(SELECT COALESCE(
  (SELECT f.occurred_at FROM org_trip_finishes f WHERE f.org_id=s.org_id AND f.trip_id=s.trip_id),
  (SELECT tr.occurred_at FROM org_trip_transitions tr WHERE tr.org_id=s.org_id AND tr.trip_id=s.trip_id AND tr.command='CANCEL' LIMIT 1)
 ) AS ended_at) end_ref ON end_ref.ended_at IS NOT NULL
 WHERE s.org_id=$1 AND end_ref.ended_at<=$2
  AND EXISTS(SELECT 1 FROM org_tracking_segments g WHERE g.org_id=s.org_id AND g.session_id=s.id)
  AND (EXISTS(SELECT 1 FROM org_tracking_summaries sm WHERE sm.org_id=s.org_id AND sm.session_id=s.id)
   OR NOT EXISTS(SELECT 1 FROM org_tracking_transitions tr WHERE tr.org_id=s.org_id AND tr.session_id=s.id AND tr.state='ENDED'))`,
				org, cutoff)
			if err != nil {
				return err
			}
			var expiredSessions []sessionTarget
			for rows.Next() {
				var st sessionTarget
				if err := rows.Scan(&st.tripID, &st.sessionID, &st.endedAt, &st.segCount); err != nil {
					rows.Close()
					return err
				}
				expiredSessions = append(expiredSessions, st)
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return err
			}
			rows.Close()

			for _, st := range expiredSessions {
				receiptID, idErr := jobs.NewUUIDv4()
				if idErr != nil {
					return app.ErrUnavailable
				}
				if _, err := tx.Exec(ctx, `INSERT INTO org_tracking_purge_receipts(org_id,id,trip_id,session_id,reason,deleted_segments,cutoff_at)
 VALUES($1,$2,$3,$4,'RETENTION_EXPIRED',$5,$6)`,
					org, receiptID, st.tripID, st.sessionID, st.segCount, cutoff); err != nil {
					return err
				}
				dtag, err := tx.Exec(ctx, `DELETE FROM org_tracking_segments WHERE org_id=$1 AND session_id=$2`, org, st.sessionID)
				if err != nil {
					return err
				}
				result.PurgedSessions++
				result.DeletedSegments += int(dtag.RowsAffected())
			}

			// Trim whole segments collected strictly after an effective trip end
			// when summarized so no post-end fixes persist unnecessarily.
			trows, err := tx.Query(ctx, `SELECT s.trip_id::text,s.id::text,end_ref.ended_at,
 (SELECT count(*) FROM org_tracking_segments g WHERE g.org_id=s.org_id AND g.session_id=s.id AND g.first_at>end_ref.ended_at)
 FROM org_tracking_sessions s
 JOIN LATERAL(SELECT COALESCE(
  (SELECT f.occurred_at FROM org_trip_finishes f WHERE f.org_id=s.org_id AND f.trip_id=s.trip_id),
  (SELECT tr.occurred_at FROM org_trip_transitions tr WHERE tr.org_id=s.org_id AND tr.trip_id=s.trip_id AND tr.command='CANCEL' LIMIT 1)
 ) AS ended_at) end_ref ON end_ref.ended_at IS NOT NULL
 WHERE s.org_id=$1
  AND EXISTS(SELECT 1 FROM org_tracking_summaries sm WHERE sm.org_id=s.org_id AND sm.session_id=s.id)
  AND EXISTS(SELECT 1 FROM org_tracking_segments g WHERE g.org_id=s.org_id AND g.session_id=s.id AND g.first_at>end_ref.ended_at)`, org)
			if err != nil {
				return err
			}
			var trimTargets []sessionTarget
			for trows.Next() {
				var st sessionTarget
				if err := trows.Scan(&st.tripID, &st.sessionID, &st.endedAt, &st.segCount); err != nil {
					trows.Close()
					return err
				}
				trimTargets = append(trimTargets, st)
			}
			if err := trows.Err(); err != nil {
				trows.Close()
				return err
			}
			trows.Close()

			for _, st := range trimTargets {
				receiptID, idErr := jobs.NewUUIDv4()
				if idErr != nil {
					return app.ErrUnavailable
				}
				if _, err := tx.Exec(ctx, `INSERT INTO org_tracking_purge_receipts(org_id,id,trip_id,session_id,reason,deleted_segments,cutoff_at)
 VALUES($1,$2,$3,$4,'TRIMMED_AFTER_END',$5,$6)`,
					org, receiptID, st.tripID, st.sessionID, st.segCount, st.endedAt.UTC()); err != nil {
					return err
				}
				dtag, err := tx.Exec(ctx, `DELETE FROM org_tracking_segments WHERE org_id=$1 AND session_id=$2 AND first_at>$3`, org, st.sessionID, st.endedAt.UTC())
				if err != nil {
					return err
				}
				result.TrimmedSegments += int(dtag.RowsAffected())
			}
			return nil
		})
		if err != nil {
			return app.RetentionResult{}, storeError(err)
		}
	}
	return result, nil
}
