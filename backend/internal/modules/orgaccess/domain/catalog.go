// Package domain owns fixed organization-role descriptors, never account
// authority. Current membership, assignment, scope, organization/plan and
// record policy must all be resolved by the owning server authorization path.
package domain

import (
	"errors"
	"slices"
	"sort"
)

const RolesVersion = "org-roles-v1"
const PermissionsVersion = "org-permissions-v4"

var (
	ErrUnknownRole       = errors.New("orgaccess: unknown system role")
	ErrUnknownPermission = errors.New("orgaccess: unknown permission")
	ErrInvalidCatalog    = errors.New("orgaccess: invalid source catalog")
)

type Role string

const (
	Owner            Role = "OWNER"
	Admin            Role = "ADMIN"
	FleetManager     Role = "FLEET_MANAGER"
	Supervisor       Role = "SUPERVISOR"
	Finance          Role = "FINANCE"
	Approver         Role = "APPROVER"
	Driver           Role = "DRIVER"
	OperationsStaff  Role = "OPERATIONS_STAFF"
	Auditor          Role = "AUDITOR"
	StationAdmin     Role = "STATION_ADMIN"
	StationManager   Role = "STATION_MANAGER"
	StationAttendant Role = "STATION_ATTENDANT"
	DataManager      Role = "DATA_MANAGER"
)

type Permission string
type Coverage uint8

const (
	Denied Coverage = iota
	// AssignedScope stays inside the live assignment's scope, not every org.
	AssignedScope
	// OwnRecords includes own records and assigned assets; OwnTripsOnly narrows it.
	OwnRecords
)

type Requirement string

const (
	ActiveMembership     Requirement = "ACTIVE_MEMBERSHIP"
	LiveScopedAssignment Requirement = "LIVE_SCOPED_ASSIGNMENT"
	OrganizationAndPlan  Requirement = "ORGANIZATION_AND_PLAN"
	RecordRules          Requirement = "RECORD_RULES"
	CannotAssignOwner    Requirement = "CANNOT_ASSIGN_OWNER"
	OwnTripsOnly         Requirement = "OWN_TRIPS_ONLY"
	RecordedTracksOnly   Requirement = "RECORDED_TRACKS_ONLY"
	FleetSideOnly        Requirement = "FLEET_SIDE_ONLY"
	StationSideOnly      Requirement = "STATION_SIDE_ONLY"
	LiveStationAuthority Requirement = "LIVE_STATION_AUTHORITY"
)

// RoleInfo describes platform source, not a role held by any account.
// LegacyStationRole is metadata only; aliases are not accepted role keys.
type RoleInfo struct {
	Key                          Role
	LegacyStationRole            string
	OwnerAssignmentRequiresOwner bool
}

// Rule is an immutable static descriptor. Even AssignedScope never means
// that an account is authorized. Denied is also the safe zero value.
type Rule struct {
	role         Role
	permission   Permission
	coverage     Coverage
	requirements []Requirement
}

func (r Rule) Role() Role                  { return r.role }
func (r Rule) Permission() Permission      { return r.permission }
func (r Rule) Coverage() Coverage          { return r.coverage }
func (r Rule) Requirements() []Requirement { return slices.Clone(r.requirements) }
func clone(r Rule) Rule                    { r.requirements = slices.Clone(r.requirements); return r }

type Catalog struct {
	roles                            map[Role]RoleInfo
	permissions                      map[Permission]struct{}
	rules                            map[Role]map[Permission]Rule
	rolesVersion, permissionsVersion string
}

// SystemCatalog exposes no runtime/custom registration mechanism.
func SystemCatalog() Catalog                 { return system }
func (c Catalog) Versions() (string, string) { return c.rolesVersion, c.permissionsVersion }
func (c Catalog) Roles() []RoleInfo {
	result := make([]RoleInfo, 0, len(c.roles))
	for _, info := range c.roles {
		result = append(result, info)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	return result
}
func (c Catalog) Permissions() []Permission {
	result := make([]Permission, 0, len(c.permissions))
	for key := range c.permissions {
		result = append(result, key)
	}
	slices.Sort(result)
	return result
}
func (c Catalog) UnassignedPermissions() []Permission {
	var result []Permission
	for _, key := range c.Permissions() {
		assigned := false
		for _, rules := range c.rules {
			if _, ok := rules[key]; ok {
				assigned = true
				break
			}
		}
		if !assigned {
			result = append(result, key)
		}
	}
	return result
}
func (c Catalog) Lookup(role Role, permission Permission) (Rule, error) {
	if _, ok := c.roles[role]; !ok {
		return Rule{}, ErrUnknownRole
	}
	if _, ok := c.permissions[permission]; !ok {
		return Rule{}, ErrUnknownPermission
	}
	if rule, ok := c.rules[role][permission]; ok {
		return clone(rule), nil
	}
	return Rule{role: role, permission: permission}, nil
}
func (c Catalog) Rules(role Role) ([]Rule, error) {
	if _, ok := c.roles[role]; !ok {
		return nil, ErrUnknownRole
	}
	var result []Rule
	for _, key := range c.Permissions() {
		if rule, ok := c.rules[role][key]; ok {
			result = append(result, clone(rule))
		}
	}
	return result, nil
}
