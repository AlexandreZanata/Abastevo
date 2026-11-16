//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	trackingpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/adapters"
	trackingapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
)

// trackingState is the trip fixture with one trip in progress, the real
// tracking owners and an intake store that declares them (EMP-GPS-004).
type trackingState struct {
	tripState
	tracking *trackingpg.Store
	handlers map[string]syncapp.Handler
	intake   *syncpg.Store
	trip     string
	tripAt   time.Time
}

func trackingGrants(t *testing.T, fx legalFixtureState) {
	t.Helper()
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT ON org_tracking_policies TO "+role+";GRANT SELECT,INSERT ON org_tracking_sessions,org_tracking_transitions,org_tracking_segments TO "+role); err != nil {
		t.Fatal(err)
	}
}

func trackingIntakeConfig(fx syncFixtureState) syncpg.Config {
	config := tripIntakeConfig(fx)
	for _, kind := range append(trackingapp.SessionOperationTypes(), trackingapp.BatchOperationType) {
		config.NoAsyncPlans[syncapp.OperationKey{Type: kind, Schema: 1}] = map[string]syncapp.FollowupMode{protocol.Accepted: syncapp.NoAsyncFollowup, protocol.Rejected: syncapp.NoAsyncFollowup}
	}
	return config
}

func (x *trackingState) install(t *testing.T, store trackingapp.Store) {
	t.Helper()
	sessions, err := trackingpg.NewSessionHandlers(x.authority, x.store, store)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := trackingpg.NewBatchHandler(x.authority, x.store, store)
	if err != nil {
		t.Fatal(err)
	}
	x.handlers = map[string]syncapp.Handler{trackingapp.BatchOperationType: batch}
	for _, h := range sessions {
		x.handlers[h.Descriptor().Type] = h
	}
	// EMP-TRIP-011: the real trip lifecycle owners on the same intake.
	for _, kind := range tripapp.LifecycleOperationTypes {
		h, err := trippg.NewTransitionHandler(kind, x.authority, x.meters, x.store, x.store)
		if err != nil {
			t.Fatal(err)
		}
		x.handlers[kind] = h
	}
	if len(x.handlers) != 10 {
		t.Fatal("tracking owners", len(x.handlers))
	}
}

func trackingFixture(t *testing.T) trackingState {
	t.Helper()
	return trackingFixtureOn(t, tripFixture(t))
}

func trackingFixtureOn(t *testing.T, base tripState) trackingState {
	t.Helper()
	x := trackingState{tripState: base}
	trackingGrants(t, x.legalFixtureState)
	tripReviewGrants(t, x.f)
	x.tracking = &trackingpg.Store{}
	x.install(t, x.tracking)
	x.intake = syncpg.NewStore(trackingIntakeConfig(x.syncFixtureState))
	d := x.syncFixtureState
	x.tripAt = x.declaredNow(t)
	op := x.startOp(t, d, x.tripAt, nil)
	ack, err := x.tripState.process(t, d, op)
	x.tripState.accepted(t, op, ack, err, 1, 1)
	x.trip = tripOf(op)
	return x
}

