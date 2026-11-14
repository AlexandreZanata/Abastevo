package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/jackc/pgx/v5"
	"math"
	"time"
)

var ErrAccessOperationReused = errors.New("membership: access operation reused")

type MembershipAccessAction string

const (
	MembershipAccessRead MembershipAccessAction = "READ"
	MembershipSuspend    MembershipAccessAction = "SUSPEND"
	MembershipReactivate MembershipAccessAction = "REACTIVATE"
)

func MembershipAccessPolicy(action MembershipAccessAction) access.Policy {
	switch action {
	case MembershipAccessRead:
		return access.Policy{Permission: "idn.membership.manage", Capability: "organizations", Effect: access.Read, Fresh: true}
	case MembershipSuspend:
		return access.Policy{Permission: "idn.membership.manage", Capability: "organizations", Effect: access.ReduceAccess, Fresh: true}
	case MembershipReactivate:
		return access.Policy{Permission: "idn.membership.manage", Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
	}
	return access.Policy{}
}

type MembershipAccessAuthority interface {
	CheckMembershipAccessTx(context.Context, pgx.Tx, kernel.OrgScope, EmployeeActor, string, MembershipAccessAction) (time.Time, error)
}

// This narrow data port supplies periods for possible reactivation, never
// authorization candidates. Ordinary AUTH candidates still require ACTIVE.
type MembershipReactivationPeriods interface {
	ReactivationCandidatesTx(context.Context, pgx.Tx, kernel.OrgScope, string) ([]permissions.Assignment, error)
}
type MembershipAccessWrite struct {
	privateRegister
	Action                            MembershipAccessAction
	OperationID, MembershipID, Reason string
	ExpectedRevision                  int64
}

func (c MembershipAccessWrite) Validate() error {
	if c.Action != MembershipSuspend && c.Action != MembershipReactivate {
		return d.ErrInvalid
	}
	if !canonicalEmployeeID(c.OperationID) || !canonicalEmployeeID(c.MembershipID) || c.ExpectedRevision < 1 || c.ExpectedRevision == math.MaxInt64 {
		return d.ErrInvalid
	}
	return d.ValidateAccessChangeReason(c.Reason)
}
func (c MembershipAccessWrite) Digest(scope kernel.OrgScope) string {
	if c.Validate() != nil || scope.Validate() != nil {
		return ""
	}
	raw, _ := json.Marshal([]any{1, "idn.membership.access", scope.OrgID().String(), c.Action, c.OperationID, c.MembershipID, c.ExpectedRevision, c.Reason})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

type MembershipAccessResult struct {
	privateRegister
	State               d.AccessState
	OperationID, Reason string
	PreviousSuspension  d.SuspensionOrigin
	OccurredAt          time.Time
	Replayed            bool
}
type MembershipAccessRepository interface {
	ChangeMembershipAccess(context.Context, kernel.OrgScope, EmployeeActor, string, MembershipAccessWrite) (MembershipAccessResult, error)
	ReadMembershipAccess(context.Context, kernel.OrgScope, EmployeeActor, string, string) (MembershipAccessResult, error)
}
type MembershipAccessService struct {
	Enabled    bool
	Repository MembershipAccessRepository
}

func (s MembershipAccessService) check(ctx context.Context, scope kernel.OrgScope, a EmployeeActor, decision access.Decision, action MembershipAccessAction) error {
	if !s.Enabled {
		return ErrRegisterDisabled
	}
	if scope.Validate() != nil {
		return d.ErrScope
	}
	p := MembershipAccessPolicy(action)
	if a.Validate() != nil || decision.Policy() != p || decision.Check(ctx, scope, a.AccountID, p.Permission, scope.OrgID().String()) != nil {
		return ErrEmployeeUnauthenticated
	}
	if s.Repository == nil {
		return ErrRegisterUnavailable
	}
	return nil
}
func (s MembershipAccessService) Change(ctx context.Context, scope kernel.OrgScope, a EmployeeActor, decision access.Decision, c MembershipAccessWrite) (MembershipAccessResult, error) {
	if e := s.check(ctx, scope, a, decision, c.Action); e != nil {
		return MembershipAccessResult{}, e
	}
	if e := c.Validate(); e != nil {
		return MembershipAccessResult{}, e
	}
	out, e := s.Repository.ChangeMembershipAccess(ctx, scope, a, decision.Subject().MembershipID(), c)
	if e != nil {
		return MembershipAccessResult{}, e
	}
	if out.State.Validate(scope) != nil || out.State.ID != c.MembershipID || out.State.Revision < 2 || out.OperationID != c.OperationID || out.Reason != c.Reason || out.OccurredAt.IsZero() {
		return MembershipAccessResult{}, ErrRegisterUnavailable
	}
	if (c.Action == MembershipSuspend && (out.State.State != d.MembershipSuspended || out.State.Suspension != d.ByManager || out.PreviousSuspension != "")) || (c.Action == MembershipReactivate && (out.State.State != d.MembershipActive || (out.PreviousSuspension != d.ByManager && out.PreviousSuspension != d.ByEmployeeInactivity))) {
		return MembershipAccessResult{}, ErrRegisterUnavailable
	}
	return out, nil
}
func (s MembershipAccessService) Read(ctx context.Context, scope kernel.OrgScope, a EmployeeActor, decision access.Decision, id string) (MembershipAccessResult, error) {
	if e := s.check(ctx, scope, a, decision, MembershipAccessRead); e != nil {
		return MembershipAccessResult{}, e
	}
	if !canonicalEmployeeID(id) {
		return MembershipAccessResult{}, d.ErrInvalid
	}
	out, e := s.Repository.ReadMembershipAccess(ctx, scope, a, decision.Subject().MembershipID(), id)
	if e != nil {
		return MembershipAccessResult{}, e
	}
	if out.State.Validate(scope) != nil || out.State.ID != id || out.OccurredAt.IsZero() {
		return MembershipAccessResult{}, ErrRegisterUnavailable
	}
	return out, nil
}
