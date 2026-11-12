package domain

import (
	"strings"
	"testing"
)

func TestInvalidDefinitionsNeverProduceAPartialCatalog(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*sourceDefinition)
	}{
		{"missing roles", func(v *sourceDefinition) { v.roles = nil }},
		{"missing registry", func(v *sourceDefinition) { v.permissions = nil }},
		{"missing groups", func(v *sourceDefinition) { v.groups = nil }},
		{"unknown role", func(v *sourceDefinition) { v.roles[0] = "CUSTOM" }},
		{"duplicate role", func(v *sourceDefinition) { v.roles[0] = v.roles[1] }},
		{"duplicate key", func(v *sourceDefinition) { v.permissions = append(v.permissions, v.permissions[0]) }},
		{"empty key", func(v *sourceDefinition) { v.permissions[0] = "" }},
		{"wildcard key", func(v *sourceDefinition) { v.permissions[0] = "org.settings.*" }},
		{"wrong segments", func(v *sourceDefinition) { v.permissions[0] = "org.settings.read.extra" }},
		{"new two segment alias", func(v *sourceDefinition) { v.permissions[0] = "stock.other" }},
		{"bad segment", func(v *sourceDefinition) { v.permissions[0] = "org..read" }},
		{"wrong initial", func(v *sourceDefinition) { v.permissions[0] = "Org.settings.read" }},
		{"invalid character", func(v *sourceDefinition) { v.permissions[0] = "org.settings.read-now" }},
		{"empty group", func(v *sourceDefinition) { v.groups[0].roles = nil }},
		{"unknown group role", func(v *sourceDefinition) { v.groups[0].roles[0] = "CUSTOM" }},
		{"duplicate group role", func(v *sourceDefinition) { v.groups[0].roles[0] = v.groups[0].roles[1] }},
		{"missing cells", func(v *sourceDefinition) { v.groups[0].rows = "idn.employee.read O" }},
		{"extra cells", func(v *sourceDefinition) { v.groups[0].rows = "idn.employee.read O O O O O O O O O O" }},
		{"unregistered reference", func(v *sourceDefinition) {
			v.groups[0].rows = strings.Replace(v.groups[0].rows, "idn.invitation.manage", "idn.unknown.manage", 1)
		}},
		{"unknown cell", func(v *sourceDefinition) {
			v.groups[0].rows = strings.Replace(v.groups[0].rows, "idn.invitation.manage O", "idn.invitation.manage ALLOW", 1)
		}},
		{"duplicate grant", func(v *sourceDefinition) { v.groups = append(v.groups, v.groups[0]) }},
		{"role without permissions", func(v *sourceDefinition) {
			for i := range v.groups {
				v.groups[i].rows = strings.ReplaceAll(v.groups[i].rows, " O", " -")
				v.groups[i].rows = strings.ReplaceAll(v.groups[i].rows, " S", " -")
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def := systemDefinition()
			tc.mutate(&def)
			catalog, err := build(def)
			if err != ErrInvalidCatalog || len(catalog.Roles()) != 0 || len(catalog.Permissions()) != 0 {
				t.Fatal("invalid source yielded catalog", err)
			}
		})
	}
}

func TestInvalidFootnotesCannotMoveToOtherRolesOrPermissions(t *testing.T) {
	for _, tc := range []struct {
		role Role
		key  Permission
		cell string
	}{
		{Owner, "auth.role.assign", "O1"}, {Admin, "fleet.asset.read", "O1"},
		{Owner, "gps.track.read", "S2"}, {Driver, "trip.trip.read", "S2"},
		{Owner, "gps.track.read", "O3"}, {Auditor, "trip.trip.read", "O3"},
		{Owner, "agree.agreement.manage", "O4"}, {FleetManager, "fleet.asset.read", "O4"},
	} {
		if _, err := decode(tc.role, tc.key, tc.cell); err != ErrInvalidCatalog {
			t.Fatal("footnote moved silently")
		}
	}
}

func TestInvalidCompiledSourceFailsInsteadOfInstallingEmptyDefaults(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != ErrInvalidCatalog {
			t.Fatal("missing invalid-source panic", recovered)
		}
	}()
	_ = mustBuild(sourceDefinition{})
}

func TestBuildCopiesItsSourceAndReturnedRules(t *testing.T) {
	def := systemDefinition()
	catalog, err := build(def)
	if err != nil {
		t.Fatal(err)
	}
	def.roles[0] = "changed"
	def.permissions[0] = "changed"
	def.groups[0].roles[0] = "changed"
	first, err := catalog.Lookup(Owner, "idn.employee.read")
	if err != nil {
		t.Fatal(err)
	}
	first.requirements[0] = "changed"
	rules, err := catalog.Rules(Owner)
	if err != nil {
		t.Fatal(err)
	}
	rules[0].requirements[0] = "changed"
	current, err := catalog.Lookup(Owner, "idn.employee.read")
	if err != nil || current.requirements[0] == "changed" {
		t.Fatal("private slice alias escaped", err)
	}
}
