package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"slices"
	"sync/atomic"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	m "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	o "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

var (
	ErrInvalidContext      = errors.New("orgaccess: invalid request context")
	ErrNoMembership        = errors.New("orgaccess: no current membership")
	ErrMembershipSuspended = errors.New("orgaccess: membership suspended")
	ErrOrgSuspended        = errors.New("orgaccess: organization suspended")
	ErrOrgArchived         = errors.New("orgaccess: organization archived")
	ErrSetupIncomplete     = errors.New("orgaccess: setup incomplete")
	ErrUnauthenticated     = errors.New("orgaccess: proof required")
	ErrFreshAuthRequired   = errors.New("orgaccess: fresh proof required")
	ErrEntitlementRequired = errors.New("orgaccess: entitlement required")
	ErrLimitReached        = errors.New("orgaccess: limit reached")
	ErrSelfApproval        = errors.New("orgaccess: self approval refused")
	ErrPeriodLocked        = errors.New("orgaccess: period locked")
)

type privateAuthority struct{}

func (privateAuthority) Format(s fmt.State, _ rune) {
	_, _ = io.WriteString(s, "[private authorization]")
}
func (privateAuthority) MarshalJSON() ([]byte, error) { return json.Marshal("[private authorization]") }
func (privateAuthority) LogValue() slog.Value         { return slog.StringValue("[private authorization]") }

// Authority is a current scoped storage snapshot, never a client DTO.
type Authority struct {
	privateAuthority
	OrgID                               kernel.OrgID
	AccountID, MembershipID, EmployeeID string
	MembershipState                     m.MembershipState
	EmployeeState                       m.EmployeeState
	Model                               m.AccessModel
	OrganizationState                   o.State
	Assignments                         []d.Assignment
}
type AuthoritySource interface {
	LoadAuthority(context.Context, kernel.OrgScope, string) (Authority, error)
}

// Actor comes from the normal proof entry, not a client authority descriptor.
type Actor struct {
	privateAuthority
	AccountID, Channel, ContributorID, DeviceKeyID string
}
type Subject struct {
	privateAuthority
	scope                              kernel.OrgScope
	account, member, employee, channel string
	contributor, device                string
	model                              m.AccessModel
}

func (s Subject) Scope() kernel.OrgScope { return s.scope }
func (s Subject) AccountID() string      { return s.account }
func (s Subject) MembershipID() string   { return s.member }
func (s Subject) EmployeeID() string     { return s.employee }
func (s Subject) Channel() string        { return s.channel }
func (s Subject) Model() m.AccessModel   { return s.model }
func (s Subject) ContributorID() string  { return s.contributor }
func (s Subject) DeviceKeyID() string    { return s.device }

type Effect uint8

const (
	Read Effect = iota + 1
	OperationalWrite
	ConfigurationWrite
	ReduceAccess
	RegularizePlan
	RestoreOrganization
)

func (e Effect) Write() bool { return e != Read && e >= OperationalWrite && e <= RestoreOrganization }

// Policy is source-owned route metadata. It is not decoded from a request.
type Policy struct {
	Permission d.Permission
	Capability string
	Effect     Effect
	Fresh      bool
}

var capabilityPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z][a-z0-9]*)*$`)

func (p Policy) Validate() error {
	if _, err := d.SystemCatalog().Lookup(d.Owner, p.Permission); err != nil {
		return ErrUnavailable
	}
	if p.Effect < Read || p.Effect > RestoreOrganization || len(p.Capability) > 64 || !capabilityPattern.MatchString(p.Capability) {
		return ErrUnavailable
	}
	// BR-AUTH-010 and organizations.md: role/ownership/credential/key changes,
	// full exports, archive/closure and grant acceptance need fresh proof.
	switch p.Permission {
	case "auth.role.assign", "auth.delegation.manage", "org.ownership.transfer", "org.ownership.respond", "idn.invitation.manage", "idn.credential.manage", "data.key.manage",
		"rpt.export.run", "org.lifecycle.manage", "org.group.manage":
		if !p.Fresh {
			return ErrUnavailable
		}
	}
	return nil
}

type ResourceResolver interface {
	Resolve(context.Context, kernel.OrgScope, Subject) (d.ResourceFacts, error)
}

// ProofGate is bound by the normal authentication entry to this request and
// subject. It rechecks current proof/freshness after permission and before plan.
type ProofGate interface {
	Confirm(context.Context, Subject, Policy) error
}
type CapabilityGate interface {
	Check(context.Context, kernel.OrgScope, Subject, Policy) error
}
type RecordGate interface {
	Check(context.Context, kernel.OrgScope, Subject, Policy, d.ResourceFacts, []d.Rule) error
}
type Authorization struct {
	Source       AuthoritySource
	Capabilities CapabilityGate
	Records      RecordGate
	Now          func() time.Time
}

type requestKey struct{}
type requestLease struct{ active atomic.Bool }

// BeginRequest always replaces an inherited context. Retained contexts and
// copied decisions cannot authorize a later request or survive its finish.
func BeginRequest(ctx context.Context) (context.Context, func()) {
	lease := &requestLease{}
	lease.active.Store(true)
	return context.WithValue(ctx, requestKey{}, lease), func() { lease.active.Store(false) }
}
func liveLease(ctx context.Context) *requestLease {
	if ctx == nil || ctx.Err() != nil {
		return nil
	}
	lease, _ := ctx.Value(requestKey{}).(*requestLease)
	if lease == nil || !lease.active.Load() {
		return nil
	}
	return lease
}

type binding struct {
	lease   *requestLease
	subject Subject
	policy  Policy
	target  d.ResourceFacts
	rules   []d.Rule
	phase   atomic.Uint32
}
type Prepared struct {
	privateAuthority
	binding *binding
}
type Decision struct {
	privateAuthority
	binding *binding
}

func (p Prepared) Subject() Subject {
	if p.binding == nil {
		return Subject{}
	}
	return p.binding.subject
}
func (p Prepared) Policy() Policy {
	if p.binding == nil {
		return Policy{}
	}
	return p.binding.policy
}
func (d Decision) Subject() Subject {
	if d.binding == nil {
		return Subject{}
	}
	return d.binding.subject
}
func (d Decision) Policy() Policy {
	if d.binding == nil {
		return Policy{}
	}
	return d.binding.policy
}
func (d Decision) TargetID() string {
	if d.binding == nil {
		return ""
	}
	return d.binding.target.ID
}
func (d Decision) Check(ctx context.Context, scope kernel.OrgScope, account string, permission d.Permission, target string) error {
	b := d.binding
	if b == nil || liveLease(ctx) != b.lease || b.phase.Load() != 2 || scope != b.subject.scope || account != b.subject.account || permission != b.policy.Permission || target != b.target.ID {
		return ErrInvalidContext
	}
	return nil
}
func validID(id string) bool { v, err := kernel.ParseOrgID(id); return err == nil && v.String() == id }
func organizationAllows(state o.State, effect Effect) error {
	switch state {
	case o.Active:
		return nil
	case o.PendingSetup:
		if effect == OperationalWrite {
			return ErrSetupIncomplete
		}
		return nil
	case o.Suspended:
		if effect == Read || effect == ReduceAccess || effect == RegularizePlan {
			return nil
		}
		return ErrOrgSuspended
	case o.Archived:
		if effect == Read || effect == ReduceAccess || effect == RegularizePlan || effect == RestoreOrganization {
			return nil
		}
		return ErrOrgArchived
	case o.Closed:
		return ErrNoMembership
	default:
		return ErrUnavailable
	}
}
func bounded(ctx context.Context, err error, known ...error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	for _, k := range known {
		if errors.Is(err, k) {
			return k
		}
	}
	return ErrUnavailable
}

// Prepare loads current authority on every call. Incoming context values never
// supply membership or roles. Resource facts are loaded only after membership,
// organization state and the presence of a positive permission are established.
func (s Authorization) Prepare(ctx context.Context, scope kernel.OrgScope, actor Actor, policy Policy, resolver ResourceResolver) (Prepared, error) {
	account, channel := actor.AccountID, actor.Channel
	lease := liveLease(ctx)
	if lease == nil {
		return Prepared{}, ErrInvalidContext
	}
	if scope.Validate() != nil || !validID(account) || (channel != "android" && channel != "web") {
		return Prepared{}, ErrUnauthenticated
	}
	if (channel == "android" && (!validID(actor.ContributorID) || !validID(actor.DeviceKeyID))) || (channel == "web" && (actor.ContributorID != "" || actor.DeviceKeyID != "")) {
		return Prepared{}, ErrUnauthenticated
	}
	if policy.Validate() != nil || s.Source == nil || s.Now == nil || resolver == nil {
		return Prepared{}, ErrUnavailable
	}
	now := s.Now()
	if now.IsZero() {
		return Prepared{}, ErrUnavailable
	}
	current, err := s.Source.LoadAuthority(ctx, scope, account)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Prepared{}, ErrNoMembership
		}
		return Prepared{}, bounded(ctx, err)
	}
	if current.OrgID != scope.OrgID() || current.AccountID != account || !validID(current.MembershipID) || !validID(current.EmployeeID) || (current.Model != m.ExistingAccount && current.Model != m.ManagedAccess) {
		return Prepared{}, ErrUnavailable
	}
	switch current.MembershipState {
	case m.MembershipSuspended:
		return Prepared{}, ErrMembershipSuspended
	case m.MembershipInvited, m.MembershipEnded:
		return Prepared{}, ErrNoMembership
	case m.MembershipActive:
	default:
		return Prepared{}, ErrUnavailable
	}
	if current.EmployeeState != m.EmployeeActive || (channel == "web" && current.Model == m.ManagedAccess) {
		return Prepared{}, ErrNoMembership
	}
	if err := organizationAllows(current.OrganizationState, policy.Effect); err != nil {
		return Prepared{}, err
	}
	subject := Subject{scope: scope, account: account, member: current.MembershipID, employee: current.EmployeeID, channel: channel, model: current.Model, contributor: actor.ContributorID, device: actor.DeviceKeyID}
	positive := false
	for _, assignment := range current.Assignments {
		v, err := assignment.Snapshot(scope)
		if err != nil {
			return Prepared{}, ErrUnavailable
		}
		if v.MembershipID != subject.member {
			continue
		}
		valid, err := assignment.ValidAt(scope, now)
		if err != nil {
			return Prepared{}, ErrUnavailable
		}
		if !valid {
			continue
		}
		rule, err := d.SystemCatalog().Lookup(v.Role, policy.Permission)
		if err != nil {
			return Prepared{}, ErrUnavailable
		}
		if rule.Coverage() != d.Denied {
			positive = true
		}
	}
	if !positive {
		return Prepared{}, d.DenyPermission(policy.Permission)
	}
	target, err := resolver.Resolve(ctx, scope, subject)
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, d.ErrResourceNotFound) {
			return Prepared{}, d.ErrResourceNotFound
		}
		return Prepared{}, bounded(ctx, err)
	}
	rules, err := d.CoveringRules(scope, subject.member, current.Assignments, policy.Permission, target, now)
	if err != nil {
		if errors.Is(err, d.ErrPermissionDenied) || errors.Is(err, d.ErrResourceNotFound) {
			return Prepared{}, err
		}
		return Prepared{}, ErrUnavailable
	}
	if liveLease(ctx) != lease {
		return Prepared{}, ErrInvalidContext
	}
	return Prepared{binding: &binding{lease: lease, subject: subject, policy: policy, target: target, rules: rules}}, nil
}

// Complete is single-use and requires the entry's current proof confirmation.
// A missing/failed independent gate yields no usable Decision, including OWNER.
func (s Authorization) Complete(ctx context.Context, p Prepared, proof ProofGate) (Decision, error) {
	b := p.binding
	if b == nil || liveLease(ctx) != b.lease || !b.phase.CompareAndSwap(0, 1) {
		return Decision{}, ErrInvalidContext
	}
	success := false
	defer func() {
		if !success {
			b.phase.Store(3)
		}
	}()
	if proof == nil {
		return Decision{}, ErrUnavailable
	}
	if err := proof.Confirm(ctx, b.subject, b.policy); err != nil {
		return Decision{}, bounded(ctx, err, ErrUnauthenticated, ErrFreshAuthRequired)
	}
	if liveLease(ctx) != b.lease {
		return Decision{}, ErrInvalidContext
	}
	if s.Capabilities == nil {
		return Decision{}, ErrUnavailable
	}
	if err := s.Capabilities.Check(ctx, b.subject.scope, b.subject, b.policy); err != nil {
		return Decision{}, bounded(ctx, err, ErrEntitlementRequired, ErrLimitReached)
	}
	if liveLease(ctx) != b.lease {
		return Decision{}, ErrInvalidContext
	}
	if s.Records == nil {
		return Decision{}, ErrUnavailable
	}
	if err := s.Records.Check(ctx, b.subject.scope, b.subject, b.policy, b.target, slices.Clone(b.rules)); err != nil {
		return Decision{}, bounded(ctx, err, ErrSelfApproval, ErrPeriodLocked, d.ErrResourceNotFound)
	}
	if liveLease(ctx) != b.lease {
		return Decision{}, ErrInvalidContext
	}
	b.phase.Store(2)
	success = true
	return Decision{binding: b}, nil
}
