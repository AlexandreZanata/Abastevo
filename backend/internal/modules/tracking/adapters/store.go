package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// Store is the PostgreSQL tracking store over the borrowed intake
// transaction (EDR-277/279): parametrized SQL only, organization-scoped by
// forced row security, insert-only. Errors carry no stored value.
type Store struct{}

var _ app.Store = (*Store)(nil)

func (s *Store) port(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope) (syncapp.DBTX, error) {
	db = syncapp.Borrow(db)
	if s == nil || ctx == nil || db == nil || database.ValidateOrgDBTX(ctx, db, scope) != nil {
		return nil, app.ErrUnavailable
	}
	return db, nil
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

// LockTripTx takes the trip row lock that serializes every tracking write
// of the trip with its finish.
func (s *Store) LockTripTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, trip string) error {
	q, err := s.port(ctx, db, scope)
	if err != nil {
		return err
	}
	var one int
	if err := q.QueryRow(ctx, `SELECT 1 FROM org_trips WHERE org_id=$1 AND id=$2 FOR UPDATE`, scope.OrgID().String(), trip).Scan(&one); err != nil {
		return storeError(err)
	}
	return nil
}

// PolicyTx reads the policy in force at the database's clock; no recorded
// version is OFF with revision 0.
func (s *Store) PolicyTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope) (app.PolicyVersion, error) {
	q, err := s.port(ctx, db, scope)
	if err != nil {
		return app.PolicyVersion{}, err
	}
	var policy string
	var revision int64
	err = q.QueryRow(ctx, `SELECT policy,revision FROM org_tracking_policy_in_force($1,clock_timestamp())`, scope.OrgID().String()).Scan(&policy, &revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.PolicyVersion{Policy: domain.PolicyOff}, nil
	}
	if err != nil {
		return app.PolicyVersion{}, storeError(err)
	}
	parsed, err := domain.ParsePolicy(policy)
	if err != nil || revision < 1 {
		return app.PolicyVersion{}, app.ErrUnavailable
	}
	return app.PolicyVersion{Policy: parsed, Revision: revision}, nil
}

func (s *Store) session(ctx context.Context, q syncapp.DBTX, scope kernel.OrgScope, byTrip bool, key string) (app.SessionRecord, error) {
	query := `SELECT id::text,trip_id::text,owner_membership_id::text,operation_id::text,policy,policy_revision,opened_at FROM org_tracking_sessions WHERE org_id=$1 AND id=$2`
	if byTrip {
		query = `SELECT id::text,trip_id::text,owner_membership_id::text,operation_id::text,policy,policy_revision,opened_at FROM org_tracking_sessions WHERE org_id=$1 AND trip_id=$2`
	}
	var r app.SessionRecord
	var policy string
	if err := q.QueryRow(ctx, query, scope.OrgID().String(), key).Scan(&r.ID, &r.TripID, &r.OwnerMembershipID, &r.OperationID, &policy, &r.PolicyRevision, &r.OpenedAt); err != nil {
		return app.SessionRecord{}, storeError(err)
	}
	var err error
	if r.Policy, err = domain.ParsePolicy(policy); err != nil {
		return app.SessionRecord{}, app.ErrUnavailable
	}
	r.OpenedAt = r.OpenedAt.UTC()
	rows, err := q.Query(ctx, `SELECT seq,state,COALESCE(cause,''),actor,COALESCE(permission,''),occurred_at,operation_id::text FROM org_tracking_transitions
 WHERE org_id=$1 AND session_id=$2 ORDER BY seq`, scope.OrgID().String(), r.ID)
	if err != nil {
		return app.SessionRecord{}, storeError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var t app.StoredTransition
		var state, cause, actor, permission string
		if err := rows.Scan(&t.Seq, &state, &cause, &actor, &permission, &t.At, &t.OperationID); err != nil {
			return app.SessionRecord{}, storeError(err)
		}
		t.State, t.Cause, t.Actor, t.Permission, t.At = domain.State(state), domain.Cause(cause), domain.Actor(actor), domain.Permission(permission), t.At.UTC()
		r.Transitions = append(r.Transitions, t)
	}
	if err := rows.Err(); err != nil {
		return app.SessionRecord{}, storeError(err)
	}
	return r, nil
}

// SessionTx reads one session with its history.
func (s *Store) SessionTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, id string) (app.SessionRecord, error) {
	q, err := s.port(ctx, db, scope)
	if err != nil {
		return app.SessionRecord{}, err
	}
	return s.session(ctx, q, scope, false, id)
}

// SessionOfTripTx reads the trip's one session with its history.
func (s *Store) SessionOfTripTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, trip string) (app.SessionRecord, error) {
	q, err := s.port(ctx, db, scope)
	if err != nil {
		return app.SessionRecord{}, err
	}
	return s.session(ctx, q, scope, true, trip)
}

