package application_test

import (
	"errors"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

// EMP-TRIP-023 (B-BR-EMP-412, EDR-265): trip.start schema 1 may name the
// mission it executes through exactly one soft reference of type mission;
// the payload is unchanged and any other reference stays unsupported.
func TestParseStartReadsOneMissionSoftReference(t *testing.T) {
	plain, err := app.ParseStart(start(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := plain.Snapshot(); v.MissionID != "" {
		t.Fatal("a start without a reference names a mission", v.MissionID)
	}
	named, err := app.ParseStart(start(t, func(e, _ map[string]any) {
		e["soft_references"] = []any{map[string]any{"type": app.MissionReferenceType, "id": id(90)}}
	}))
	if err != nil {
		t.Fatal("mission reference refused", err)
	}
	v, err := named.Snapshot()
	if err != nil || v.MissionID != id(90) || v.AssetID != id(10) || v.DriverID != id(30) {
		t.Fatal("mission reference not kept", v.MissionID, err)
	}
	for name, refs := range map[string][]any{
		"another type":        {map[string]any{"type": "fueling", "id": id(90)}},
		"two missions":        {map[string]any{"type": app.MissionReferenceType, "id": id(90)}, map[string]any{"type": app.MissionReferenceType, "id": id(91)}},
		"mission and another": {map[string]any{"type": app.MissionReferenceType, "id": id(90)}, map[string]any{"type": "trip", "id": id(91)}},
	} {
		if _, err := app.ParseStart(start(t, func(e, _ map[string]any) { e["soft_references"] = refs })); !errors.Is(err, app.ErrUnsupported) {
			t.Fatal("unsupported reference form admitted", name, err)
		}
	}
	// A malformed payload stays MALFORMED whatever the reference says.
	if _, err := app.ParseStart(start(t, func(e, p map[string]any) {
		e["soft_references"] = []any{map[string]any{"type": app.MissionReferenceType, "id": id(90)}}
		p["mission_id"] = id(90)
	})); !errors.Is(err, app.ErrMalformed) {
		t.Fatal("payload mission key admitted", err)
	}
}

// B-BR-EMP-412: eligibility of the named mission at receipt.
func TestMissionEligibleForTheStart(t *testing.T) {
	author, driver, asset := id(3), id(30), id(10)
	ok := app.MissionStart{State: trip.MissionAccepted, AssigneeMembershipID: author, AssigneeEmployeeID: driver, AssetID: asset}
	if !app.MissionEligible(ok, author, driver, asset) {
		t.Fatal("eligible mission refused")
	}
	anyVehicle := ok
	anyVehicle.AssetID = ""
	if !app.MissionEligible(anyVehicle, author, driver, asset) {
		t.Fatal("a mission naming no vehicle refused")
	}
	for name, edit := range map[string]func(*app.MissionStart){
		"assigned not accepted": func(m *app.MissionStart) { m.State = trip.MissionAssigned },
		"in progress":           func(m *app.MissionStart) { m.State = trip.MissionInProgress },
		"cancelled":             func(m *app.MissionStart) { m.State = trip.MissionCancelled },
		"another member":        func(m *app.MissionStart) { m.AssigneeMembershipID = id(5) },
		"another driver":        func(m *app.MissionStart) { m.AssigneeEmployeeID = id(31) },
		"another vehicle":       func(m *app.MissionStart) { m.AssetID = id(11) },
		"already linked":        func(m *app.MissionStart) { m.Linked = true },
		"no assignee":           func(m *app.MissionStart) { m.AssigneeMembershipID, m.AssigneeEmployeeID = "", "" },
	} {
		m := ok
		edit(&m)
		if app.MissionEligible(m, author, driver, asset) {
			t.Fatal("ineligible mission admitted", name)
		}
	}
	for _, code := range []string{app.AssetBusy, app.AssetUnavailable, app.PermissionChanged, app.OrphanReference} {
		if !app.StartHeldCode(code) || !app.HeldCode(app.StartOperationType, code) {
			t.Fatal("start held code refused", code)
		}
	}
	if app.StartHeldCode(app.TripAlreadyClosed) || app.HeldCode(app.FinishOperationType, app.OrphanReference) {
		t.Fatal("foreign held code admitted for a start")
	}
}

// B-BR-EMP-414: the RESERVE command carries the mission, its revision and the
// purpose only; the vehicle and window come from the stored mission.
func TestMissionReserveCommandShape(t *testing.T) {
	good := app.MissionCommandInput{Kind: app.MissionReserveCommand, OperationID: id(20), MissionID: id(90), ExpectedRevision: 3, Reason: "Entrega sintética"}
	c, err := app.NewMissionCommand(good)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := c.Snapshot(); v.MissionID != id(90) || v.ExpectedRevision != 3 || v.Reason != "Entrega sintética" || len(c.Digest(missionScope(t, 1))) != 64 {
		t.Fatal("reserve command lost its fields")
	}
	for name, edit := range map[string]func(*app.MissionCommandInput){
		"no purpose":      func(v *app.MissionCommandInput) { v.Reason = "" },
		"control purpose": func(v *app.MissionCommandInput) { v.Reason = "a\x00b" },
		"no revision":     func(v *app.MissionCommandInput) { v.ExpectedRevision = 0 },
		"caller asset":    func(v *app.MissionCommandInput) { v.AssetID = id(10) },
		"caller employee": func(v *app.MissionCommandInput) { v.AssigneeEmployeeID = id(30) },
		"no mission":      func(v *app.MissionCommandInput) { v.MissionID = "" },
	} {
		v := good
		edit(&v)
		if _, err := app.NewMissionCommand(v); err == nil {
			t.Fatal("invalid reserve command admitted", name)
		}
	}
}
