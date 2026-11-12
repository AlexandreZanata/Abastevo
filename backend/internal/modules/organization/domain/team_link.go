package domain

import (
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"time"
)

type TeamLinkEndReason string

const (
	TeamLinkRemoved             TeamLinkEndReason = "REMOVED"
	TeamLinkEmployeeInactive    TeamLinkEndReason = "EMPLOYEE_INACTIVE"
	TeamLinkEmployeeTerminated  TeamLinkEndReason = "EMPLOYEE_TERMINATED"
	TeamLinkMembershipSuspended TeamLinkEndReason = "MEMBERSHIP_SUSPENDED"
	TeamLinkMembershipEnded     TeamLinkEndReason = "MEMBERSHIP_ENDED"
)

func validTeamLinkEnd(v TeamLinkEndReason) bool {
	switch v {
	case TeamLinkRemoved, TeamLinkEmployeeInactive, TeamLinkEmployeeTerminated, TeamLinkMembershipSuspended, TeamLinkMembershipEnded:
		return true
	}
	return false
}

type TeamLinkSnapshot struct {
	privateTeam
	OrgID                  kernel.OrgID
	ID, TeamID, EmployeeID string
	StartedAt, EndedAt     time.Time
	EndReason              TeamLinkEndReason
}
type TeamLink struct {
	privateTeam
	value TeamLinkSnapshot
}

func NewTeamLink(scope kernel.OrgScope, id string, team Team, employee TeamEmployee, at time.Time) (TeamLink, error) {
	v, err := team.Snapshot(scope)
	if err != nil {
		return TeamLink{}, err
	}
	if _, err := team.MemberChange(scope, v.Revision, employee, true); err != nil {
		return TeamLink{}, err
	}
	return RehydrateTeamLink(scope, TeamLinkSnapshot{OrgID: scope.OrgID(), ID: id, TeamID: v.ID, EmployeeID: employee.ID, StartedAt: at.UTC()})
}
func RehydrateTeamLink(scope kernel.OrgScope, v TeamLinkSnapshot) (TeamLink, error) {
	if scope.Validate() != nil || v.OrgID != scope.OrgID() {
		return TeamLink{}, ErrScope
	}
	if !canonicalLegalID(v.ID) || !canonicalLegalID(v.TeamID) || !canonicalLegalID(v.EmployeeID) || v.StartedAt.IsZero() || v.StartedAt.Year() < 1 || v.StartedAt.Year() > 9999 {
		return TeamLink{}, ErrInvalid
	}
	if v.EndedAt.IsZero() {
		if v.EndReason != "" {
			return TeamLink{}, ErrInvalid
		}
	} else if v.EndedAt.Before(v.StartedAt) || v.EndedAt.Year() > 9999 || !validTeamLinkEnd(v.EndReason) {
		return TeamLink{}, ErrInvalid
	}
	v.StartedAt = v.StartedAt.UTC()
	if !v.EndedAt.IsZero() {
		v.EndedAt = v.EndedAt.UTC()
	}
	return TeamLink{value: v}, nil
}
func (l TeamLink) Snapshot(scope kernel.OrgScope) (TeamLinkSnapshot, error) {
	v, e := RehydrateTeamLink(scope, l.value)
	return v.value, e
}
func (l TeamLink) End(scope kernel.OrgScope, at time.Time, reason TeamLinkEndReason) (TeamLink, error) {
	v, e := l.Snapshot(scope)
	if e != nil {
		return TeamLink{}, e
	}
	if !v.EndedAt.IsZero() {
		return TeamLink{}, ErrTransition
	}
	if at.IsZero() {
		return TeamLink{}, ErrInvalid
	}
	v.EndedAt = at.UTC()
	v.EndReason = reason
	return RehydrateTeamLink(scope, v)
}
