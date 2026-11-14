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
	"regexp"
	"slices"
	"time"
)

// Manager operations retain the existing fresh read/reduction permission.
func OffboardingPolicy(write bool) access.Policy {
	if write {
		return MembershipAccessPolicy(MembershipSuspend)
	}
	return MembershipAccessPolicy(MembershipAccessRead)
}

type OffboardingAuthority interface {
	CheckOffboardingTx(context.Context, pgx.Tx, kernel.OrgScope, EmployeeActor, string, bool, bool) error
}
type OffboardingCommand struct {
	privateRegister
	OperationID, MembershipID, Reason, HandoverDigest string
	ExpectedRevision, EmployeeRevision                int64
	TerminateEmployee, Self                           bool
	Flags                                             []string
}

var offboardingDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (c OffboardingCommand) Validate() error {
	if !canonicalEmployeeID(c.OperationID) || !canonicalEmployeeID(c.MembershipID) || c.ExpectedRevision < 1 || c.ExpectedRevision == math.MaxInt64 || c.EmployeeRevision < 1 || c.EmployeeRevision == math.MaxInt64 || !offboardingDigest.MatchString(c.HandoverDigest) || c.Self && c.TerminateEmployee || len(c.Flags) > d.MaxHandoverItems || !slices.IsSorted(c.Flags) {
		return d.ErrInvalid
	}
	for i, k := range c.Flags {
		if (d.HandoverItem{Key: k, Revision: 1}).Validate() != nil || i > 0 && c.Flags[i-1] == k {
			return d.ErrInvalid
		}
	}
	return d.ValidateAccessChangeReason(c.Reason)
}
func (c OffboardingCommand) Digest(scope kernel.OrgScope) string {
	if c.Validate() != nil || scope.Validate() != nil {
		return ""
	}
	raw, _ := json.Marshal([]any{1, "idn.offboarding", scope.OrgID().String(), c.OperationID, c.MembershipID, c.Reason, c.HandoverDigest, c.ExpectedRevision, c.EmployeeRevision, c.TerminateEmployee, c.Self, append([]string{}, c.Flags...)})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

type OffboardingView struct {
	privateRegister
	State            d.AccessState
	EmployeeState    d.EmployeeState
	EmployeeRevision int64
	Items            []d.HandoverItem
	Digest           string
}
type OffboardingResult struct {
	privateRegister
	View                OffboardingView
	OperationID, Reason string
	OccurredAt          time.Time
	Replayed            bool
}
type OffboardingRepository interface {
	ReadOffboarding(context.Context, kernel.OrgScope, EmployeeActor, string, string, bool) (OffboardingView, error)
	CompleteOffboarding(context.Context, kernel.OrgScope, EmployeeActor, string, OffboardingCommand) (OffboardingResult, error)
}
type OffboardingService struct {
	Enabled    bool
	Repository OffboardingRepository
}

func (s OffboardingService) check(ctx context.Context, scope kernel.OrgScope, a EmployeeActor, decision access.Decision, write bool) error {
	if !s.Enabled {
		return ErrRegisterDisabled
	}
	p := OffboardingPolicy(write)
	if scope.Validate() != nil || a.Validate() != nil || decision.Policy() != p || decision.Check(ctx, scope, a.AccountID, p.Permission, scope.OrgID().String()) != nil {
		return ErrEmployeeUnauthenticated
	}
	if s.Repository == nil {
		return ErrRegisterUnavailable
	}
	return nil
}
func (s OffboardingService) Read(ctx context.Context, scope kernel.OrgScope, a EmployeeActor, decision access.Decision, target string) (OffboardingView, error) {
	if e := s.check(ctx, scope, a, decision, false); e != nil {
		return OffboardingView{}, e
	}
	if !canonicalEmployeeID(target) {
		return OffboardingView{}, d.ErrInvalid
	}
	out, e := s.Repository.ReadOffboarding(ctx, scope, a, decision.Subject().MembershipID(), target, false)
	if e != nil {
		return OffboardingView{}, e
	}
	if out.Validate(scope) != nil || out.State.ID != target {
		return OffboardingView{}, ErrRegisterUnavailable
	}
	return out, nil
}
func (s OffboardingService) Complete(ctx context.Context, scope kernel.OrgScope, a EmployeeActor, decision access.Decision, c OffboardingCommand) (OffboardingResult, error) {
	if e := s.check(ctx, scope, a, decision, true); e != nil {
		return OffboardingResult{}, e
	}
	if c.Self {
		return OffboardingResult{}, d.ErrInvalid
	}
	if e := c.Validate(); e != nil {
		return OffboardingResult{}, e
	}
	out, e := s.Repository.CompleteOffboarding(ctx, scope, a, decision.Subject().MembershipID(), c)
	if e != nil {
		return OffboardingResult{}, e
	}
	if out.Validate(scope, c) != nil {
		return OffboardingResult{}, ErrRegisterUnavailable
	}
	return out, nil
}

func (v OffboardingView) Validate(scope kernel.OrgScope) error {
	if v.State.Validate(scope) != nil || v.State.Model != d.ExistingAccount || v.EmployeeRevision < 1 || !offboardingDigest.MatchString(v.Digest) || len(v.Items) > d.MaxHandoverItems || v.Items == nil {
		return ErrRegisterUnavailable
	}
	switch v.EmployeeState {
	case d.EmployeeActive, d.EmployeeInactive, d.EmployeeTerminated:
	default:
		return ErrRegisterUnavailable
	}
	last := ""
	for _, i := range v.Items {
		if i.Validate() != nil || i.Key <= last {
			return ErrRegisterUnavailable
		}
		last = i.Key
	}
	return nil
}
func (v OffboardingResult) Validate(scope kernel.OrgScope, c OffboardingCommand) error {
	if v.View.Validate(scope) != nil || v.View.State.ID != c.MembershipID || v.View.State.State != d.MembershipEnded || v.View.State.Revision != c.ExpectedRevision+1 || v.OperationID != c.OperationID || v.Reason != c.Reason || v.View.Digest != c.HandoverDigest || v.OccurredAt.IsZero() || len(v.View.Items) != len(c.Flags) {
		return ErrRegisterUnavailable
	}
	reason := d.Offboarded
	if c.Self {
		reason = d.Left
	}
	if v.View.State.EndReason != reason {
		return ErrRegisterUnavailable
	}
	revision := c.EmployeeRevision
	if c.TerminateEmployee {
		revision++
		if v.View.EmployeeState != d.EmployeeTerminated {
			return ErrRegisterUnavailable
		}
	}
	if v.View.EmployeeRevision != revision {
		return ErrRegisterUnavailable
	}
	for n, i := range v.View.Items {
		if i.Key != c.Flags[n] {
			return ErrRegisterUnavailable
		}
	}
	return nil
}