// policy records one policy version the way a settings owner would.
func (x trackingState) policy(t *testing.T, policy string, notice *time.Time) {
	t.Helper()
	tx, err := x.f.Admin.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	org := x.scope.OrgID().String()
	var version *string
	if notice != nil {
		v := "notice-2026.11"
		version = &v
	}
	if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `INSERT INTO org_tracking_policies(org_id,revision,policy,notice_version,notice_in_force_at)
 SELECT $1,COALESCE(max(revision),0)+1,$2,$3,$4 FROM org_tracking_policies WHERE org_id=$1`, org, policy, version, notice); err != nil {
		t.Fatal("tracking policy fixture", err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// at is a whole-microsecond instant n seconds after the trip started.
func (x trackingState) at(seconds int) time.Time {
	return x.tripAt.Truncate(time.Microsecond).Add(time.Microsecond).Add(time.Duration(seconds) * time.Second)
}

func (x trackingState) sessionOp(t *testing.T, d syncFixtureState, kind, session string, declared time.Time, payload map[string]any, edit func(envelope, payload map[string]any)) protocol.Operation {
	t.Helper()
	return syncHTTPOperation(t, d.deviceFixtureState, d.source.EnrollmentID, func(raw map[string]any) {
		raw["type"] = kind
		raw["entity"] = map[string]any{"type": trackingapp.EntityType, "id": session, "expected_revision": nil}
		raw["occurred_at"] = orgtime.FormatInstant(declared)
		raw["entry_zone"] = "America/Cuiaba"
		raw["device_offset_seconds"] = -14400
		raw["payload"] = payload
		if edit != nil {
			edit(raw, payload)
		}
	})
}

func (x trackingState) driverOp(t *testing.T, d syncFixtureState, kind, session string, declared time.Time, permission string, edit func(envelope, payload map[string]any)) protocol.Operation {
	t.Helper()
	return x.sessionOp(t, d, kind, session, declared, map[string]any{"trip_id": x.trip, "origin": "TRIP_SCREEN", "permission": permission, "notification_visible": true}, edit)
}

func (x trackingState) eventOp(t *testing.T, d syncFixtureState, kind, session string, declared time.Time, cause string) protocol.Operation {
	t.Helper()
	return x.sessionOp(t, d, kind, session, declared, map[string]any{"trip_id": x.trip, "cause": cause}, nil)
}

// batchOp sends fixes at the given seconds after the trip start, the first
// one as the base instant; positions move by one unit per fix.
func (x trackingState) batchOp(t *testing.T, d syncFixtureState, session string, seq int, seconds []int, edit func(envelope, payload map[string]any)) protocol.Operation {
	t.Helper()
	base := x.at(seconds[0])
	list := make([]any, len(seconds))
	for i, s := range seconds {
		list[i] = []any{(s - seconds[0]) * 1000, 1, -1, 8, 1389, 270, 0}
	}
	payload := map[string]any{"trip_id": x.trip, "seq": seq, "source": "PHONE", "boot_id": "boot-a1", "base_elapsed_millis": 9063000,
		"base": map[string]any{"lat_e7": -100000000, "lon_e7": -500000000}, "fixes": list, "dropped": map[string]any{"accuracy": 1, "stale": 0, "jump": 0, "mock": 0}}
	return x.sessionOp(t, d, trackingapp.BatchOperationType, session, base, payload, edit)
}

func (x trackingState) process(t *testing.T, d syncFixtureState, op protocol.Operation) (protocol.Acknowledgement, error) {
	t.Helper()
	return x.intake.Process(t.Context(), d.source, op, protocol.DefaultLimits(), x.handlers[op.Snapshot().Type])
}

type trackingCounts struct{ sessions, transitions, segments, claims, receipts, audits, effects int }

func (x trackingState) count(t *testing.T, op protocol.Operation) trackingCounts {
	t.Helper()
	var c trackingCounts
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_tracking_sessions WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_tracking_transitions WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_tracking_segments WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_sync_operations WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_sync_effects WHERE org_id=$1 AND operation_id=$2)`, x.scope.OrgID().String(), op.ID()).Scan(&c.sessions, &c.transitions, &c.segments, &c.claims, &c.receipts, &c.audits, &c.effects); err != nil {
		t.Fatal(err)
	}
	return c
}

func (x trackingState) accepted(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error, revision int64, want trackingCounts) {
	t.Helper()
	if err != nil || ack.Result() != protocol.Accepted || ack.Snapshot().EntityRevision == nil || *ack.Snapshot().EntityRevision != revision || len(ack.Warnings()) != 0 {
		t.Fatal("tracking operation not accepted", err, ack.Result(), ack.Code())
	}
	want.claims, want.receipts, want.audits, want.effects = 1, 1, 1, 1
	if c := x.count(t, op); c != want {
		t.Fatalf("accepted tracking operation not stored exactly once: %+v", c)
	}
	if strings.Contains(string(ack.CanonicalBytes()), "100000") {
		t.Fatal("a receipt carries a position")
	}
}

// rejected proves a final refusal with its code that stores no tracking row.
func (x trackingState) rejected(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error, code string) {
	t.Helper()
	if err != nil || ack.Result() != protocol.Rejected || ack.Code() != code {
		t.Fatal("expected a final refusal", code, err, ack.Result(), ack.Code())
	}
	if c := x.count(t, op); c != (trackingCounts{claims: 1, receipts: 1, audits: 1}) {
		t.Fatalf("refused tracking operation stored a fact: %+v", c)
	}
}

func (x trackingState) pending(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error) {
	t.Helper()
	if err != nil || ack.Result() != protocol.Retry || ack.Code() != "DEPENDENCY_PENDING" {
		t.Fatal("expected RETRY/DEPENDENCY_PENDING", err, ack.Result(), ack.Code())
	}
	if c := x.count(t, op); c != (trackingCounts{}) {
		t.Fatalf("pending tracking operation left state: %+v", c)
	}
}

func (x trackingState) refused(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error, want orgapi.Code) {
	t.Helper()
	if code, _ := orgapi.Registered(err); !errors.Is(err, syncapp.ErrSourceDenied) || code != want || len(ack.CanonicalBytes()) != 0 {
		t.Fatalf("expected %s refusal, got %v", want, err)
	}
	if c := x.count(t, op); c != (trackingCounts{}) {
		t.Fatalf("refused tracking operation left state: %+v", c)
	}
}

func (x trackingState) replays(t *testing.T, d syncFixtureState, op protocol.Operation, ack protocol.Acknowledgement) {
	t.Helper()
	for range 2 {
		replay, err := x.process(t, d, op)
		if err != nil || !bytes.Equal(replay.CanonicalBytes(), ack.CanonicalBytes()) {
			t.Fatal("resend did not return the stored ACK", err)
		}
	}
}

type segmentRow struct {
	received, stored, duplicates, outside, mock int
	coarse                                      []bool
	instants                                    []time.Time
	lat, lon                                    []int32
	srid, points                                int
}

func (x trackingState) segment(t *testing.T, op protocol.Operation) segmentRow {
	t.Helper()
	var r segmentRow
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT received_count,stored_count,duplicate_count,outside_session_count,mock_count,coarse,collected_at,lat_e7,lon_e7,
 COALESCE(ST_SRID(points),0),COALESCE(ST_NumGeometries(points),0) FROM org_tracking_segments WHERE org_id=$1 AND operation_id=$2`, x.scope.OrgID().String(), op.ID()).
		Scan(&r.received, &r.stored, &r.duplicates, &r.outside, &r.mock, &r.coarse, &r.instants, &r.lat, &r.lon, &r.srid, &r.points); err != nil {
		t.Fatal("stored segment", err)
	}
	return r
}

