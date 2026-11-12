package domain

import (
	"slices"
	"strings"
)

type sourceGroup struct {
	roles []Role
	rows  string
}
type sourceDefinition struct {
	roles       []Role
	permissions []Permission
	groups      []sourceGroup
}

func knownRole(role Role) bool {
	switch role {
	case Owner, Admin, FleetManager, Supervisor, Finance, Approver, Driver, OperationsStaff, Auditor, StationAdmin, StationManager, StationAttendant, DataManager:
		return true
	}
	return false
}
func stationRole(role Role) bool {
	return role == StationAdmin || role == StationManager || role == StationAttendant
}
func validPermission(key Permission) bool {
	// Preserve the literal published exception, without accepting new aliases.
	if key == "stock.reconcile" {
		return true
	}
	parts := strings.Split(string(key), ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" || part[0] < 'a' || part[0] > 'z' {
			return false
		}
		for _, r := range part {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
				return false
			}
		}
	}
	return true
}
func mustBuild(def sourceDefinition) Catalog {
	catalog, err := build(def)
	if err != nil {
		panic(err)
	}
	return catalog
}
func build(def sourceDefinition) (Catalog, error) {
	catalog := Catalog{roles: map[Role]RoleInfo{}, permissions: map[Permission]struct{}{}, rules: map[Role]map[Permission]Rule{}, rolesVersion: RolesVersion, permissionsVersion: PermissionsVersion}
	if len(def.roles) != 13 || len(def.permissions) == 0 || len(def.groups) == 0 {
		return Catalog{}, ErrInvalidCatalog
	}
	for _, role := range def.roles {
		if !knownRole(role) {
			return Catalog{}, ErrInvalidCatalog
		}
		if _, exists := catalog.roles[role]; exists {
			return Catalog{}, ErrInvalidCatalog
		}
		info := RoleInfo{Key: role, OwnerAssignmentRequiresOwner: role == Owner}
		switch role {
		case StationAdmin:
			info.LegacyStationRole = "administrator"
		case StationManager:
			info.LegacyStationRole = "manager"
		}
		catalog.roles[role] = info
		catalog.rules[role] = map[Permission]Rule{}
	}
	for _, permission := range def.permissions {
		if !validPermission(permission) {
			return Catalog{}, ErrInvalidCatalog
		}
		if _, exists := catalog.permissions[permission]; exists {
			return Catalog{}, ErrInvalidCatalog
		}
		catalog.permissions[permission] = struct{}{}
	}
	for _, group := range def.groups {
		seen := map[Role]bool{}
		if len(group.roles) == 0 {
			return Catalog{}, ErrInvalidCatalog
		}
		for _, role := range group.roles {
			if !knownRole(role) || seen[role] {
				return Catalog{}, ErrInvalidCatalog
			}
			seen[role] = true
		}
		for _, line := range strings.Split(strings.TrimSpace(group.rows), "\n") {
			cells := strings.Fields(line)
			if len(cells) != len(group.roles)+1 {
				return Catalog{}, ErrInvalidCatalog
			}
			permission := Permission(cells[0])
			if _, known := catalog.permissions[permission]; !known {
				return Catalog{}, ErrInvalidCatalog
			}
			for i, role := range group.roles {
				rule, err := decode(role, permission, cells[i+1])
				if err != nil {
					return Catalog{}, err
				}
				if rule.coverage == Denied {
					continue
				}
				if _, exists := catalog.rules[role][permission]; exists {
					return Catalog{}, ErrInvalidCatalog
				}
				catalog.rules[role][permission] = rule
			}
		}
	}
	for _, rules := range catalog.rules {
		if len(rules) == 0 {
			return Catalog{}, ErrInvalidCatalog
		}
	}
	return catalog, nil
}
func decode(role Role, permission Permission, cell string) (Rule, error) {
	rule := Rule{role: role, permission: permission}
	switch cell {
	case "-":
		return rule, nil
	case "O":
		rule.coverage = AssignedScope
	case "S":
		rule.coverage = OwnRecords
	case "O1":
		if role != Admin || permission != "auth.role.assign" {
			return Rule{}, ErrInvalidCatalog
		}
		rule.coverage = AssignedScope
	case "S2":
		if role != Driver || (permission != "gps.location.read_live" && permission != "gps.track.read") {
			return Rule{}, ErrInvalidCatalog
		}
		rule.coverage = OwnRecords
	case "O3":
		if role != Auditor || permission != "gps.track.read" {
			return Rule{}, ErrInvalidCatalog
		}
		rule.coverage = AssignedScope
	case "O4":
		if role != FleetManager || permission != "agree.agreement.manage" {
			return Rule{}, ErrInvalidCatalog
		}
		rule.coverage = AssignedScope
	default:
		return Rule{}, ErrInvalidCatalog
	}
	rule.requirements = []Requirement{ActiveMembership, LiveScopedAssignment, OrganizationAndPlan, RecordRules}
	if role == Admin && permission == "auth.role.assign" {
		rule.requirements = append(rule.requirements, CannotAssignOwner)
	}
	if role == Driver && (permission == "gps.location.read_live" || permission == "gps.track.read") {
		rule.requirements = append(rule.requirements, OwnTripsOnly)
	}
	if role == Auditor && permission == "gps.track.read" {
		rule.requirements = append(rule.requirements, RecordedTracksOnly)
	}
	if stationRole(role) {
		rule.requirements = append(rule.requirements, LiveStationAuthority)
	}
	if permission == "agree.agreement.manage" {
		if stationRole(role) {
			rule.requirements = append(rule.requirements, StationSideOnly)
		} else {
			rule.requirements = append(rule.requirements, FleetSideOnly)
		}
	}
	slices.Sort(rule.requirements)
	return rule, nil
}

