package domain_test

import (
	"encoding/csv"
	"os"
	"slices"
	"testing"

	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

// B-BR-EMP-292/293, EDR-190/191: org-permissions-v5 grants only the three
// registered-but-unassigned keys their recorded module rows. The literal map
// below is independent of both PSV files and of the compiled source.
var v5Amendments = map[string][]string{
	"fleet.reservation.manage": {"OWNER", "ADMIN", "FLEET_MANAGER", "SUPERVISOR"},
	"check.template.manage":    {"OWNER", "ADMIN", "FLEET_MANAGER"},
	"check.incident.manage":    {"OWNER", "ADMIN", "FLEET_MANAGER", "SUPERVISOR"},
}

var v6Amendments = map[string][]string{
	"gps.geofence.manage": {"OWNER", "ADMIN", "FLEET_MANAGER"},
	"route.plan.manage":   {"OWNER", "ADMIN", "FLEET_MANAGER", "SUPERVISOR"},
}

var v7Amendments = map[string][]string{
	"find.finding.read":      {"OWNER", "ADMIN", "FLEET_MANAGER", "FINANCE", "AUDITOR"},
	"maint.plan.manage":      {"OWNER", "ADMIN", "FLEET_MANAGER"},
	"maint.workorder.review": {"OWNER", "ADMIN", "FINANCE", "APPROVER"},
	"org.group.manage":       {"OWNER"},
	"stock.tank.manage":      {"OWNER", "ADMIN", "FLEET_MANAGER"},
}

func readCatalogVector(t *testing.T, version string) [][]string {
	t.Helper()
	file, err := os.Open("../../../../../contracts/testdata/enterprise/authorization-catalog-" + version + ".psv")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.Comma = '|'
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestCatalogV5RetainsEveryV4CellExceptTheRecordedGrants(t *testing.T) {
	old, current := readCatalogVector(t, "v4"), readCatalogVector(t, "v5")
	if len(old) != len(current) || len(current) != 86 || !slices.Equal(old[0], current[0]) {
		t.Fatal("v5 must keep the v4 shape: no key is added or removed")
	}
	changed := 0
	for i, row := range old {
		if row[0] != current[i][0] || len(row) != len(current[i]) {
			t.Fatal("v4 row order or width changed", i)
		}
		granted := v5Amendments[row[0]]
		for column, role := range old[0][1:] {
			want := row[column+1]
			if slices.Contains(granted, role) {
				if want != "-" {
					t.Fatal("amended cell was not unassigned in v4", row[0], role)
				}
				want = "O"
				changed++
			}
			if current[i][column+1] != want {
				t.Fatal("v5 cell differs from v4 outside the recorded amendment", row[0], role, current[i][column+1])
			}
		}
	}
	if changed != 11 {
		t.Fatal("expected exactly eleven amended cells", changed)
	}
}

func TestCatalogV5GrantsReservationAndCheckKeysOnlyToTheRecordedRoles(t *testing.T) {
	catalog := d.SystemCatalog()
	if _, version := catalog.Versions(); version != "org-permissions-v8" {
		t.Fatal("matrix amendment did not change its version", version)
	}
	for key, granted := range v5Amendments {
		for _, role := range catalog.Roles() {
			rule, err := catalog.Lookup(role.Key, d.Permission(key))
			if err != nil {
				t.Fatal(err)
			}
			want := d.Denied
			if slices.Contains(granted, string(role.Key)) {
				want = d.AssignedScope
				requirements := []d.Requirement{d.ActiveMembership, d.LiveScopedAssignment, d.OrganizationAndPlan, d.RecordRules}
				slices.Sort(requirements)
				if !slices.Equal(rule.Requirements(), requirements) {
					t.Fatal("amended grant gained or lost a restriction", key, role.Key, rule.Requirements())
				}
			}
			if rule.Coverage() != want {
				t.Fatal("amended key coverage", key, role.Key, rule.Coverage())
			}
		}
	}
	// fleet.reservation.manage takes exactly the fleet.asset.assign column
	// (fleet.md §7); check.template.manage the maint.workorder.manage column.
	for _, pair := range [][2]d.Permission{{"fleet.reservation.manage", "fleet.asset.assign"}, {"check.template.manage", "maint.workorder.manage"}} {
		for _, role := range catalog.Roles() {
			a, _ := catalog.Lookup(role.Key, pair[0])
			b, _ := catalog.Lookup(role.Key, pair[1])
			if a.Coverage() != b.Coverage() {
				t.Fatal("amended key does not follow its recorded column", pair, role.Key)
			}
		}
	}
}

func TestCatalogV6RetainsEveryV5CellExceptTheRecordedGrants(t *testing.T) {
	old, current := readCatalogVector(t, "v5"), readCatalogVector(t, "v6")
	if len(old) != len(current) || len(current) != 86 || !slices.Equal(old[0], current[0]) {
		t.Fatal("v6 must keep the v5 shape: no key is added or removed")
	}
	changed := 0
	for i, row := range old {
		if row[0] != current[i][0] || len(row) != len(current[i]) {
			t.Fatal("v5 row order or width changed", i)
		}
		granted := v6Amendments[row[0]]
		for column, role := range old[0][1:] {
			want := row[column+1]
			if slices.Contains(granted, role) {
				if want != "-" {
					t.Fatal("amended cell was not unassigned in v5", row[0], role)
				}
				want = "O"
				changed++
			}
			if current[i][column+1] != want {
				t.Fatal("v6 cell differs from v5 outside the recorded amendment", row[0], role, current[i][column+1])
			}
		}
	}
	if changed != 7 {
		t.Fatal("expected exactly seven amended cells", changed)
	}
	catalog := d.SystemCatalog()
	for key, granted := range v6Amendments {
		for _, role := range catalog.Roles() {
			rule, err := catalog.Lookup(role.Key, d.Permission(key))
			if err != nil {
				t.Fatal(err)
			}
			want := d.Denied
			if slices.Contains(granted, string(role.Key)) {
				want = d.AssignedScope
			}
			if rule.Coverage() != want {
				t.Fatal("v6 amended key coverage", key, role.Key, rule.Coverage())
			}
		}
	}
	for _, pair := range [][2]d.Permission{{"gps.geofence.manage", "check.template.manage"}, {"route.plan.manage", "trip.mission.manage"}} {
		for _, role := range catalog.Roles() {
			a, _ := catalog.Lookup(role.Key, pair[0])
			b, _ := catalog.Lookup(role.Key, pair[1])
			if a.Coverage() != b.Coverage() {
				t.Fatal("v6 amended key does not follow its recorded column", pair, role.Key)
			}
		}
	}
}

func TestCatalogV7RetainsEveryV6CellExceptTheRecordedGrants(t *testing.T) {
	old, current := readCatalogVector(t, "v6"), readCatalogVector(t, "v7")
	if len(old) != len(current) || len(current) != 86 || !slices.Equal(old[0], current[0]) {
		t.Fatal("v7 must keep the v6 shape: no key is added or removed")
	}
	changed := 0
	for i, row := range old {
		if row[0] != current[i][0] || len(row) != len(current[i]) {
			t.Fatal("v6 row order or width changed", i)
		}
		granted := v7Amendments[row[0]]
		for column, role := range old[0][1:] {
			want := row[column+1]
			if slices.Contains(granted, role) {
				if want != "-" {
					t.Fatal("amended cell was not unassigned in v6", row[0], role)
				}
				want = "O"
				changed++
			}
			if current[i][column+1] != want {
				t.Fatal("v7 cell differs from v6 outside the recorded amendment", row[0], role, current[i][column+1])
			}
		}
	}
	if changed != 16 {
		t.Fatal("expected exactly sixteen amended cells", changed)
	}
	catalog := d.SystemCatalog()
	for key, granted := range v7Amendments {
		for _, role := range catalog.Roles() {
			rule, err := catalog.Lookup(role.Key, d.Permission(key))
			if err != nil {
				t.Fatal(err)
			}
			want := d.Denied
			if slices.Contains(granted, string(role.Key)) {
				want = d.AssignedScope
			}
			if rule.Coverage() != want {
				t.Fatal("v7 amended key coverage", key, role.Key, rule.Coverage())
			}
		}
	}
}
