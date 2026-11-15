package domain_test

import (
	"slices"
	"testing"

	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

// B-BR-EMP-247, EDR-166: AUDITOR holds exactly the documented 07-permissions
// column and no write key. The literal is independent of the catalog source.
func TestAuditorHoldsExactlyTheDocumentedReadOnlySet(t *testing.T) {
	documented := []d.Permission{"audit.event.read", "doc.document.view_content", "exp.expense.read", "fleet.asset.read", "fuel.fueling.read", "gps.track.read", "idn.employee.read",
		"kpi.dashboard.read", "org.ownership.respond", "org.settings.read", "rpt.export.run", "trip.trip.read"}
	catalog := d.SystemCatalog()
	if roles, permissions := catalog.Versions(); roles != "org-roles-v1" || permissions != "org-permissions-v5" {
		t.Fatal("catalog version changed: revisit the auditor column", roles, permissions)
	}
	rules, err := catalog.Rules(d.Auditor)
	if err != nil {
		t.Fatal(err)
	}
	var held []d.Permission
	for _, rule := range rules {
		held = append(held, rule.Permission())
		if rule.Coverage() != d.AssignedScope {
			t.Fatal("auditor rule is not organization/unit-scoped", rule.Permission())
		}
		want := []d.Requirement{d.ActiveMembership, d.LiveScopedAssignment, d.OrganizationAndPlan, d.RecordRules}
		if rule.Permission() == "gps.track.read" {
			want = append(want, d.RecordedTracksOnly)
		}
		got := rule.Requirements()
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Fatal("auditor rule restrictions drifted", rule.Permission(), got)
		}
	}
	if !slices.Equal(held, documented) {
		t.Fatal("auditor permissions differ from the documented column", held)
	}
	// Every other key, including every write and live-location key, is denied.
	for _, key := range catalog.Permissions() {
		rule, err := catalog.Lookup(d.Auditor, key)
		if err != nil {
			t.Fatal(err)
		}
		if (rule.Coverage() != d.Denied) != slices.Contains(documented, key) {
			t.Fatal("auditor coverage outside the documented column", key)
		}
	}
	if live, _ := catalog.Lookup(d.Auditor, "gps.location.read_live"); live.Coverage() != d.Denied {
		t.Fatal("auditor sees live positions")
	}
}
