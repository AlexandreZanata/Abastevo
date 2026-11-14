package domain_test

import (
	"encoding/csv"
	"os"
	"slices"
	"strings"
	"testing"

	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

func TestOwnershipResponseV4RetainsEveryV3Cell(t *testing.T) {
	read := func(name string) [][]string {
		t.Helper()
		file, err := os.Open("../../../../../contracts/testdata/enterprise/authorization-catalog-" + name + ".psv")
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
	old, current := read("v3"), read("v4")
	if len(current) != len(old)+1 {
		t.Fatal("unexpected v4 matrix amendment")
	}
	for i, row := range old {
		if !slices.Equal(row, current[i]) {
			t.Fatal("historical v3 cell changed", i)
		}
	}
	row := current[len(current)-1]
	if row[0] != "org.ownership.respond" {
		t.Fatal("wrong participant permission")
	}
	for i, role := range current[0][1:] {
		rule, err := d.SystemCatalog().Lookup(d.Role(role), "org.ownership.respond")
		if err != nil || rule.Coverage() != d.AssignedScope || strings.SplitN(row[i+1], ":", 2)[0] != "O" {
			t.Fatal("participant response descriptor lost", role, err)
		}
	}
}
