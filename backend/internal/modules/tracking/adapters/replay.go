package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	auditapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReplayStore serves full trip replay with logbook events, gaps and
// simplified summary fallback when raw fixes are expired or purged.
type ReplayStore struct {
	Pool      *pgxpool.Pool
	Authority TrackAuthority
	Access    auditapp.AccessRecorder
	NewID     func() (string, error)
}

var _ app.ReplayRepository = (*ReplayStore)(nil)

func replayBinding(scope kernel.OrgScope, v app.ReplayView) string {
	raw, _ := json.Marshal([]any{"tracking.track-replay.v1", scope.OrgID().String(), v.TripID, v.RawExpired, len(v.Points), len(v.Logbook)})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (s *ReplayStore) ReadReplay(ctx context.Context, scope kernel.OrgScope, actor app.TrackActor, decision access.Decision, request app.ReplayRequest) (app.ReplayView, error) {
	if s == nil || s.Pool == nil || absent(s.Authority) || absent(s.Access) || s.NewID == nil {
		return app.ReplayView{}, app.ErrUnavailable
	}
	if err := decision.Check(ctx, scope, actor.AccountID, app.TrackPermission, request.TripID); err != nil {
		return app.ReplayView{}, app.ErrUnavailable
	}
	member := decision.Subject().MembershipID()
	org := scope.OrgID().String()
	view := app.ReplayView{TripID: request.TripID}

	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckTrackTx(ctx, tx, scope, actor, member, request.TripID); err != nil {
			return err
		}
		var tripStartedAt time.Time
		if err := tx.QueryRow(ctx, `SELECT state,occurred_at FROM org_trips WHERE org_id=$1 AND id=$2`, org, request.TripID).
			Scan(&view.TripState, &tripStartedAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return app.ErrNotFound
			}
			return err
		}
		view.Logbook = append(view.Logbook, app.ReplayLogbookEvent{Kind: "TRIP_STARTED", At: tripStartedAt.UTC(), Summary: "Viagem iniciada"})

		trows, err := tx.Query(ctx, `SELECT command,occurred_at FROM org_trip_transitions WHERE org_id=$1 AND trip_id=$2 ORDER BY occurred_at ASC, revision ASC`, org, request.TripID)
		if err != nil {
			return err
		}
		for trows.Next() {
			var cmd string
			var at time.Time
			if err := trows.Scan(&cmd, &at); err != nil {
				trows.Close()
				return err
			}
			view.Logbook = append(view.Logbook, app.ReplayLogbookEvent{Kind: "TRIP_" + cmd, At: at.UTC(), Summary: "Transição " + cmd})
		}
		if err := trows.Err(); err != nil {
			trows.Close()
			return err
		}
		trows.Close()

		erows, err := tx.Query(ctx, `SELECT event_type,COALESCE(event_text,''),occurred_at FROM org_trip_events WHERE org_id=$1 AND trip_id=$2 ORDER BY occurred_at ASC`, org, request.TripID)
		if err != nil {
			return err
		}
		for erows.Next() {
			var kind, text string
			var at time.Time
			if err := erows.Scan(&kind, &text, &at); err != nil {
				erows.Close()
				return err
			}
			view.Logbook = append(view.Logbook, app.ReplayLogbookEvent{Kind: kind, At: at.UTC(), Summary: text})
		}
		if err := erows.Err(); err != nil {
			erows.Close()
			return err
		}
		erows.Close()

		var finishedAt *time.Time
		if err := tx.QueryRow(ctx, `SELECT occurred_at FROM org_trip_finishes WHERE org_id=$1 AND trip_id=$2`, org, request.TripID).Scan(&finishedAt); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if finishedAt != nil {
			view.Logbook = append(view.Logbook, app.ReplayLogbookEvent{Kind: "TRIP_FINISHED", At: finishedAt.UTC(), Summary: "Viagem encerrada"})
		}
		sort.Slice(view.Logbook, func(i, j int) bool { return view.Logbook[i].At.Before(view.Logbook[j].At) })

		db := syncapp.Borrow(tx)
		session, err := (&Store{}).SessionOfTripTx(ctx, db, scope, request.TripID)
		switch {
		case err == nil:
			served := &app.TrackSession{ID: session.ID, Policy: string(session.Policy), OpenedAt: session.OpenedAt}
			for _, t := range session.Transitions {
				served.Transitions = append(served.Transitions, app.TrackTransition{Sequence: t.Seq, State: string(t.State), Cause: string(t.Cause), Actor: string(t.Actor), Permission: string(t.Permission), At: t.At})
			}
			if n := len(session.Transitions); n > 0 {
				served.State = string(session.Transitions[n-1].State)
			}
			view.Session = served
		case !errors.Is(err, app.ErrNotFound):
			return err
		}

		// Latest tracking summary (if session ended and processed).
		var rev int
		var quality string
		var cov *int
		var dist *int64
		var expSec, gapSec int64
		var coarseSum bool
		var procAt time.Time
		var gapFrom, gapTo []time.Time
		var gapReason, gapDet []string
		var lineParts []int
		var lineLat, lineLon []int32
		err = tx.QueryRow(ctx, `SELECT revision,quality,coverage_permille,distance_m,expected_seconds,gap_seconds,coarse,processed_at,
 gap_from,gap_to,gap_reason,gap_detected_by,line_parts,line_lat_e7,line_lon_e7
 FROM org_tracking_summaries WHERE org_id=$1 AND trip_id=$2 ORDER BY revision DESC LIMIT 1`, org, request.TripID).
			Scan(&rev, &quality, &cov, &dist, &expSec, &gapSec, &coarseSum, &procAt, &gapFrom, &gapTo, &gapReason, &gapDet, &lineParts, &lineLat, &lineLon)
		if err == nil {
			view.Summary = &app.ReplaySummary{
				Revision:         rev,
				Quality:          quality,
				CoveragePermille: cov,
				DistanceMetres:   dist,
				ExpectedSeconds:  expSec,
				GapSeconds:       gapSec,
				Coarse:           coarseSum,
				ProcessedAt:      procAt.UTC(),
			}
			for i := range gapFrom {
				view.Gaps = append(view.Gaps, app.ReplayGap{From: gapFrom[i].UTC(), To: gapTo[i].UTC(), Reason: gapReason[i], DetectedBy: gapDet[i]})
			}
			if quality != "INSUFFICIENT" {
				cursor := 0
				for _, partLen := range lineParts {
					if cursor+partLen <= len(lineLat) {
						part := make([]domain.Point, partLen)
						for j := 0; j < partLen; j++ {
							part[j] = domain.Point{Lat: lineLat[cursor+j], Lon: lineLon[cursor+j]}
						}
						view.SimplifiedLines = append(view.SimplifiedLines, part)
						cursor += partLen
					}
				}
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		effectiveEnd, retentionDays, dbNow, err := tripRetentionWindowTx(ctx, tx, scope, request.TripID)
		if err != nil {
			return err
		}
		expired, err := domain.RawTrackExpired(effectiveEnd, retentionDays, dbNow)
		if err != nil {
			return err
		}
		if !expired && view.Session != nil {
			var hasPurge bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM org_tracking_purge_receipts WHERE org_id=$1 AND session_id=$2 AND reason='RETENTION_EXPIRED')`,
				org, view.Session.ID).Scan(&hasPurge); err != nil {
				return err
			}
			expired = hasPurge
		}
		view.RawExpired = expired
		if expired || (view.Summary != nil && view.Summary.Quality == "INSUFFICIENT") {
			return nil
		}

		srows, err := tx.Query(ctx, `SELECT seq,received_at,collected_at,lat_e7,lon_e7,accuracy_m,speed_cm_s,bearing_deg,coarse
 FROM org_tracking_segments WHERE org_id=$1 AND trip_id=$2 ORDER BY seq ASC`, org, request.TripID)
		if err != nil {
			return err
		}
		defer srows.Close()
		for srows.Next() {
			var seq int64
			var rcv time.Time
			var at []time.Time
			var lat, lon, acc []int32
			var spd []*int32
			var brg []*int16
			var crs []bool
			if err := srows.Scan(&seq, &rcv, &at, &lat, &lon, &acc, &spd, &brg, &crs); err != nil {
				return err
			}
			rcv = rcv.UTC()
			for i := range at {
				ts := at[i].UTC()
				if effectiveEnd != nil && ts.After(effectiveEnd.UTC()) {
					view.TrimmedCount++
					continue
				}
				view.Points = append(view.Points, app.ReplayPoint{
					Sequence:   seq,
					At:         ts,
					ReceivedAt: rcv,
					Lat:        lat[i],
					Lon:        lon[i],
					Accuracy:   acc[i],
					Speed:      spd[i],
					Bearing:    brg[i],
					Coarse:     crs[i],
				})
			}
		}
		return srows.Err()
	})
	if err != nil {
		return app.ReplayView{}, err
	}
	issued, err := s.NewID()
	if err != nil {
		return app.ReplayView{}, app.ErrAccessUnrecorded
	}
	served, err := s.NewID()
	if err != nil {
		return app.ReplayView{}, app.ErrAccessUnrecorded
	}
	record := audit.SensitiveAccess{
		Category:     audit.LocationTrack,
		Stage:        audit.AccessIssued,
		AccountID:    actor.AccountID,
		MembershipID: member,
		TargetID:     request.TripID,
		AccessID:     issued,
		RequestID:    issued,
		Binding:      replayBinding(scope, view),
	}
	if err := s.Access.RecordAccess(ctx, scope, record); err != nil {
		return app.ReplayView{}, app.ErrAccessUnrecorded
	}
	record.Stage, record.RequestID = audit.AccessServed, served
	if err := s.Access.RecordAccess(ctx, scope, record); err != nil {
		return app.ReplayView{}, app.ErrAccessUnrecorded
	}
	return view, nil
}
