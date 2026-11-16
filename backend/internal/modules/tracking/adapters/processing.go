package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ProcessPayload is the reference-only versioned job envelope: the
// organization and the session, never a coordinate (BR-PRIV-003).
type ProcessPayload struct {
	Version   int    `json:"version"`
	OrgID     string `json:"org_id"`
	SessionID string `json:"session_id"`
}

var errProcessPayload = errors.New("tracking: unsupported processing payload")

// ProcessDedupe is the queue key of one processing input: a new segment or
// transition is a new input and a new job; a repeat converges.
func ProcessDedupe(scope kernel.OrgScope, session string, w app.Watermark) string {
	if scope.Validate() != nil || !canonicalID(session) || w.Segments < 0 || w.Transitions < 2 {
		return ""
	}
	return app.ProcessKind + ":" + scope.OrgID().String() + ":" + session + ":" + strconv.Itoa(w.Segments) + ":" + strconv.Itoa(w.Transitions)
}

// ParseProcess accepts only the exact version-1 envelope.
func ParseProcess(raw []byte) (kernel.OrgScope, string, error) {
	var p ProcessPayload
	if httpapi.DecodeJSON(raw, &p) != nil || p.Version != 1 || !canonicalID(p.OrgID) || !canonicalID(p.SessionID) {
		return kernel.OrgScope{}, "", errProcessPayload
	}
	id, err := kernel.ParseOrgID(p.OrgID)
	if err != nil {
		return kernel.OrgScope{}, "", errProcessPayload
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		return kernel.OrgScope{}, "", errProcessPayload
	}
	return scope, p.SessionID, nil
}

func watermark(ctx context.Context, q syncapp.DBTX, org, session string) (app.Watermark, bool, error) {
	var w app.Watermark
	var ended bool
	err := q.QueryRow(ctx, `SELECT(SELECT count(*) FROM org_tracking_segments WHERE org_id=$1 AND session_id=$2),(SELECT count(*) FROM org_tracking_transitions WHERE org_id=$1 AND session_id=$2),
 EXISTS(SELECT 1 FROM org_tracking_transitions WHERE org_id=$1 AND session_id=$2 AND state='ENDED')`, org, session).Scan(&w.Segments, &w.Transitions, &ended)
	return w, ended, err
}

// requestProcessing queues the processing of an ended session's current
// input in the caller's transaction (B-BR-EMP-461); a session still open
// queues nothing.
func requestProcessing(ctx context.Context, q syncapp.DBTX, scope kernel.OrgScope, session string) error {
	w, ended, err := watermark(ctx, q, scope.OrgID().String(), session)
	if err != nil {
		return storeError(err)
	}
	if !ended {
		return nil
	}
	dedupe := ProcessDedupe(scope, session, w)
	payload, err := json.Marshal(ProcessPayload{Version: 1, OrgID: scope.OrgID().String(), SessionID: session})
	if err != nil || dedupe == "" {
		return app.ErrUnavailable
	}
	if _, err := jobs.Enqueue(ctx, q, app.ProcessKind, payload, dedupe, app.ProcessAttempts, time.Time{}); err != nil {
		return storeError(err)
	}
	return nil
}

// ProcessingStore runs one fenced processing of a session in one scoped
// transaction (B-BR-EMP-461/466): the job's own queue row under its lease,
// then the trip row, then the stored input, then the summary.
type ProcessingStore struct {
	Pool    *pgxpool.Pool
	Trips   app.Trips
	Session *Store
}

var _ app.Processor = (*ProcessingStore)(nil)