func (x trackingState) startTracking(t *testing.T, d syncFixtureState, session string, seconds int, permission string, revision int64) protocol.Operation {
	t.Helper()
	op := x.driverOp(t, d, trackingapp.StartOperationType, session, x.at(seconds), permission, nil)
	ack, err := x.process(t, d, op)
	want := trackingCounts{transitions: 1}
	if revision == 2 {
		want = trackingCounts{sessions: 1, transitions: 2}
	}
	x.accepted(t, op, ack, err, revision, want)
	return op
}

func (x trackingState) inForce(t *testing.T) {
	t.Helper()
	notice := lifecycleDBNow(t, x.legalFixtureState).Add(-time.Hour)
	x.policy(t, "EXPECTED", &notice)
}

// BUC-EMP-217..219: the session lifecycle and its batches through the real
// signed intake; one effect per batch, exact replay, overlaps counted once,
// fixes outside the session or after the trip's finish discarded and counted.
func TestTrackingIntakeRecordsTheSessionAndStoresEachBatchOnce(t *testing.T) {
	x := trackingFixture(t)
	d := x.syncFixtureState
	x.inForce(t)
	session := onboardingID(t)
	start := x.startTracking(t, d, session, 1, "PRECISE", 2)
	startAck, _ := x.process(t, d, start)
	x.replays(t, d, start, startAck)
	var exact bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT s.trip_id=$3 AND s.owner_membership_id=$4 AND s.policy='EXPECTED' AND s.policy_revision=1 AND s.opened_at=$5
 AND(SELECT array_agg(state||'/'||COALESCE(cause,'')||'/'||actor||'/'||COALESCE(permission,'') ORDER BY seq) FROM org_tracking_transitions t WHERE t.org_id=s.org_id AND t.session_id=s.id)
  =ARRAY['NOT_STARTED/NOT_STARTED/DRIVER/','RECORDING//DRIVER/PRECISE']
 FROM org_tracking_sessions s WHERE s.org_id=$1 AND s.id=$2`, x.scope.OrgID().String(), session, x.trip, d.source.MembershipID, x.at(1)).Scan(&exact); err != nil || !exact {
		t.Fatal("stored session is not the exact opening and start", err)
	}
	first := x.batchOp(t, d, session, 1, []int{1, 11, 21}, nil)
	ack, err := x.process(t, d, first)
	x.accepted(t, first, ack, err, 1, trackingCounts{segments: 1})
	row := x.segment(t, first)
	if row.received != 3 || row.stored != 3 || row.duplicates != 0 || row.outside != 0 || row.srid != 4326 || row.points != 3 || row.coarse[0] || len(row.instants) != 3 ||
		!row.instants[2].Equal(x.at(21)) || row.lat[2] != -99999997 || row.lon[2] != -500000003 {
		t.Fatalf("stored segment: %+v", row)
	}
	x.replays(t, d, first, ack)
	reused := x.batchOp(t, d, session, 1, []int{1, 12, 21}, func(raw, _ map[string]any) { raw["operation_id"] = first.ID() })
	refusal, err := x.process(t, d, reused)
	if err != nil || refusal.Result() != protocol.Rejected || refusal.Code() != "OPERATION_ID_REUSED" {
		t.Fatal("mismatched digest under the same batch identifier was not refused", err, refusal.Code())
	}
	x.accepted(t, first, ack, nil, 1, trackingCounts{segments: 1})
	// An overlapping batch stores only its new instant.
	overlap := x.batchOp(t, d, session, 2, []int{11, 21, 31}, nil)
	ack, err = x.process(t, d, overlap)
	x.accepted(t, overlap, ack, err, 2, trackingCounts{segments: 1})
	if row := x.segment(t, overlap); row.stored != 1 || row.duplicates != 2 || !row.instants[0].Equal(x.at(31)) {
		t.Fatalf("overlapping batch: %+v", row)
	}
	// A second batch with a used sequence stores nothing.
	used := x.batchOp(t, d, session, 2, []int{41}, nil)
	ack, err = x.process(t, d, used)
	x.rejected(t, used, ack, err, "MALFORMED")
	// Interrupted at 60 s: the period ends there (half-open).
	stop := x.eventOp(t, d, trackingapp.InterruptOperationType, session, x.at(60), "PROCESS_STOPPED")
	ack, err = x.process(t, d, stop)
	x.accepted(t, stop, ack, err, 3, trackingCounts{transitions: 1})
	gap := x.batchOp(t, d, session, 3, []int{50, 60, 70}, nil)
	ack, err = x.process(t, d, gap)
	x.accepted(t, gap, ack, err, 3, trackingCounts{segments: 1})
	if row := x.segment(t, gap); row.stored != 1 || row.outside != 2 || !row.instants[0].Equal(x.at(50)) {
		t.Fatalf("fixes of an interrupted session: %+v", row)
	}
	// Resumed with approximate permission: stored coarse.
	resume := x.driverOp(t, d, trackingapp.ResumeOperationType, session, x.at(80), "APPROXIMATE", nil)
	ack, err = x.process(t, d, resume)
	x.accepted(t, resume, ack, err, 4, trackingCounts{transitions: 1})
	coarse := x.batchOp(t, d, session, 4, []int{90}, nil)
	ack, err = x.process(t, d, coarse)
	x.accepted(t, coarse, ack, err, 4, trackingCounts{segments: 1})
	if row := x.segment(t, coarse); row.stored != 1 || !row.coarse[0] {
		t.Fatalf("approximate fix: %+v", row)
	}
	// The trip finishes at 120 s; a fix at or after it is outside the session.
	end := x.finishOp(t, d, x.trip, x.at(120), nil)
	endAck, err := x.tripState.process(t, d, end)
	x.tripState.accepted(t, end, endAck, err, 2, 1)
	after := x.batchOp(t, d, session, 5, []int{110, 120, 130}, nil)
	ack, err = x.process(t, d, after)
	x.accepted(t, after, ack, err, 5, trackingCounts{segments: 1})
	if row := x.segment(t, after); row.stored != 1 || row.outside != 2 || !row.instants[0].Equal(x.at(110)) {
		t.Fatalf("fixes after the trip ended: %+v", row)
	}
	ended := x.eventOp(t, d, trackingapp.EndOperationType, session, x.at(120), "TRIP_COMPLETED")
	ack, err = x.process(t, d, ended)
	x.accepted(t, ended, ack, err, 5, trackingCounts{transitions: 1})
	// Buffered fixes collected before the end are still stored after it.
	buffered := x.batchOp(t, d, session, 6, []int{100}, nil)
	ack, err = x.process(t, d, buffered)
	x.accepted(t, buffered, ack, err, 6, trackingCounts{segments: 1})
	if row := x.segment(t, buffered); row.stored != 1 || !row.coarse[0] {
		t.Fatalf("a buffered fix after the end: %+v", row)
	}
	// Nothing is accepted after the end; every receipt still replays.
	late := x.driverOp(t, d, trackingapp.ResumeOperationType, session, x.at(121), "PRECISE", nil)
	ack, err = x.process(t, d, late)
	x.rejected(t, late, ack, err, "MALFORMED")
	for _, op := range []protocol.Operation{start, first, overlap, stop, gap, resume, coarse, after, ended, buffered} {
		stored, err := x.process(t, d, op)
		if err != nil || stored.Result() != protocol.Accepted {
			t.Fatal("an accepted receipt no longer replays", err)
		}
	}
	var total, sessions int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT sum(stored_count) FROM org_tracking_segments WHERE org_id=$1 AND session_id=$2),(SELECT count(*) FROM org_tracking_sessions WHERE org_id=$1 AND trip_id=$3)`,
		x.scope.OrgID().String(), session, x.trip).Scan(&total, &sessions); err != nil || total != 8 || sessions != 1 {
		t.Fatal("the session's stored fixes", total, sessions, err)
	}
}