var system = mustBuild(systemDefinition())

// These code-frozen cells are transcribed independently of the test oracle.
// A new row/grant requires the canonical matrix and catalog version to change.
func systemDefinition() sourceDefinition {
	roles := []Role{Owner, Admin, FleetManager, Supervisor, Finance, Approver, Driver, OperationsStaff, Auditor, StationAdmin, StationManager, StationAttendant, DataManager}
	keys := strings.Fields(permissionKeys)
	permissions := make([]Permission, len(keys))
	for i, key := range keys {
		permissions[i] = Permission(key)
	}
	return sourceDefinition{roles: roles, permissions: permissions, groups: []sourceGroup{
		{roles: append([]Role(nil), roles[:9]...), rows: mainMatrix},
		{roles: []Role{StationAdmin, StationManager, StationAttendant, DataManager, Owner, Admin}, rows: stationDataMatrix},
	}}
}

const permissionKeys = `
org.settings.read org.settings.manage org.structure.manage org.legal_entity.manage
org.ownership.transfer org.lifecycle.manage org.group.manage
idn.employee.read idn.employee.manage idn.invitation.manage idn.credential.manage idn.membership.manage
auth.role.read auth.role.assign auth.delegation.manage
fleet.asset.read fleet.asset.manage fleet.asset.assign fleet.reservation.manage fleet.supplier.manage
meter.reading.record meter.reading.correct trip.mission.manage trip.trip.start trip.trip.read trip.trip.close_for_other trip.trip.review
check.checklist.execute check.template.manage check.incident.report check.incident.manage route.plan.manage
gps.location.read_live gps.track.read gps.geofence.manage
fuel.fueling.record fuel.fueling.read fuel.fueling.review fuel.fueling.amend
doc.document.attach doc.document.read doc.document.view_content doc.document.confer doc.document.allocate
exp.expense.record exp.expense.read exp.expense.review exp.category.manage appr.policy.manage
budg.budget.read budg.budget.manage fin.payable.read fin.payment.record fin.period.close fin.period.reopen
maint.request.create maint.workorder.manage maint.workorder.review maint.plan.manage
stock.tank.manage stock.movement.record stock.reconcile kpi.dashboard.read find.finding.read find.finding.resolve
rpt.export.run audit.event.read import.batch.run notif.rule.manage integ.connection.manage subs.plan.read subs.plan.manage
data.key.manage data.usage.read claim.claim.submit stn.team.manage stn.profile.edit stn.price.publish stn.reply.official stn.metrics.read
agree.agreement.manage agree.authorization.issue agree.redemption.confirm back.support.approve
`

// OWNER ADMIN FLEET_MANAGER SUPERVISOR FINANCE APPROVER DRIVER OPS AUDITOR
const mainMatrix = `
idn.invitation.manage O O - - - - - - -
idn.employee.manage O O - - - - - - -
idn.employee.read O O O O O O - - O
idn.credential.manage O O - - - - - - -
idn.membership.manage O O - - - - - - -
auth.role.assign O O1 - - - - - - -
auth.delegation.manage O O O O O O - - -
org.settings.manage O O - - - - - - -
org.structure.manage O O - - - - - - -
org.legal_entity.manage O O - - - - - - -
org.ownership.transfer O - - - - - - - -
fleet.asset.manage O O O - - - - - -
fleet.asset.read O O O O O O S S O
fleet.asset.assign O O O O - - - - -
meter.reading.record O O O O - - S S -
meter.reading.correct O O O O - - - - -
trip.mission.manage O O O O - - - - -
trip.trip.start S S S S - - S - -
trip.trip.read O O O O O O S S O
trip.trip.close_for_other O O O O - - - - -
trip.trip.review O O O O - O - - -
check.checklist.execute S S S S - - S S -
check.incident.report S S S S - - S S -
gps.location.read_live O O O O - - S2 - -
gps.track.read O O O O - - S2 - O3
fuel.fueling.record S S S S - - S S -
fuel.fueling.read O O O O O O S S O
fuel.fueling.review O O O - O O - - -
fuel.fueling.amend O O O - O - - - -
doc.document.attach S S S S O - S S -
doc.document.view_content O O O - O O S S O
doc.document.confer O O - - O - - - -
exp.expense.record S S S S S - S S -
exp.expense.read O O O O O O S S O
exp.expense.review O O - - O O - - -
budg.budget.manage O O - - O - - - -
fin.payment.record O - - - O - - - -
fin.period.close O - - - O - - - -
fin.period.reopen O - - - O - - - -
maint.workorder.manage O O O - - - - - -
maint.request.create S S O O - - S S -
stock.movement.record O O O - - - - O -
kpi.dashboard.read O O O O O - - - O
find.finding.resolve O O O - O - - - -
rpt.export.run O O O - O - - - O
audit.event.read O O - - - - - - O
import.batch.run O O O - O - - - -
integ.connection.manage O O - - - - - - -
subs.plan.manage O - - - - - - - -
subs.plan.read O O - - O - - - -
back.support.approve O O - - - - - - -
claim.claim.submit O O - - - - - - -
agree.agreement.manage O O O4 - O - - - -
agree.authorization.issue O O O O - - - - -
`

// STATION_ADMIN STATION_MANAGER STATION_ATTENDANT DATA_MANAGER OWNER ADMIN
const stationDataMatrix = `
stn.profile.edit O O - - - -
stn.price.publish O O - - - -
stn.reply.official O O - - - -
stn.team.manage O - - - - -
stn.metrics.read O O - - - -
agree.agreement.manage O - - - - -
agree.redemption.confirm O O O - - -
data.key.manage - - - O O O
data.usage.read - - - O O O
`
