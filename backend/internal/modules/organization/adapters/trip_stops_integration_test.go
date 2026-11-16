//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	tripdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// EMP-TRIP-010 (B-BR-EMP-475..480, EDR-307..310): planned stops on PLAN,
// observed stops and legs derived from the logbook and the member's own read,
// on real PostGIS with the ordinary runtime role.

// The one file-name constant of EMP-TRIP-010; the coordinator may renumber it.
const tripStopsMigration = "000256_organization_trip_mission_plan_stops.sql"

// tripStopsState is the handover fixture (signed start, finish, logbook and
// lifecycle intake; the own-trips authority) plus the real mission store and
// the stops store.
type tripStopsState struct {
	tripHandoverState
	m     missionState
	stops *trippg.StopsStore
}

func tripStopsFixture(t *testing.T) tripStopsState {
	t.Helper()
	x := tripHandoverFixture(t)
	return tripStopsState{tripHandoverState: x, m: missionFixtureOn(t, x.eventState.tripState), stops: &trippg.StopsStore{Pool: x.f.Runtime, Authority: x.handovers.Authority}}
}

// stopsPlan is the BUC-EMP-242 plan: a text destination, a reason, no load
// and the given planned stops (nil for none).
func stopsPlan(stops []string) *tripapp.MissionPlanFields {
	return &tripapp.MissionPlanFields{DestinationText: planText("Centro de distribuição Norte"), Reason: "Entrega de peças", Load: "NONE", Stops: stops}
}

var bucStops = []string{"Cliente Jardim Primavera", "Posto de apoio BR-163", "Cliente Vila Rica"}

// planStopRows is every stored planned stop of the mission by revision.
func (x tripStopsState) planStopRows(t *testing.T, mission string) map[int64][]string {
	t.Helper()
	rows, err := x.f.Admin.Query(t.Context(), `SELECT revision,sequence,place_text FROM org_trip_mission_plan_stops WHERE org_id=$1 AND mission_id=$2 ORDER BY revision,sequence`,
		x.scope.OrgID().String(), mission)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int64][]string{}
	for rows.Next() {
		var revision int64
		var sequence int16
		var place string
		if err := rows.Scan(&revision, &sequence, &place); err != nil {
			t.Fatal(err)
		}
		if int(sequence) != len(out[revision])+1 {
			t.Fatal("stored stops out of sequence", revision, sequence)
		}
		out[revision] = append(out[revision], place)
	}
	return out
}

// plannedMission creates a mission on the vehicle, plans it with the stops
// (revision 2), assigns member d (3) and records d's acceptance (4).
func (x tripStopsState) plannedMission(t *testing.T, d syncFixtureState, stops []string) string {
	t.Helper()
	m := x.m
	mission := m.create(t, m.unit, m.vehicle.AssetID).MissionID
	if ack, err := m.plan(t, mission, 1, stopsPlan(stops)); err != nil || ack.Revision != 2 {
		t.Fatal("planned stops", err)
	}
	if _, err := m.assignTo(t, mission, 2, d.person.employee); err != nil {
		t.Fatal("mission assignment", err)
	}
	ack, err := m.deliver(t, d, m.respondOp(t, d, mission, 3, "ACCEPT", nil, nil))
	if err != nil || ack.Result() != protocol.Accepted {
		t.Fatal("mission acceptance", err, ack.Result(), ack.Code())
	}
	return mission
}

// read is member d's own-trip read through the store and the shared build.
func (x tripStopsState) read(t *testing.T, d syncFixtureState, trip string) (tripapp.TripStopsView, error) {
	t.Helper()
	r := deviceReviewer(d)
	record, err := x.stops.ReadOwnTripStops(t.Context(), x.scope, r.actor, r.member, trip)
	if err != nil {
		return tripapp.TripStopsView{}, err
	}
	return tripapp.BuildTripStops(record)
}

func (x tripStopsState) mustRead(t *testing.T, d syncFixtureState, trip string) tripapp.TripStopsView {
	t.Helper()
	v, err := x.read(t, d, trip)
	if err != nil {
		t.Fatal("own-trip stops read", err)
	}
	return v
}

