package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

var ErrTeamNameDuplicate = errors.New("organization: duplicate active team name")

type TeamWriteKind string

const (
	CreateTeam         TeamWriteKind = "CREATE"
	ChangeTeam         TeamWriteKind = "CHANGE"
	ArchiveTeam        TeamWriteKind = "ARCHIVE"
	RestoreTeam        TeamWriteKind = "RESTORE"
	AddTeamEmployee    TeamWriteKind = "ADD"
	RemoveTeamEmployee TeamWriteKind = "REMOVE"
)

type TeamWriteInput struct {
	privateOnboarding
	Kind                            TeamWriteKind
	OperationID, TeamID, EmployeeID string
	ExpectedRevision                int64
	Profile                         d.TeamProfile
}
type TeamWrite struct {
	privateOnboarding
	value TeamWriteInput
}

func NewTeamWrite(v TeamWriteInput) (TeamWrite, error) {
	if !canonicalID(v.OperationID) {
		return TeamWrite{}, d.ErrInvalid
	}
	switch v.Kind {
	case CreateTeam:
		if v.TeamID != "" || v.ExpectedRevision != 0 {
			return TeamWrite{}, d.ErrInvalid
		}
	case ChangeTeam, ArchiveTeam, RestoreTeam, AddTeamEmployee, RemoveTeamEmployee:
		if !canonicalID(v.TeamID) || v.ExpectedRevision < 1 || v.ExpectedRevision == math.MaxInt64 {
			return TeamWrite{}, d.ErrInvalid
		}
	default:
		return TeamWrite{}, d.ErrInvalid
	}
	if v.Kind == AddTeamEmployee || v.Kind == RemoveTeamEmployee {
		if !canonicalID(v.EmployeeID) {
			return TeamWrite{}, d.ErrInvalid
		}
	} else if v.EmployeeID != "" {
		return TeamWrite{}, d.ErrInvalid
	}

	if v.Kind == CreateTeam || v.Kind == ChangeTeam {
		p, err := d.CanonicalTeamProfile(v.Profile)
		if err != nil {
			return TeamWrite{}, err
		}
		v.Profile = p
	} else if v.Profile != (d.TeamProfile{}) {
		return TeamWrite{}, d.ErrInvalid
	}
	return TeamWrite{value: v}, nil
}
func (c TeamWrite) Snapshot() (TeamWriteInput, error) {
	v, err := NewTeamWrite(c.value)
	if err != nil {
		return TeamWriteInput{}, err
	}
	return v.value, nil
}
func (c TeamWrite) Digest(scope kernel.OrgScope) string {
	v, err := c.Snapshot()
	if err != nil || scope.Validate() != nil {
		return ""
	}
	raw, _ := json.Marshal([]any{1, "org.team.command", scope.OrgID().String(), v.Kind, v.OperationID, v.TeamID, v.ExpectedRevision, v.Profile.Name, v.Profile.UnitID, v.EmployeeID})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

type TeamResult struct {
	TeamID, LinkID string
	State          d.TeamState
	Revision       int64
}
type TeamFilter struct {
	AfterID string
	State   d.TeamState
	Limit   int
}

func (f TeamFilter) Validate() error {
	if f.Limit < 1 || f.Limit > 100 || f.AfterID != "" && !canonicalID(f.AfterID) || f.State != "" && f.State != d.TeamActive && f.State != d.TeamArchived {
		return d.ErrInvalid
	}
	return nil
}

type TeamPage struct {
	privateOnboarding
	Teams  []d.Team
	NextID string
}
type TeamRepository interface {
	WriteTeam(context.Context, kernel.OrgScope, CompanyActor, string, string, TeamWrite) (TeamResult, bool, error)
	ReadTeam(context.Context, kernel.OrgScope, CompanyActor, string, string) (d.Team, error)
	ListTeams(context.Context, kernel.OrgScope, CompanyActor, string, TeamFilter) (TeamPage, error)
	ListTeamLinks(context.Context, kernel.OrgScope, CompanyActor, string, string, TeamLinkFilter) (TeamLinkPage, error)
}
type TeamLookup interface {
	FindTeam(context.Context, kernel.OrgScope, string) (d.Team, error)
}
type TeamService struct {
	Enabled    bool
	Repository TeamRepository
	NewID      func() (string, error)
}

func (s TeamService) Write(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, decision access.Decision, command TeamWrite) (TeamResult, bool, error) {
	if !s.Enabled {
		return TeamResult{}, false, ErrSurfaceDisabled
	}
	v, err := command.Snapshot()
	if err != nil {
		return TeamResult{}, false, err
	}
	target := v.TeamID
	if v.Kind == CreateTeam {
		target = scope.OrgID().String()
	}
	if err := structureDecision(ctx, scope, actor, decision, target, true); err != nil {
		return TeamResult{}, false, err
	}
	if s.Repository == nil {
		return TeamResult{}, false, ErrUnavailable
	}
	id := ""
	if v.Kind == CreateTeam || v.Kind == AddTeamEmployee {
		if s.NewID == nil {
			return TeamResult{}, false, ErrUnavailable
		}
		id, err = s.NewID()
		if err != nil || !canonicalID(id) {
			return TeamResult{}, false, ErrUnavailable
		}
	}
	return s.Repository.WriteTeam(ctx, scope, actor, decision.Subject().MembershipID(), id, command)
}
func (s TeamService) Read(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, decision access.Decision, id string) (d.Team, error) {
	if !s.Enabled {
		return d.Team{}, ErrSurfaceDisabled
	}
	if !canonicalID(id) {
		return d.Team{}, d.ErrInvalid
	}
	if err := structureDecision(ctx, scope, actor, decision, id, false); err != nil {
		return d.Team{}, err
	}
	if s.Repository == nil {
		return d.Team{}, ErrUnavailable
	}
	return s.Repository.ReadTeam(ctx, scope, actor, decision.Subject().MembershipID(), id)
}
func (s TeamService) List(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, decision access.Decision, filter TeamFilter) (TeamPage, error) {
	if !s.Enabled {
		return TeamPage{}, ErrSurfaceDisabled
	}
	if err := filter.Validate(); err != nil {
		return TeamPage{}, err
	}
	if err := structureDecision(ctx, scope, actor, decision, scope.OrgID().String(), false); err != nil {
		return TeamPage{}, err
	}
	if s.Repository == nil {
		return TeamPage{}, ErrUnavailable
	}
	return s.Repository.ListTeams(ctx, scope, actor, decision.Subject().MembershipID(), filter)
}

// Link period pages contain scoped employee references, never account data.
type TeamLinkFilter struct {
	AfterID, State string
	Limit          int
}

func (f TeamLinkFilter) Validate() error {
	if f.Limit < 1 || f.Limit > 100 || f.AfterID != "" && !canonicalID(f.AfterID) || f.State != "" && f.State != "ACTIVE" && f.State != "ENDED" {
		return d.ErrInvalid
	}
	return nil
}

type TeamLinkPage struct {
	privateOnboarding
	Links  []d.TeamLink
	NextID string
}

func (s TeamService) Links(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, decision access.Decision, id string, f TeamLinkFilter) (TeamLinkPage, error) {
	if !s.Enabled {
		return TeamLinkPage{}, ErrSurfaceDisabled
	}
	if !canonicalID(id) || f.Validate() != nil {
		return TeamLinkPage{}, d.ErrInvalid
	}
	if err := structureDecision(ctx, scope, actor, decision, id, false); err != nil {
		return TeamLinkPage{}, err
	}
	if s.Repository == nil {
		return TeamLinkPage{}, ErrUnavailable
	}
	return s.Repository.ListTeamLinks(ctx, scope, actor, decision.Subject().MembershipID(), id, f)
}
