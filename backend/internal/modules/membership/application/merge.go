package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/jackc/pgx/v5"
	"math"
	"time"
)

func EmployeeMergePolicy(write bool) access.Policy {
	effect := access.Read
	if write {
		effect = access.ConfigurationWrite
	}
	return access.Policy{Permission: "idn.employee.manage", Capability: "organizations", Effect: effect, Fresh: true}
}

type EmployeeMergeAuthority interface {
	CheckEmployeeMergeTx(context.Context, pgx.Tx, kernel.OrgScope, EmployeeActor, string, bool) error
}
type MergeTeamPlan struct {
	TeamID                 string `json:"team_id"`
	SourceLinkID           string `json:"source_link_id"`
	ExistingSurvivorLinkID string `json:"existing_survivor_link_id"`
	TeamRevision           int64  `json:"team_revision"`
	Name                   string `json:"name"`
	Admissible             bool   `json:"admissible"`
}
type MergeTeamEffect struct {
	TeamID         string `json:"team_id"`
	SourceLinkID   string `json:"source_link_id"`
	SurvivorLinkID string `json:"survivor_link_id"`
	Disposition    string `json:"disposition"`
}

// The team owner supplies actual plans and projects only those source links.
type EmployeeMergeTeams interface {
	PreviewEmployeeMergeTx(context.Context, pgx.Tx, kernel.OrgScope, string, string) ([]MergeTeamPlan, error)
	TransferEmployeeTeamsTx(context.Context, pgx.Tx, kernel.OrgScope, string, string, string, []MergeTeamPlan) ([]MergeTeamEffect, error)
}
type EmployeeMergeView struct {
	privateRegister
	Source, Survivor         d.EmployeeRecord
	SourceLive, SurvivorLive int64
	Teams                    []MergeTeamPlan
	Digest                   string
}

func MergeReviewDigest(scope kernel.OrgScope, v EmployeeMergeView) (string, error) {
	a, e := v.Source.Snapshot(scope)
	if e != nil {
		return "", e
	}
	b, e := v.Survivor.Snapshot(scope)
	if e != nil {
		return "", e
	}
	av, e := a.Employee.Snapshot(scope)
	if e != nil {
		return "", e
	}
	bv, e := b.Employee.Snapshot(scope)
	if e != nil {
		return "", e
	}
	if len(v.Teams) > d.MaxEmployeeMergeTeams || v.Teams == nil {
		return "", d.ErrMergeConflict
	}
	last := ""
	for _, p := range v.Teams {
		if !canonicalEmployeeID(p.TeamID) || !canonicalEmployeeID(p.SourceLinkID) || p.ExistingSurvivorLinkID != "" && !canonicalEmployeeID(p.ExistingSurvivorLinkID) || p.TeamRevision < 1 || p.SourceLinkID <= last {
			return "", ErrRegisterUnavailable
		}
		last = p.SourceLinkID
	}
	raw, _ := json.Marshal([]any{1, "idn.employee.merge.review", scope.OrgID().String(), av.ID, av.Revision, av.State, av.MergedInto, a.ProfileRevision, bv.ID, bv.Revision, bv.State, bv.MergedInto, b.ProfileRevision, v.SourceLive, v.SurvivorLive, v.Teams})
	return fmt.Sprintf("%x", sha256.Sum256(raw)), nil
}

type EmployeeMergeCommand struct {
	privateRegister
	OperationID, SourceID, SurvivorID, ReviewDigest, Reason string
	SourceRevision, SurvivorRevision                        int64
	Reviewed                                                bool
}

