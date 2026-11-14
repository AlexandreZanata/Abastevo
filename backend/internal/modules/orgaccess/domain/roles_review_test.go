package domain_test

import (
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"testing"
)

func TestRoleGrantCannotInventFinancialAuthority(t *testing.T) {
	c := d.SystemCatalog()
	if d.CanAssignRole(c, []d.Role{d.Admin}, d.Finance) {
		t.Fatal("ADMIN lacking fin.payment.record granted FINANCE")
	}
	if !d.CanAssignRole(c, []d.Role{d.Admin, d.Finance}, d.Finance) {
		t.Fatal("effective role union not honored")
	}
}

func TestRoleGrantPreservesCoverageAndSourceGaps(t *testing.T) {
	c := d.SystemCatalog()
	for _, role := range []d.Role{d.FleetManager, d.Supervisor, d.Finance} {
		if d.CanAssignScopedRole(c, []d.Role{d.Owner}, role, d.OrgScope) {
			t.Fatal("OWNER implicitly widened a frozen own-record descriptor", role)
		}
		if !d.CanAssignScopedRole(c, []d.Role{d.Owner}, role, d.SelfScope) {
			t.Fatal("valid SELF narrowing rejected", role)
		}
	}
	if d.CanAssignScopedRole(c, []d.Role{d.Admin}, d.Finance, d.SelfScope) {
		t.Fatal("SELF narrowed coverage but invented missing financial permission")
	}
	if !d.PermissionsWithin(c, []d.Role{d.Finance}, d.Finance, true) {
		t.Fatal("self-reduction wrongly requires grant authority on the assignment reduced")
	}
	if d.PermissionsWithin(c, []d.Role{d.Driver}, d.Supervisor, true) {
		t.Fatal("role restrictions or missing permission removed")
	}
	if d.CanAssignScopedRole(c, []d.Role{d.Owner}, d.Admin, "UNIT") {
		t.Fatal("AUTH006 scope silently admitted in AUTH005")
	}
}