func segments(ctx context.Context, tx pgx.Tx, org, session string) ([]app.ProcessingSegment, error) {
	rows, err := tx.Query(ctx, `SELECT seq,boot_id,base_elapsed_millis,base_at,collected_at,lat_e7,lon_e7,accuracy_m,coarse,received_count,duplicate_count,outside_session_count,mock_count,
 dropped_accuracy,dropped_stale,dropped_jump,dropped_mock FROM org_tracking_segments WHERE org_id=$1 AND session_id=$2 ORDER BY seq`, org, session)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []app.ProcessingSegment
	for rows.Next() {
		var g app.ProcessingSegment
		var at []time.Time
		var lat, lon, accuracy []int32
		var coarse []bool
		if err := rows.Scan(&g.Seq, &g.BootID, &g.BaseElapsed, &g.Base, &at, &lat, &lon, &accuracy, &coarse, &g.Received, &g.Duplicates, &g.Outside, &g.Mock,
			&g.Dropped.Accuracy, &g.Dropped.Stale, &g.Dropped.Jump, &g.Dropped.Mock); err != nil {
			return nil, err
		}
		if len(lat) != len(at) || len(lon) != len(at) || len(accuracy) != len(at) || len(coarse) != len(at) {
			return nil, app.ErrUnavailable
		}
		g.Base = g.Base.UTC()
		for i := range at {
			g.Fixes = append(g.Fixes, app.StoredFix{At: at[i].UTC(), Lat: lat[i], Lon: lon[i], Accuracy: accuracy[i], Coarse: coarse[i]})
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func insertSummary(ctx context.Context, tx pgx.Tx, org string, revision int, lease app.Lease, s app.Summary) error {
	r := s.Result
	n := len(r.Gaps)
	from, to, reason, by := make([]time.Time, n), make([]time.Time, n), make([]string, n), make([]string, n)
	for i, g := range r.Gaps {
		from[i], to[i], reason[i], by[i] = g.From, g.To, string(g.Reason), string(g.Detector)
	}
	parts, lat, lon := []int32{}, []int32{}, []int32{}
	for _, line := range r.Lines {
		parts = append(parts, int32(len(line)))
		for _, p := range line {
			lat, lon = append(lat, p.Lat), append(lon, p.Lon)
		}
	}
	var coverage *int64
	if v, ok := r.Classification.Coverage(); ok {
		coverage = &v
	}
	var method *string
	if r.DistanceMetres != nil {
		m := domain.DistanceMethod
		method = &m
	}
	var tolerance *int
	if len(r.Lines) > 0 {
		tolerance = &r.ToleranceMetres
	}
	_, err := tx.Exec(ctx, `INSERT INTO org_tracking_summaries(org_id,session_id,revision,trip_id,method,input_segments,input_transitions,job_id,lease_token,expected_seconds,gap_seconds,
 coverage_permille,coarse,quality,accepted_fixes,coarse_fixes,rejected_accuracy,rejected_speed,rejected_outside,received_fixes,duplicate_fixes,outside_session_fixes,mock_fixes,
 device_dropped_accuracy,device_dropped_stale,device_dropped_jump,device_dropped_mock,clock_uncertain_segments,distance_m,distance_method,gap_from,gap_to,gap_reason,gap_detected_by,
 line_parts,line_lat_e7,line_lon_e7,tolerance_m)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36,$37,$38)`,
		org, s.SessionID, revision, s.TripID, domain.MethodVersion, s.Watermark.Segments, s.Watermark.Transitions, lease.JobID, lease.Token, r.ExpectedSeconds, r.GapSeconds,
		coverage, r.CoarsePeriod, string(r.Classification.Quality()), r.Accepted, r.CoarseAccepted, r.RejectedAccuracy, r.RejectedSpeed, r.RejectedOutside, s.Received, s.Duplicates,
		s.OutsideSession, s.Mock, s.DroppedAccuracy, s.DroppedStale, s.DroppedJump, s.DroppedMock, s.ClockUncertain, r.DistanceMetres, method, from, to, reason, by, parts, lat, lon, tolerance)
	return err
}

// Process runs one fenced processing of a session (B-BR-EMP-461..466).
func (s *ProcessingStore) Process(ctx context.Context, scope kernel.OrgScope, session string, lease app.Lease) (app.ProcessOutcome, int, error) {
	if s == nil || s.Pool == nil || absent(s.Trips) || s.Session == nil || scope.Validate() != nil || !canonicalID(session) || !canonicalID(lease.JobID) || lease.Token < 1 {
		return "", 0, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	var outcome app.ProcessOutcome
	revision := 0
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		// The lease fence: only the current holder of this job writes, and
		// holding its row keeps another claim of the job out until commit.
		var held int
		if err := tx.QueryRow(ctx, `SELECT 1 FROM job_queue WHERE id=$1 AND lease_token=$2 AND status='leased' AND kind=$3 FOR UPDATE`, lease.JobID, lease.Token, app.ProcessKind).Scan(&held); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return app.ErrStaleLease
			}
			return err
		}
		var trip string
		err := tx.QueryRow(ctx, `SELECT trip_id::text FROM org_tracking_sessions WHERE org_id=$1 AND id=$2`, org, session).Scan(&trip)
		if errors.Is(err, pgx.ErrNoRows) {
			outcome = app.ProcessMissing
			return nil
		}
		if err != nil {
			return err
		}
		if err := s.Session.LockTripTx(ctx, tx, scope, trip); err != nil {
			return err
		}
		w, ended, err := watermark(ctx, tx, org, session)
		if err != nil {
			return err
		}
		if !ended {
			outcome = app.ProcessNotEnded
			return nil
		}
		var current app.Watermark
		err = tx.QueryRow(ctx, `SELECT revision,input_segments,input_transitions FROM org_tracking_summaries WHERE org_id=$1 AND session_id=$2 ORDER BY revision DESC LIMIT 1`, org, session).
			Scan(&revision, &current.Segments, &current.Transitions)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return err
		case current == w:
			outcome = app.ProcessCurrent
			return nil
		case !w.Newer(current):
			return app.ErrUnavailable
		}
		record, err := s.Session.SessionTx(ctx, tx, scope, session)
		if err != nil {
			return err
		}
		tripRecord, err := s.Trips.ReadTripTx(ctx, tx, scope, trip)
		if err != nil {
			return err
		}
		stored, err := segments(ctx, tx, org, session)
		if err != nil {
			return err
		}
		summary, err := app.Summarize(scope, record, tripRecord, stored)
		if err != nil {
			return err
		}
		if summary.Watermark != w {
			return app.ErrUnavailable
		}
		revision++
		outcome = app.ProcessWritten
		return insertSummary(ctx, tx, org, revision, lease, summary)
	})
	if err != nil {
		if errors.Is(err, app.ErrStaleLease) || ctx.Err() != nil {
			return "", 0, err
		}
		return "", 0, storeError(err)
	}
	return outcome, revision, nil
}

