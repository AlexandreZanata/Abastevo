//go:build integration

package adapters_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	trackingpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/adapters"
	trackingapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

const processingMigration = "000246_organization_tracking_summaries.sql"

// processingGrants are the runtime grants of the processing job (test role
// only; production grants stay retained).
func processingGrants(t *testing.T, x trackingState) {
	t.Helper()
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_tracking_summaries TO "+role+";GRANT SELECT,INSERT,UPDATE ON job_queue TO "+role); err != nil {
		t.Fatal(err)
	}
}

func (x trackingState) processor() *trackingpg.ProcessingStore {
	return &trackingpg.ProcessingStore{Pool: x.f.Runtime, Trips: &trippg.Store{}, Session: &trackingpg.Store{}}
}

// dispatcher is the real queue dispatcher on its processing slot.
func (x trackingState) dispatcher() *jobs.Dispatcher {
	return &jobs.Dispatcher{Queue: jobs.NewQueue(x.f.Runtime), Handlers: map[string]jobs.Handler{trackingapp.ProcessKind: trackingpg.ProcessHandler{Processor: x.processor()}},
		WorkerID: "test-tracking-process", Kind: trackingapp.ProcessKind, LeaseTTL: time.Minute, RetryDelay: time.Second}
}

// fixSpec is one synthetic fix: seconds after the trip start and position.
type fixSpec struct {
	at       int
	lat, lon int32
	accuracy int
}

// westward returns fixes every step seconds from..to moving west by dlon
// 10⁻⁷ degrees per fix at 10° south, starting at lon.
func westward(from, to, step int, lon, dlon int32) []fixSpec {
	var out []fixSpec
	for s := from; s <= to; s += step {
		out = append(out, fixSpec{at: s, lat: -100000000, lon: lon, accuracy: 8})
		lon -= dlon
	}
	return out
}

// trackBatch sends the given fixes as one batch with a monotonic base that
// agrees with the wall base (one boot).
func (x trackingState) trackBatch(t *testing.T, d syncFixtureState, session string, seq int, specs []fixSpec) {
	t.Helper()
	seconds := make([]int, len(specs))
	for i, s := range specs {
		seconds[i] = s.at
	}
	op := x.batchOp(t, d, session, seq, seconds, func(_, p map[string]any) {
		list := make([]any, len(specs))
		for i, s := range specs {
			dlat, dlon := int32(0), int32(0)
			if i > 0 {
				dlat, dlon = s.lat-specs[i-1].lat, s.lon-specs[i-1].lon
			}
			list[i] = []any{(s.at - specs[0].at) * 1000, dlat, dlon, s.accuracy, 1389, 270, 0}
		}
		p["fixes"], p["base"] = list, map[string]any{"lat_e7": specs[0].lat, "lon_e7": specs[0].lon}
		p["base_elapsed_millis"] = 9063000 + specs[0].at*1000
	})
	ack, err := x.process(t, d, op)
	x.accepted(t, op, ack, err, int64(seq), trackingCounts{segments: 1})
}

type summaryRow struct {
	revision, segments, transitions                               int
	quality                                                       string
	coverage                                                      *int
	expected, gap                                                 int64
	accepted, coarseFixes, rejectedAccuracy, rejectedSpeed, outOf int
	received, duplicates, outside, mock, droppedAccuracy          int64
	clock                                                         int
	distance                                                      *int64
	method                                                        *string
	reasons, detectors                                            []string
	from, to                                                      []time.Time
	parts                                                         []int32
	tolerance                                                     *int
	lines, vertices                                               int
	lease                                                         int64
}

