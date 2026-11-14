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

var (
	ErrInvitationConflict        = errors.New("membership: invitation conflict")
	ErrInvitationOperationReused = errors.New("membership: invitation operation reused")
	ErrEmployeeHasAccess         = errors.New("membership: employee already has access")
)

type InvitationAction string

const (
	InvitationIssue  InvitationAction = "ISSUE"
	InvitationResend InvitationAction = "RESEND"
	InvitationRevoke InvitationAction = "REVOKE"
	InvitationRead   InvitationAction = "READ"
)

func InvitationPolicy(action InvitationAction) access.Policy {
	switch action {
	case InvitationIssue, InvitationResend:
		return access.Policy{Permission: "idn.invitation.manage", Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
	case InvitationRevoke:
		return access.Policy{Permission: "idn.invitation.manage", Capability: "organizations", Effect: access.ReduceAccess, Fresh: true}
	case InvitationRead:
		return access.Policy{Permission: "idn.invitation.manage", Capability: "organizations", Effect: access.Read, Fresh: true}
	}
	return access.Policy{}
}

type InvitationAuthorityFacts struct {
	privateRegister
	Assignments      []permissions.Assignment
	EmployeeID       string
	ValidityDays     int
	SettingsRevision int64
	Now              time.Time
}
type InvitationAuthority interface {
	CheckInvitationTx(context.Context, pgx.Tx, kernel.OrgScope, EmployeeActor, string, InvitationAction) (InvitationAuthorityFacts, error)
}
type InvitationWrite struct {
	privateRegister
	Action                                InvitationAction
	OperationID, EmployeeID, InvitationID string
	ExpectedRevision                      int64
	Grants                                []d.InvitationGrant
}

func (c InvitationWrite) Canonical() (InvitationWrite, error) {
	if !canonicalEmployeeID(c.OperationID) {
		return InvitationWrite{}, d.ErrInvalid
	}
	switch c.Action {
	case InvitationIssue:
		if !canonicalEmployeeID(c.EmployeeID) || c.InvitationID != "" || c.ExpectedRevision != 0 {
			return InvitationWrite{}, d.ErrInvalid
		}
		g, e := d.CanonicalInvitationGrants(c.Grants)
		if e != nil {
			return InvitationWrite{}, e
		}
		c.Grants = g
	case InvitationResend, InvitationRevoke:
		if !canonicalEmployeeID(c.InvitationID) || c.EmployeeID != "" || c.ExpectedRevision < 1 || c.ExpectedRevision == math.MaxInt64 || len(c.Grants) != 0 {
			return InvitationWrite{}, d.ErrInvalid
		}
	default:
		return InvitationWrite{}, d.ErrInvalid
	}
	return c, nil
}
func (c InvitationWrite) Digest(scope kernel.OrgScope) string {
	v, e := c.Canonical()
	if e != nil || scope.Validate() != nil {
		return ""
	}
	raw, _ := json.Marshal([]any{1, "idn.invitation.command", scope.OrgID().String(), v.Action, v.OperationID, v.EmployeeID, v.InvitationID, v.ExpectedRevision, v.Grants})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

type InvitationPlan struct {
	privateRegister
	InvitationID, MembershipID, Verifier string
}
type InvitationSecret struct {
	privateRegister
	Code, Verifier string
}
type InvitationResult struct {
	privateRegister
	Invitation d.Invitation
	Replayed   bool
	Code       string
}
type InvitationFilter struct {
	privateRegister
	ID, EmployeeID, AfterID string
	State                   d.InvitationState
	Limit                   int
}

func (f InvitationFilter) Validate() error {
	if f.Limit < 1 || f.Limit > 100 {
		return d.ErrInvalid
	}
	for _, id := range []string{f.ID, f.EmployeeID, f.AfterID} {
		if id != "" && !canonicalEmployeeID(id) {
			return d.ErrInvalid
		}
	}
	if f.State != "" && !d.ValidInvitationState(f.State) {
		return d.ErrInvalid
	}
	if f.ID != "" && (f.AfterID != "" || f.EmployeeID != "" || f.State != "") {
		return d.ErrInvalid
	}
	return nil
}

type InvitationPage struct {
	privateRegister
	Invitations []d.Invitation
	NextID      string
}
type InvitationRepository interface {
	WriteInvitation(context.Context, kernel.OrgScope, EmployeeActor, string, InvitationWrite, InvitationPlan) (d.Invitation, bool, error)
	ListInvitations(context.Context, kernel.OrgScope, EmployeeActor, string, InvitationFilter) (InvitationPage, error)
}
type InvitationService struct {
	Enabled    bool
	Repository InvitationRepository
	NewID      func() (string, error)
	NewSecret  func(kernel.OrgScope, string) (InvitationSecret, error)
}

func (s InvitationService) check(ctx context.Context, scope kernel.OrgScope, a EmployeeActor, decision access.Decision, action InvitationAction) error {
	if !s.Enabled {
		return ErrRegisterDisabled
	}
	if scope.Validate() != nil {
		return d.ErrScope
	}
	p := InvitationPolicy(action)
	if a.Validate() != nil || decision.Policy() != p || decision.Check(ctx, scope, a.AccountID, p.Permission, scope.OrgID().String()) != nil {
		return ErrEmployeeUnauthenticated
	}
	if s.Repository == nil {
		return ErrRegisterUnavailable
	}
	return nil
}
func (s InvitationService) Write(ctx context.Context, scope kernel.OrgScope, a EmployeeActor, decision access.Decision, c InvitationWrite) (InvitationResult, error) {
	if e := s.check(ctx, scope, a, decision, c.Action); e != nil {
		return InvitationResult{}, e
	}
	c, e := c.Canonical()
	if e != nil {
		return InvitationResult{}, e
	}
	var plan InvitationPlan
	var secret InvitationSecret
	if c.Action != InvitationRevoke {
		if s.NewID == nil || s.NewSecret == nil {
			return InvitationResult{}, ErrRegisterUnavailable
		}
		plan.InvitationID, e = s.NewID()
		if e != nil || !canonicalEmployeeID(plan.InvitationID) {
			return InvitationResult{}, ErrRegisterUnavailable
		}
		plan.MembershipID, e = s.NewID()
		if e != nil || !canonicalEmployeeID(plan.MembershipID) {
			return InvitationResult{}, ErrRegisterUnavailable
		}
		secret, e = s.NewSecret(scope, plan.InvitationID)
		if e != nil || secret.Code == "" || len(secret.Verifier) != 64 {
			return InvitationResult{}, ErrRegisterUnavailable
		}
		plan.Verifier = secret.Verifier
	}
	v, replay, e := s.Repository.WriteInvitation(ctx, scope, a, decision.Subject().MembershipID(), c, plan)
	if e != nil {
		return InvitationResult{}, e
	}
	if _, e := v.Snapshot(scope); e != nil {
		return InvitationResult{}, ErrRegisterUnavailable
	}
	result := InvitationResult{Invitation: v, Replayed: replay}
	if !replay {
		result.Code = secret.Code
	}
	return result, nil
}
func (s InvitationService) List(ctx context.Context, scope kernel.OrgScope, a EmployeeActor, decision access.Decision, f InvitationFilter) (InvitationPage, error) {
	if e := s.check(ctx, scope, a, decision, InvitationRead); e != nil {
		return InvitationPage{}, e
	}
	if e := f.Validate(); e != nil {
		return InvitationPage{}, e
	}
	return s.Repository.ListInvitations(ctx, scope, a, decision.Subject().MembershipID(), f)
}
