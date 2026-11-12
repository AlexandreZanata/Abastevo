package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/jackc/pgx/v5"
)

var (
	ErrRegisterUnavailable     = errors.New("membership: register unavailable")
	ErrEmployeeNotFound        = errors.New("membership: employee not found")
	ErrEmployeeUnauthenticated = errors.New("membership: current company proof required")
	ErrRegisterDisabled        = errors.New("membership: register disabled")
	ErrRegistrationDuplicate   = errors.New("membership: duplicate registration")
	ErrDuplicateReview         = errors.New("membership: possible duplicate requires review")
	ErrEmployeeOperationReused = errors.New("membership: employee operation reused")
)

// Explicit field mapping is required at every private boundary.
type privateRegister struct{}

func (privateRegister) String() string                 { return "EmployeeRegister(redacted)" }
func (p privateRegister) Format(f fmt.State, _ rune)   { _, _ = fmt.Fprint(f, p.String()) }
func (p privateRegister) MarshalJSON() ([]byte, error) { return json.Marshal(p.String()) }
func (p privateRegister) LogValue() slog.Value         { return slog.StringValue(p.String()) }

// EmployeeActor contains verified metadata, never authority or credential bytes.
type EmployeeActor struct{ AccountID, SessionID string }

func (EmployeeActor) String() string                 { return "EmployeeActor(redacted)" }
func (a EmployeeActor) Format(f fmt.State, _ rune)   { _, _ = fmt.Fprint(f, a.String()) }
func (a EmployeeActor) MarshalJSON() ([]byte, error) { return json.Marshal(a.String()) }
func canonicalEmployeeID(id string) bool {
	v, e := kernel.ParseOrgID(id)
	return e == nil && v.String() == id
}
func (a EmployeeActor) LogValue() slog.Value { return slog.StringValue(a.String()) }
func (a EmployeeActor) Validate() error {
	if !canonicalEmployeeID(a.AccountID) || !canonicalEmployeeID(a.SessionID) {
		return d.ErrInvalid
	}
	return nil
}

type EmployeeAction string

const (
	EmployeeRead   EmployeeAction = "READ"
	EmployeeReview EmployeeAction = "REVIEW"
	EmployeeManage EmployeeAction = "MANAGE"
)

