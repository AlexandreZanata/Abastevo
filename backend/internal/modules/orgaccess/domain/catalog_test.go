package domain_test

import (
	"reflect"
	"slices"
	"sync"
	"testing"

	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

func rule(t *testing.T, role d.Role, key d.Permission) d.Rule {
	t.Helper()
	v, err := d.SystemCatalog().Lookup(role, key)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func require(t *testing.T, role d.Role, key d.Permission, coverage d.Coverage, requirements ...d.Requirement) {
	t.Helper()
	v := rule(t, role, key)
	if v.Coverage() != coverage {
		t.Fatalf("%s %s coverage=%d", role, key, v.Coverage())
	}
	for _, requirement := range requirements {
		if !slices.Contains(v.Requirements(), requirement) {
			t.Fatalf("missing %s on %s %s", requirement, role, key)
		}
	}
}

func TestUnknownKeysAndZeroCatalogFailClosed(t *testing.T) {
	catalog := d.SystemCatalog()
	for _, role := range []d.Role{"", "owner", "ADMIN ", "FLEET_MGR", "OPS_STAFF", "administrator", "manager", "Premium", "ENTERPRISE", "community_moderator", "*", "sensitive unknown role"} {
		result, err := catalog.Lookup(role, "fleet.asset.read")
		if err != d.ErrUnknownRole || result.Coverage() != d.Denied {
			t.Fatal("unknown role produced descriptor", err)
		}
		if _, err := catalog.Rules(role); err != d.ErrUnknownRole {
			t.Fatal(err)
		}
	}
	for _, key := range []d.Permission{"", "fleet.asset.*", "fleet.*.read", "FLEET.asset.read", " fleet.asset.read", "fleet.asset.read ", "org.settings.delete", "stock.other", "private.secret.value"} {
		result, err := catalog.Lookup(d.Owner, key)
		if err != d.ErrUnknownPermission || result.Coverage() != d.Denied {
			t.Fatal("unknown key produced descriptor", err)
		}
	}
	var zero d.Catalog
	if r, p := zero.Versions(); r != "" || p != "" {
		t.Fatal("zero catalog claimed version")
	}
	if len(zero.Roles()) != 0 || len(zero.Permissions()) != 0 || len(zero.UnassignedPermissions()) != 0 {
		t.Fatal("zero catalog exposed source")
	}
	if v, err := zero.Lookup(d.Owner, "fleet.asset.read"); err != d.ErrUnknownRole || v.Coverage() != d.Denied {
		t.Fatal(err)
	}
	if _, err := zero.Rules(d.Owner); err != d.ErrUnknownRole {
		t.Fatal(err)
	}
	if (d.Rule{}).Coverage() != d.Denied {
		t.Fatal("zero rule grants")
	}
	if r, p := catalog.Versions(); r != "org-roles-v1" || p != "org-permissions-v1" {
		t.Fatal("unversioned catalog")
	}
}

func TestRegisteredMatrixGapsRemainDeniedWithoutInheritance(t *testing.T) {
	catalog := d.SystemCatalog()
	expected := []d.Permission{"appr.policy.manage", "auth.role.read", "budg.budget.read", "check.incident.manage", "check.template.manage", "doc.document.allocate", "doc.document.read", "exp.category.manage", "fin.payable.read", "find.finding.read", "fleet.reservation.manage", "fleet.supplier.manage", "gps.geofence.manage", "maint.plan.manage", "maint.workorder.review", "notif.rule.manage", "org.group.manage", "org.legal_entity.manage", "org.lifecycle.manage", "org.settings.read", "route.plan.manage", "stock.reconcile", "stock.tank.manage"}
	if !slices.Equal(catalog.UnassignedPermissions(), expected) {
		t.Fatal("unapproved matrix grants changed")
	}
	for _, role := range catalog.Roles() {
		for _, key := range expected {
			require(t, role.Key, key, d.Denied)
		}
	}
	require(t, d.Owner, "org.settings.manage", d.AssignedScope)
	require(t, d.Owner, "org.settings.read", d.Denied)
	require(t, d.Admin, "fin.payment.record", d.Denied)
	require(t, d.Owner, "fin.payment.record", d.AssignedScope)
	require(t, d.Finance, "fin.payment.record", d.AssignedScope)
	// Knowing the two-segment compatibility key introduces no new alias/grant.
	require(t, d.Owner, "stock.reconcile", d.Denied)
}

func TestMatrixFootnotesAndRecordRulesSurviveRoleCombinations(t *testing.T) {
	require(t, d.Admin, "auth.role.assign", d.AssignedScope, d.CannotAssignOwner, d.RecordRules)
	require(t, d.Driver, "gps.location.read_live", d.OwnRecords, d.OwnTripsOnly)
	require(t, d.Driver, "gps.track.read", d.OwnRecords, d.OwnTripsOnly)
	require(t, d.Auditor, "gps.track.read", d.AssignedScope, d.RecordedTracksOnly)
	require(t, d.Auditor, "gps.location.read_live", d.Denied)
	for _, role := range []d.Role{d.Owner, d.Admin, d.FleetManager, d.Finance} {
		require(t, role, "agree.agreement.manage", d.AssignedScope, d.FleetSideOnly)
	}
	require(t, d.StationAdmin, "agree.agreement.manage", d.AssignedScope, d.StationSideOnly, d.LiveStationAuthority)
	require(t, d.StationManager, "agree.agreement.manage", d.Denied)
	require(t, d.Owner, "trip.trip.start", d.OwnRecords, d.RecordRules)
	require(t, d.Owner, "fuel.fueling.record", d.OwnRecords, d.RecordRules)
	// Multiple descriptors retain each originating role and its restrictions.
	owner := rule(t, d.Owner, "agree.agreement.manage")
	station := rule(t, d.StationAdmin, "agree.agreement.manage")
	if owner.Role() == station.Role() || slices.Contains(owner.Requirements(), d.StationSideOnly) || slices.Contains(station.Requirements(), d.FleetSideOnly) {
		t.Fatal("party sides were flattened")
	}
	for _, role := range d.SystemCatalog().Roles() {
		rules, err := d.SystemCatalog().Rules(role.Key)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rules {
			for _, required := range []d.Requirement{d.ActiveMembership, d.LiveScopedAssignment, d.OrganizationAndPlan, d.RecordRules} {
				if !slices.Contains(r.Requirements(), required) {
					t.Fatal("role bypassed a server condition", role.Key)
				}
			}
			if r.Role() != role.Key || r.Coverage() == d.Denied {
				t.Fatal("inconsistent positive rule")
			}
		}
	}
}

func TestPrivateRolesPreserveStationAndAccountBoundaries(t *testing.T) {
	catalog := d.SystemCatalog()
	owners := 0
	for _, role := range catalog.Roles() {
		if role.OwnerAssignmentRequiresOwner {
			owners++
			if role.Key != d.Owner {
				t.Fatal("nonowner has owner assignment metadata")
			}
		}
		switch role.Key {
		case d.StationAdmin:
			if role.LegacyStationRole != "administrator" {
				t.Fatal("station administrator mapping")
			}
		case d.StationManager:
			if role.LegacyStationRole != "manager" {
				t.Fatal("station manager mapping")
			}
		default:
			if role.LegacyStationRole != "" {
				t.Fatal("unexpected representation alias")
			}
		}
	}
	if owners != 1 {
		t.Fatal("owner assignment lost owner requirement")
	}
	for _, key := range []d.Permission{"stn.profile.edit", "stn.price.publish", "stn.reply.official", "stn.team.manage", "stn.metrics.read", "agree.redemption.confirm"} {
		require(t, d.Owner, key, d.Denied)
		require(t, d.Admin, key, d.Denied)
	}
	for _, role := range []d.Role{d.StationAdmin, d.StationManager, d.StationAttendant} {
		rules, err := catalog.Rules(role)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rules {
			if !slices.Contains(r.Requirements(), d.LiveStationAuthority) {
				t.Fatal("station rule lost authority condition")
			}
		}
	}
	require(t, d.StationAttendant, "agree.redemption.confirm", d.AssignedScope, d.LiveStationAuthority)
	require(t, d.StationAttendant, "stn.profile.edit", d.Denied)
	require(t, d.DataManager, "data.key.manage", d.AssignedScope)
	require(t, d.DataManager, "stn.price.publish", d.Denied)
}

func TestCatalogViewsAreImmutableSortedAndSafeForConcurrentReaders(t *testing.T) {
	catalog := d.SystemCatalog()
	beforeRoles := catalog.Roles()
	beforeKeys := catalog.Permissions()
	beforeMissing := catalog.UnassignedPermissions()
	beforeRules, err := catalog.Rules(d.Owner)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.IsSorted(beforeKeys) || !slices.IsSorted(beforeMissing) || !slices.IsSortedFunc(beforeRoles, func(a, b d.RoleInfo) int {
		if a.Key < b.Key {
			return -1
		}
		if a.Key > b.Key {
			return 1
		}
		return 0
	}) {
		t.Fatal("nondeterministic source order")
	}
	keys := catalog.Permissions()
	keys[0] = "changed"
	roles := catalog.Roles()
	roles[0].Key = "changed"
	roles[0].LegacyStationRole = "changed"
	missing := catalog.UnassignedPermissions()
	missing[0] = "changed"
	rules, err := catalog.Rules(d.Owner)
	if err != nil {
		t.Fatal(err)
	}
	requirements := rules[0].Requirements()
	requirements[0] = "changed"
	rules[0] = d.Rule{}
	single := rule(t, d.Owner, "fuel.fueling.review")
	requirements = single.Requirements()
	requirements[0] = "changed"
	if !reflect.DeepEqual(catalog.Roles(), beforeRoles) || !slices.Equal(catalog.Permissions(), beforeKeys) || !slices.Equal(catalog.UnassignedPermissions(), beforeMissing) {
		t.Fatal("source views mutated")
	}
	now, err := catalog.Rules(d.Owner)
	if err != nil || !reflect.DeepEqual(now, beforeRules) {
		t.Fatal("rule views mutated", err)
	}
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			for _, role := range beforeRoles {
				rules, err := catalog.Rules(role.Key)
				if err != nil {
					t.Error(err)
					return
				}
				for _, r := range rules {
					got, err := catalog.Lookup(role.Key, r.Permission())
					if err != nil || !reflect.DeepEqual(got, r) {
						t.Error("concurrent descriptor changed", err)
					}
					copy := got.Requirements()
					copy[0] = "caller mutation"
				}
			}
		})
	}
	wg.Wait()
	if !reflect.DeepEqual(catalog.Roles(), beforeRoles) {
		t.Fatal("concurrent role metadata changed")
	}
}