func (x trackingState) summaries(t *testing.T, session string) []summaryRow {
	t.Helper()
	rows, err := x.f.Admin.Query(t.Context(), `SELECT revision,input_segments,input_transitions,quality,coverage_permille,expected_seconds,gap_seconds,accepted_fixes,coarse_fixes,
 rejected_accuracy,rejected_speed,rejected_outside,received_fixes,duplicate_fixes,outside_session_fixes,mock_fixes,device_dropped_accuracy,clock_uncertain_segments,distance_m,
 distance_method,gap_reason,gap_detected_by,gap_from,gap_to,line_parts,tolerance_m,COALESCE(public.ST_NumGeometries(geometry),0),COALESCE(public.ST_NPoints(geometry),0),lease_token
 FROM org_tracking_summaries WHERE org_id=$1 AND session_id=$2 ORDER BY revision`, x.scope.OrgID().String(), session)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []summaryRow
	for rows.Next() {
		var r summaryRow
		if err := rows.Scan(&r.revision, &r.segments, &r.transitions, &r.quality, &r.coverage, &r.expected, &r.gap, &r.accepted, &r.coarseFixes, &r.rejectedAccuracy, &r.rejectedSpeed,
			&r.outOf, &r.received, &r.duplicates, &r.outside, &r.mock, &r.droppedAccuracy, &r.clock, &r.distance, &r.method, &r.reasons, &r.detectors, &r.from, &r.to, &r.parts,
			&r.tolerance, &r.lines, &r.vertices, &r.lease); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// processJobs lists the dedupe keys and payloads of the processing jobs.
func (x trackingState) processJobs(t *testing.T) ([]string, []string) {
	t.Helper()
	rows, err := x.f.Admin.Query(t.Context(), `SELECT dedupe_key,payload::text FROM job_queue WHERE kind=$1 AND dedupe_key IS NOT NULL ORDER BY created_at,id`, trackingapp.ProcessKind)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var keys, payloads []string
	for rows.Next() {
		var k, p string
		if err := rows.Scan(&k, &p); err != nil {
			t.Fatal(err)
		}
		keys, payloads = append(keys, k), append(payloads, p)
	}
	return keys, payloads
}

// leasedJob inserts one leased processing job row for a session (a second
// delivery of the same work), returning its lease.
func (x trackingState) leasedJob(t *testing.T, scope kernel.OrgScope, session string) trackingapp.Lease {
	t.Helper()
	id := onboardingID(t)
	payload, _ := json.Marshal(trackingpg.ProcessPayload{Version: 1, OrgID: scope.OrgID().String(), SessionID: session})
	if _, err := x.f.Admin.Exec(t.Context(), `INSERT INTO job_queue(id,kind,payload,status,attempts,max_attempts,lease_token,lease_expires_at,worker_id)
 VALUES($1,$2,$3::jsonb,'leased',1,5,1,now()+interval '5 minutes','test')`, id, trackingapp.ProcessKind, string(payload)); err != nil {
		t.Fatal(err)
	}
	return trackingapp.Lease{JobID: id, Token: 1}
}

func runDistance(runs ...[]fixSpec) int64 {
	rad := func(e7 int32) float64 { return float64(e7) / 1e7 * math.Pi / 180 }
	total := 0.0
	for _, run := range runs {
		for i := 1; i < len(run); i++ {
			a, b := run[i-1], run[i]
			dp, dl := rad(b.lat)-rad(a.lat), rad(b.lon)-rad(a.lon)
			h := math.Sin(dp/2)*math.Sin(dp/2) + math.Cos(rad(a.lat))*math.Cos(rad(b.lat))*math.Sin(dl/2)*math.Sin(dl/2)
			total += 2 * 6371008.8 * math.Asin(math.Sqrt(h))
		}
	}
	return int64(math.Floor(total))
}

// BUC-EMP-234/235/236, B-BR-EMP-461..466 on real PostGIS through the real
// signed intake and the real queue dispatcher: nothing is processed while
// the session records; its end queues one job; the summary filters the
// outlier and the inaccurate fix, records the gaps, measures only inside
// runs, simplifies the geometry and records quality and method; a late batch
// supersedes it with a new revision; a redelivered job and a stale lease
// change nothing.
func TestTrackProcessingDerivesTheSummaryOnceTheSessionEnds(t *testing.T) {
	x := trackingFixture(t)
	processingGrants(t, x)
	d := x.syncFixtureState
	x.inForce(t)
	session := onboardingID(t)
	x.startTracking(t, d, session, 1, "PRECISE", 2)
	first := westward(1, 2001, 20, -500000000, 25340)
	outlier := fixSpec{at: 1011, lat: -100000000 + 500000, lon: first[50].lon, accuracy: 5}
	inaccurate := fixSpec{at: 1511, lat: -100000000, lon: first[75].lon - 100, accuracy: 60}
	batch := append(append(append([]fixSpec{}, first[:51]...), outlier), first[51:76]...)
	batch = append(append(batch, inaccurate), first[76:]...)
	x.trackBatch(t, d, session, 1, batch)
	second := westward(2201, 4001, 20, first[len(first)-1].lon-10*25340, 25340)
	x.trackBatch(t, d, session, 2, second)
	if keys, _ := x.processJobs(t); len(keys) != 0 || len(x.summaries(t, session)) != 0 {
		t.Fatal("a recording session was processed", keys)
	}
	// The trip finishes and the session ends: one job for that input.
	end := x.finishOp(t, d, x.trip, x.at(4010), nil)
	endAck, err := x.tripState.process(t, d, end)
	x.tripState.accepted(t, end, endAck, err, 2, 1)
	ended := x.eventOp(t, d, trackingapp.EndOperationType, session, x.at(4010), "TRIP_COMPLETED")
	ack, err := x.process(t, d, ended)
	x.accepted(t, ended, ack, err, 3, trackingCounts{transitions: 1})
	keys, payloads := x.processJobs(t)
	if len(keys) != 1 || keys[0] != trackingpg.ProcessDedupe(x.scope, session, trackingapp.Watermark{Segments: 2, Transitions: 3}) {
		t.Fatal("processing job for the ended session", keys)
	}
	var payload map[string]any
	if json.Unmarshal([]byte(payloads[0]), &payload) != nil || len(payload) != 3 || payload["org_id"] != x.scope.OrgID().String() || payload["session_id"] != session || payload["version"] != float64(1) {
		t.Fatal("the job payload carries more than references", payloads[0])
	}
	// Replaying the end operation queues nothing new.
	if replay, err := x.process(t, d, ended); err != nil || string(replay.CanonicalBytes()) != string(ack.CanonicalBytes()) {
		t.Fatal("end replay", err)
	}
	if keys, _ := x.processJobs(t); len(keys) != 1 {
		t.Fatal("a replay queued another job", keys)
	}
	if found, err := x.dispatcher().RunOnce(t.Context()); !found || err != nil {
		t.Fatal("processing job not run", found, err)
	}
	rows := x.summaries(t, session)
	if len(rows) != 1 {
		t.Fatal("summary revisions", len(rows))
	}
	r := rows[0]
	if r.revision != 1 || r.segments != 2 || r.transitions != 3 || r.quality != "PRECISE" || r.coverage == nil || *r.coverage < 900 || r.expected != 4010 {
		t.Fatalf("summary basis %+v", r)
	}
	if r.accepted != len(first)+len(second) || r.rejectedSpeed != 1 || r.rejectedAccuracy != 1 || r.outOf != 0 || r.received != int64(len(batch)+len(second)) ||
		r.droppedAccuracy != 2 || r.clock != 0 || r.coarseFixes != 0 {
		t.Fatalf("summary counters %+v", r)
	}
	if !slices.Equal(r.reasons, []string{"NOT_STARTED", "SIGNAL_LOST"}) || !slices.Equal(r.detectors, []string{"SERVER", "SERVER"}) ||
		!r.from[1].Equal(x.at(2001)) || !r.to[1].Equal(x.at(2201)) || !r.to[0].Equal(x.at(1)) {
		t.Fatalf("summary gaps %v %v %v %v", r.reasons, r.detectors, r.from, r.to)
	}
	if r.distance == nil || *r.distance != runDistance(first, second) || r.method == nil || *r.method != domain.DistanceMethod {
		t.Fatal("GPS distance", r.distance, runDistance(first, second))
	}
	if bridged := runDistance(append(append([]fixSpec{}, first...), second...)); *r.distance >= bridged {
		t.Fatal("the gap was bridged", *r.distance, bridged)
	}
	if !slices.Equal(r.parts, []int32{2, 2}) || r.tolerance == nil || *r.tolerance != 20 || r.lines != 2 || r.vertices != 4 {
		t.Fatalf("simplified geometry %+v", r)
	}

	// A late batch collected before the end supersedes the summary; a
	// redelivery after a lost acknowledgement and the stale lease change
	// nothing.
	late := westward(4003, 4007, 2, second[len(second)-1].lon-25340/10, 2534)
	x.trackBatch(t, d, session, 3, late)
	keys, _ = x.processJobs(t)
	if len(keys) != 2 || keys[1] != trackingpg.ProcessDedupe(x.scope, session, trackingapp.Watermark{Segments: 3, Transitions: 3}) {
		t.Fatal("late batch job", keys)
	}
	queue := jobs.NewQueue(x.f.Runtime)
	handler := trackingpg.ProcessHandler{Processor: x.processor()}
	held, err := queue.Claim(t.Context(), trackingapp.ProcessKind, "worker-a", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.Handle(t.Context(), held); err != nil {
		t.Fatal("late processing", err)
	}
	if _, err := x.f.Admin.Exec(t.Context(), `UPDATE job_queue SET lease_expires_at=now()-interval '1 second' WHERE id=$1`, held.ID); err != nil {
		t.Fatal(err)
	}
	again, err := queue.Claim(t.Context(), trackingapp.ProcessKind, "worker-b", time.Minute)
	if err != nil || again.ID != held.ID || again.LeaseToken != held.LeaseToken+1 {
		t.Fatal("redelivery", err)
	}
	if err := handler.Handle(t.Context(), held); !errors.Is(err, jobs.ErrStaleLease) {
		t.Fatal("a superseded lease processed", err)
	}
	if err := handler.Handle(t.Context(), again); err != nil {
		t.Fatal("redelivered processing", err)
	}
	if err := queue.Complete(t.Context(), again.ID, again.LeaseToken); err != nil {
		t.Fatal(err)
	}
	if err := queue.Complete(t.Context(), held.ID, held.LeaseToken); !errors.Is(err, jobs.ErrStaleLease) {
		t.Fatal("stale acknowledgement", err)
	}
	rows = x.summaries(t, session)
	if len(rows) != 2 || rows[1].revision != 2 || rows[1].segments != 3 || rows[1].lease != held.LeaseToken || rows[1].accepted != r.accepted+len(late) {
		t.Fatal("superseding revision", len(rows))
	}
	if *rows[1].distance <= *r.distance || rows[0].revision != 1 || *rows[0].distance != *r.distance {
		t.Fatal("the first revision changed or the late fixes were not measured")
	}
	// A second delivery of the same input converges.
	if outcome, revision, err := x.processor().Process(t.Context(), x.scope, session, x.leasedJob(t, x.scope, session)); err != nil || outcome != trackingapp.ProcessCurrent || revision != 2 {
		t.Fatal("job run twice", outcome, revision, err)
	}
	if len(x.summaries(t, session)) != 2 {
		t.Fatal("a repeated run wrote a revision")
	}
}

// B-BR-EMP-466 negatives on real PostGIS: an open session, a foreign
// organization's job, a stale lease and every summary the database must
// refuse (an older input, a skipped revision, counts that are not the
// session's own, gaps and lines that break the rules, an overclaimed
// quality, another scope) store nothing; the summaries are insert-only and
// scoped. A structurally valid candidate is the positive control.
func TestTrackProcessingRefusesStaleForeignAndOpenInputs(t *testing.T) {
	x := trackingFixture(t)
	processingGrants(t, x)
	d := x.syncFixtureState
	org := x.scope.OrgID().String()
	x.inForce(t)
	session := onboardingID(t)
	x.startTracking(t, d, session, 1, "PRECISE", 2)
	first := westward(1, 201, 20, -500000000, 25340)
	x.trackBatch(t, d, session, 1, append(append([]fixSpec{}, first...), westward(401, 601, 20, first[len(first)-1].lon-25340, 25340)...))
	runtimeExec := func(scope, sql string, args ...any) error {
		tx, err := x.f.Runtime.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, scope); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(t.Context(), sql, args...); err != nil {
			return err
		}
		return tx.Commit(t.Context())
	}
	columns := []string{"org_id", "session_id", "revision", "trip_id", "method", "input_segments", "input_transitions", "job_id", "lease_token", "expected_seconds", "gap_seconds",
		"coverage_permille", "coarse", "quality", "accepted_fixes", "coarse_fixes", "rejected_accuracy", "rejected_speed", "rejected_outside", "received_fixes", "duplicate_fixes",
		"outside_session_fixes", "mock_fixes", "device_dropped_accuracy", "device_dropped_stale", "device_dropped_jump", "device_dropped_mock", "clock_uncertain_segments", "distance_m",
		"distance_method", "gap_from", "gap_to", "gap_reason", "gap_detected_by", "line_parts", "line_lat_e7", "line_lon_e7", "tolerance_m"}
	// Open session: the job completes without effect and the database
	// refuses a summary of a session that has not ended.
	if outcome, _, err := x.processor().Process(t.Context(), x.scope, session, x.leasedJob(t, x.scope, session)); err != nil || outcome != trackingapp.ProcessNotEnded {
		t.Fatal("an open session was processed", outcome, err)
	}
	literal := `INSERT INTO org_tracking_summaries(` + strings.Join(columns, ",") + `)VALUES($1,$2,1,$3,'gps-track-v1',1,2,$4,1,0,0,NULL,false,'INSUFFICIENT',
 22,0,0,0,0,22,0,0,0,1,0,0,0,0,NULL,NULL,'{}','{}','{}','{}','{}','{}','{}',NULL)`
	if code := policyGuardCode(runtimeExec(org, literal, org, session, x.trip, onboardingID(t))); code != "23514" {
		t.Fatal("a summary of an open session was admitted", code)
	}
	end := x.finishOp(t, d, x.trip, x.at(610), nil)
	endAck, err := x.tripState.process(t, d, end)
	x.tripState.accepted(t, end, endAck, err, 2, 1)
	finished := x.eventOp(t, d, trackingapp.EndOperationType, session, x.at(610), "TRIP_COMPLETED")
	ack, err := x.process(t, d, finished)
	x.accepted(t, finished, ack, err, 3, trackingCounts{transitions: 1})
	// A foreign organization's job and a stale lease store nothing.
	foreignOrg, _ := kernel.ParseOrgID(onboardingID(t))
	foreign, _ := kernel.NewOrgScope(foreignOrg)
	if outcome, _, err := x.processor().Process(t.Context(), foreign, session, x.leasedJob(t, foreign, session)); err != nil || outcome != trackingapp.ProcessMissing {
		t.Fatal("a foreign organization's job processed the session", outcome, err)
	}
	if _, _, err := x.processor().Process(t.Context(), x.scope, session, trackingapp.Lease{JobID: x.leasedJob(t, x.scope, session).JobID, Token: 2}); !errors.Is(err, trackingapp.ErrStaleLease) {
		t.Fatal("a stale lease processed", err)
	}
	if len(x.summaries(t, session)) != 0 {
		t.Fatal("a refused run wrote a summary")
	}
	if found, err := x.dispatcher().RunOnce(t.Context()); !found || err != nil {
		t.Fatal("processing", found, err)
	}
	rows := x.summaries(t, session)
	if len(rows) != 1 || !slices.Equal(rows[0].reasons, []string{"NOT_STARTED", "SIGNAL_LOST"}) || len(rows[0].parts) != 2 {
		t.Fatalf("first summary %+v", rows)
	}
	// A late batch makes a newer input that nobody processed yet; the base
	// candidate names it exactly, so each refusal below breaks one rule.
	late := westward(603, 607, 2, -500000000-21*25340-1000, 100)
	x.trackBatch(t, d, session, 2, late)
	k := strconv.Itoa(len(late))
	base := map[string]string{"revision": "2", "input_segments": "2", "accepted_fixes": "accepted_fixes+" + k, "received_fixes": "received_fixes+" + k,
		"device_dropped_accuracy": "device_dropped_accuracy+1", "job_id": "'" + onboardingID(t) + "'"}
	candidate := func(overrides map[string]string) error {
		values := make([]string, len(columns))
		for i, c := range columns {
			values[i] = c
			if v, ok := base[c]; ok {
				values[i] = v
			}
			if v, ok := overrides[c]; ok {
				values[i] = v
			}
		}
		return runtimeExec(org, `INSERT INTO org_tracking_summaries(`+strings.Join(columns, ",")+`) SELECT `+strings.Join(values, ",")+
			` FROM org_tracking_summaries WHERE org_id=$1 AND session_id=$2 AND revision=1`, org, session)
	}
	refused := map[string]map[string]string{
		"older input (stale job)": {"input_segments": "1", "accepted_fixes": "accepted_fixes", "received_fixes": "received_fixes", "device_dropped_accuracy": "device_dropped_accuracy"},
		"skipped revision":        {"revision": "3"},
		"repeated revision":       {"revision": "1"},
		"more transitions":        {"input_transitions": "input_transitions+1"},
		"foreign fix total":       {"accepted_fixes": "accepted_fixes+" + k + "+1"},
		"foreign counters":        {"received_fixes": "received_fixes+" + k + "+5"},
		"overlapping gaps":        {"gap_from": "ARRAY[gap_from[1],gap_from[1]]"},
		"device signal lost":      {"gap_detected_by": "ARRAY['SERVER','DEVICE']::text[]"},
		"unknown gap reason":      {"gap_reason": "ARRAY['NOT_STARTED','PROVIDER_DISABLED']::text[]"},
		"gap seconds":             {"gap_seconds": "gap_seconds-1", "coverage_permille": "((expected_seconds-gap_seconds+1)*1000)/expected_seconds"},
		"single vertex line":      {"line_parts": "ARRAY[1,line_parts[1]+line_parts[2]-1]::integer[]"},
		"overclaimed quality":     {"quality": "'PRECISE'"},
		"distance without method": {"distance_method": "NULL"},
	}
	for name, overrides := range refused {
		if code := policyGuardCode(candidate(overrides)); code != "23514" && !(name == "repeated revision" && code == "23505") {
			t.Fatal("the database admitted a summary it must refuse", name, code)
		}
	}
	if code := policyGuardCode(runtimeExec(onboardingID(t), literal, org, session, x.trip, onboardingID(t))); code != "42501" {
		t.Fatal("another scope wrote a summary", code)
	}
	if rows := x.summaries(t, session); len(rows) != 1 {
		t.Fatal("a refused summary was stored")
	}
	if err := candidate(nil); err != nil {
		t.Fatal("the structurally valid candidate is refused", err)
	}
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE ON org_tracking_summaries TO "+role); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{`UPDATE org_tracking_summaries SET distance_m=0 WHERE org_id=$1 AND session_id=$2`, `DELETE FROM org_tracking_summaries WHERE org_id=$1 AND session_id=$2`} {
		if code := policyGuardCode(runtimeExec(org, sql, org, session)); code != "55000" {
			t.Fatal("summaries are not insert-only", sql, code)
		}
	}
	if _, err := x.f.Admin.Exec(t.Context(), `TRUNCATE org_tracking_summaries`); policyGuardCode(err) != "55000" {
		t.Fatal("summaries can be truncated", err)
	}
	var forced bool
	var visible int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT relrowsecurity AND relforcerowsecurity FROM pg_class WHERE oid='org_tracking_summaries'::regclass`).Scan(&forced); err != nil || !forced {
		t.Fatal("summaries are not forced-RLS", err)
	}
	tx, err := x.f.Runtime.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, onboardingID(t)); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(t.Context(), `SELECT count(*) FROM org_tracking_summaries`).Scan(&visible); err != nil || visible != 0 {
		t.Fatal("another scope sees summaries", visible, err)
	}
	// The real job now finds its input already summarized and converges.
	if found, err := x.dispatcher().RunOnce(t.Context()); !found || err != nil || len(x.summaries(t, session)) != 2 {
		t.Fatal("the job after the candidate", found, err)
	}
}

// B-BR-EMP-466: two processing runs of one session queued behind a held
// trip row lock converge on one revision, in both orders.
func TestTrackProcessingConcurrentRunsConvergeInBothOrders(t *testing.T) {
	x := trackingFixture(t)
	processingGrants(t, x)
	d := x.syncFixtureState
	org := x.scope.OrgID().String()
	x.inForce(t)
	session := onboardingID(t)
	x.startTracking(t, d, session, 1, "PRECISE", 2)
	x.trackBatch(t, d, session, 1, westward(1, 401, 20, -500000000, 25340))
	end := x.finishOp(t, d, x.trip, x.at(1000), nil)
	endAck, err := x.tripState.process(t, d, end)
	x.tripState.accepted(t, end, endAck, err, 2, 1)
	finished := x.eventOp(t, d, trackingapp.EndOperationType, session, x.at(1000), "TRIP_COMPLETED")
	ack, err := x.process(t, d, finished)
	x.accepted(t, finished, ack, err, 3, trackingCounts{transitions: 1})
	race := func(order [2]int) [2]trackingapp.ProcessOutcome {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		leases := [2]trackingapp.Lease{x.leasedJob(t, x.scope, session), x.leasedJob(t, x.scope, session)}
		holder, err := x.f.Runtime.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer holder.Rollback(ctx)
		if _, err := holder.Exec(ctx, `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
			t.Fatal(err)
		}
		if _, err := holder.Exec(ctx, `SELECT 1 FROM org_trips WHERE org_id=$1 AND id=$2 FOR UPDATE`, org, x.trip); err != nil {
			t.Fatal(err)
		}
		waitBlocked := func(n int) {
			ticker := time.NewTicker(5 * time.Millisecond)
			defer ticker.Stop()
			for {
				var blocked int
				if err := x.f.Admin.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND usename=$1 AND wait_event_type='Lock' AND pg_blocking_pids(pid)<>'{}'`, x.f.RuntimeRole).Scan(&blocked); err != nil {
					t.Fatal(err)
				}
				if blocked >= n {
					return
				}
				select {
				case <-ticker.C:
				case <-ctx.Done():
					t.Fatal("no lock wait observed")
				}
			}
		}
		type result struct {
			outcome trackingapp.ProcessOutcome
			err     error
		}
		var results [2]chan result
		for i := range results {
			results[i] = make(chan result, 1)
		}
		run := func(i int) {
			outcome, _, err := x.processor().Process(ctx, x.scope, session, leases[i])
			results[i] <- result{outcome, err}
		}
		go run(order[0])
		waitBlocked(1)
		go run(order[1])
		waitBlocked(2)
		if err := holder.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		a, b := <-results[order[0]], <-results[order[1]]
		if a.err != nil || b.err != nil {
			t.Fatal("concurrent processing", a.err, b.err)
		}
		return [2]trackingapp.ProcessOutcome{a.outcome, b.outcome}
	}
	if got := race([2]int{0, 1}); got != [2]trackingapp.ProcessOutcome{trackingapp.ProcessWritten, trackingapp.ProcessCurrent} {
		t.Fatal("first order", got)
	}
	x.trackBatch(t, d, session, 2, westward(405, 409, 2, -500000000-20*25340-1000, 100))
	if got := race([2]int{1, 0}); got != [2]trackingapp.ProcessOutcome{trackingapp.ProcessWritten, trackingapp.ProcessCurrent} {
		t.Fatal("second order", got)
	}
	if rows := x.summaries(t, session); len(rows) != 2 || rows[0].segments != 1 || rows[1].segments != 2 {
		t.Fatal("concurrent runs did not converge on one revision per input", len(rows))
	}
}

// A populated head before 000246 (an ended session with its segments whose
// processing job was queued before the upgrade) upgrades by an ordinary
// non-superuser migrator: an injected failure leaves schema and data
// untouched, recovery applies once, replay is a no-op, the previous image
// stays compatible, and the queued job then writes the first summary.
func TestTrackProcessingPopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= processingMigration || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	if _, ok := previous[trackingMigration]; !ok {
		t.Fatal("previous image does not contain", trackingMigration)
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	legal := legalFixtureFor(t, f, onboarding, actor, flags)
	x := trackingFixtureOn(t, tripFixtureOn(t, selfIntakeFixtureOn(t, syncFixtureOn(t, devicesFixtureOn(t, legal)))))
	d := x.syncFixtureState
	x.inForce(t)
	session := onboardingID(t)
	x.startTracking(t, d, session, 1, "PRECISE", 2)
	fixes := westward(1, 401, 20, -500000000, 25340)
	x.trackBatch(t, d, session, 1, fixes)
	end := x.finishOp(t, d, x.trip, x.at(410), nil)
	endAck, err := x.tripState.process(t, d, end)
	x.tripState.accepted(t, end, endAck, err, 2, 1)
	finished := x.eventOp(t, d, trackingapp.EndOperationType, session, x.at(410), "TRIP_COMPLETED")
	ack, err := x.process(t, d, finished)
	x.accepted(t, finished, ack, err, 3, trackingCounts{transitions: 1})
	if keys, _ := x.processJobs(t); len(keys) != 1 {
		t.Fatal("job queued before the upgrade", keys)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_tracking_sessions x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,session_id,seq)FROM org_tracking_transitions x),(SELECT jsonb_agg(to_jsonb(x)-'points' ORDER BY org_id,operation_id)FROM org_tracking_segments x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY id)FROM job_queue x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_summary_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+
		";GRANT REFERENCES ON organizations,org_tracking_sessions,org_trips TO "+role); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := f.Admin.Exec(ctx, "REASSIGN OWNED BY "+role+" TO CURRENT_USER;DROP OWNED BY "+role+";DROP ROLE "+role); err != nil {
			t.Error(err)
		}
	})
	location, err := url.Parse(f.AdminDSN)
	if err != nil {
		t.Fatal(err)
	}
	query := location.Query()
	query.Set("options", "-c role="+migrator)
	location.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	raw, err := fs.ReadFile(migrations.Files, processingMigration)
	if err != nil {
		t.Fatal(err)
	}
	version := strings.SplitN(processingMigration, "_", 2)[0]
	broken := fstest.MapFS{processingMigration: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected processing migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_tracking_summaries') IS NULL AND to_regprocedure('org_tracking_summary_guard()') IS NULL
 AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&clean); err != nil || !clean || before != snapshot() {
		t.Fatal("failed processing migration changed the original schema or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[processingMigration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != version {
		t.Fatal("ordinary processing migration recovery", applied, err)
	}
	if before != snapshot() {
		t.Fatal("processing upgrade rewrote existing state")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("processing migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin AND(SELECT relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1)
 FROM pg_class WHERE oid='org_tracking_summaries'::regclass)FROM pg_roles WHERE rolname=$1`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("the summary table is not owned by the ordinary forced-RLS migrator", err)
	}
	processingGrants(t, x)
	if found, err := x.dispatcher().RunOnce(t.Context()); !found || err != nil {
		t.Fatal("the pre-upgrade job did not run", found, err)
	}
	if rows := x.summaries(t, session); len(rows) != 1 || rows[0].segments != 1 || rows[0].accepted != len(fixes) || rows[0].distance == nil || *rows[0].distance != runDistance(fixes) {
		t.Fatal("summary after the upgrade", len(rows))
	}
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, migrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 246 rehearsal", err)
	}
}