// B-BR-EMP-428: a session opens only under a policy with its notice in
// force; the policy is read only at opening.
func TestTrackingIntakeOpensOnlyUnderAPolicyWithItsNoticeInForce(t *testing.T) {
	x := trackingFixture(t)
	d := x.syncFixtureState
	session := onboardingID(t)
	try := func(t *testing.T) (protocol.Operation, protocol.Acknowledgement, error) {
		op := x.driverOp(t, d, trackingapp.StartOperationType, session, x.at(1), "PRECISE", nil)
		ack, err := x.process(t, d, op)
		return op, ack, err
	}
	op, ack, err := try(t)
	x.rejected(t, op, ack, err, "FORBIDDEN")
	x.replays(t, d, op, ack)
	x.policy(t, "OFF", nil)
	op, ack, err = try(t)
	x.rejected(t, op, ack, err, "FORBIDDEN")
	future := lifecycleDBNow(t, x.legalFixtureState).Add(time.Hour)
	x.policy(t, "EXPECTED", &future)
	op, ack, err = try(t)
	x.rejected(t, op, ack, err, "FORBIDDEN")
	past := lifecycleDBNow(t, x.legalFixtureState).Add(-time.Minute)
	x.policy(t, "OPTIONAL", &past)
	x.startTracking(t, d, session, 1, "PRECISE", 2)
	var revision int64
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT policy_revision FROM org_tracking_sessions WHERE org_id=$1 AND id=$2 AND policy='OPTIONAL'`, x.scope.OrgID().String(), session).Scan(&revision); err != nil || revision != 3 {
		t.Fatal("the session does not keep the policy version it opened under", revision, err)
	}
	// Turning tracking off keeps the session working.
	x.policy(t, "OFF", nil)
	batch := x.batchOp(t, d, session, 1, []int{2, 3}, nil)
	ack, err = x.process(t, d, batch)
	x.accepted(t, batch, ack, err, 1, trackingCounts{segments: 1})
	suspend := x.eventOp(t, d, trackingapp.SuspendOperationType, session, x.at(4), "LONG_STATIONARY")
	ack, err = x.process(t, d, suspend)
	x.accepted(t, suspend, ack, err, 3, trackingCounts{transitions: 1})
}

// The named negatives: nothing stored for a foreign or missing trip or
// session, a personal or implicit start, another member, an oversized or
// malformed batch, a session of another trip, a start before the trip.
func TestTrackingIntakeStoresNothingItMustRefuse(t *testing.T) {
	x := trackingFixture(t)
	d := x.syncFixtureState
	x.inForce(t)
	session := onboardingID(t)
	x.startTracking(t, d, session, 1, "PRECISE", 2)
	t.Run("personal-origin-malformed", func(t *testing.T) {
		op := x.driverOp(t, d, trackingapp.StartOperationType, onboardingID(t), x.at(2), "PRECISE", func(_, p map[string]any) { p["origin"] = "PERSONAL" })
		ack, err := x.process(t, d, op)
		x.rejected(t, op, ack, err, "MALFORMED")
	})
	t.Run("implicit-boot-start-malformed", func(t *testing.T) {
		op := x.driverOp(t, d, trackingapp.StartOperationType, onboardingID(t), x.at(2), "PRECISE", func(_, p map[string]any) { p["origin"] = "BOOT" })
		ack, err := x.process(t, d, op)
		x.rejected(t, op, ack, err, "MALFORMED")
	})
	t.Run("second-session-malformed", func(t *testing.T) {
		op := x.driverOp(t, d, trackingapp.StartOperationType, onboardingID(t), x.at(2), "PRECISE", nil)
		ack, err := x.process(t, d, op)
		x.rejected(t, op, ack, err, "MALFORMED")
	})
	t.Run("start-before-the-trip-malformed", func(t *testing.T) {
		op := x.driverOp(t, d, trackingapp.StartOperationType, onboardingID(t), x.tripAt.Truncate(time.Microsecond).Add(-time.Second), "PRECISE", nil)
		ack, err := x.process(t, d, op)
		x.rejected(t, op, ack, err, "MALFORMED")
	})
	t.Run("missing-session-pending", func(t *testing.T) {
		op := x.batchOp(t, d, onboardingID(t), 1, []int{2}, nil)
		ack, err := x.process(t, d, op)
		x.pending(t, op, ack, err)
		op = x.eventOp(t, d, trackingapp.SuspendOperationType, onboardingID(t), x.at(2), "LONG_STATIONARY")
		ack, err = x.process(t, d, op)
		x.pending(t, op, ack, err)
	})
	t.Run("foreign-or-missing-trip-pending", func(t *testing.T) {
		op := x.batchOp(t, d, session, 1, []int{2}, func(_, p map[string]any) { p["trip_id"] = onboardingID(t) })
		ack, err := x.process(t, d, op)
		x.pending(t, op, ack, err)
		op = x.driverOp(t, d, trackingapp.StartOperationType, onboardingID(t), x.at(2), "PRECISE", func(_, p map[string]any) { p["trip_id"] = onboardingID(t) })
		ack, err = x.process(t, d, op)
		x.pending(t, op, ack, err)
	})
	t.Run("oversized-batch-malformed", func(t *testing.T) {
		seconds := make([]int, trackingapp.MaxFixes+1)
		for i := range seconds {
			seconds[i] = 2 + i
		}
		op := x.batchOp(t, d, session, 1, seconds, nil)
		ack, err := x.process(t, d, op)
		x.rejected(t, op, ack, err, "MALFORMED")
	})
	t.Run("malformed-batch", func(t *testing.T) {
		op := x.batchOp(t, d, session, 1, []int{2}, func(_, p map[string]any) { p["base"] = map[string]any{"lat_e7": 900000001, "lon_e7": 0} })
		ack, err := x.process(t, d, op)
		x.rejected(t, op, ack, err, "MALFORMED")
	})
	t.Run("another-member-forbidden", func(t *testing.T) {
		driver := x.driver(t)
		x.assign(t, driver.person.employee, x.vehicle.AssetID, fleet.AdditionalAssignment, nil, nil)
		op := x.batchOp(t, driver, session, 1, []int{2}, nil)
		ack, err := x.process(t, driver, op)
		x.refused(t, op, ack, err, orgapi.Forbidden)
		op = x.eventOp(t, driver, trackingapp.InterruptOperationType, session, x.at(2), "STOPPED_BY_DRIVER")
		ack, err = x.process(t, driver, op)
		x.refused(t, op, ack, err, orgapi.Forbidden)
	})
	t.Run("session-of-another-trip-malformed", func(t *testing.T) {
		// A second vehicle and trip of the same driver.
		other := x.otherTrip(t, d)
		op := x.batchOp(t, d, session, 1, []int{2}, func(_, p map[string]any) { p["trip_id"] = other })
		ack, err := x.process(t, d, op)
		x.rejected(t, op, ack, err, "MALFORMED")
	})
	var segments int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_tracking_segments WHERE org_id=$1`, x.scope.OrgID().String()).Scan(&segments); err != nil || segments != 0 {
		t.Fatal("a refused batch stored a segment", segments, err)
	}
}