// ProcessHandler runs the queued processing of one session.
type ProcessHandler struct{ Processor app.Processor }

var _ jobs.Handler = ProcessHandler{}

func (ProcessHandler) Kind() string { return app.ProcessKind }
func (ProcessHandler) Version() int { return 1 }

// Handle reports only static module errors to the queue: a missing or
// another organization's session, a session not ended yet and an input
// already summarized complete the job with no change; a superseded lease
// writes nothing.
func (h ProcessHandler) Handle(ctx context.Context, job jobs.Job) error {
	if absent(h.Processor) {
		return app.ErrUnavailable
	}
	if job.Kind != app.ProcessKind {
		return errProcessPayload
	}
	scope, session, err := ParseProcess(job.Payload)
	if err != nil {
		return errProcessPayload
	}
	prefix := app.ProcessKind + ":" + scope.OrgID().String() + ":" + session + ":"
	if !strings.HasPrefix(job.DedupeKey, prefix) || len(job.DedupeKey) == len(prefix) {
		return errProcessPayload
	}
	_, _, err = h.Processor.Process(ctx, scope, session, app.Lease{JobID: job.ID, Token: job.LeaseToken})
	switch {
	case err == nil:
		return nil
	case errors.Is(err, app.ErrStaleLease):
		return jobs.ErrStaleLease
	}
	return app.ErrUnavailable
}
