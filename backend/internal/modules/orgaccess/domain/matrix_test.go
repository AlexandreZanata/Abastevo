package domain_test

import (
	"encoding/csv"
	"os"
	"slices"
	"strings"
	"testing"

	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

func TestCatalogMatrix(t *testing.T) {
	f, err := os.Open("../../../../../contracts/testdata/enterprise/authorization-catalog-v2.psv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	parser := csv.NewReader(f)
	parser.Comma = '|'
	rows, err := parser.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	header := []string{"permission", "OWNER", "ADMIN", "FLEET_MANAGER", "SUPERVISOR", "FINANCE", "APPROVER", "DRIVER", "OPERATIONS_STAFF", "AUDITOR", "STATION_ADMIN", "STATION_MANAGER", "STATION_ATTENDANT", "DATA_MANAGER"}
	if len(rows) != 85 || !slices.Equal(rows[0], header) {
		t.Fatal("fixture shape/header/count")
	}
	catalog := d.SystemCatalog()
	if len(catalog.Roles()) != 13 || len(catalog.Permissions()) != 84 {
		t.Fatal("system catalog missing fixed roles or keys")
	}
	seen := map[string]bool{}
	checks := 0
	for _, row := range rows[1:] {
		if len(row) != 14 || seen[row[0]] {
			t.Fatal("duplicate or malformed permission vector")
		}
		seen[row[0]] = true
		for column, role := range header[1:] {
			t.Run(row[0]+"/"+role, func(t *testing.T) {
				expected := strings.SplitN(row[column+1], ":", 2)
				rule, err := catalog.Lookup(d.Role(role), d.Permission(row[0]))
				if err != nil {
					t.Fatal(err)
				}
				coverage := d.Denied
				var requirements []string
				switch expected[0] {
				case "O":
					coverage = d.AssignedScope
				case "S":
					coverage = d.OwnRecords
				case "-":
				default:
					t.Fatal("unknown vector coverage")
				}
				if coverage != d.Denied {
					requirements = []string{"ACTIVE_MEMBERSHIP", "LIVE_SCOPED_ASSIGNMENT", "ORGANIZATION_AND_PLAN", "RECORD_RULES"}
				}
				if len(expected) == 2 {
					requirements = append(requirements, strings.Split(expected[1], ",")...)
				}
				slices.Sort(requirements)
				var actual []string
				for _, requirement := range rule.Requirements() {
					actual = append(actual, string(requirement))
				}
				if rule.Role() != d.Role(role) || rule.Permission() != d.Permission(row[0]) || rule.Coverage() != coverage || !slices.Equal(actual, requirements) {
					t.Fatalf("wrong descriptor coverage=%d requirements=%v", rule.Coverage(), actual)
				}
			})
			checks++
		}
	}
	if checks != 1092 {
		t.Fatal("incomplete role/permission cross product")
	}
}
