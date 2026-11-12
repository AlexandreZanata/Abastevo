package domain_test

import (
	"encoding/csv"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"os"
	"strings"
	"testing"
)

func TestLegalManagementV2GrantsOnlyTheRecordedRoles(t *testing.T) {
	for _, role := range d.SystemCatalog().Roles() {
		rule, err := d.SystemCatalog().Lookup(role.Key, "org.legal_entity.manage")
		if err != nil {
			t.Fatal(err)
		}
		expected := d.Denied
		if role.Key == d.Owner || role.Key == d.Admin {
			expected = d.AssignedScope
		}
		if rule.Coverage() != expected {
			t.Fatal("legal declaration role coverage mismatch", role.Key)
		}
	}
	_, version := d.SystemCatalog().Versions()
	if version != "org-permissions-v4" {
		t.Fatal("matrix amendment did not change its version")
	}
}

func TestCatalogV3RetainsEveryOtherV1Cell(t *testing.T) {
	raw, err := os.ReadFile("../../../../../contracts/testdata/enterprise/authorization-catalog-v1.psv")
	if err != nil {
		t.Fatal(err)
	}
	reader := csv.NewReader(strings.NewReader(string(raw)))
	reader.Comma = '|'
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	changes := 0
	for _, row := range rows[1:] {
		for i, name := range rows[0][1:] {
			expected := row[i+1]
			if row[0] == "org.legal_entity.manage" && (name == "OWNER" || name == "ADMIN") {
				expected = "O"
				changes++
			}
			if row[0] == "org.settings.read" && (name == "OWNER" || name == "ADMIN" || name == "FLEET_MANAGER" || name == "SUPERVISOR" || name == "FINANCE" || name == "APPROVER" || name == "AUDITOR") {
				expected = "O"
				changes++
			}

			rule, err := d.SystemCatalog().Lookup(d.Role(name), d.Permission(row[0]))
			if err != nil {
				t.Fatal(err)
			}
			positive := strings.SplitN(expected, ":", 2)[0]
			coverage := d.Denied
			if positive == "O" {
				coverage = d.AssignedScope
			} else if positive == "S" {
				coverage = d.OwnRecords
			}
			if rule.Coverage() != coverage {
				t.Fatal("unrelated v1 coverage changed", row[0], name)
			}
		}
	}
	if changes != 9 {
		t.Fatal("expected exactly nine explicit amendments")
	}
}