// RecordSessionTx inserts the opening session row and the new transitions.
func (s *Store) RecordSessionTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, change app.SessionChange) error {
	q, err := s.port(ctx, db, scope)
	if err != nil {
		return err
	}
	org := scope.OrgID().String()
	if source.Scope != scope || len(change.Transitions) == 0 {
		return app.ErrUnavailable
	}
	if change.Open {
		v := change.Session
		if v.OwnerMembershipID != source.MembershipID {
			return app.ErrUnavailable
		}
		if _, err := q.Exec(ctx, `INSERT INTO org_tracking_sessions(org_id,id,trip_id,owner_membership_id,policy,policy_revision,operation_id,opened_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
			org, v.ID, v.TripID, v.OwnerMembershipID, string(v.Policy), v.PolicyRevision, v.OperationID, v.OpenedAt); err != nil {
			return storeError(err)
		}
	}
	for _, t := range change.Transitions {
		var cause, permission *string
		if t.Cause != "" {
			c := string(t.Cause)
			cause = &c
		}
		if t.Permission != "" {
			p := string(t.Permission)
			permission = &p
		}
		if _, err := q.Exec(ctx, `INSERT INTO org_tracking_transitions(org_id,session_id,seq,state,cause,actor,permission,occurred_at,operation_id)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			org, change.Session.ID, t.Seq, string(t.State), cause, string(t.Actor), permission, t.At, t.OperationID); err != nil {
			return storeError(err)
		}
	}
	return nil
}

// SegmentTx reads the stored identity and counters of one batch.
func (s *Store) SegmentTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, operation string) (app.SegmentRecord, error) {
	q, err := s.port(ctx, db, scope)
	if err != nil {
		return app.SegmentRecord{}, err
	}
	var r app.SegmentRecord
	if err := q.QueryRow(ctx, `SELECT operation_id::text,payload_digest,session_id::text,trip_id::text,author_membership_id::text,enrollment_id::text,seq,
 received_count,stored_count,duplicate_count,outside_session_count,mock_count FROM org_tracking_segments WHERE org_id=$1 AND operation_id=$2`, scope.OrgID().String(), operation).
		Scan(&r.OperationID, &r.PayloadDigest, &r.SessionID, &r.TripID, &r.AuthorMembershipID, &r.EnrollmentID, &r.Seq, &r.Received, &r.Stored, &r.Duplicates, &r.Outside, &r.Mock); err != nil {
		return app.SegmentRecord{}, storeError(err)
	}
	return r, nil
}

// SequenceTakenTx reports whether the session already holds a segment with
// this sequence.
func (s *Store) SequenceTakenTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, session string, seq int64) (bool, error) {
	q, err := s.port(ctx, db, scope)
	if err != nil {
		return false, err
	}
	var taken bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM org_tracking_segments WHERE org_id=$1 AND session_id=$2 AND seq=$3)`, scope.OrgID().String(), session, seq).Scan(&taken); err != nil {
		return false, storeError(err)
	}
	return taken, nil
}

// StoredInstantsTx lists the session's stored fix instants inside a span.
func (s *Store) StoredInstantsTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, session string, from, to time.Time) ([]time.Time, error) {
	q, err := s.port(ctx, db, scope)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `SELECT u.at FROM org_tracking_segments s CROSS JOIN LATERAL unnest(s.collected_at) AS u(at)
 WHERE s.org_id=$1 AND s.session_id=$2 AND s.first_at<=$4 AND s.last_at>=$3 AND u.at BETWEEN $3 AND $4`, scope.OrgID().String(), session, from, to)
	if err != nil {
		return nil, storeError(err)
	}
	defer rows.Close()
	var out []time.Time
	for rows.Next() {
		var at time.Time
		if err := rows.Scan(&at); err != nil {
			return nil, storeError(err)
		}
		out = append(out, at.UTC())
	}
	if err := rows.Err(); err != nil {
		return nil, storeError(err)
	}
	return out, nil
}

// InsertSegmentTx stores one accepted batch; the database derives the
// geometry and re-proves every fix.
func (s *Store) InsertSegmentTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, segment app.Segment) error {
	q, err := s.port(ctx, db, scope)
	if err != nil {
		return err
	}
	b := segment.Batch
	if source.Scope != scope || segment.AuthorMembershipID != source.MembershipID || segment.EnrollmentID != source.EnrollmentID ||
		len(segment.Fixes)+segment.Duplicates+segment.Outside+segment.Mock != len(b.Fixes) {
		return app.ErrUnavailable
	}
	n := len(segment.Fixes)
	instants, lat, lon, accuracy := make([]time.Time, n), make([]int32, n), make([]int32, n), make([]int32, n)
	speed, bearing, coarse := make([]*int32, n), make([]*int16, n), make([]bool, n)
	for i, f := range segment.Fixes {
		instants[i], lat[i], lon[i], accuracy[i], speed[i], bearing[i], coarse[i] = f.At, f.Lat, f.Lon, f.Accuracy, f.Speed, f.Bearing, f.Coarse
	}
	var first, last *time.Time
	if n > 0 {
		first, last = &instants[0], &instants[n-1]
	}
	if _, err := q.Exec(ctx, `INSERT INTO org_tracking_segments(org_id,operation_id,payload_digest,session_id,trip_id,seq,author_membership_id,enrollment_id,source,boot_id,base_elapsed_millis,
 base_at,first_at,last_at,collected_at,lat_e7,lon_e7,accuracy_m,speed_cm_s,bearing_deg,coarse,received_count,stored_count,duplicate_count,outside_session_count,mock_count,
 dropped_accuracy,dropped_stale,dropped_jump,dropped_mock)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30)`,
		scope.OrgID().String(), b.OperationID, b.PayloadDigest, b.SessionID, b.TripID, b.Seq, segment.AuthorMembershipID, segment.EnrollmentID, b.Source, b.BootID, b.BaseElapsed,
		b.Base, first, last, instants, lat, lon, accuracy, speed, bearing, coarse, len(b.Fixes), n, segment.Duplicates, segment.Outside, segment.Mock,
		b.Dropped.Accuracy, b.Dropped.Stale, b.Dropped.Jump, b.Dropped.Mock); err != nil {
		return storeError(err)
	}
	return nil
}
