package domain_test

import (
	"errors"
	"testing"
	"time"

	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

// EMP-TRIP-013 (B-BR-EMP-387, EDR-250): the vehicle change is a server-only
// operation of the mission with a fixed failure order; it keeps state,
// assignee and answer, moves the revision by one, never changes its input
// and is not a lifecycle command (the table and vocabularies are unchanged).
func TestMissionVehicleChangeRulesAndOrder(t *testing.T) {
	scope := vectorScope("A")
	ref, _ := trip.NewMissionRef(scope, missionID("MI"))
	unit, _ := trip.NewUnitRef(scope, missionID("U"))
	current, _ := trip.NewMissionAssetRef(scope, fixtureID("V"))
	next, _ := trip.NewMissionAssetRef(scope, fixtureID("V2"))
	foreign, _ := trip.NewMissionAssetRef(vectorScope("B"), fixtureID("V2"))
	employee, _ := trip.NewEmployeeRef(scope, fixtureID("D"))
	member, _ := trip.NewMembershipRef(scope, fixtureID("M"))
	start := time.Date(2026, 11, 20, 11, 0, 0, 0, time.UTC)
	reason := "Veiculo em manutencao"
	stored := func(state trip.MissionState, revision int64, asset *trip.MissionAssetRef, assigned bool, answer trip.MissionResponse) trip.Mission {
		t.Helper()
		s := trip.MissionSnapshot{Reference: ref, Unit: unit, Asset: asset, Title: "Entrega", Window: trip.MissionWindow{From: start, To: start.Add(time.Hour)}, State: state, Revision: revision}
		if assigned {
			s.Assignee = &trip.MissionAssignee{Employee: employee, Member: member}
		}
		if answer != "" {
			a := trip.MissionAnswer{Kind: answer, Member: member}
			if answer == trip.ResponseDecline {
				r := "Sem disponibilidade"
				a.Reason = &r
			}
			s.Answer = &a
		}
		m, err := trip.RestoreMission(scope, s)
		if err != nil {
			t.Fatal("fixture mission", state, err)
		}
		return m
	}
	for name, c := range map[string]struct {
		mission  trip.Mission
		expected int64
	}{
		"planned-without-vehicle": {stored(trip.MissionPlanned, 2, nil, false, ""), 2},
		"planned":                 {stored(trip.MissionPlanned, 2, &current, false, ""), 2},
		"assigned":                {stored(trip.MissionAssigned, 3, &current, true, ""), 3},
		"accepted":                {stored(trip.MissionAccepted, 4, &current, true, trip.ResponseAccept), 4},
		"declined":                {stored(trip.MissionDeclined, 4, &current, true, trip.ResponseDecline), 4},
	} {
		t.Run(name, func(t *testing.T) {
			before := c.mission.Snapshot()
			changed, err := c.mission.ChangeVehicle(scope, c.expected, next, reason)
			if err != nil {
				t.Fatal(err)
			}
			got, after := changed.Snapshot(), c.mission.Snapshot()
			if got.Revision != before.Revision+1 || got.State != before.State || got.Asset == nil || *got.Asset != next {
				t.Fatal("the change did not move only the vehicle and the revision", got.State, got.Revision)
			}
			if (got.Assignee == nil) != (before.Assignee == nil) || got.Assignee != nil && *got.Assignee != *before.Assignee || (got.Answer == nil) != (before.Answer == nil) ||
				got.Unit != before.Unit || got.Title != before.Title || got.Window != before.Window {
				t.Fatal("the change touched the assignee, answer, unit, title or window")
			}
			if after.Revision != before.Revision || (after.Asset == nil) != (before.Asset == nil) || after.Asset != nil && *after.Asset != *before.Asset {
				t.Fatal("the change mutated its input mission")
			}
		})
	}
	accepted := stored(trip.MissionAccepted, 4, &current, true, trip.ResponseAccept)
	for name, c := range map[string]struct {
		mission  trip.Mission
		scope    string
		expected int64
		to       trip.MissionAssetRef
		reason   string
		want     error
	}{
		"draft":                    {stored(trip.MissionDraft, 1, &current, false, ""), "A", 1, next, reason, trip.ErrInvalidTransition},
		"draft-without-vehicle":    {stored(trip.MissionDraft, 1, nil, false, ""), "A", 1, next, reason, trip.ErrInvalidTransition},
		"in-progress":              {stored(trip.MissionInProgress, 5, &current, true, trip.ResponseAccept), "A", 5, next, reason, trip.ErrUnsupportedCommand},
		"completed":                {stored(trip.MissionCompleted, 6, &current, true, trip.ResponseAccept), "A", 6, next, reason, trip.ErrInvalidTransition},
		"cancelled":                {stored(trip.MissionCancelled, 3, &current, false, ""), "A", 3, next, reason, trip.ErrInvalidTransition},
		"same-vehicle":             {accepted, "A", 4, current, reason, trip.ErrInvalidTransition},
		"stale":                    {accepted, "A", 3, next, reason, trip.ErrConflict},
		"future":                   {accepted, "A", 5, next, reason, trip.ErrConflict},
		"expected-zero":            {accepted, "A", 0, next, reason, trip.ErrInvalid},
		"scope-other":              {accepted, "B", 4, next, reason, trip.ErrScope},
		"scope-malformed":          {accepted, "not-an-org", 4, next, reason, trip.ErrScope},
		"vehicle-other-org":        {accepted, "A", 4, foreign, reason, trip.ErrScope},
		"vehicle-unconstructed":    {accepted, "A", 4, trip.MissionAssetRef{}, reason, trip.ErrInvalid},
		"reason-blank":             {accepted, "A", 4, next, " ", trip.ErrInvalid},
		"reason-281":               {accepted, "A", 4, next, stringsOf('a', 281), trip.ErrInvalid},
		"reason-control":           {accepted, "A", 4, next, "Motivo\u0007", trip.ErrInvalid},
		"order-vehicle-before-rev": {accepted, "A", 3, foreign, reason, trip.ErrScope},
		"order-reason-before-rev":  {accepted, "A", 9, next, "", trip.ErrInvalid},
		"order-stale-before-state": {stored(trip.MissionCancelled, 3, &current, false, ""), "A", 2, next, reason, trip.ErrConflict},
		"order-state-before-same":  {stored(trip.MissionInProgress, 5, &current, true, trip.ResponseAccept), "A", 5, current, reason, trip.ErrUnsupportedCommand},
		"unconstructed-mission":    {trip.Mission{}, "A", 1, next, reason, trip.ErrInvalid},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := c.mission.ChangeVehicle(vectorScope(c.scope), c.expected, c.to, c.reason); !errors.Is(err, c.want) {
				t.Fatalf("want %v, got %v", c.want, err)
			}
		})
	}
	for _, s := range trip.MissionStates() {
		want := error(nil)
		switch s {
		case trip.MissionDraft, trip.MissionCompleted, trip.MissionCancelled:
			want = trip.ErrInvalidTransition
		case trip.MissionInProgress:
			want = trip.ErrUnsupportedCommand
		}
		if err := trip.VehicleChangeAllowed(s); !errors.Is(err, want) || want == nil && err != nil {
			t.Fatal("vehicle change states", s, err)
		}
	}
	if trip.VehicleChangeAllowed("PAUSED") == nil || len(trip.MissionCommands()) != 7 {
		t.Fatal("an unknown state admits a vehicle change or the command vocabulary grew")
	}
}

func stringsOf(r rune, n int) string {
	out := make([]rune, n)
	for i := range out {
		out[i] = r
	}
	return string(out)
}
