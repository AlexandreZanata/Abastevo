package domain_test

import (
	"testing"
	"time"

	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

// EMP-FLEET-006 (B-BR-EMP-246): only a current SELF grant with a positive
// rule for the permission makes an assignee lookup necessary.
func TestSelfCandidateOnlyForACurrentPositiveSelfGrant(t *testing.T) {
	s := assignmentScope(t, "7b0e2a54-0000-4000-8000-000000000301")
	self := structuralAssignment(t, d.Driver, d.SelfScope, "")
	for _, tc := range []struct {
		name        string
		assignments []d.Assignment
		permission  d.Permission
		at          time.Time
		want        bool
	}{
		{"driver self fleet read", []d.Assignment{self}, "fleet.asset.read", structuralNow, true},
		{"operations staff self fleet read", []d.Assignment{structuralAssignment(t, d.OperationsStaff, d.SelfScope, "")}, "fleet.asset.read", structuralNow, true},
		{"self grant without the permission", []d.Assignment{self}, "fleet.asset.manage", structuralNow, false},
		{"self grant without assign", []d.Assignment{self}, "fleet.asset.assign", structuralNow, false},
		{"organization only", []d.Assignment{structuralAssignment(t, d.FleetManager, d.OrgScope, "")}, "fleet.asset.read", structuralNow, false},
		{"unit only", []d.Assignment{structuralAssignment(t, d.Supervisor, d.UnitScope, unitNorte)}, "fleet.asset.read", structuralNow, false},
		{"not yet valid", []d.Assignment{self}, "fleet.asset.read", structuralNow.Add(-2 * time.Hour), false},
		{"supervisor self assign", []d.Assignment{structuralAssignment(t, d.Supervisor, d.SelfScope, "")}, "fleet.asset.assign", structuralNow, true},
	} {
		got, err := d.SelfCandidate(s, structuralMember, tc.assignments, tc.permission, tc.at)
		if err != nil || got != tc.want {
			t.Fatal(tc.name, got, err)
		}
	}
	if got, err := d.SelfCandidate(s, structuralOther, []d.Assignment{self}, "fleet.asset.read", structuralNow); err != nil || got {
		t.Fatal("another member's SELF grant considered", got, err)
	}
	if _, err := d.SelfCandidate(s, structuralMember, []d.Assignment{self}, "unknown.key.value", structuralNow); err == nil {
		t.Fatal("unknown permission accepted")
	}
	if _, err := d.SelfCandidate(s, "bad", []d.Assignment{self}, "fleet.asset.read", structuralNow); err == nil {
		t.Fatal("invalid member accepted")
	}
	if _, err := d.SelfCandidate(s, structuralMember, []d.Assignment{self}, "fleet.asset.read", time.Time{}); err == nil {
		t.Fatal("zero instant accepted")
	}
}

// The assignee fact reaches coverage only through AssignedTo: a SELF fleet
// read covers the record exactly when it names the member.
func TestSelfFleetReadCoversOnlyARecordAssignedToTheMember(t *testing.T) {
	s := assignmentScope(t, "7b0e2a54-0000-4000-8000-000000000301")
	self := structuralAssignment(t, d.Driver, d.SelfScope, "")
	facts := d.ResourceFacts{OrgID: s.OrgID(), ID: vehicleV}
	if _, err := d.CoveringRules(s, structuralMember, []d.Assignment{self}, "fleet.asset.read", facts, structuralNow); err == nil {
		t.Fatal("unassigned asset covered by SELF")
	}
	facts.AssignedTo = structuralMember
	if _, err := d.CoveringRules(s, structuralMember, []d.Assignment{self}, "fleet.asset.read", facts, structuralNow); err != nil {
		t.Fatal("assigned asset refused to SELF", err)
	}
	facts.AssignedTo = structuralOther
	if _, err := d.CoveringRules(s, structuralMember, []d.Assignment{self}, "fleet.asset.read", facts, structuralNow); err == nil {
		t.Fatal("another member's assignment covered")
	}
}