// observedFacts is every stored observation and trip fact a plan must never
// touch.
func (x tripStopsState) observedFacts(t *testing.T) string {
	t.Helper()
	var v string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(e) ORDER BY id) FROM org_trip_events e WHERE org_id=$1),
(SELECT jsonb_agg(to_jsonb(h) ORDER BY id) FROM org_trip_held_events h WHERE org_id=$1),(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM org_trips r WHERE org_id=$1),
(SELECT jsonb_agg(to_jsonb(f) ORDER BY trip_id) FROM org_trip_finishes f WHERE org_id=$1),(SELECT jsonb_agg(to_jsonb(l) ORDER BY trip_id) FROM org_trip_mission_links l WHERE org_id=$1))::text`,
		x.scope.OrgID().String()).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func boundaryID(f *tripapp.ObservedFact) string {
	if f == nil {
		return "-"
	}
	return string(f.Mark) + "/" + f.FactID
}

// B-BR-EMP-475/479/480, BUC-EMP-242: each planning revision keeps its own
// stops; replay stores nothing new, another digest is OPERATION_ID_REUSED, a
// plan without stops keeps no stop and an accepted mission cannot be
// re-planned.
func TestTripStopsPlanRevisionsKeepTheirStopsAndReplay(t *testing.T) {
	x := tripStopsFixture(t)
	m := x.m
	d := x.assignedDriver(t)
	mission := m.create(t, m.unit, m.vehicle.AssetID).MissionID
	operation := onboardingID(t)
	command := func(op string, revision int64, stops []string) tripapp.MissionCommand {
		return missionCommand(t, tripapp.MissionCommandInput{Kind: tripapp.MissionPlanCommand, OperationID: op, MissionID: mission, ExpectedRevision: revision, Plan: stopsPlan(stops)})
	}
	first := []string{"Cliente Jardim Primavera", "Posto de apoio BR-163"}
	ack, replayed, err := m.run(t, ownerMissionActor(m.legalFixtureState), m.member, command(operation, 1, first))
	if err != nil || replayed || ack.Revision != 2 || ack.State != tripdomain.MissionPlanned {
		t.Fatal("plan with stops", ack, replayed, err)
	}
	if rows := x.planStopRows(t, mission); !reflect.DeepEqual(rows, map[int64][]string{2: first}) {
		t.Fatal("stored planned stops", rows)
	}
	if c := m.planCounts(t, operation); c != (planCounts{plans: 1, transitions: 1, receipts: 1, audits: 1}) {
		t.Fatalf("one plan, transition, receipt and audit event: %+v", c)
	}
	again, replayed, err := m.run(t, ownerMissionActor(m.legalFixtureState), m.member, command(operation, 1, first))
	if err != nil || !replayed || again != ack {
		t.Fatal("plan replay", again, replayed, err)
	}
	_, _, err = m.run(t, ownerMissionActor(m.legalFixtureState), m.member, command(operation, 1, []string{"Outro lugar"}))
	wantMissionError(t, err, tripapp.ErrOperationReused)
	_, _, err = m.run(t, ownerMissionActor(m.legalFixtureState), m.member, command(operation, 1, nil))
	wantMissionError(t, err, tripapp.ErrOperationReused)
	if rows := x.planStopRows(t, mission); !reflect.DeepEqual(rows, map[int64][]string{2: first}) {
		t.Fatal("a replay or reuse stored stops", rows)
	}
	read := func() *tripapp.MissionPlanView {
		t.Helper()
		v, err := m.missions.ReadMission(t.Context(), m.scope, ownerMissionActor(m.legalFixtureState), m.member, mission)
		if err != nil {
			t.Fatal("mission read", err)
		}
		return v.Plan
	}
	if p := read(); p == nil || p.Revision != 2 || !slices.Equal(p.Fields.Stops, first) {
		t.Fatal("the mission read lost its planned stops", p)
	}
	// A re-plan appends a revision with its own stops; the first ones stay.
	if ack, err := m.plan(t, mission, 2, stopsPlan(bucStops)); err != nil || ack.Revision != 3 {
		t.Fatal("re-plan with three stops", err)
	}
	if rows := x.planStopRows(t, mission); !reflect.DeepEqual(rows, map[int64][]string{2: first, 3: bucStops}) {
		t.Fatal("a re-plan rewrote earlier stops", rows)
	}
	if p := read(); p.Revision != 3 || !slices.Equal(p.Fields.Stops, bucStops) {
		t.Fatal("the current plan is not the latest revision", p.Revision)
	}
	// A plan without stops stores none and shows none.
	if ack, err := m.plan(t, mission, 3, stopsPlan(nil)); err != nil || ack.Revision != 4 {
		t.Fatal("re-plan without stops", err)
	}
	if p := read(); p.Revision != 4 || p.Fields.Stops != nil {
		t.Fatal("a plan without stops shows stops", p.Fields.Stops)
	}
	if ack, err := m.plan(t, mission, 4, stopsPlan(bucStops)); err != nil || ack.Revision != 5 {
		t.Fatal("re-plan with stops again", err)
	}
	if _, err := m.assignTo(t, mission, 5, d.person.employee); err != nil {
		t.Fatal(err)
	}
	// The assignee's own list carries the planned stops it accepts.
	own, err := m.missions.ListMissions(t.Context(), m.scope, personMissionActor(d.person), d.person.member, true, tripapp.MissionFilter{Limit: 10})
	if err != nil || len(own.Missions) != 1 || own.Missions[0].Plan == nil || !slices.Equal(own.Missions[0].Plan.Fields.Stops, bucStops) {
		t.Fatal("the assignee's missions lost the planned stops", err)
	}
	if ack, err := m.deliver(t, d, m.respondOp(t, d, mission, 6, "ACCEPT", nil, nil)); err != nil || ack.Result() != protocol.Accepted {
		t.Fatal("acceptance", err)
	}
	before := x.planStopRows(t, mission)
	_, err = m.plan(t, mission, 7, stopsPlan([]string{"Mudança depois do aceite"}))
	wantMissionError(t, err, tripdomain.ErrInvalidTransition)
	if rows := x.planStopRows(t, mission); !reflect.DeepEqual(rows, before) || m.head(t, mission).revision != 7 {
		t.Fatal("an accepted mission was re-planned", rows)
	}
}

// BUC-EMP-243, B-BR-EMP-476/478..480: the member reads the plan beside the
// stops and legs derived from the trip's logbook, each boundary with its
// recorded times; delivery order, replays, identifier reuse and refused plan
// changes never change the observed side.
func TestTripStopsOwnReadKeepsThePlanApartFromObservedTimes(t *testing.T) {
	x := tripStopsFixture(t)
	d := x.assignedDriver(t)
	mission := x.plannedMission(t, d, bucStops)
	start := x.m.missionStartOp(t, d, mission, x.declaredNow(t))
	ack, err := x.process(t, d, start)
	x.eventState.tripState.accepted(t, start, ack, err, 1, 1)
	trip := tripOf(start)
	if x.m.linkedMission(t, trip) != mission {
		t.Fatal("the start did not link its mission")
	}
	acks := map[string]protocol.Acknowledgement{}
	event := func(kind string, text any, at time.Time) protocol.Operation {
		t.Helper()
		op := x.eventOp(t, d, trip, kind, text, at, nil)
		ack, err := x.process(t, d, op)
		x.eventState.accepted(t, op, ack, err)
		acks[op.ID()] = ack
		return op
	}
	t1 := x.declaredNow(t)
	arrival := event("ARRIVAL", "Cliente Jardim Primavera, doca 2", t1)
	event("NOTE", "Cliente ausente", x.declaredNow(t))
	stop := event("STOP", nil, x.declaredNow(t))
	// The departure from the first stop is delivered after the next stop.
	t2 := t1.Add(x.declaredNow(t).Sub(t1) / 3).Truncate(time.Microsecond)
	departure := event("DEPARTURE", nil, t2)
	leave := event("DEPARTURE", nil, x.declaredNow(t))
	// A plan change while the mission executes is refused (B-BR-EMP-413).
	_, err = x.m.plan(t, mission, 5, stopsPlan([]string{"Parada nova"}))
	wantMissionError(t, err, tripdomain.ErrInvalidTransition)
	finish := x.finishOp(t, d, trip, x.declaredNow(t), nil)
	ack, err = x.process(t, d, finish)
	x.eventState.tripState.accepted(t, finish, ack, err, 2, 1)
	if h := x.m.head(t, mission); h.state != "COMPLETED" {
		t.Fatal("the linked mission did not complete", h)
	}
	view := x.mustRead(t, d, trip)
	p := view.Planned
	if p == nil || p.MissionID != mission || p.Revision != 2 || !slices.Equal(p.Stops, bucStops) || len(p.Legs) != 4 || p.DestinationText == nil ||
		*p.DestinationText != "Centro de distribuição Norte" || p.Legs[0].From.Kind != tripdomain.PointStart || p.Legs[3].To.Kind != tripdomain.PointDestination {
		t.Fatal("planned side", p)
	}
	if view.TripID != trip || view.State != tripdomain.Completed || view.End == nil || view.End.Mark != tripdomain.MarkFinish || len(view.Held) != 0 || len(view.AfterEnd) != 0 {
		t.Fatal("trip summary", view.State, view.End)
	}
	stops := []string{}
	for _, s := range view.Stops {
		stops = append(stops, boundaryID(s.Arrival)+">"+boundaryID(s.Departure)+":"+string(s.Issue))
	}
	if want := []string{"ARRIVAL/" + eventOf(arrival) + ">DEPARTURE/" + eventOf(departure) + ":", "STOP/" + eventOf(stop) + ">DEPARTURE/" + eventOf(leave) + ":"}; !slices.Equal(stops, want) {
		t.Fatal("observed stops", stops)
	}
	legs := []string{}
	for _, l := range view.Legs {
		legs = append(legs, boundaryID(l.Departure)+">"+boundaryID(l.Arrival)+":"+string(l.Issue))
	}
	if want := []string{"START/" + trip + ">ARRIVAL/" + eventOf(arrival) + ":", "DEPARTURE/" + eventOf(departure) + ">STOP/" + eventOf(stop) + ":",
		"DEPARTURE/" + eventOf(leave) + ">FINISH/" + trip + ":"}; !slices.Equal(legs, want) {
		t.Fatal("observed legs", legs)
	}
	// Each boundary keeps the stored declaration and receipt (BR-TIME-001/002).
	org := x.scope.OrgID().String()
	for _, f := range []*tripapp.ObservedFact{view.Stops[0].Arrival, view.Stops[0].Departure, view.Stops[1].Arrival, view.Stops[1].Departure} {
		var exact bool
		if err := x.f.Admin.QueryRow(t.Context(), `SELECT occurred_at+make_interval(secs=>occurred_submicro/1e9)=$3 AND received_at=$4 AND entry_zone=$5 AND device_offset_seconds=$6
 FROM org_trip_events WHERE org_id=$1 AND id=$2`, org, f.FactID, f.Declared.OccurredAt, f.ReceivedAt, f.Declared.EntryZone, f.Declared.DeviceOffsetSeconds).Scan(&exact); err != nil || !exact ||
			f.Declared.EntryZone != "America/Cuiaba" || f.Declared.DeviceOffsetSeconds != -14400 || f.AfterReceipt() {
			t.Fatal("observed boundary is not the stored fact", f.FactID, err)
		}
	}
	if !view.Stops[0].Departure.Declared.OccurredAt.Before(view.Stops[1].Arrival.Declared.OccurredAt) || !view.Stops[0].Departure.ReceivedAt.After(view.Stops[1].Arrival.ReceivedAt) {
		t.Fatal("the late departure lost its own receipt or moved")
	}
	if view.Start.FactID != trip || view.Start.AfterReceipt() || view.Start.ReceivedAt.IsZero() {
		t.Fatal("start boundary", view.Start)
	}
	// Duplicate transport: resends return the stored ACK byte for byte, another
	// digest under the same identifier is OPERATION_ID_REUSED, and the read is
	// unchanged.
	facts := x.observedFacts(t)
	x.replays(t, d, arrival, acks[arrival.ID()])
	x.replays(t, d, departure, acks[departure.ID()])
	reused := x.eventOp(t, d, trip, "DEPARTURE", nil, t1, func(raw, _ map[string]any) {
		raw["operation_id"], raw["entity"] = arrival.ID(), map[string]any{"type": tripapp.EventEntityType, "id": eventOf(arrival), "expected_revision": nil}
	})
	if refusal, err := x.process(t, d, reused); err != nil || refusal.Result() != protocol.Rejected || refusal.Code() != "OPERATION_ID_REUSED" {
		t.Fatal("same identifier with another digest", err, refusal.Code())
	}
	// Planned data never rewrites observations: a refused plan change on the
	// completed mission stores nothing and the observed side is identical.
	_, err = x.m.plan(t, mission, 6, stopsPlan([]string{"Parada nova"}))
	wantMissionError(t, err, tripdomain.ErrInvalidTransition)
	if rows := x.planStopRows(t, mission); !reflect.DeepEqual(rows, map[int64][]string{2: bucStops}) {
		t.Fatal("planned stops changed after the observations", rows)
	}
	if again := x.mustRead(t, d, trip); x.observedFacts(t) != facts || !reflect.DeepEqual(again, view) {
		t.Fatal("a replay, a reuse or a refused plan change rewrote the observed side")
	}
}

// BUC-EMP-244, B-BR-EMP-477: unpaired departures and arrivals are surfaced
// as missing boundaries, a boundary declared after its receipt is flagged and,
// once a finish is declared before it, listed apart; an observation after the
// finish is held; nothing is dropped, moved or paired.
func TestTripStopsSurfaceIncoherentObservations(t *testing.T) {
	x := tripStopsFixture(t)
	d := x.assignedDriver(t)
	start := x.startOp(t, d, x.declaredNow(t), absentReading)
	ack, err := x.process(t, d, start)
	x.eventState.tripState.accepted(t, start, ack, err, 1, 0)
	trip := tripOf(start)
	ids := map[string]string{}
	event := func(name, kind string, at time.Time) {
		t.Helper()
		op := x.eventOp(t, d, trip, kind, nil, at, nil)
		ack, err := x.process(t, d, op)
		x.eventState.accepted(t, op, ack, err)
		ids[name] = eventOf(op)
	}
	event("d1", "DEPARTURE", x.declaredNow(t))
	event("d2", "DEPARTURE", x.declaredNow(t))
	event("a3", "ARRIVAL", x.declaredNow(t))
	event("a4", "ARRIVAL", x.declaredNow(t))
	event("d5", "DEPARTURE", x.declaredNow(t))
	// A device clock ahead of the server: accepted while the trip is open.
	ahead := x.declaredNow(t).Add(10 * time.Minute)
	event("a6", "ARRIVAL", ahead)
	open := x.mustRead(t, d, trip)
	if len(open.AfterEnd) != 0 || open.End != nil || len(open.Stops) != 5 || open.Stops[4].Arrival.FactID != ids["a6"] || !open.Stops[4].Arrival.AfterReceipt() || open.Stops[4].Departure != nil {
		t.Fatal("an open trip lost the boundary declared after its receipt", len(open.Stops))
	}
	// The finish is then declared before that arrival.
	finished := x.declaredNow(t)
	finish := x.finishOp(t, d, trip, finished, absentReading)
	ack, err = x.process(t, d, finish)
	x.eventState.tripState.accepted(t, finish, ack, err, 2, 0)
	late := x.eventOp(t, d, trip, "ARRIVAL", "Chegada depois do fim", finished.Add(time.Second), nil)
	ack, err = x.process(t, d, late)
	x.eventState.held(t, late, ack, err)
	view := x.mustRead(t, d, trip)
	got := []string{}
	for _, s := range view.Stops {
		got = append(got, boundaryID(s.Arrival)+">"+boundaryID(s.Departure)+":"+string(s.Issue))
	}
	want := []string{"->DEPARTURE/" + ids["d1"] + ":ARRIVAL_MISSING", "->DEPARTURE/" + ids["d2"] + ":ARRIVAL_MISSING", "ARRIVAL/" + ids["a3"] + ">-:DEPARTURE_MISSING",
		"ARRIVAL/" + ids["a4"] + ">DEPARTURE/" + ids["d5"] + ":"}
	if !slices.Equal(got, want) {
		t.Fatal("incoherent stops", got)
	}
	got = got[:0]
	for _, l := range view.Legs {
		got = append(got, boundaryID(l.Departure)+">"+boundaryID(l.Arrival)+":"+string(l.Issue))
	}
	want = []string{"START/" + trip + ">-:ARRIVAL_MISSING", "DEPARTURE/" + ids["d1"] + ">-:ARRIVAL_MISSING", "DEPARTURE/" + ids["d2"] + ">ARRIVAL/" + ids["a3"] + ":",
		"->ARRIVAL/" + ids["a4"] + ":DEPARTURE_MISSING", "DEPARTURE/" + ids["d5"] + ">FINISH/" + trip + ":"}
	if !slices.Equal(got, want) {
		t.Fatal("incoherent legs", got)
	}
	if len(view.AfterEnd) != 1 || view.AfterEnd[0].FactID != ids["a6"] || !view.AfterEnd[0].AfterReceipt() || !view.AfterEnd[0].Declared.OccurredAt.Equal(ahead) {
		t.Fatal("the boundary declared after the finish was not listed apart with its own time", view.AfterEnd)
	}
	if len(view.Held) != 1 || view.Held[0].EventID != eventOf(late) || view.Held[0].Code != tripapp.TripAlreadyClosed || view.Held[0].Type != tripdomain.LogbookArrival {
		t.Fatal("the held observation is not surfaced", view.Held)
	}
	if view.Planned != nil {
		t.Fatal("a trip without a mission shows a plan")
	}
	// Nothing was dropped: every accepted observation appears exactly once
	// as a stop boundary or after the end, and the held one as held.
	seen := map[string]int{}
	for _, s := range view.Stops {
		for _, f := range []*tripapp.ObservedFact{s.Arrival, s.Departure} {
			if f != nil {
				seen[f.FactID]++
			}
		}
	}
	for _, f := range view.AfterEnd {
		seen[f.FactID]++
	}
	for name, id := range ids {
		if seen[id] != 1 {
			t.Fatal("an observation was dropped or repeated", name, seen[id])
		}
	}
}

// B-BR-EMP-478: the read answers only the member who started the trip or
// currently holds it; another member's, another organization's and an
// unknown trip are the same not-found, and a member without trip.trip.start
// is denied.
func TestTripStopsOwnReadIsScopedToTheMember(t *testing.T) {
	x := tripStopsFixture(t)
	d1, d2 := x.assignedDriver(t), x.assignedDriver(t)
	start := x.startOp(t, d1, x.declaredNow(t), absentReading)
	ack, err := x.process(t, d1, start)
	x.eventState.tripState.accepted(t, start, ack, err, 1, 0)
	trip := tripOf(start)
	arrival := x.eventOp(t, d1, trip, "ARRIVAL", nil, x.declaredNow(t), nil)
	ack, err = x.process(t, d1, arrival)
	x.eventState.accepted(t, arrival, ack, err)
	view := x.mustRead(t, d1, trip)
	if view.Planned != nil || view.End != nil || len(view.Stops) != 1 || view.Stops[0].Departure != nil || view.Stops[0].Issue != "" || len(view.Legs) != 1 {
		t.Fatal("an open trip stopped at its first stop", view.Stops)
	}
	for name, id := range map[string]string{"another member's trip": trip, "unknown trip": onboardingID(t)} {
		reader := d2
		if name == "unknown trip" {
			reader = d1
		}
		if _, err := x.read(t, reader, id); !errors.Is(err, tripapp.ErrNotFound) {
			t.Fatal(name, err)
		}
	}
	if _, err := x.read(t, d1, x.foreignTrip(t)); !errors.Is(err, tripapp.ErrNotFound) {
		t.Fatal("another organization's trip", err)
	}
	auditor := x.staff(t, "AUDITOR", "ORG", "")
	if _, err := x.stops.ReadOwnTripStops(t.Context(), x.scope, auditor.actor, auditor.member, trip); err == nil || errors.Is(err, tripapp.ErrNotFound) {
		t.Fatal("a member without trip.trip.start read a trip", err)
	}
	// After a handover the receiver holds the trip and the starter keeps it.
	supervisor := x.staff(t, "SUPERVISOR", "ORG", "")
	if _, _, err := x.handOver(x.scope, supervisor, trip, 1, d2.person.member, "Troca de turno", onboardingID(t)); err != nil {
		t.Fatal("handover", err)
	}
	for _, reader := range []syncFixtureState{d1, d2} {
		if v := x.mustRead(t, reader, trip); !reflect.DeepEqual(v, view) {
			t.Fatal("the starter and the receiver read different trips")
		}
	}
	third := x.assignedDriver(t)
	if _, err := x.read(t, third, trip); !errors.Is(err, tripapp.ErrNotFound) {
		t.Fatal("a third member read the trip", err)
	}
}

// Concurrency: two plans with different stops on one revision queue behind
// the held organization root; the first queued wins with its stops only and
// the other is CONFLICT with no stop, in both orders.
func TestTripStopsConcurrentPlansKeepOneStopList(t *testing.T) {
	x := tripStopsFixture(t)
	m := x.m
	for _, order := range []string{"first-queued-wins", "second-queued-wins"} {
		t.Run(order, func(t *testing.T) {
			mission := m.create(t, m.unit, "").MissionID
			lists := [][]string{{"Cliente Norte"}, {"Cliente Sul", "Posto Leste"}}
			if order == "second-queued-wins" {
				lists[0], lists[1] = lists[1], lists[0]
			}
			operations := []string{onboardingID(t), onboardingID(t)}
			errs := make([]error, 2)
			release := holdRoot(t, m)
			t.Cleanup(release)
			var wg sync.WaitGroup
			for i := range lists {
				command := missionCommand(t, tripapp.MissionCommandInput{Kind: tripapp.MissionPlanCommand, OperationID: operations[i], MissionID: mission, ExpectedRevision: 1, Plan: stopsPlan(lists[i])})
				wg.Go(func() {
					_, _, errs[i] = m.missions.CommandMission(context.Background(), m.scope, ownerMissionActor(m.legalFixtureState), m.member, "", command)
				})
				waitLockWaiters(t, m, i+1)
			}
			release()
			wg.Wait()
			if errs[0] != nil || !errors.Is(errs[1], tripdomain.ErrConflict) {
				t.Fatal("the first queued plan did not win alone", errs)
			}
			if rows := x.planStopRows(t, mission); !reflect.DeepEqual(rows, map[int64][]string{2: lists[0]}) {
				t.Fatal("the stops are not the winner's", rows)
			}
			if c := m.planCounts(t, operations[1]); c != (planCounts{}) {
				t.Fatalf("the losing plan stored state: %+v", c)
			}
		})
	}
}

// B-BR-EMP-479, EDR-310: forced RLS, insert-only history and the guards that
// keep stop rows inside their own PLAN transaction.
func TestTripStopsSchemaIsForcedRLSInsertOnlyAndGuarded(t *testing.T) {
	x := tripStopsFixture(t)
	m := x.m
	mission := m.create(t, m.unit, "").MissionID
	if _, err := m.plan(t, mission, 1, stopsPlan([]string{"Cliente Norte"})); err != nil {
		t.Fatal(err)
	}
	org := x.scope.OrgID().String()
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE,TRUNCATE ON org_trip_mission_plan_stops TO "+role); err != nil {
		t.Fatal(err)
	}
	var forced bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT relrowsecurity AND relforcerowsecurity FROM pg_class WHERE oid='org_trip_mission_plan_stops'::regclass`).Scan(&forced); err != nil || !forced {
		t.Fatal("the stop table is not forced RLS", err)
	}
	var visible int
	if err := x.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_mission_plan_stops`).Scan(&visible); err != nil || visible != 0 {
		t.Fatal("stops visible without an organization setting", visible, err)
	}
	exec := func(statement string, args ...any) error {
		return database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
			_, err := tx.Exec(t.Context(), statement, args...)
			return err
		})
	}
	var pgErr *pgconn.PgError
	for _, statement := range []string{
		`UPDATE org_trip_mission_plan_stops SET place_text='Reescrito' WHERE org_id=$1`,
		`DELETE FROM org_trip_mission_plan_stops WHERE org_id=$1`,
	} {
		if err := exec(statement, org); !errors.As(err, &pgErr) || pgErr.Code != "55000" {
			t.Fatal("planned stops were rewritten", statement, err)
		}
	}
	if err := exec(`TRUNCATE org_trip_mission_plan_stops`); !errors.As(err, &pgErr) || pgErr.Code != "55000" {
		t.Fatal("planned stops were truncated", err)
	}
	insert := `INSERT INTO org_trip_mission_plan_stops(org_id,mission_id,revision,sequence,place_text) VALUES($1,$2,$3,$4,$5)`
	for name, c := range map[string]struct {
		args []any
		code string
	}{
		// The PLAN receipt exists: the published plan cannot gain a stop.
		"after-the-receipt": {[]any{org, mission, int64(2), 2, "Parada tardia"}, "23514"},
		"out-of-sequence":   {[]any{org, mission, int64(2), 3, "Parada tardia"}, "23514"},
		"no-planning-row":   {[]any{org, mission, int64(1), 1, "Parada"}, "23503"},
		"unknown-mission":   {[]any{org, onboardingID(t), int64(2), 1, "Parada"}, "23503"},
		"cpf-shaped-text":   {[]any{org, mission, int64(2), 2, "Contato 123.456.789-09"}, "23514"},
		"sequence-above-20": {[]any{org, mission, int64(2), 21, "Parada"}, "23514"},
		"blank-text":        {[]any{org, mission, int64(2), 2, "  "}, "23514"},
	} {
		if err := exec(insert, c.args...); !errors.As(err, &pgErr) || pgErr.Code != c.code {
			t.Fatal("a stop row outside its PLAN transaction was admitted", name, err)
		}
	}
	if err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, onboardingID(t))
		if err != nil {
			return err
		}
		_, err = tx.Exec(t.Context(), insert, org, mission, int64(2), 2, "Parada")
		return err
	}); err == nil {
		t.Fatal("a stop row under another organization setting was admitted")
	}
	// A stop row admitted inside a hand-made PLAN transaction that never
	// writes the receipt is refused at commit (the stop's and the mission's
	// deferred publication checks) and leaves nothing.
	orphan := m.create(t, m.unit, "").MissionID
	err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
		op := onboardingID(t)
		if _, err := tx.Exec(t.Context(), `INSERT INTO org_trip_mission_transitions(org_id,mission_id,revision,kind,from_state,to_state,source,actor_account_id,actor_membership_id,operation_id)
VALUES($1,$2,2,'PLAN','DRAFT','PLANNED','COMPANY',$3,$4,$5)`, org, orphan, x.actor.AccountID, m.member, op); err != nil {
			return err
		}
		if _, err := tx.Exec(t.Context(), `INSERT INTO org_trip_mission_plans(org_id,mission_id,revision,operation_id,destination_text,reason,load_kind,planned_by,planned_by_membership_id)
VALUES($1,$2,2,$3,'Centro','Entrega','NONE',$4,$5)`, org, orphan, op, x.actor.AccountID, m.member); err != nil {
			return err
		}
		if _, err := tx.Exec(t.Context(), `UPDATE org_trip_missions SET state='PLANNED',revision=2 WHERE org_id=$1 AND id=$2`, org, orphan); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), insert, org, orphan, int64(2), 1, "Parada")
		return err
	})
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatal("an unpublished stop committed", err)
	}
	if rows := x.planStopRows(t, orphan); len(rows) != 0 {
		t.Fatal("the refused transaction left stops", rows)
	}
}