// otherTrip starts a trip of member d on a second vehicle.
func (x trackingState) otherTrip(t *testing.T, d syncFixtureState) string {
	t.Helper()
	m := missionFixtureOn(t, x.tripState)
	spare := m.otherVehicle(t, d.person.employee)
	op := m.startOn(t, d, spare, "1000")
	ack, err := x.tripState.process(t, d, op)
	if err != nil || ack.Result() != protocol.Accepted {
		t.Fatal("second trip", err, ack.Result(), ack.Code())
	}
	return tripOf(op)
}

// barrierStore holds the first write that reaches it (under the trip row
// lock) until another intake transaction waits on a lock.
type barrierStore struct {
	trackingapp.Store
	inside, proceed chan struct{}
	once            *sync.Once
}

func (s barrierStore) hold() {
	s.once.Do(func() {
		close(s.inside)
		<-s.proceed
	})
}

func (s barrierStore) InsertSegmentTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, segment trackingapp.Segment) error {
	s.hold()
	return s.Store.InsertSegmentTx(ctx, db, scope, source, segment)
}

func (s barrierStore) RecordSessionTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, change trackingapp.SessionChange) error {
	s.hold()
	return s.Store.RecordSessionTx(ctx, db, scope, source, change)
}

// race runs two operations; the first holds its write (under the trip row
// lock) until the second waits on a lock it holds.
func (x trackingState) race(t *testing.T, d syncFixtureState, first, second protocol.Operation) (protocol.Acknowledgement, error, protocol.Acknowledgement, error) {
	t.Helper()
	barrier := barrierStore{Store: x.tracking, inside: make(chan struct{}), proceed: make(chan struct{}), once: &sync.Once{}}
	racing := x
	racing.install(t, barrier)
	config := trackingIntakeConfig(x.syncFixtureState)
	config.Pool = x.f.RuntimePool(t, 4)
	store := syncpg.NewStore(config)
	var acks [2]protocol.Acknowledgement
	var errs [2]error
	var done sync.WaitGroup
	run := func(i int, op protocol.Operation) {
		done.Go(func() {
			acks[i], errs[i] = store.Process(context.Background(), d.source, op, protocol.DefaultLimits(), racing.handlers[op.Snapshot().Type])
		})
	}
	run(0, first)
	select {
	case <-barrier.inside:
	case <-time.After(30 * time.Second):
		close(barrier.proceed)
		done.Wait()
		t.Fatal("the first write never reached the store", errs)
	}
	run(1, second)
	// Proof of contention: the second intake waits on a lock held by the
	// first one's open transaction (the organization's operational-write
	// lock or the trip row) before the first continues.
	waited := false
	for deadline := time.Now().Add(8 * time.Second); !waited && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if err := x.f.Admin.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND usename=$1 AND wait_event_type='Lock' AND pg_blocking_pids(pid)<>'{}')`, x.f.RuntimeRole).Scan(&waited); err != nil {
			t.Fatal(err)
		}
	}
	close(barrier.proceed)
	done.Wait()
	if !waited {
		t.Fatal("the second operation never waited on the first one's lock")
	}
	return acks[0], errs[0], acks[1], errs[1]
}

// B-BR-EMP-430/433: concurrent batches of one session store each instant
// once and concurrent starts keep one session, in both orders.
func TestTrackingIntakeConcurrentWritesConverge(t *testing.T) {
	for _, order := range []string{"a-first", "b-first"} {
		t.Run("batches-"+order, func(t *testing.T) {
			x := trackingFixture(t)
			d := x.syncFixtureState
			x.inForce(t)
			session := onboardingID(t)
			x.startTracking(t, d, session, 1, "PRECISE", 2)
			a := x.batchOp(t, d, session, 1, []int{2, 3, 4, 5}, nil)
			b := x.batchOp(t, d, session, 2, []int{4, 5, 6}, nil)
			first, second := a, b
			if order == "b-first" {
				first, second = b, a
			}
			seq := map[string]int64{a.ID(): 1, b.ID(): 2}
			size := map[string]int{a.ID(): 4, b.ID(): 3}
			ack1, err1, ack2, err2 := x.race(t, d, first, second)
			x.accepted(t, first, ack1, err1, seq[first.ID()], trackingCounts{segments: 1})
			x.accepted(t, second, ack2, err2, seq[second.ID()], trackingCounts{segments: 1})
			w, l := x.segment(t, first), x.segment(t, second)
			if w.stored != size[first.ID()] || w.duplicates != 0 || l.duplicates != 2 || w.stored+l.stored != 5 {
				t.Fatalf("winner %+v loser %+v", w, l)
			}
			x.replays(t, d, first, ack1)
			x.replays(t, d, second, ack2)
		})
		t.Run("starts-"+order, func(t *testing.T) {
			x := trackingFixture(t)
			d := x.syncFixtureState
			x.inForce(t)
			a := x.driverOp(t, d, trackingapp.StartOperationType, onboardingID(t), x.at(1), "PRECISE", nil)
			b := x.driverOp(t, d, trackingapp.StartOperationType, onboardingID(t), x.at(2), "PRECISE", nil)
			first, second := a, b
			if order == "b-first" {
				first, second = b, a
			}
			ack1, err1, ack2, err2 := x.race(t, d, first, second)
			x.accepted(t, first, ack1, err1, 2, trackingCounts{sessions: 1, transitions: 2})
			x.rejected(t, second, ack2, err2, "MALFORMED")
			var sessions int
			if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_tracking_sessions WHERE org_id=$1 AND trip_id=$2`, x.scope.OrgID().String(), x.trip).Scan(&sessions); err != nil || sessions != 1 {
				t.Fatal("one session per trip", sessions, err)
			}
			x.replays(t, d, second, ack2)
		})
	}
}

