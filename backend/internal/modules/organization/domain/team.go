package domain

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

type privateTeam struct{}

func (privateTeam) Format(f fmt.State, _ rune)   { redact(f, "Team(redacted)") }
func (privateTeam) MarshalJSON() ([]byte, error) { return json.Marshal("Team(redacted)") }
func (privateTeam) LogValue() slog.Value         { return slog.StringValue("Team(redacted)") }

type TeamState string

const (
	TeamActive   TeamState = "ACTIVE"
	TeamArchived TeamState = "ARCHIVED"
)

type TeamProfile struct {
	privateTeam
	Name, UnitID string
}
type TeamSnapshot struct {
	privateTeam
	OrgID kernel.OrgID
	ID    string
	TeamProfile
	State    TeamState
	Revision int64
}
type Team struct {
	privateTeam
	value TeamSnapshot
}

func CanonicalTeamProfile(v TeamProfile) (TeamProfile, error) {
	name, err := legalName(v.Name, true)
	if err != nil {
		return TeamProfile{}, ErrInvalid
	}
	if v.UnitID != "" && !canonicalLegalID(v.UnitID) {
		return TeamProfile{}, ErrInvalid
	}
	return TeamProfile{Name: name, UnitID: v.UnitID}, nil
}
func NewTeam(scope kernel.OrgScope, id string, p TeamProfile) (Team, error) {
	if scope.Validate() != nil {
		return Team{}, ErrScope
	}
	if !canonicalLegalID(id) {
		return Team{}, ErrInvalid
	}
	profile, err := CanonicalTeamProfile(p)
	if err != nil {
		return Team{}, err
	}
	return Team{value: TeamSnapshot{OrgID: scope.OrgID(), ID: id, TeamProfile: profile, State: TeamActive, Revision: 1}}, nil
}
func RehydrateTeam(scope kernel.OrgScope, v TeamSnapshot) (Team, error) {
	if scope.Validate() != nil || v.OrgID != scope.OrgID() {
		return Team{}, ErrScope
	}
	if !canonicalLegalID(v.ID) || v.Revision < 1 || v.State != TeamActive && v.State != TeamArchived {
		return Team{}, ErrInvalid
	}
	p, err := CanonicalTeamProfile(v.TeamProfile)
	if err != nil || p != v.TeamProfile {
		return Team{}, ErrInvalid
	}
	return Team{value: v}, nil
}
func (c Team) Snapshot(scope kernel.OrgScope) (TeamSnapshot, error) {
	if _, err := RehydrateTeam(scope, c.value); err != nil {
		return TeamSnapshot{}, err
	}
	return c.value, nil
}
func (c Team) next(scope kernel.OrgScope, expected int64) (TeamSnapshot, error) {
	v, err := c.Snapshot(scope)
	if err != nil {
		return TeamSnapshot{}, err
	}
	if expected < 1 || expected != v.Revision {
		return TeamSnapshot{}, ErrConflict
	}
	if v.Revision == math.MaxInt64 {
		return TeamSnapshot{}, ErrRevision
	}
	v.Revision++
	return v, nil
}

func (c Team) Change(scope kernel.OrgScope, expected int64, p TeamProfile) (Team, error) {
	v, err := c.next(scope, expected)
	if err != nil {
		return Team{}, err
	}
	profile, err := CanonicalTeamProfile(p)
	if err != nil {
		return Team{}, err
	}
	v.TeamProfile = profile
	return Team{value: v}, nil
}
func (c Team) Archive(scope kernel.OrgScope, expected int64, usage StructureUsage) (Team, error) {
	v, err := c.next(scope, expected)
	if err != nil {
		return Team{}, err
	}
	if v.State != TeamActive {
		return Team{}, ErrTransition
	}
	if usage.Validate() != nil {
		return Team{}, ErrConditions
	}
	if usage.InUse() {
		return Team{}, ErrStructureInUse
	}
	v.State = TeamArchived
	return Team{value: v}, nil
}
func (c Team) Restore(scope kernel.OrgScope, expected int64) (Team, error) {
	v, err := c.next(scope, expected)
	if err != nil {
		return Team{}, err
	}
	if v.State != TeamArchived {
		return Team{}, ErrTransition
	}
	v.State = TeamActive
	return Team{value: v}, nil
}

// TeamEmployee is a scoped state fact resolved by the owning repository in the
// current transaction. It is never decoded as a client permission assertion.
type TeamEmployee struct {
	OrgID  kernel.OrgID
	ID     string
	Active bool
}

func (c Team) MemberChange(scope kernel.OrgScope, expected int64, employee TeamEmployee, adding bool) (Team, error) {
	v, err := c.next(scope, expected)
	if err != nil {
		return Team{}, err
	}
	if employee.OrgID != scope.OrgID() {
		return Team{}, ErrScope
	}
	if !canonicalLegalID(employee.ID) {
		return Team{}, ErrInvalid
	}
	if adding {
		if v.State != TeamActive {
			return Team{}, ErrTransition
		}
		if !employee.Active {
			return Team{}, ErrConditions
		}
	}
	return Team{value: v}, nil
}
