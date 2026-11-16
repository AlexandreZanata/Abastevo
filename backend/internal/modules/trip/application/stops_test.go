package application_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

// EMP-TRIP-010 (B-BR-EMP-475, B-BR-EMP-480): planned stops ride on PLAN; a
// plan without stops keeps the v1 digest byte for byte, stops are bound in v2.
func TestMissionPlanStopsKeepTheV1DigestAndBindStops(t *testing.T) {
	text := func(v string) *string { return &v }
	plan := app.MissionPlanFields{DestinationText: text("Centro de distribuição Norte"), Reason: "Entrega de peças", Load: "NONE"}
	base := app.MissionCommandInput{Kind: app.MissionPlanCommand, OperationID: id(1), MissionID: id(4), ExpectedRevision: 2, Plan: &plan}
	scope := missionScope(t, 1)
	c, err := app.NewMissionCommand(base)
	if err != nil {
		t.Fatal(err)
	}
	var none *int
	raw, _ := json.Marshal([]any{"trip.mission.plan.v1", scope.OrgID().String(), app.MissionPlanCommand, id(1), id(4), int64(2), plan.DestinationText, "", "Entrega de peças", "", "NONE", (*string)(nil), none})
	if sum := sha256.Sum256(raw); c.Digest(scope) != hex.EncodeToString(sum[:]) {
		t.Fatal("a plan without stops changed the EMP-TRIP-009 digest")
	}
	stops := []string{"Cliente Jardim Primavera", "Posto de apoio BR-163"}
	withStops := plan
	withStops.Stops = stops
	in := base
	in.Plan = &withStops
	s, err := app.NewMissionCommand(in)
	if err != nil {
		t.Fatal("valid stops refused", err)
	}
	raw, _ = json.Marshal([]any{"trip.mission.plan.v2", scope.OrgID().String(), app.MissionPlanCommand, id(1), id(4), int64(2), plan.DestinationText, "", "Entrega de peças", "", "NONE", (*string)(nil), none,
		[]string{"Cliente Jardim Primavera", "Posto de apoio BR-163"}})
	if sum := sha256.Sum256(raw); s.Digest(scope) != hex.EncodeToString(sum[:]) {
		t.Fatal("the stops are not bound by the v2 digest")
	}
	stops[0] = "changed after validation"
	if again, _ := s.Snapshot(); again.Plan.Stops[0] != "Cliente Jardim Primavera" {
		t.Fatal("the command shares the caller's stops")
	}
	digests := map[string]bool{c.Digest(scope): true, s.Digest(scope): true}
	for name, other := range map[string][]string{
		"reordered": {"Posto de apoio BR-163", "Cliente Jardim Primavera"},
		"one":       {"Cliente Jardim Primavera"},
		"three":     {"Cliente Jardim Primavera", "Posto de apoio BR-163", "Cliente Vila Rica"},
	} {
		p := plan
		p.Stops = other
		in := base
		in.Plan = &p
		v, err := app.NewMissionCommand(in)
		if err != nil {
			t.Fatal(name, err)
		}
		if d := v.Digest(scope); digests[d] {
			t.Fatal("the stops digest does not bind", name)
		}
		digests[v.Digest(scope)] = true
	}
	twentyOne := make([]string, trip.MaxPlannedStops+1)
	for i := range twentyOne {
		twentyOne[i] = "Parada"
	}
	for name, bad := range map[string][]string{
		"empty-list": {}, "blank": {"  "}, "control": {"Cliente\u0007"}, "cpf": {"Contato 123.456.789-09"}, "too-many": twentyOne,
		"281": {strings.Repeat("a", 281)},
	} {
		p := plan
		p.Stops = bad
		in := base
		in.Plan = &p
		if _, err := app.NewMissionCommand(in); !errors.Is(err, trip.ErrInvalid) {
			t.Fatal("invalid stops admitted", name, err)
		}
	}
}

func stopsDeclared(at time.Time) orgtime.Declaration {
	return orgtime.Declaration{OccurredAt: at, EntryZone: "America/Cuiaba", DeviceOffsetSeconds: -14400}
}

func stopsFact(mark trip.StopMark, factID string, at time.Time) app.ObservedFact {
	return app.ObservedFact{Mark: mark, FactID: factID, Declared: stopsDeclared(at), ReceivedAt: at.Add(time.Minute)}
}