func (c EmployeeMergeCommand) Validate() error {
	if !canonicalEmployeeID(c.OperationID) || !canonicalEmployeeID(c.SourceID) || !canonicalEmployeeID(c.SurvivorID) || c.SourceID == c.SurvivorID || c.SourceRevision < 1 || c.SourceRevision == math.MaxInt64 || c.SurvivorRevision < 1 || !offboardingDigest.MatchString(c.ReviewDigest) || !c.Reviewed {
		return d.ErrInvalid
	}
	return d.ValidateAccessChangeReason(c.Reason)
}
func (c EmployeeMergeCommand) Digest(scope kernel.OrgScope) string {
	if scope.Validate() != nil || c.Validate() != nil {
		return ""
	}
	raw, _ := json.Marshal([]any{1, "idn.employee.merge", scope.OrgID().String(), c.OperationID, c.SourceID, c.SurvivorID, c.SourceRevision, c.SurvivorRevision, c.ReviewDigest, c.Reviewed, c.Reason})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

type EmployeeMergeResult struct {
	privateRegister
	OperationID, SourceID, SurvivorID, Reason string
	SourceRevision, SurvivorRevision          int64
	SourceState, SurvivorState                d.EmployeeState
	Teams                                     []MergeTeamEffect
	OccurredAt                                time.Time
	Replayed                                  bool
}

func (r EmployeeMergeResult) Validate(c EmployeeMergeCommand) error {
	if r.OperationID != c.OperationID || r.SourceID != c.SourceID || r.SurvivorID != c.SurvivorID || r.SourceRevision != c.SourceRevision+1 || r.SurvivorRevision != c.SurvivorRevision || r.Reason != c.Reason || r.SourceState != d.EmployeeTerminated || r.OccurredAt.IsZero() || r.Teams == nil || len(r.Teams) > d.MaxEmployeeMergeTeams {
		return ErrRegisterUnavailable
	}
	switch r.SurvivorState {
	case d.EmployeeActive, d.EmployeeInactive, d.EmployeeTerminated:
	default:
		return ErrRegisterUnavailable
	}
	last := ""
	for _, p := range r.Teams {
		if !canonicalEmployeeID(p.TeamID) || !canonicalEmployeeID(p.SourceLinkID) || !canonicalEmployeeID(p.SurvivorLinkID) || p.SourceLinkID <= last || (p.Disposition != "CREATED" && p.Disposition != "EXISTING") {
			return ErrRegisterUnavailable
		}
		last = p.SourceLinkID
	}
	return nil
}

type EmployeeResolution struct {
	privateRegister
	Source, Canonical d.EmployeeRecord
	Chain             []string
}
type EmployeeMergeRepository interface {
	PreviewEmployeeMerge(context.Context, kernel.OrgScope, EmployeeActor, string, string, string) (EmployeeMergeView, error)
	MergeEmployees(context.Context, kernel.OrgScope, EmployeeActor, string, EmployeeMergeCommand) (EmployeeMergeResult, error)
	ResolveEmployee(context.Context, kernel.OrgScope, EmployeeActor, string, string) (EmployeeResolution, error)
}
type EmployeeMergeService struct {
	Enabled    bool
	Repository EmployeeMergeRepository
}

func (s EmployeeMergeService) check(ctx context.Context, scope kernel.OrgScope, a EmployeeActor, decision access.Decision, write bool) error {
	if !s.Enabled {
		return ErrRegisterDisabled
	}
	if scope.Validate() != nil {
		return d.ErrScope
	}
	p := EmployeeMergePolicy(write)
	if a.Validate() != nil || decision.Policy() != p || decision.Check(ctx, scope, a.AccountID, p.Permission, scope.OrgID().String()) != nil {
		return ErrEmployeeUnauthenticated
	}
	if s.Repository == nil {
		return ErrRegisterUnavailable
	}
	return nil
}
func (s EmployeeMergeService) Preview(ctx context.Context, scope kernel.OrgScope, a EmployeeActor, decision access.Decision, source, survivor string) (EmployeeMergeView, error) {
	if e := s.check(ctx, scope, a, decision, false); e != nil {
		return EmployeeMergeView{}, e
	}
	if !canonicalEmployeeID(source) || !canonicalEmployeeID(survivor) {
		return EmployeeMergeView{}, d.ErrInvalid
	}
	out, e := s.Repository.PreviewEmployeeMerge(ctx, scope, a, decision.Subject().MembershipID(), source, survivor)
	if e != nil {
		return EmployeeMergeView{}, e
	}
	if out.Validate(scope, source, survivor) != nil {
		return EmployeeMergeView{}, ErrRegisterUnavailable
	}
	return out, nil
}
func (s EmployeeMergeService) Merge(ctx context.Context, scope kernel.OrgScope, a EmployeeActor, decision access.Decision, c EmployeeMergeCommand) (EmployeeMergeResult, error) {
	if e := s.check(ctx, scope, a, decision, true); e != nil {
		return EmployeeMergeResult{}, e
	}
	if e := c.Validate(); e != nil {
		return EmployeeMergeResult{}, e
	}
	out, e := s.Repository.MergeEmployees(ctx, scope, a, decision.Subject().MembershipID(), c)
	if e != nil {
		return EmployeeMergeResult{}, e
	}
	if e := out.Validate(c); e != nil {
		return EmployeeMergeResult{}, e
	}
	return out, nil
}
func (s EmployeeMergeService) Resolve(ctx context.Context, scope kernel.OrgScope, a EmployeeActor, decision access.Decision, id string) (EmployeeResolution, error) {
	if e := s.check(ctx, scope, a, decision, false); e != nil {
		return EmployeeResolution{}, e
	}
	if !canonicalEmployeeID(id) {
		return EmployeeResolution{}, d.ErrInvalid
	}
	out, e := s.Repository.ResolveEmployee(ctx, scope, a, decision.Subject().MembershipID(), id)
	if e != nil {
		return EmployeeResolution{}, e
	}
	if e := out.Validate(scope, id); e != nil {
		return EmployeeResolution{}, e
	}
	return out, nil
}

func (v EmployeeMergeView) Validate(scope kernel.OrgScope, source, survivor string) error {
	a, e := v.Source.Snapshot(scope)
	if e != nil {
		return ErrRegisterUnavailable
	}
	b, e := v.Survivor.Snapshot(scope)
	if e != nil {
		return ErrRegisterUnavailable
	}
	av, e := a.Employee.Snapshot(scope)
	if e != nil {
		return ErrRegisterUnavailable
	}
	bv, e := b.Employee.Snapshot(scope)
	if e != nil {
		return ErrRegisterUnavailable
	}
	if av.ID != source || bv.ID != survivor {
		return ErrRegisterUnavailable
	}
	h, e := MergeReviewDigest(scope, v)
	if e != nil || h != v.Digest {
		return ErrRegisterUnavailable
	}
	for _, p := range v.Teams {
		if !p.Admissible {
			return ErrRegisterUnavailable
		}
	}
	if _, e := d.PlanEmployeeMerge(scope, a.Employee, b.Employee, av.Revision, bv.Revision, v.SourceLive, v.SurvivorLive, int64(len(v.Teams)), "Review eligibility"); e != nil {
		return ErrRegisterUnavailable
	}
	return nil
}
func (v EmployeeResolution) Validate(scope kernel.OrgScope, id string) error {
	a, e := v.Source.Snapshot(scope)
	if e != nil {
		return ErrRegisterUnavailable
	}
	b, e := v.Canonical.Snapshot(scope)
	if e != nil {
		return ErrRegisterUnavailable
	}
	av, e := a.Employee.Snapshot(scope)
	if e != nil {
		return ErrRegisterUnavailable
	}
	bv, e := b.Employee.Snapshot(scope)
	if e != nil {
		return ErrRegisterUnavailable
	}
	if len(v.Chain) < 1 || len(v.Chain) > d.MaxEmployeeMergeDepth+1 || av.ID != id || v.Chain[0] != id || v.Chain[len(v.Chain)-1] != bv.ID || bv.MergedInto != "" {
		return ErrRegisterUnavailable
	}
	seen := map[string]bool{}
	for _, key := range v.Chain {
		if !canonicalEmployeeID(key) || seen[key] {
			return ErrRegisterUnavailable
		}
		seen[key] = true
	}
	if len(v.Chain) == 1 && av.MergedInto != "" || len(v.Chain) > 1 && av.MergedInto != v.Chain[1] {
		return ErrRegisterUnavailable
	}
	return nil
}