// The store rebuilds what it stored, inside the organization only.
func TestTrackingStoreReadsBackTheSessionInScopeOnly(t *testing.T) {
	x := trackingFixture(t)
	d := x.syncFixtureState
	x.inForce(t)
	session := onboardingID(t)
	x.startTracking(t, d, session, 1, "PRECISE", 2)
	var record trackingapp.SessionRecord
	if err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
		var err error
		record, err = x.tracking.SessionOfTripTx(t.Context(), syncapp.Borrow(tx), x.scope, x.trip)
		return err
	}); err != nil || record.ID != session || len(record.Transitions) != 2 || !record.OpenedAt.Equal(x.at(1)) {
		t.Fatal("stored session record", err)
	}
	if _, err := trackingapp.Rebuild(x.scope, record); err != nil {
		t.Fatal("stored session does not rebuild", err)
	}
	other, _, err := x.onboarding.Create(t.Context(), x.actor, onboardingCommand(t, onboardingID(t)))
	if err != nil {
		t.Fatal(err)
	}
	foreign := scopeFor(t, other.OrgID.String())
	if err := database.WithOrgTx(t.Context(), x.f.Runtime, foreign, func(tx pgx.Tx) error {
		_, err := x.tracking.SessionTx(t.Context(), syncapp.Borrow(tx), foreign, session)
		return err
	}); !errors.Is(err, trackingapp.ErrNotFound) {
		t.Fatal("another organization reads the session", err)
	}
}