// BUC-EMP-243: the plan and the observed stops and legs stay apart, each
// observed boundary with its recorded times.
func stopsRecord() app.StopsRecord {
	day := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	destination := "Centro de distribuição Norte"
	end := stopsFact(trip.MarkFinish, id(10), day.Add(270*time.Minute))
	return app.StopsRecord{TripID: id(10), AssetID: id(11), State: trip.Completed, Start: stopsFact(trip.MarkStart, id(10), day), End: &end,
		Events: []app.ObservedFact{
			stopsFact(trip.MarkDeparture, id(23), day.Add(195*time.Minute)), stopsFact(trip.MarkStop, id(22), day.Add(180*time.Minute)),
			stopsFact(trip.MarkDeparture, id(21), day.Add(100*time.Minute)), stopsFact(trip.MarkArrival, id(20), day.Add(70*time.Minute)),
		},
		Held: []app.HeldObservation{{EventID: id(30), Type: trip.LogbookArrival, Declared: stopsDeclared(day.Add(5 * time.Hour)), Code: app.TripAlreadyClosed}},
		Plan: &app.PlannedRecord{MissionID: id(40), Revision: 3, PlannedAt: day.Add(-24 * time.Hour), StartsAt: day, EndsAt: day.Add(6 * time.Hour), DestinationText: &destination,
			Stops: []string{"Cliente Jardim Primavera", "Posto de apoio BR-163", "Cliente Vila Rica"}}}
}

func TestBuildTripStopsKeepsThePlanApartFromTheObservedTimes(t *testing.T) {
	r := stopsRecord()
	v, err := app.BuildTripStops(r)
	if err != nil {
		t.Fatal(err)
	}
	if v.TripID != r.TripID || v.State != trip.Completed || v.Planned == nil || len(v.Planned.Stops) != 3 || len(v.Planned.Legs) != 4 || v.Planned.Revision != 3 {
		t.Fatal("planned side", v.Planned)
	}
	if len(v.Stops) != 2 || len(v.Legs) != 3 || len(v.Held) != 1 || v.Held[0].Code != app.TripAlreadyClosed {
		t.Fatal("observed side", len(v.Stops), len(v.Legs), len(v.Held))
	}
	first, second := v.Stops[0], v.Stops[1]
	if first.Arrival.FactID != id(20) || first.Departure.FactID != id(21) || second.Arrival.Mark != trip.MarkStop || second.Departure.FactID != id(23) || first.Issue != "" {
		t.Fatal("observed stops", first, second)
	}
	if first.Arrival.Declared.EntryZone != "America/Cuiaba" || first.Arrival.Declared.DeviceOffsetSeconds != -14400 || first.Arrival.ReceivedAt.IsZero() || first.Arrival.AfterReceipt() {
		t.Fatal("observed times lost their context")
	}
	if v.Legs[0].Departure.Mark != trip.MarkStart || v.Legs[2].Arrival.Mark != trip.MarkFinish || v.Legs[1].Departure.FactID != id(21) || v.Legs[1].Arrival.FactID != id(22) {
		t.Fatal("observed legs", v.Legs)
	}
	// The view is independent of the record.
	r.Plan.Stops[0] = "changed"
	r.Events[0].FactID = "changed"
	if v.Planned.Stops[0] != "Cliente Jardim Primavera" || v.Stops[1].Departure.FactID != id(23) {
		t.Fatal("the view shares the stored record")
	}
	// A device clock ahead of the server is flagged, never moved.
	ahead := stopsRecord()
	ahead.Events[3].ReceivedAt = ahead.Events[3].Declared.OccurredAt.Add(-time.Minute)
	got, err := app.BuildTripStops(ahead)
	if err != nil || !got.Stops[0].Arrival.AfterReceipt() || !got.Stops[0].Arrival.Declared.OccurredAt.Equal(ahead.Events[3].Declared.OccurredAt) {
		t.Fatal("after-receipt boundary", err)
	}
	// An accepted event declared after the recorded end (a device clock that
	// moved) is listed apart, never a stop and never dropped.
	late := stopsRecord()
	late.Events[0].Declared.OccurredAt = late.End.Declared.OccurredAt.Add(time.Second)
	got, err = app.BuildTripStops(late)
	if err != nil || len(got.AfterEnd) != 1 || got.AfterEnd[0].FactID != id(23) || len(got.Stops) != 2 || got.Stops[1].Departure != nil {
		t.Fatal("an event after the end", err, len(got.AfterEnd))
	}
	// A trip without a mission has no plan; a closure has no end.
	closed := stopsRecord()
	closed.Plan, closed.End = nil, nil
	if got, err := app.BuildTripStops(closed); err != nil || got.Planned != nil || got.Legs[len(got.Legs)-1].Arrival != nil {
		t.Fatal("closed trip without a mission", err)
	}
}

