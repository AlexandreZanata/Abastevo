//go:build integration

package adapters_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	trackingapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func trackingSQLCode(t *testing.T, err error, code, constraint string) {
	t.Helper()
	var pe *pgconn.PgError
	if !errors.As(err, &pe) || pe.Code != code || constraint != "" && pe.ConstraintName != constraint {
		t.Fatalf("tracking SQL refusal mismatch, want %s %s, got %v", code, constraint, err)
	}
	if strings.Contains(pe.Detail+pe.Message, "100000") || strings.Contains(pe.Detail+pe.Message, "-99") {
		t.Fatal("a SQL refusal carries a position")
	}
}

// insertSegment writes a segment row directly under the runtime login.
func (x trackingState) insertSegment(t *testing.T, op protocol.Operation, session string, seq int, instants []time.Time, coarse bool) error {
	t.Helper()
	n := len(instants)
	lat, lon, accuracy, flags := make([]int32, n), make([]int32, n), make([]int32, n), make([]bool, n)
	speed, bearing := make([]*int32, n), make([]*int16, n)
	for i := range instants {
		lat[i], lon[i], accuracy[i], flags[i] = -100000000+int32(i), -500000000-int32(i), 8, coarse
	}
	return database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO org_tracking_segments(org_id,operation_id,payload_digest,session_id,trip_id,seq,author_membership_id,enrollment_id,source,base_at,first_at,last_at,
 collected_at,lat_e7,lon_e7,accuracy_m,speed_cm_s,bearing_deg,coarse,received_count,stored_count,duplicate_count,outside_session_count,mock_count,dropped_accuracy,dropped_stale,dropped_jump,dropped_mock)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,'PHONE',$9,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$18,0,0,0,0,0,0,0)`,
			x.scope.OrgID().String(), op.ID(), op.PayloadDigest(), session, x.trip, seq, x.source.MembershipID, x.source.EnrollmentID, instants[0], instants[n-1],
			instants, lat, lon, accuracy, speed, bearing, flags, n)
		return err
	})
}

// B-BR-EMP-430/432..434: forced row security, insert-only history, and the
// database's own refusals of a fix outside the session, after the finish,
// with the wrong precision or already stored, and of an unpublished row.
func TestTrackingSchemaGuardsSessionsAndSegments(t *testing.T) {
	x := trackingFixture(t)
	d := x.syncFixtureState
	x.inForce(t)
	org := x.scope.OrgID().String()
	var forced bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT bool_and(relrowsecurity AND relforcerowsecurity) AND count(*)=4 FROM pg_class
 WHERE oid=ANY(ARRAY['org_tracking_policies'::regclass,'org_tracking_sessions'::regclass,'org_tracking_transitions'::regclass,'org_tracking_segments'::regclass])`).Scan(&forced); err != nil || !forced {
		t.Fatal("tracking tables without forced row security", err)
	}
	var keys bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*)=3 FROM pg_constraint WHERE conname IN('org_tracking_sessions_trip_key','org_tracking_segments_sequence_key','org_tracking_segments_pkey')
 AND contype IN('u','p')`).Scan(&keys); err != nil || !keys {
		t.Fatal("one session per trip and one segment per batch and sequence are not database keys", err)
	}
	session := onboardingID(t)
	x.startTracking(t, d, session, 10, "PRECISE", 2)
	first := x.batchOp(t, d, session, 1, []int{10, 20}, nil)
	ack, err := x.process(t, d, first)
	x.accepted(t, first, ack, err, 1, trackingCounts{segments: 1})
	// A rejected batch claim carries a valid source but no publication.
	used := x.batchOp(t, d, session, 1, []int{40}, nil)
	ack, err = x.process(t, d, used)
	x.rejected(t, used, ack, err, "MALFORMED")
	t.Run("guards", func(t *testing.T) {
		trackingSQLCode(t, x.insertSegment(t, first, session, 9, []time.Time{x.at(5)}, false), "23514", "")
		trackingSQLCode(t, x.insertSegment(t, first, session, 9, []time.Time{x.at(30)}, true), "23514", "")
		trackingSQLCode(t, x.insertSegment(t, first, session, 9, []time.Time{x.at(30), x.at(30)}, false), "23514", "")
		trackingSQLCode(t, x.insertSegment(t, first, session, 9, []time.Time{x.at(20), x.at(30)}, false), "23505", "org_tracking_segments_fix_key")
		trackingSQLCode(t, x.insertSegment(t, first, session, 9, []time.Time{x.at(30)}, false), "23505", "org_tracking_segments_pkey")
		trackingSQLCode(t, x.insertSegment(t, used, session, 1, []time.Time{x.at(30)}, false), "23505", "org_tracking_segments_sequence_key")
		start := x.count(t, first)
		if start.segments != 1 {
			t.Fatal("a refused insert changed the stored segment")
		}
	})
	t.Run("unpublished", func(t *testing.T) {
		trackingSQLCode(t, x.insertSegment(t, used, session, 9, []time.Time{x.at(30)}, false), "23514", "")
	})
	t.Run("while-paused", func(t *testing.T) {
		// EMP-TRIP-011: an instant while the trip is paused is not a recording
		// instant, even inside the stored recording period.
		x.lifecycle(t, d, tripapp.PauseOperationType, 1, 45)
		trackingSQLCode(t, x.insertSegment(t, used, session, 9, []time.Time{x.at(50)}, false), "23514", "")
	})
	t.Run("after-finish", func(t *testing.T) {
		end := x.finishOp(t, d, x.trip, x.at(60), nil)
		endAck, err := x.tripState.process(t, d, end)
		x.tripState.accepted(t, end, endAck, err, 3, 1)
		trackingSQLCode(t, x.insertSegment(t, used, session, 9, []time.Time{x.at(60)}, false), "23514", "")
	})
	t.Run("insert-only", func(t *testing.T) {
		role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
		if _, err := x.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE ON org_tracking_policies,org_tracking_sessions,org_tracking_transitions,org_tracking_segments TO "+role); err != nil {
			t.Fatal(err)
		}
		for _, statement := range []string{
			`UPDATE org_tracking_segments SET seq=seq WHERE org_id=$1`, `DELETE FROM org_tracking_segments WHERE org_id=$1`,
			`UPDATE org_tracking_transitions SET seq=seq WHERE org_id=$1`, `DELETE FROM org_tracking_transitions WHERE org_id=$1`,
			`UPDATE org_tracking_sessions SET policy=policy WHERE org_id=$1`, `DELETE FROM org_tracking_sessions WHERE org_id=$1`,
			`UPDATE org_tracking_policies SET policy=policy WHERE org_id=$1`, `DELETE FROM org_tracking_policies WHERE org_id=$1`,
		} {
			err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error { _, err := tx.Exec(t.Context(), statement, org); return err })
			trackingSQLCode(t, err, "55000", "")
		}
		for _, table := range []string{"org_tracking_segments", "org_tracking_transitions", "org_tracking_sessions", "org_tracking_policies"} {
			_, err := x.f.Admin.Exec(t.Context(), "TRUNCATE "+table+" CASCADE")
			trackingSQLCode(t, err, "55000", "")
		}
	})
	t.Run("other-organization", func(t *testing.T) {
		other, _, err := x.onboarding.Create(t.Context(), x.actor, onboardingCommand(t, onboardingID(t)))
		if err != nil {
			t.Fatal(err)
		}
		foreign := scopeFor(t, other.OrgID.String())
		var seen int
		if err := database.WithOrgTx(t.Context(), x.f.Runtime, foreign, func(tx pgx.Tx) error {
			return tx.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_tracking_policies)+(SELECT count(*) FROM org_tracking_sessions)+(SELECT count(*) FROM org_tracking_transitions)+(SELECT count(*) FROM org_tracking_segments)`).Scan(&seen)
		}); err != nil || seen != 0 {
			t.Fatal("another organization sees tracking rows", seen, err)
		}
		err = database.WithOrgTx(t.Context(), x.f.Runtime, foreign, func(tx pgx.Tx) error {
			_, err := tx.Exec(t.Context(), `INSERT INTO org_tracking_transitions(org_id,session_id,seq,state,cause,actor,occurred_at,operation_id)VALUES($1,$2,9,'ENDED','DEVICE_REVOKED','SERVER',$3,$4)`, org, session, x.at(70), first.ID())
			return err
		})
		trackingSQLCode(t, err, "42501", "")
		var record trackingapp.SessionRecord
		err = database.WithOrgTx(t.Context(), x.f.Runtime, foreign, func(tx pgx.Tx) error {
			record, err = x.tracking.SessionTx(t.Context(), syncapp.Borrow(tx), foreign, session)
			return err
		})
		if !errors.Is(err, trackingapp.ErrNotFound) || record.ID != "" {
			t.Fatal("another organization reads the session", err)
		}
	})
}
