package domain_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

// EDR-228: a mission, its snapshot and inputs never print their content, and
// a snapshot is an independent copy.
func TestMissionValuesAreRedactedAndSnapshotsIndependent(t *testing.T) {
	scope := vectorScope("A")
	ref, _ := trip.NewMissionRef(scope, missionID("MI"))
	unit, _ := trip.NewUnitRef(scope, missionID("U"))
	start := time.Date(2026, 11, 20, 11, 0, 0, 0, time.UTC)
	m, err := trip.NewMission(scope, trip.MissionInput{Reference: ref, Unit: unit, Title: "Segredo sintético", Window: trip.MissionWindow{From: start, To: start.Add(time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	employee, _ := trip.NewEmployeeRef(scope, fixtureID("D"))
	member, _ := trip.NewMembershipRef(scope, fixtureID("M"))
	assigned, err := m.Assign(scope, 1, trip.MissionAssignee{Employee: employee, Member: member})
	if err != nil {
		t.Fatal(err)
	}
	reason := "Motivo privado"
	declined, err := assigned.Respond(scope, 2, member, "DECLINE", &reason)
	if err != nil {
		t.Fatal(err)
	}
	reason = "changed"
	snapshot := declined.Snapshot()
	if *snapshot.Answer.Reason != "Motivo privado" {
		t.Fatal("the answer shares the caller's reason")
	}
	*snapshot.Answer.Reason = "changed again"
	if *declined.Snapshot().Answer.Reason != "Motivo privado" {
		t.Fatal("a snapshot shares the mission's reason")
	}
	for _, v := range []any{declined, snapshot, *snapshot.Answer, *snapshot.Assignee, trip.MissionInput{Title: "Segredo sintético"}} {
		raw, _ := json.Marshal(v)
		for _, out := range []string{fmt.Sprint(v), fmt.Sprintf("%+v", v), string(raw)} {
			if strings.Contains(out, "Segredo") || strings.Contains(out, "Motivo") || strings.Contains(out, fixtureID("M")) {
				t.Fatal("mission value printed private content", out)
			}
		}
	}
	if _, err := (trip.Mission{}).Assign(scope, 1, trip.MissionAssignee{Employee: employee, Member: member}); err == nil {
		t.Fatal("an unconstructed mission accepted a command")
	}
}

// Characterization of guard branches the shared vectors reach only through
// the fixture's own parsing: raw tokens straight into the table, an answer
// with an unknown kind or another organization's member, and a malformed
// command scope.
func TestMissionGuardsBeyondTheSharedVectors(t *testing.T) {
	if _, err := trip.MissionTransition("PAUSED", trip.MissionAssign); err == nil {
		t.Fatal("unknown state token reached the table")
	}
	if _, err := trip.MissionTransition(trip.MissionAssigned, "FINISH"); err == nil {
		t.Fatal("unknown command token reached the table")
	}
	scope := vectorScope("A")
	ref, _ := trip.NewMissionRef(scope, missionID("MI"))
	unit, _ := trip.NewUnitRef(scope, missionID("U"))
	employee, _ := trip.NewEmployeeRef(scope, fixtureID("D"))
	member, _ := trip.NewMembershipRef(scope, fixtureID("M"))
	foreign, _ := trip.NewMembershipRef(vectorScope("B"), fixtureID("M"))
	start := time.Date(2026, 11, 20, 11, 0, 0, 0, time.UTC)
	base := trip.MissionSnapshot{Reference: ref, Unit: unit, Title: "Entrega", Window: trip.MissionWindow{From: start, To: start.Add(time.Hour)}, State: trip.MissionAccepted, Revision: 3,
		Assignee: &trip.MissionAssignee{Employee: employee, Member: member}}
	for name, answer := range map[string]trip.MissionAnswer{"unknown-kind": {Kind: "MAYBE", Member: member}, "foreign-member": {Kind: trip.ResponseAccept, Member: foreign}} {
		s := base
		a := answer
		s.Answer = &a
		if _, err := trip.RestoreMission(scope, s); err == nil {
			t.Fatal("invalid stored answer restored", name)
		}
	}
	draft, err := trip.NewMission(scope, trip.MissionInput{Reference: ref, Unit: unit, Title: "Entrega", Window: base.Window})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := draft.Cancel(vectorScope("not-an-org"), 1, "Motivo"); err == nil {
		t.Fatal("a malformed scope cancelled a mission")
	}
}

// EDR-246/247: a plan, its snapshot and input never print their content, a
// snapshot is independent of the caller's input and of later snapshots, and
// an unconstructed mission cannot be planned.
func TestPlanValuesAreRedactedAndSnapshotsIndependent(t *testing.T) {
	scope := vectorScope("A")
	ref, _ := trip.NewMissionRef(scope, missionID("MI"))
	unit, _ := trip.NewUnitRef(scope, missionID("U"))
	centre, _ := trip.NewCostCentreRef(scope, missionID("CC"))
	start := time.Date(2026, 11, 20, 11, 0, 0, 0, time.UTC)
	m, err := trip.NewMission(scope, trip.MissionInput{Reference: ref, Unit: unit, Title: "Entrega", Window: trip.MissionWindow{From: start, To: start.Add(time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	destination, cargo := "Destino sigiloso", "Carga sigilosa"
	in := trip.PlanInput{DestinationText: &destination, Reason: "Motivo sigiloso", CostCentre: &centre, Load: "CARGO", CargoDescription: &cargo}
	planned, plan, err := m.Plan(scope, 1, in)
	if err != nil || planned.Snapshot().State != trip.MissionPlanned || planned.Snapshot().Revision != 2 || !plan.Valid() {
		t.Fatal("plan of a draft", err)
	}
	destination, cargo = "changed", "changed"
	snapshot := plan.Snapshot()
	if *snapshot.DestinationText != "Destino sigiloso" || *snapshot.CargoDescription != "Carga sigilosa" {
		t.Fatal("the plan shares the caller's input")
	}
	*snapshot.DestinationText = "changed again"
	if *plan.Snapshot().DestinationText != "Destino sigiloso" {
		t.Fatal("a snapshot shares the plan's text")
	}
	for _, v := range []any{plan, snapshot, in, centre} {
		raw, _ := json.Marshal(v)
		for _, out := range []string{fmt.Sprint(v), fmt.Sprintf("%+v", v), string(raw)} {
			if strings.Contains(out, "sigilos") || strings.Contains(out, missionID("CC")) {
				t.Fatal("plan value printed private content", out)
			}
		}
	}
	if _, _, err := (trip.Mission{}).Plan(scope, 1, in); err == nil {
		t.Fatal("an unconstructed mission accepted a plan")
	}
	if (trip.Plan{}).Valid() {
		t.Fatal("an unconstructed plan is valid")
	}
	for _, raw := range []string{"Ligue 12345678909 hoje", "CPF 123.456.789-09.", "linha 12345678909"} {
		if trip.ValidPlanText(raw) == nil {
			t.Fatal("a CPF-shaped token passed the plan text profile", raw)
		}
	}
}