func TestBuildTripStopsRefusesBrokenStoredInvariants(t *testing.T) {
	// Precondition: the unchanged record builds, so each refusal below is
	// caused by its own change.
	if _, err := app.BuildTripStops(stopsRecord()); err != nil {
		t.Fatal("the valid stored record is refused", err)
	}
	for name, change := range map[string]func(*app.StopsRecord){
		"trip-id":          func(r *app.StopsRecord) { r.TripID = "x" },
		"asset-id":         func(r *app.StopsRecord) { r.AssetID = "" },
		"state":            func(r *app.StopsRecord) { r.State = "DONE" },
		"start-mark":       func(r *app.StopsRecord) { r.Start.Mark = trip.MarkArrival },
		"start-identity":   func(r *app.StopsRecord) { r.Start.FactID = id(99) },
		"start-zone":       func(r *app.StopsRecord) { r.Start.Declared.EntryZone = "Mars/Base" },
		"start-receipt":    func(r *app.StopsRecord) { r.Start.ReceivedAt = time.Time{} },
		"finish-identity":  func(r *app.StopsRecord) { r.End.FactID = id(99) },
		"end-while-open":   func(r *app.StopsRecord) { r.State = trip.InProgress },
		"cancel-by-finish": func(r *app.StopsRecord) { r.State = trip.Cancelled },
		"note-event":       func(r *app.StopsRecord) { r.Events[0].Mark = "NOTE" },
		"event-identity":   func(r *app.StopsRecord) { r.Events[0].FactID = "e" },
		"event-before-start": func(r *app.StopsRecord) {
			r.Events[0].Declared.OccurredAt = r.Start.Declared.OccurredAt.Add(-time.Second)
		},
		"repeated-event":    func(r *app.StopsRecord) { r.Events[1].FactID = r.Events[0].FactID },
		"held-type":         func(r *app.StopsRecord) { r.Held[0].Type = trip.LogbookNote },
		"held-code":         func(r *app.StopsRecord) { r.Held[0].Code = "ASSET_BUSY" },
		"plan-revision":     func(r *app.StopsRecord) { r.Plan.Revision = 1 },
		"plan-mission":      func(r *app.StopsRecord) { r.Plan.MissionID = "" },
		"plan-window":       func(r *app.StopsRecord) { r.Plan.EndsAt = r.Plan.StartsAt },
		"plan-destinations": func(r *app.StopsRecord) { r.Plan.DestinationUnitID = id(50) },
		"plan-stop-text":    func(r *app.StopsRecord) { r.Plan.Stops[1] = "CPF 12345678909" },
		"plan-empty-stops":  func(r *app.StopsRecord) { r.Plan.Stops = []string{} },
	} {
		r := stopsRecord()
		change(&r)
		if _, err := app.BuildTripStops(r); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("a broken stored record was served", name, err)
		}
	}
}

func TestStopsServiceProvesTheOwnTripsPolicy(t *testing.T) {
	if app.StopsPolicy() != app.HandoverPolicy(app.OwnTripsAction) || app.StopsPolicy().Permission != "trip.trip.start" || app.StopsPolicy().Effect != access.Read {
		t.Fatal("the stops read is not the own-trips read policy")
	}
	ctx, scope := t.Context(), kernel.OrgScope{}
	if _, err := (app.StopsService{}).Read(ctx, scope, app.ReviewActor{}, access.Decision{}, id(1)); !errors.Is(err, app.ErrReviewDisabled) {
		t.Fatal("disabled read", err)
	}
	if _, err := (app.StopsService{Enabled: true}).Read(ctx, scope, app.ReviewActor{}, access.Decision{}, "not-a-trip"); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("non-canonical trip", err)
	}
	if _, err := (app.StopsService{Enabled: true}).Read(ctx, scope, app.ReviewActor{}, access.Decision{}, id(1)); !errors.Is(err, app.ErrReviewUnauthorized) {
		t.Fatal("a read without a current decision", err)
	}
}
