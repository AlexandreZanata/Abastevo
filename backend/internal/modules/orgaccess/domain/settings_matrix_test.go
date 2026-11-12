package domain_test

import (
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"testing"
)

func TestSettingsReadV3GrantsOnlyTheRecordedRoles(t *testing.T) {
	allowed := map[d.Role]bool{d.Owner: true, d.Admin: true, d.FleetManager: true, d.Supervisor: true, d.Finance: true, d.Approver: true, d.Auditor: true}
	for _, role := range d.SystemCatalog().Roles() {
		r, e := d.SystemCatalog().Lookup(role.Key, "org.settings.read")
		if e != nil {
			t.Fatal(e)
		}
		want := d.Denied
		if allowed[role.Key] {
			want = d.AssignedScope
		}
		if r.Coverage() != want {
			t.Fatal("settings read row", role.Key)
		}
	}
	_, version := d.SystemCatalog().Versions()
	if version != "org-permissions-v3" {
		t.Fatal("unversioned read amendment")
	}
}
