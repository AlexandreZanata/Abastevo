package domain

import (
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"slices"
	"time"
)

type InvitationState string

const (
	InvitationPending    InvitationState = "PENDING"
	InvitationExpired    InvitationState = "EXPIRED"
	InvitationRevoked    InvitationState = "REVOKED"
	InvitationSuperseded InvitationState = "SUPERSEDED"
	InvitationAccepted   InvitationState = "ACCEPTED"
	InvitationDeclined   InvitationState = "DECLINED"
)

type InvitationGrant struct {
	Role  string `json:"role"`
	Scope string `json:"scope"`
}

func CanonicalInvitationGrants(input []InvitationGrant) ([]InvitationGrant, error) {
	if len(input) < 1 || len(input) > 10 {
		return nil, ErrInvalid
	}
	v := slices.Clone(input)
	for _, g := range v {
		if g.Scope != "ORG" && g.Scope != "SELF" {
			return nil, ErrInvalid
		}
		switch g.Role {
		case "OWNER":
			if g.Scope != "ORG" {
				return nil, ErrInvalid
			}
		case "ADMIN", "FLEET_MANAGER", "SUPERVISOR", "FINANCE", "APPROVER", "DRIVER", "OPERATIONS_STAFF", "AUDITOR", "DATA_MANAGER":
		default:
			return nil, ErrInvalid
		}
	}
	slices.SortFunc(v, func(a, b InvitationGrant) int {
		if a.Role < b.Role || a.Role == b.Role && a.Scope < b.Scope {
			return -1
		}
		if a == b {
			return 0
		}
		return 1
	})
	for i := 1; i < len(v); i++ {
		if v[i] == v[i-1] {
			return nil, ErrInvalid
		}
	}
	return v, nil
}

type InvitationSnapshot struct {
	privateValue
	OrgID                                                         kernel.OrgID
	ID, EmployeeID, MembershipID, IssuerMembershipID, OperationID string
	State                                                         InvitationState
	Revision, SettingsRevision                                    int64
	ValidityDays                                                  int
	FailedAttempts                                                int
	CreatedAt, ExpiresAt, ChangedAt                               time.Time
	Grants                                                        []InvitationGrant
	Cause, SourceType, SourceID                                   string
	SourceRevision                                                int64
}
type Invitation struct {
	privateValue
	value InvitationSnapshot
}

func ValidInvitationState(s InvitationState) bool {
	switch s {
	case InvitationPending, InvitationExpired, InvitationRevoked, InvitationSuperseded, InvitationAccepted, InvitationDeclined:
		return true
	}
	return false
}
func invitationInstant(at time.Time) bool {
	return !at.IsZero() && at.UTC().Year() >= 1 && at.UTC().Year() <= 9999
}
func RehydrateInvitation(scope kernel.OrgScope, v InvitationSnapshot) (Invitation, error) {
	if scope.Validate() != nil || v.OrgID != scope.OrgID() {
		return Invitation{}, ErrScope
	}
	for _, raw := range []string{v.ID, v.EmployeeID, v.MembershipID, v.IssuerMembershipID, v.OperationID} {
		id, e := canonicalID(raw)
		if e != nil || id != raw {
			return Invitation{}, ErrInvalid
		}
	}
	grants, e := CanonicalInvitationGrants(v.Grants)
	if e != nil {
		return Invitation{}, e
	}
	if v.FailedAttempts < 0 || v.FailedAttempts > 5 {
		return Invitation{}, ErrInvalid
	}
	if !ValidInvitationState(v.State) || v.Revision < 1 || v.SettingsRevision < 1 || v.ValidityDays < 1 || v.ValidityDays > 30 || !invitationInstant(v.CreatedAt) || !invitationInstant(v.ExpiresAt) || !invitationInstant(v.ChangedAt) || v.ChangedAt.Before(v.CreatedAt) || !v.ExpiresAt.Equal(v.CreatedAt.Add(time.Duration(v.ValidityDays)*24*time.Hour)) {
		return Invitation{}, ErrInvalid
	}
	if v.State == InvitationPending && (v.Revision != 1 || !v.ChangedAt.Equal(v.CreatedAt) || v.Cause != "" || v.SourceType != "" || v.SourceID != "" || v.SourceRevision != 0) {
		return Invitation{}, ErrInvalid
	}
	if v.State != InvitationPending && (v.Revision < 2 || v.Cause == "") {
		return Invitation{}, ErrInvalid
	}
	if (v.SourceID == "") != (v.SourceType == "") || (v.SourceID == "") != (v.SourceRevision == 0) {
		return Invitation{}, ErrInvalid
	}
	if v.SourceID != "" {
		id, e := canonicalID(v.SourceID)
		if e != nil || id != v.SourceID || v.SourceRevision < 1 {
			return Invitation{}, ErrInvalid
		}
		switch v.SourceType {
		case "PROFILE", "EMPLOYEE", "MEMBERSHIP", "ORGANIZATION":
		default:
			return Invitation{}, ErrInvalid
		}
	}
	if v.State == InvitationExpired && (v.Cause != "EXPIRED" || v.ChangedAt.Before(v.ExpiresAt) || v.SourceID != "") {
		return Invitation{}, ErrInvalid
	}
	if v.State == InvitationSuperseded && (v.Cause != "REPLACED" || v.SourceID != "") {
		return Invitation{}, ErrInvalid
	}
	if v.State == InvitationRevoked {
		switch v.Cause {
		case "MANAGER_REVOKED", "TOO_MANY_ATTEMPTS":
			if v.SourceID != "" {
				return Invitation{}, ErrInvalid
			}
		case "CONTACT_CHANGED":
			if v.SourceType != "PROFILE" || v.SourceID != v.EmployeeID {
				return Invitation{}, ErrInvalid
			}
		case "EMPLOYEE_UNAVAILABLE":
			if v.SourceType != "EMPLOYEE" || v.SourceID != v.EmployeeID {
				return Invitation{}, ErrInvalid
			}
		case "ISSUER_REDUCED":
			if v.SourceType != "MEMBERSHIP" || v.SourceID != v.IssuerMembershipID {
				return Invitation{}, ErrInvalid
			}
		case "RESERVATION_ENDED":
			if v.SourceType != "MEMBERSHIP" || v.SourceID != v.MembershipID {
				return Invitation{}, ErrInvalid
			}
		case "ORGANIZATION_ARCHIVED":
			if v.SourceType != "ORGANIZATION" || v.SourceID != scope.OrgID().String() {
				return Invitation{}, ErrInvalid
			}
		default:
			return Invitation{}, ErrInvalid
		}
	}
	if v.State == InvitationAccepted && (v.Cause != "PERSON_ACCEPTED" || v.SourceID != "") {
		return Invitation{}, ErrInvalid
	}
	if v.State == InvitationDeclined && (v.Cause != "PERSON_DECLINED" || v.SourceID != "") {
		return Invitation{}, ErrInvalid
	}
	v.Grants = grants
	return Invitation{value: v}, nil
}
func (v Invitation) Snapshot(scope kernel.OrgScope) (InvitationSnapshot, error) {
	checked, e := RehydrateInvitation(scope, v.value)
	if e != nil {
		return InvitationSnapshot{}, e
	}
	return checked.value, nil
}
func (v Invitation) UsableAt(scope kernel.OrgScope, now time.Time) bool {
	s, e := v.Snapshot(scope)
	return e == nil && s.State == InvitationPending && !now.Before(s.CreatedAt) && now.Before(s.ExpiresAt)
}