// lifecycle sends member d's trip.pause, trip.resume or trip.cancel of the
// fixture trip at its expected revision and proves its acceptance.
func (x trackingState) lifecycle(t *testing.T, d syncFixtureState, kind string, expected int64, seconds int) {
	t.Helper()
	var reason any
	if kind == tripapp.CancelOperationType {
		reason = "Viagem cancelada pelo motorista"
	}
	op := x.operation(t, d, kind, x.trip, x.at(seconds), map[string]any{"reason": reason}, func(raw, _ map[string]any) {
		raw["entity"] = map[string]any{"type": tripapp.EntityType, "id": x.trip, "expected_revision": expected}
	})
	ack, err := x.process(t, d, op)
	if err != nil || ack.Result() != protocol.Accepted || ack.Snapshot().EntityRevision == nil || *ack.Snapshot().EntityRevision != expected+1 {
		t.Fatal("trip lifecycle command not accepted", kind, err, ack.Result(), ack.Code())
	}
}

// EMP-TRIP-011 with the GPS-001 FOLLOW rule: a pause suspends the session
// (recording returns only through the driver's resume), fixes while the trip
// is paused or after its cancellation are outside the session, and a
// cancellation ends it.
func TestTrackingIntakeFollowsTripPauseResumeAndCancellation(t *testing.T) {
	x := trackingFixture(t)
	d := x.syncFixtureState
	x.inForce(t)
	session := onboardingID(t)
	x.startTracking(t, d, session, 1, "PRECISE", 2)
	batch := func(seq int, seconds []int, stored, outside int) {
		t.Helper()
		op := x.batchOp(t, d, session, seq, seconds, nil)
		ack, err := x.process(t, d, op)
		x.accepted(t, op, ack, err, int64(seq), trackingCounts{segments: 1})
		if row := x.segment(t, op); row.stored != stored || row.outside != outside {
			t.Fatalf("batch %d: %+v", seq, row)
		}
	}
	command := func(op protocol.Operation, revision int64, rows int) {
		t.Helper()
		ack, err := x.process(t, d, op)
		x.accepted(t, op, ack, err, revision, trackingCounts{transitions: rows})
	}
	x.lifecycle(t, d, tripapp.PauseOperationType, 1, 30)
	batch(1, []int{10, 40}, 1, 1)
	command(x.eventOp(t, d, trackingapp.SuspendOperationType, session, x.at(30), "TRIP_PAUSED"), 3, 1)
	x.lifecycle(t, d, tripapp.ResumeOperationType, 2, 60)
	command(x.driverOp(t, d, trackingapp.ResumeOperationType, session, x.at(60), "PRECISE", nil), 4, 1)
	batch(2, []int{70, 80}, 2, 0)
	// The device does not report the second pause: the server follows it.
	x.lifecycle(t, d, tripapp.PauseOperationType, 3, 90)
	batch(3, []int{95, 100}, 0, 2)
	x.lifecycle(t, d, tripapp.ResumeOperationType, 4, 110)
	batch(4, []int{120}, 0, 1)
	stopped := x.eventOp(t, d, trackingapp.InterruptOperationType, session, x.at(130), "PROCESS_STOPPED")
	ack, err := x.process(t, d, stopped)
	x.rejected(t, stopped, ack, err, "MALFORMED")
	command(x.driverOp(t, d, trackingapp.ResumeOperationType, session, x.at(130), "PRECISE", nil), 6, 2)
	batch(5, []int{135}, 1, 0)
	x.lifecycle(t, d, tripapp.CancelOperationType, 5, 150)
	batch(6, []int{140, 150, 160}, 1, 2)
	command(x.eventOp(t, d, trackingapp.EndOperationType, session, x.at(150), "TRIP_CANCELLED"), 7, 1)
	var history []string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT array_agg(state||'/'||COALESCE(cause,'')||'/'||actor ORDER BY seq) FROM org_tracking_transitions WHERE org_id=$1 AND session_id=$2`,
		x.scope.OrgID().String(), session).Scan(&history); err != nil {
		t.Fatal(err)
	}
	want := []string{"NOT_STARTED/NOT_STARTED/DRIVER", "RECORDING//DRIVER", "SUSPENDED/TRIP_PAUSED/TRIP", "RECORDING//DRIVER", "SUSPENDED/TRIP_PAUSED/TRIP", "RECORDING//DRIVER", "ENDED/TRIP_CANCELLED/TRIP"}
	if strings.Join(history, ",") != strings.Join(want, ",") {
		t.Fatal("the session did not follow the trip", history)
	}
	var stored int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT sum(stored_count) FROM org_tracking_segments WHERE org_id=$1 AND session_id=$2`, x.scope.OrgID().String(), session).Scan(&stored); err != nil || stored != 5 {
		t.Fatal("stored fixes of a paused and cancelled trip", stored, err)
	}
}
