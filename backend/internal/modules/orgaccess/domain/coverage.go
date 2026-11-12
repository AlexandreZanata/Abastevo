package domain

import (
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

var (
	ErrPermissionDenied = errors.New("orgaccess: permission denied")
	ErrResourceNotFound = errors.New("orgaccess: resource not found")
)

// PermissionDenial exposes only a source-registered key, never a record ID.
type PermissionDenial struct{ permission Permission }

func (e *PermissionDenial) Error() string { return ErrPermissionDenied.Error() }
func (e *PermissionDenial) Unwrap() error { return ErrPermissionDenied }
func (e *PermissionDenial) Permission() Permission {
	if e == nil {
		return ""
	}
	return e.permission
}

// DenyPermission constructs a bounded denial for a registered source key.
func DenyPermission(permission Permission) error {
	if _, err := SystemCatalog().Lookup(Owner, permission); err != nil {
		return err
	}
	return &PermissionDenial{permission: permission}
}

type AgreementParty string

const (
	FleetParty   AgreementParty = "FLEET"
	StationParty AgreementParty = "STATION"
)

// ResourceFacts come from a scoped server-owned resource resolver, never the
// request body, selected client context or cached authority. The owning record
// policy still checks lifecycle, separation of duties and other record rules.
type ResourceFacts struct {
	privateAssignment
	OrgID                                  kernel.OrgID
	ID, AuthoredBy, AssignedTo, TripDriver string
	AgreementSide                          AgreementParty
	RecordedTrack, StationAuthorityLive    bool
	RequestedRole                          Role
}

func (r ResourceFacts) Validate(scope kernel.OrgScope) error {
	if scope.Validate() != nil {
		return ErrAssignmentScope
	}
	if r.OrgID != scope.OrgID() {
		return ErrResourceNotFound
	}
	if _, err := kernel.ParseOrgID(r.ID); err != nil {
		return ErrAssignmentInvalid
	}
	for _, id := range []string{r.AuthoredBy, r.AssignedTo, r.TripDriver} {
		if id != "" {
			if _, err := kernel.ParseOrgID(id); err != nil {
				return ErrAssignmentInvalid
			}
		}
	}
	if r.AgreementSide != "" && r.AgreementSide != FleetParty && r.AgreementSide != StationParty {
		return ErrAssignmentInvalid
	}
	if r.RequestedRole != "" && !knownRole(r.RequestedRole) {
		return ErrUnknownRole
	}
	return nil
}

// CoveringRules proves only source role/period/scope restrictions over supplied
// current server facts. It never substitutes for membership/account, org/plan,
// verified proof, freshness or the owning record policy. Every returned rule
// retains its mandatory requirements; permissions do not remove restrictions.
func CoveringRules(scope kernel.OrgScope, member string, assignments []Assignment, permission Permission, target ResourceFacts, now time.Time) ([]Rule, error) {
	if err := target.Validate(scope); err != nil {
		return nil, err
	}
	if _, err := kernel.ParseOrgID(member); err != nil || now.IsZero() {
		return nil, ErrAssignmentInvalid
	}
	catalog := SystemCatalog()
	if _, err := catalog.Lookup(Owner, permission); err != nil {
		return nil, err
	}
	var result []Rule
	for _, assignment := range assignments {
		v, err := assignment.Snapshot(scope)
		if err != nil {
			return nil, err
		}
		if v.MembershipID != member {
			continue
		}
		valid, err := assignment.ValidAt(scope, now)
		if err != nil {
			return nil, err
		}
		if !valid {
			continue
		}
		rule, err := catalog.Lookup(v.Role, permission)
		if err != nil {
			return nil, err
		}
		if rule.Coverage() == Denied {
			continue
		}
		own := target.AuthoredBy == member || target.AssignedTo == member
		if (v.Scope == SelfScope || rule.Coverage() == OwnRecords) && !own {
			continue
		}
		if !restrictionsCover(rule, member, target) {
			continue
		}
		result = append(result, rule)
	}
	if len(result) == 0 {
		return nil, &PermissionDenial{permission: permission}
	}
	return result, nil
}
func restrictionsCover(rule Rule, member string, target ResourceFacts) bool {
	for _, requirement := range rule.Requirements() {
		switch requirement {
		case ActiveMembership, LiveScopedAssignment, OrganizationAndPlan, RecordRules:
			// Independent gates remain required on the returned descriptor.
		case CannotAssignOwner:
			if target.RequestedRole == "" || target.RequestedRole == Owner {
				return false
			}
		case OwnTripsOnly:
			if target.TripDriver != member {
				return false
			}
		case RecordedTracksOnly:
			if !target.RecordedTrack {
				return false
			}
		case FleetSideOnly:
			if target.AgreementSide != FleetParty {
				return false
			}
		case StationSideOnly:
			if target.AgreementSide != StationParty {
				return false
			}
		case LiveStationAuthority:
			if !target.StationAuthorityLive {
				return false
			}
		default:
			return false
		}
	}
	return true
}
