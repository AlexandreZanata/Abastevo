package application_test

import (
	"errors"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
)

func runRef(n int) map[string]any {
	return map[string]any{"type": app.ChecklistRunReferenceType, "id": id(n)}
}

// EMP-TRIP-019 (B-BR-EMP-511, EDR-331): trip.start may name one departure
// run beside its mission; trip.finish one return run; nothing else.
func TestParseStartAndFinishReadTheChecklistRunReference(t *testing.T) {
	named, err := app.ParseStart(start(t, func(e, _ map[string]any) { e["soft_references"] = []any{runRef(91)} }))
	if err != nil {
		t.Fatal("departure run reference refused", err)
	}
	if v, err := named.Snapshot(); err != nil || v.DepartureRunID != id(91) || v.MissionID != "" || v.AssetID != id(10) {
		t.Fatal("departure run not kept", v.DepartureRunID, v.MissionID, err)
	}
	both, err := app.ParseStart(start(t, func(e, _ map[string]any) {
		e["soft_references"] = []any{runRef(91), map[string]any{"type": app.MissionReferenceType, "id": id(90)}}
	}))
	if err != nil {
		t.Fatal("mission and departure run refused", err)
	}
	if v, err := both.Snapshot(); err != nil || v.DepartureRunID != id(91) || v.MissionID != id(90) {
		t.Fatal("mission and departure run not kept", v.DepartureRunID, v.MissionID, err)
	}
	if v, _ := mustStart(t, nil).Snapshot(); v.DepartureRunID != "" {
		t.Fatal("a start without a reference names a run")
	}
	for name, refs := range map[string][]any{
		"two runs":          {runRef(91), runRef(92)},
		"run and another":   {runRef(91), map[string]any{"type": "fueling", "id": id(93)}},
		"three references":  {runRef(91), map[string]any{"type": app.MissionReferenceType, "id": id(90)}, map[string]any{"type": "trip", "id": id(94)}},
		"two missions, run": {runRef(91), map[string]any{"type": app.MissionReferenceType, "id": id(90)}, map[string]any{"type": app.MissionReferenceType, "id": id(95)}},
	} {
		if _, err := app.ParseStart(start(t, func(e, _ map[string]any) { e["soft_references"] = refs })); !errors.Is(err, app.ErrUnsupported) {
			t.Fatal("unsupported start reference form admitted", name, err)
		}
	}
	finished, err := app.ParseFinish(finish(t, func(e, _ map[string]any) { e["soft_references"] = []any{runRef(96)} }))
	if err != nil {
		t.Fatal("return run reference refused", err)
	}
	if v, err := finished.Snapshot(); err != nil || v.ReturnRunID != id(96) {
		t.Fatal("return run not kept", v.ReturnRunID, err)
	}
	plain, err := app.ParseFinish(finish(t, nil))
	if v, _ := plain.Snapshot(); err != nil || v.ReturnRunID != "" {
		t.Fatal("a finish without a reference names a run", err)
	}
	for name, refs := range map[string][]any{
		"two runs":    {runRef(96), runRef(97)},
		"run mission": {runRef(96), map[string]any{"type": app.MissionReferenceType, "id": id(90)}},
		"mission":     {map[string]any{"type": app.MissionReferenceType, "id": id(90)}},
	} {
		if _, err := app.ParseFinish(finish(t, func(e, _ map[string]any) { e["soft_references"] = refs })); !errors.Is(err, app.ErrUnsupported) {
			t.Fatal("unsupported finish reference form admitted", name, err)
		}
	}
	// A malformed payload stays MALFORMED whatever the reference says.
	if _, err := app.ParseStart(start(t, func(e, p map[string]any) {
		e["soft_references"] = []any{runRef(91)}
		p["departure_run_id"] = id(91)
	})); !errors.Is(err, app.ErrMalformed) {
		t.Fatal("payload run key admitted", err)
	}
}