func EmployeePolicy(action EmployeeAction) access.Policy {
	switch action {
	case EmployeeRead:
		return access.Policy{Permission: "idn.employee.read", Capability: "organizations", Effect: access.Read}
	case EmployeeReview:
		return access.Policy{Permission: "idn.employee.manage", Capability: "organizations", Effect: access.Read}
	case EmployeeManage:
		return access.Policy{Permission: "idn.employee.manage", Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
	default:
		return access.Policy{}
	}
}

// EmployeeAuthority composes current source-owned proof/root/role checks in
// the caller's scoped transaction. Implementations preserve account -> proof
// -> capability -> root lock order, and never accept a caller-chosen policy.
type EmployeeAuthority interface {
	CheckEmployeeTx(context.Context, pgx.Tx, kernel.OrgScope, EmployeeActor, string, string, EmployeeAction) error
}
type EmployeeWriteKind string

const (
	CreateEmployee   EmployeeWriteKind = "CREATE"
	CompleteEmployee EmployeeWriteKind = "COMPLETE"
	ChangeEmployee   EmployeeWriteKind = "CHANGE"
)

type EmployeeWriteInput struct {
	privateRegister
	Kind                    EmployeeWriteKind
	OperationID, EmployeeID string
	ExpectedRevision        int64
	Profile                 d.EmployeeProfile
	AllowPossibleDuplicate  bool
}
type EmployeeWrite struct {
	privateRegister
	value EmployeeWriteInput
}

func NewEmployeeWrite(v EmployeeWriteInput) (EmployeeWrite, error) {
	if !canonicalEmployeeID(v.OperationID) {
		return EmployeeWrite{}, d.ErrInvalid
	}
	switch v.Kind {
	case CreateEmployee:
		if v.EmployeeID != "" || v.ExpectedRevision != 0 {
			return EmployeeWrite{}, d.ErrInvalid
		}
	case CompleteEmployee, ChangeEmployee:
		if !canonicalEmployeeID(v.EmployeeID) || v.ExpectedRevision < 1 || v.ExpectedRevision == math.MaxInt64 {
			return EmployeeWrite{}, d.ErrInvalid
		}
	default:
		return EmployeeWrite{}, d.ErrInvalid
	}
	p, err := d.CanonicalEmployeeProfile(v.Profile)
	if err != nil {
		return EmployeeWrite{}, err
	}
	v.Profile = p
	return EmployeeWrite{value: v}, nil
}
func (c EmployeeWrite) Snapshot() (EmployeeWriteInput, error) {
	v, e := NewEmployeeWrite(c.value)
	return v.value, e
}
func (c EmployeeWrite) Digest(scope kernel.OrgScope) string {
	v, e := c.Snapshot()
	if e != nil || scope.Validate() != nil {
		return ""
	}
	raw, _ := json.Marshal([]any{1, "idn.employee.command", scope.OrgID().String(), v.Kind, v.OperationID, v.EmployeeID, v.ExpectedRevision, v.Profile.DisplayName, v.Profile.RegistrationNumber, v.Profile.WorkEmail, v.Profile.WorkPhone, v.Profile.JobTitle, v.Profile.HomeUnitID, v.AllowPossibleDuplicate})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

type EmployeeResult struct {
	EmployeeID                        string
	State                             d.EmployeeState
	IdentityRevision, ProfileRevision int64
}
type EmployeeFilter struct {
	AfterID string
	State   d.EmployeeState
	Limit   int
}

func (f EmployeeFilter) Validate() error {
	if f.Limit < 1 || f.Limit > 100 || f.AfterID != "" && !canonicalEmployeeID(f.AfterID) || f.State != "" && f.State != d.EmployeeActive && f.State != d.EmployeeInactive && f.State != d.EmployeeTerminated {
		return d.ErrInvalid
	}
	return nil
}

type EmployeePage struct {
	privateRegister
	Employees []d.EmployeeRecord
	NextID    string
}
type EmployeeMatch struct {
	privateRegister
	Profile            d.EmployeeProfile
	ExcludeID, AfterID string
	Limit              int
}

func (f EmployeeMatch) Validate() error {
	p, e := d.CanonicalEmployeeProfile(f.Profile)
	if e != nil || p != f.Profile || f.ExcludeID != "" && !canonicalEmployeeID(f.ExcludeID) || f.AfterID != "" && !canonicalEmployeeID(f.AfterID) || f.Limit < 1 || f.Limit > 100 {
		return d.ErrInvalid
	}
	return nil
}

type EmployeeRegister interface {
	WriteEmployee(context.Context, kernel.OrgScope, EmployeeActor, string, string, EmployeeWrite) (EmployeeResult, bool, error)
	ReadEmployee(context.Context, kernel.OrgScope, EmployeeActor, string, string) (d.EmployeeRecord, error)
	ListEmployees(context.Context, kernel.OrgScope, EmployeeActor, string, EmployeeFilter) (EmployeePage, error)
	MatchEmployees(context.Context, kernel.OrgScope, EmployeeActor, string, EmployeeMatch) (EmployeePage, error)
}
type EmployeeLookup interface {
	FindEmployee(context.Context, kernel.OrgScope, string) (d.EmployeeRecord, error)
}
type EmployeeService struct {
	Enabled    bool
	Repository EmployeeRegister
	NewID      func() (string, error)
}

func (s EmployeeService) check(ctx context.Context, scope kernel.OrgScope, a EmployeeActor, decision access.Decision, target string, action EmployeeAction) error {
	if !s.Enabled {
		return ErrRegisterDisabled
	}
	if scope.Validate() != nil {
		return d.ErrScope
	}
	policy := EmployeePolicy(action)
	if a.Validate() != nil || decision.Policy() != policy || decision.Check(ctx, scope, a.AccountID, policy.Permission, target) != nil {
		return ErrEmployeeUnauthenticated
	}
	if s.Repository == nil {
		return ErrRegisterUnavailable
	}
	return nil
}
func (s EmployeeService) Write(ctx context.Context, scope kernel.OrgScope, a EmployeeActor, decision access.Decision, c EmployeeWrite) (EmployeeResult, bool, error) {
	if !s.Enabled {
		return EmployeeResult{}, false, ErrRegisterDisabled
	}
	v, e := c.Snapshot()
	if e != nil {
		return EmployeeResult{}, false, e
	}
	target := v.EmployeeID
	if v.Kind == CreateEmployee {
		target = scope.OrgID().String()
	}
	if e = s.check(ctx, scope, a, decision, target, EmployeeManage); e != nil {
		return EmployeeResult{}, false, e
	}
	id := ""
	if v.Kind == CreateEmployee {
		if s.NewID == nil {
			return EmployeeResult{}, false, ErrRegisterUnavailable
		}
		id, e = s.NewID()
		if e != nil || !canonicalEmployeeID(id) {
			return EmployeeResult{}, false, ErrRegisterUnavailable
		}
	}
	return s.Repository.WriteEmployee(ctx, scope, a, decision.Subject().MembershipID(), id, c)
}
func (s EmployeeService) Read(ctx context.Context, scope kernel.OrgScope, a EmployeeActor, decision access.Decision, id string) (d.EmployeeRecord, error) {
	if !s.Enabled {
		return d.EmployeeRecord{}, ErrRegisterDisabled
	}
	if !canonicalEmployeeID(id) {
		return d.EmployeeRecord{}, d.ErrInvalid
	}
	if e := s.check(ctx, scope, a, decision, id, EmployeeRead); e != nil {
		return d.EmployeeRecord{}, e
	}
	return s.Repository.ReadEmployee(ctx, scope, a, decision.Subject().MembershipID(), id)
}
func (s EmployeeService) List(ctx context.Context, scope kernel.OrgScope, a EmployeeActor, decision access.Decision, f EmployeeFilter) (EmployeePage, error) {
	if !s.Enabled {
		return EmployeePage{}, ErrRegisterDisabled
	}
	if e := f.Validate(); e != nil {
		return EmployeePage{}, e
	}
	if e := s.check(ctx, scope, a, decision, scope.OrgID().String(), EmployeeRead); e != nil {
		return EmployeePage{}, e
	}
	return s.Repository.ListEmployees(ctx, scope, a, decision.Subject().MembershipID(), f)
}
func (s EmployeeService) Matches(ctx context.Context, scope kernel.OrgScope, a EmployeeActor, decision access.Decision, f EmployeeMatch) (EmployeePage, error) {
	if !s.Enabled {
		return EmployeePage{}, ErrRegisterDisabled
	}
	if e := f.Validate(); e != nil {
		return EmployeePage{}, e
	}
	if e := s.check(ctx, scope, a, decision, scope.OrgID().String(), EmployeeReview); e != nil {
		return EmployeePage{}, e
	}
	return s.Repository.MatchEmployees(ctx, scope, a, decision.Subject().MembershipID(), f)
}
