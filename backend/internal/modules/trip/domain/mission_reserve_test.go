package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

// EMP-TRIP-023 (B-BR-EMP-414): a reservation request keeps state, assignee
// and answer at the next revision for a PLANNED, ASSIGNED, ACCEPTED or
// DECLINED mission that names a vehicle; stale revision CONFLICT first.
func TestMissionReserveKeepsTheMissionAtTheNextRevision(t *testing.T) {
	org, err := kernel.ParseOrgID("5e000000-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	ref := func(n string) string { return "5e000000-0000-4000-8000-0000000000" + n }
	mission, _ := trip.NewMissionRef(scope, ref("91"))
	unit, _ := trip.NewUnitRef(scope, ref("95"))
	asset, _ := trip.NewMissionAssetRef(scope, ref("21"))
	employee, _ := trip.NewEmployeeRef(scope, ref("31"))
	member, _ := trip.NewMembershipRef(scope, ref("41"))
	from := time.Date(2026, 11, 20, 11, 0, 0, 0, time.UTC)
	restore := func(state trip.MissionState, revision int64, vehicle bool) trip.Mission {
		t.Helper()
		s := trip.MissionSnapshot{Reference: mission, Unit: unit, Title: "Entrega", Window: trip.MissionWindow{From: from, To: from.Add(4 * time.Hour)}, State: state, Revision: revision}
		if vehicle {
			s.Asset = &asset
		}
		switch state {
		case trip.MissionAssigned:
			s.Assignee = &trip.MissionAssignee{Employee: employee, Member: member}
		case trip.MissionAccepted, trip.MissionInProgress, trip.MissionCompleted:
			s.Assignee = &trip.MissionAssignee{Employee: employee, Member: member}
			s.Answer = &trip.MissionAnswer{Kind: trip.ResponseAccept, Member: member}
		case trip.MissionDeclined:
			reason := "Ocupado"
			s.Assignee = &trip.MissionAssignee{Employee: employee, Member: member}
			s.Answer = &trip.MissionAnswer{Kind: trip.ResponseDecline, Member: member, Reason: &reason}
		}
		m, err := trip.RestoreMission(scope, s)
		if err != nil {
			t.Fatal(state, err)
		}
		return m
	}
	for _, state := range []trip.MissionState{trip.MissionPlanned, trip.MissionAssigned, trip.MissionAccepted, trip.MissionDeclined} {
		before := restore(state, 3, true)
		after, err := before.Reserve(scope, 3)
		if err != nil {
			t.Fatal(state, err)
		}
		a, b := after.Snapshot(), before.Snapshot()
		if a.Revision != 4 || a.State != state || (a.Assignee == nil) != (b.Assignee == nil) || (a.Answer == nil) != (b.Answer == nil) || a.Asset == nil || *a.Asset != asset {
			t.Fatal("reserve changed the mission", state, a)
		}
	}
	for name, c := range map[string]struct {
		m        trip.Mission
		expected int64
		want     error
	}{
		"stale":        {restore(trip.MissionAccepted, 3, true), 2, trip.ErrConflict},
		"non-positive": {restore(trip.MissionAccepted, 3, true), 0, trip.ErrInvalid},
		"draft":        {restore(trip.MissionDraft, 1, true), 1, trip.ErrInvalidTransition},
		"in progress":  {restore(trip.MissionInProgress, 4, true), 4, trip.ErrInvalidTransition},
		"completed":    {restore(trip.MissionCompleted, 5, true), 5, trip.ErrInvalidTransition},
		"cancelled":    {restore(trip.MissionCancelled, 3, true), 3, trip.ErrInvalidTransition},
		"no vehicle":   {restore(trip.MissionPlanned, 2, false), 2, trip.ErrInvalid},
		"stale wins":   {restore(trip.MissionCancelled, 3, true), 2, trip.ErrConflict},
	} {
		if _, err := c.m.Reserve(scope, c.expected); !errors.Is(err, c.want) {
			t.Fatal(name, err)
		}
	}
	other, _ := kernel.ParseOrgID("5e000000-0000-4000-8000-000000000002")
	otherScope, _ := kernel.NewOrgScope(other)
	if _, err := restore(trip.MissionAccepted, 3, true).Reserve(otherScope, 3); err == nil {
		t.Fatal("another organization's scope admitted")
	}
	if err := trip.ReservationRequestAllowed("UNKNOWN"); !errors.Is(err, trip.ErrInvalid) {
		t.Fatal("unknown state admitted", err)
	}
}