func mustStart(t *testing.T, change func(e, p map[string]any)) app.StartCommand {
	t.Helper()
	c, err := app.ParseStart(start(t, change))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// B-BR-EMP-512/513: the departure gate, first failure first, with the held
// acknowledgement code each reason implies (EDR-332/333).
func TestDepartureGateDecidesTheReasonFirstFailureFirst(t *testing.T) {
	author, asset, other := id(3), id(10), id(11)
	completed := time.Date(2026, 11, 15, 10, 40, 0, 123, time.UTC)
	at := completed.Add(15 * time.Minute)
	valid := app.ChecklistRun{ID: id(91), AssetID: asset, Context: app.ChecklistDeparture, AuthorMembershipID: author, CompletedAt: completed, ValiditySeconds: 43200}
	with := func(change func(*app.ChecklistRun)) *app.ChecklistRun {
		r := valid
		change(&r)
		return &r
	}
	pass := app.GateDecision{}
	link := app.GateDecision{Link: true}
	required := app.GateDecision{Reason: app.ChecklistRequired, Code: app.AssetUnavailable}
	requiredOrphan := app.GateDecision{Reason: app.ChecklistRequired, Code: app.OrphanReference}
	orphan := app.GateDecision{Reason: app.OrphanReference, Code: app.OrphanReference}
	blocking := app.GateDecision{Reason: app.ChecklistBlocking, Code: app.AssetUnavailable}
	expired := app.GateDecision{Reason: app.ChecklistExpired, Code: app.AssetUnavailable}
	for _, tc := range []struct {
		name     string
		required bool
		named    string
		run      *app.ChecklistRun
		at       time.Time
		want     app.GateDecision
	}{
		{"nothing required nothing named", false, "", nil, at, pass},
		{"required nothing named", true, "", nil, at, required},
		{"required valid run", true, id(91), &valid, at, link},
		{"not required valid run still linked", false, id(91), &valid, at, link},
		{"required unknown or foreign run", true, id(91), nil, at, requiredOrphan},
		{"not required unknown or foreign run", false, id(91), nil, at, orphan},
		{"another asset's run", true, id(91), with(func(r *app.ChecklistRun) { r.AssetID = other }), at, requiredOrphan},
		{"a return run", true, id(91), with(func(r *app.ChecklistRun) { r.Context = app.ChecklistReturn }), at, requiredOrphan},
		{"an ad hoc run", false, id(91), with(func(r *app.ChecklistRun) { r.Context = "AD_HOC" }), at, orphan},
		{"another member's run", true, id(91), with(func(r *app.ChecklistRun) { r.AuthorMembershipID = id(4) }), at, requiredOrphan},
		{"a run another trip used", true, id(91), with(func(r *app.ChecklistRun) { r.Linked = true }), at, requiredOrphan},
		{"facts of another run", true, id(91), with(func(r *app.ChecklistRun) { r.ID = id(92) }), at, requiredOrphan},
		{"blocking at the start", true, id(91), with(func(r *app.ChecklistRun) { r.BlockingAt = true }), at, blocking},
		{"blocking before expired", true, id(91), with(func(r *app.ChecklistRun) { r.BlockingAt = true }), completed.Add(13 * time.Hour), blocking},
		{"blocking named while not required", false, id(91), with(func(r *app.ChecklistRun) { r.BlockingAt = true }), at, blocking},
		{"at the completion", true, id(91), &valid, completed, link},
		{"one nanosecond before the completion", true, id(91), &valid, completed.Add(-time.Nanosecond), expired},
		{"last nanosecond of the validity", true, id(91), &valid, completed.Add(12*time.Hour - time.Nanosecond), link},
		{"at the end of the validity", true, id(91), &valid, completed.Add(12 * time.Hour), expired},
		{"server policy shorter than the cache", true, id(91), with(func(r *app.ChecklistRun) { r.ValiditySeconds = 3600 }), completed.Add(13 * time.Hour), expired},
		{"expired named while not required", false, id(91), &valid, completed.Add(72 * time.Hour), expired},
		{"no validity is never valid", true, id(91), with(func(r *app.ChecklistRun) { r.ValiditySeconds = 0 }), at, expired},
	} {
		if got := app.DepartureGate(tc.required, tc.named, tc.run, author, asset, tc.at); got != tc.want {
			t.Fatalf("%s: got %+v want %+v", tc.name, got, tc.want)
		}
	}
}

// B-BR-EMP-515 (BR-CHECK-013): a return run is linked only when it counts,
// whatever its outcome; nothing about it holds the finish.
func TestReturnLinkableChecksIdentityAndTheTripWindow(t *testing.T) {
	author, asset := id(3), id(10)
	started := time.Date(2026, 11, 15, 8, 0, 0, 500, time.UTC)
	finished := started.Add(4 * time.Hour)
	run := app.ChecklistRun{ID: id(96), AssetID: asset, Context: app.ChecklistReturn, AuthorMembershipID: author, CompletedAt: finished.Add(-10 * time.Minute), BlockingAt: true}
	if !app.ReturnLinkable(id(96), &run, author, asset, started, finished) {
		t.Fatal("a blocking return run of the trip was not linked")
	}
	for name, change := range map[string]func(*app.ChecklistRun){
		"departure context": func(r *app.ChecklistRun) { r.Context = app.ChecklistDeparture },
		"another asset":     func(r *app.ChecklistRun) { r.AssetID = id(11) },
		"another member":    func(r *app.ChecklistRun) { r.AuthorMembershipID = id(4) },
		"already linked":    func(r *app.ChecklistRun) { r.Linked = true },
		"before the start":  func(r *app.ChecklistRun) { r.CompletedAt = started.Add(-time.Nanosecond) },
		"after the finish":  func(r *app.ChecklistRun) { r.CompletedAt = finished.Add(time.Nanosecond) },
		"another run":       func(r *app.ChecklistRun) { r.ID = id(97) },
	} {
		r := run
		change(&r)
		if app.ReturnLinkable(id(96), &r, author, asset, started, finished) {
			t.Fatal("a return run that does not count was linked", name)
		}
	}
	for name, at := range map[string]time.Time{"at the start": started, "at the finish": finished} {
		r := run
		r.CompletedAt = at
		if !app.ReturnLinkable(id(96), &r, author, asset, started, finished) {
			t.Fatal("a return run on the trip's bound was not linked", name)
		}
	}
	if app.ReturnLinkable(id(96), nil, author, asset, started, finished) || app.ReturnLinkable("", &run, author, asset, started, finished) {
		t.Fatal("a missing or unnamed return run was linked")
	}
}
