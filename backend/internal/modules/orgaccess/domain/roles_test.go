package domain_test

import (
	"testing"

	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

func TestCanAssignRoleEscalation(t *testing.T) {
	catalog := d.SystemCatalog()

	// Owner can assign any valid role in F1
	for _, role := range []d.Role{d.Owner, d.Admin, d.FleetManager, d.Supervisor, d.Approver, d.Driver, d.OperationsStaff, d.Auditor, d.DataManager, d.Finance} {
		if !d.CanAssignRole(catalog, []d.Role{d.Owner}, role) {
			t.Errorf("expected Owner to be able to assign %s", role)
		}
	}

	// Admin CANNOT assign Owner (BR-AUTH-007 and footnote 1)
	if d.CanAssignRole(catalog, []d.Role{d.Admin}, d.Owner) {
		t.Error("expected Admin NOT to be able to assign Owner")
	}

	// Admin CAN assign other system roles
	for _, role := range []d.Role{d.Admin, d.FleetManager, d.Supervisor, d.Finance, d.Approver, d.Driver, d.OperationsStaff, d.Auditor, d.DataManager} {
		if !d.CanAssignRole(catalog, []d.Role{d.Admin}, role) {
			t.Errorf("expected Admin to be able to assign %s", role)
		}
	}

	// Non-assigning roles cannot assign anything
	for _, nonAssigner := range []d.Role{d.Driver, d.Approver, d.Auditor, d.FleetManager, d.Finance, d.Supervisor, d.OperationsStaff, d.DataManager} {
		for _, target := range []d.Role{d.Driver, d.Admin, d.Owner} {
			if d.CanAssignRole(catalog, []d.Role{nonAssigner}, target) {
				t.Errorf("expected %s NOT to be able to assign %s", nonAssigner, target)
			}
		}
	}

	// Station roles are not assignable in F1
	for _, stationRole := range []d.Role{d.StationAdmin, d.StationManager, d.StationAttendant} {
		if d.CanAssignRole(catalog, []d.Role{d.Owner}, stationRole) {
			t.Errorf("expected station role %s NOT to be assignable in F1", stationRole)
		}
		if d.CanAssignRole(catalog, []d.Role{d.Admin}, stationRole) {
			t.Errorf("expected station role %s NOT to be assignable by admin in F1", stationRole)
		}
	}

	// Unknown role returns false
	if d.CanAssignRole(catalog, []d.Role{d.Owner}, "UNKNOWN_ROLE") {
		t.Error("expected unknown role NOT to be assignable")
	}
}

func TestCanModifyAssignmentSelf(t *testing.T) {
	actorID := "7b0e2a54-0000-4000-8000-000000000101"
	otherID := "7b0e2a54-0000-4000-8000-000000000102"

	// Modifying other member's assignment is allowed (escalation checked separately)
	if !d.CanModifyAssignment(actorID, otherID, []d.Role{d.Admin}, false) {
		t.Error("expected Admin to be able to modify another member's assignment")
	}
	if !d.CanModifyAssignment(actorID, otherID, []d.Role{d.Owner}, true) {
		t.Error("expected Owner to be able to end another member's assignment")
	}

	// Modifying own assignment:
	// 1. Granting/changing to self is ALWAYS forbidden (nobody assigns to self)
	if d.CanModifyAssignment(actorID, actorID, []d.Role{d.Owner}, false) {
		t.Error("expected Owner NOT to be able to grant/change role to self")
	}
	if d.CanModifyAssignment(actorID, actorID, []d.Role{d.Admin}, false) {
		t.Error("expected Admin NOT to be able to grant/change role to self")
	}

	// 2. Ending own assignment:
	// Non-owner cannot end own assignment
	if d.CanModifyAssignment(actorID, actorID, []d.Role{d.Admin}, true) {
		t.Error("expected Admin NOT to be able to end own assignment")
	}
	if d.CanModifyAssignment(actorID, actorID, []d.Role{d.Driver}, true) {
		t.Error("expected Driver NOT to be able to end own assignment")
	}

	// Only Owner reducing own assignment is allowed
	if !d.CanModifyAssignment(actorID, actorID, []d.Role{d.Owner}, true) {
		t.Error("expected Owner to be able to reduce own assignment")
	}
}
