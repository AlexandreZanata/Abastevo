package application_test

import (
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

func testScope(t *testing.T) kernel.OrgScope {
	t.Helper()
	id, err := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

func TestRolePolicy(t *testing.T) {
	assignP := app.RolePolicy(app.RoleAssignAction)
	if assignP.Permission != "auth.role.assign" || !assignP.Fresh || assignP.Effect != app.ConfigurationWrite {
		t.Errorf("unexpected assign policy: %+v", assignP)
	}
	if err := assignP.Validate(); err != nil {
		t.Errorf("assign policy failed validation: %v", err)
	}

	endP := app.RolePolicy(app.RoleEndAction)
	if endP.Permission != "auth.role.assign" || !endP.Fresh || endP.Effect != app.ReduceAccess {
		t.Errorf("unexpected end policy: %+v", endP)
	}
	if err := endP.Validate(); err != nil {
		t.Errorf("end policy failed validation: %v", err)
	}

	readP := app.RolePolicy(app.RoleReadAction)
	if readP.Permission != "auth.role.assign" || !readP.Fresh || readP.Effect != app.Read {
		t.Errorf("unexpected read policy: %+v", readP)
	}
	if err := readP.Validate(); err != nil {
		t.Errorf("read policy failed validation: %v", err)
	}
}

func TestRoleAssignInputValidation(t *testing.T) {
	scope := testScope(t)
	valid := app.RoleAssignInput{
		OperationID:  "7b0e2a54-0000-4000-8000-000000000010",
		MembershipID: "7b0e2a54-0000-4000-8000-000000000020",
		Role:         d.Admin,
		Scope:        d.OrgScope,
		TargetID:     scope.OrgID().String(),
		Reason:       "Promoted to admin",
	}
	if err := valid.Validate(scope); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}

	digest := valid.Digest(scope)
	if len(digest) != 64 {
		t.Errorf("unexpected digest length: %d", len(digest))
	}

	// Invalid target for ORG scope
	invalidTarget := valid
	invalidTarget.TargetID = "7b0e2a54-0000-4000-8000-000000000999"
	if err := invalidTarget.Validate(scope); err == nil {
		t.Error("expected invalid target for ORG scope to fail")
	}

	// SELF scope requires target == membership
	selfScope := valid
	selfScope.Scope = d.SelfScope
	selfScope.TargetID = valid.MembershipID
	selfScope.Role = d.Driver
	if err := selfScope.Validate(scope); err != nil {
		t.Errorf("valid SELF scope rejected: %v", err)
	}

	selfScopeBadTarget := selfScope
	selfScopeBadTarget.TargetID = scope.OrgID().String()
	if err := selfScopeBadTarget.Validate(scope); err == nil {
		t.Error("expected SELF scope with wrong target to fail")
	}

	// OWNER requires ORG scope
	ownerSelf := selfScope
	ownerSelf.Role = d.Owner
	if err := ownerSelf.Validate(scope); err == nil {
		t.Error("expected OWNER with SELF scope to fail")
	}

	// Station roles rejected in F1
	stationRole := valid
	stationRole.Role = d.StationAdmin
	if err := stationRole.Validate(scope); err == nil {
		t.Error("expected StationAdmin to be rejected in F1")
	}

	// Empty reason rejected
	emptyReason := valid
	emptyReason.Reason = "   "
	if err := emptyReason.Validate(scope); err == nil {
		t.Error("expected empty reason to fail")
	}

	// Bad dates
	badDates := valid
	badDates.ValidFrom = time.Now().Add(24 * time.Hour)
	badDates.ValidTo = time.Now().Add(12 * time.Hour)
	if err := badDates.Validate(scope); err == nil {
		t.Error("expected ValidTo before ValidFrom to fail")
	}
}

func TestRoleEndInputValidation(t *testing.T) {
	scope := testScope(t)
	valid := app.RoleEndInput{
		OperationID:      "7b0e2a54-0000-4000-8000-000000000010",
		AssignmentID:     "7b0e2a54-0000-4000-8000-000000000030",
		ExpectedRevision: 1,
		Reason:           "Role no longer needed",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid end input rejected: %v", err)
	}

	digest := valid.Digest(scope)
	if len(digest) != 64 {
		t.Errorf("unexpected digest length: %d", len(digest))
	}

	// Revision < 1 rejected
	zeroRev := valid
	zeroRev.ExpectedRevision = 0
	if err := zeroRev.Validate(); err == nil {
		t.Error("expected revision 0 to fail")
	}

	// Empty reason rejected
	emptyReason := valid
	emptyReason.Reason = ""
	if err := emptyReason.Validate(); err == nil {
		t.Error("expected empty reason to fail")
	}
}

func TestRoleListFilterValidation(t *testing.T) {
	valid := app.RoleListFilter{
		State: "ACTIVE",
		Limit: 20,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid filter rejected: %v", err)
	}

	badState := valid
	badState.State = "INVALID"
	if err := badState.Validate(); err == nil {
		t.Error("expected invalid state to fail")
	}

	badLimit := valid
	badLimit.Limit = 101
	if err := badLimit.Validate(); err == nil {
		t.Error("expected limit > 100 to fail")
	}
}
