package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/jackc/pgx/v5"
)

var (
	ErrRoleDisabled        = errors.New("orgaccess: role management disabled")
	ErrRoleTargetNotFound  = errors.New("orgaccess: target membership not found")
	ErrRoleTargetNotActive = errors.New("orgaccess: target membership not active")
	ErrRoleInvalid         = errors.New("orgaccess: invalid role request")
	ErrRoleOperationReused = errors.New("orgaccess: role operation reused")
	ErrRoleUnauthenticated = errors.New("orgaccess: company proof required")
)

type privateRoleDTO struct{}

func (privateRoleDTO) String() string                 { return "RoleRequest(redacted)" }
func (p privateRoleDTO) Format(f fmt.State, _ rune)   { _, _ = fmt.Fprint(f, p.String()) }
func (p privateRoleDTO) MarshalJSON() ([]byte, error) { return json.Marshal(p.String()) }
func (p privateRoleDTO) LogValue() slog.Value         { return slog.StringValue(p.String()) }

type CompanyActor struct {
	AccountID, SessionID string
}

func (a CompanyActor) Validate() error {
	if id, err := kernel.ParseOrgID(a.AccountID); err != nil || id.String() != a.AccountID {
		return d.ErrAssignmentInvalid
	}
	if id, err := kernel.ParseOrgID(a.SessionID); err != nil || id.String() != a.SessionID {
		return d.ErrAssignmentInvalid
	}
	return nil
}

type RoleAction string

const (
	RoleAssignAction RoleAction = "ASSIGN"
	RoleEndAction    RoleAction = "END"
	RoleChangeAction RoleAction = "CHANGE"
	RoleReadAction   RoleAction = "READ"
	RoleListAction   RoleAction = "LIST"
)

func RolePolicy(action RoleAction) Policy {
	switch action {
	case RoleAssignAction, RoleChangeAction:
		return Policy{Permission: "auth.role.assign", Capability: "organizations", Effect: ConfigurationWrite, Fresh: true}
	case RoleEndAction:
		return Policy{Permission: "auth.role.assign", Capability: "organizations", Effect: ReduceAccess, Fresh: true}
	case RoleReadAction, RoleListAction:
		return Policy{Permission: "auth.role.assign", Capability: "organizations", Effect: Read, Fresh: true}
	default:
		return Policy{}
	}
}

type RoleAssignInput struct {
	privateRoleDTO
	OperationID  string
	MembershipID string
	Role         d.Role
	Scope        d.ScopeType
	TargetID     string
	ValidFrom    time.Time
	ValidTo      time.Time
	Reason       string
}

func (in RoleAssignInput) Validate(scope kernel.OrgScope) error {
	if scope.Validate() != nil {
		return d.ErrAssignmentScope
	}
	for _, id := range []string{in.OperationID, in.MembershipID, in.TargetID} {
		if parsed, err := kernel.ParseOrgID(id); err != nil || parsed.String() != id {
			return ErrRoleInvalid
		}
	}
	if _, err := d.SystemCatalog().Rules(in.Role); err != nil {
		return ErrRoleInvalid
	}
	if in.Scope != d.OrgScope && in.Scope != d.SelfScope {
		return d.ErrAssignmentScope
	}
	if in.Scope == d.OrgScope && in.TargetID != scope.OrgID().String() {
		return d.ErrAssignmentScope
	}
	if in.Scope == d.SelfScope && in.TargetID != in.MembershipID {
		return d.ErrAssignmentScope
	}
	if in.Role == d.Owner && in.Scope != d.OrgScope {
		return d.ErrAssignmentScope
	}
	if in.Role == d.StationAdmin || in.Role == d.StationManager || in.Role == d.StationAttendant {
		return d.ErrAssignmentScope
	}
	if !validRoleTime(in.ValidFrom) || !validRoleTime(in.ValidTo) {
		return ErrRoleInvalid
	}
	if !in.ValidTo.IsZero() && !in.ValidFrom.IsZero() && !in.ValidFrom.Before(in.ValidTo) {
		return ErrRoleInvalid
	}
	return ValidateRoleReason(in.Reason)
}

func roleDigest(values ...any) string {
	raw, _ := json.Marshal(values)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func roleIntentTime(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.UTC().Format(time.RFC3339Nano)
}
func validRoleTime(at time.Time) bool {
	return at.IsZero() || at.UTC().Year() >= 1 && at.UTC().Year() <= 9999
}
func (in RoleAssignInput) Digest(scope kernel.OrgScope) string {
	return roleDigest("assign.v2", scope.OrgID().String(), in.OperationID, in.MembershipID, string(in.Role), string(in.Scope), in.TargetID, roleIntentTime(in.ValidFrom), roleIntentTime(in.ValidTo), in.Reason)
}

type RoleEndInput struct {
	privateRoleDTO
	OperationID      string
	AssignmentID     string
	ExpectedRevision int64
	Reason           string
}

func (in RoleEndInput) Validate() error {
	for _, id := range []string{in.OperationID, in.AssignmentID} {
		if parsed, err := kernel.ParseOrgID(id); err != nil || parsed.String() != id {
			return ErrRoleInvalid
		}
	}
	if in.ExpectedRevision < 1 || in.ExpectedRevision == math.MaxInt64 {
		return ErrRoleInvalid
	}
	return ValidateRoleReason(in.Reason)
}

func (in RoleEndInput) Digest(scope kernel.OrgScope) string {
	return roleDigest("end.v2", scope.OrgID().String(), in.OperationID, in.AssignmentID, in.ExpectedRevision, in.Reason)
}

type RoleChangeInput struct {
	privateRoleDTO
	OperationID      string
	AssignmentID     string
	ExpectedRevision int64
	Role             d.Role
	Scope            d.ScopeType
	TargetID         string
	ValidFrom        time.Time
	ValidTo          time.Time
	Reason           string
}

func (in RoleChangeInput) Validate(scope kernel.OrgScope) error {
	if scope.Validate() != nil {
		return d.ErrAssignmentScope
	}
	for _, id := range []string{in.OperationID, in.AssignmentID, in.TargetID} {
		if parsed, err := kernel.ParseOrgID(id); err != nil || parsed.String() != id {
			return ErrRoleInvalid
		}
	}
	if in.ExpectedRevision < 1 || in.ExpectedRevision == math.MaxInt64 {
		return ErrRoleInvalid
	}
	if _, err := d.SystemCatalog().Rules(in.Role); err != nil {
		return ErrRoleInvalid
	}
	if in.Scope != d.OrgScope && in.Scope != d.SelfScope {
		return d.ErrAssignmentScope
	}
	if in.Scope == d.OrgScope && in.TargetID != scope.OrgID().String() {
		return d.ErrAssignmentScope
	}
	if in.Role == d.Owner && in.Scope != d.OrgScope {
		return d.ErrAssignmentScope
	}
	if in.Role == d.StationAdmin || in.Role == d.StationManager || in.Role == d.StationAttendant {
		return d.ErrAssignmentScope
	}
	if !validRoleTime(in.ValidFrom) || !validRoleTime(in.ValidTo) {
		return ErrRoleInvalid
	}
	if !in.ValidTo.IsZero() && !in.ValidFrom.IsZero() && !in.ValidFrom.Before(in.ValidTo) {
		return ErrRoleInvalid
	}
	return ValidateRoleReason(in.Reason)
}

func (in RoleChangeInput) Digest(scope kernel.OrgScope) string {
	return roleDigest("change.v2", scope.OrgID().String(), in.OperationID, in.AssignmentID, in.ExpectedRevision, string(in.Role), string(in.Scope), in.TargetID, roleIntentTime(in.ValidFrom), roleIntentTime(in.ValidTo), in.Reason)
}

type RoleListFilter struct {
	MembershipID string
	Role         d.Role
	State        string // "ACTIVE", "ENDED", "ALL"
	AfterID      string
	Limit        int
}

func (f RoleListFilter) Validate() error {
	if f.MembershipID != "" {
		if parsed, err := kernel.ParseOrgID(f.MembershipID); err != nil || parsed.String() != f.MembershipID {
			return ErrRoleInvalid
		}
	}
	if f.AfterID != "" {
		if parsed, err := kernel.ParseOrgID(f.AfterID); err != nil || parsed.String() != f.AfterID {
			return ErrRoleInvalid
		}
	}
	if f.Role != "" {
		if _, err := d.SystemCatalog().Rules(f.Role); err != nil {
			return ErrRoleInvalid
		}
	}
	if f.State != "" && f.State != "ACTIVE" && f.State != "ENDED" && f.State != "ALL" {
		return ErrRoleInvalid
	}
	if f.Limit < 1 || f.Limit > 100 {
		return ErrRoleInvalid
	}
	return nil
}

type RoleResult struct {
	privateRoleDTO
	Assignment d.Assignment
	Replayed   bool
}

type RoleChangeResult struct {
	privateRoleDTO
	EndedAssignment d.Assignment
	NewAssignment   d.Assignment
	Replayed        bool
}

type RolePage struct {
	privateRoleDTO
	Assignments []d.Assignment
	NextID      string
}

// RoleAuthority verifies current account/proof, binding, root/plan and scoped
// permission in the same mutation/read transaction. Returned candidates are
// current server authority, never caller roles.
type RoleAuthority interface {
	CheckRoleTx(context.Context, pgx.Tx, kernel.OrgScope, CompanyActor, string, string, RoleAction, d.Role) ([]d.Assignment, error)
}

type RoleManagementRepository interface {
	AssignRole(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, actorMemberID, newID string, input RoleAssignInput) (RoleResult, error)
	EndRole(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, actorMemberID string, input RoleEndInput) (RoleResult, error)
	ChangeRole(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, actorMemberID, newID string, input RoleChangeInput) (RoleChangeResult, error)
	ReadRole(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, actorMemberID, assignmentID string) (d.Assignment, error)
	ListRoles(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, actorMemberID string, filter RoleListFilter) (RolePage, error)
}

type RoleService struct {
	Enabled    bool
	Repository RoleManagementRepository
	NewID      func() (string, error)
}

func (s RoleService) check(ctx context.Context, scope kernel.OrgScope, a CompanyActor, decision Decision, target string, action RoleAction) error {
	if !s.Enabled {
		return ErrRoleDisabled
	}
	if scope.Validate() != nil {
		return d.ErrAssignmentScope
	}
	policy := RolePolicy(action)
	if a.Validate() != nil || decision.Policy() != policy || decision.Check(ctx, scope, a.AccountID, policy.Permission, target) != nil {
		return ErrRoleUnauthenticated
	}
	if s.Repository == nil {
		return ErrUnavailable
	}
	return nil
}

func (s RoleService) Assign(ctx context.Context, scope kernel.OrgScope, a CompanyActor, decision Decision, input RoleAssignInput) (RoleResult, error) {
	if !s.Enabled {
		return RoleResult{}, ErrRoleDisabled
	}
	if err := input.Validate(scope); err != nil {
		return RoleResult{}, err
	}
	if err := s.check(ctx, scope, a, decision, scope.OrgID().String(), RoleAssignAction); err != nil {
		return RoleResult{}, err
	}
	if s.NewID == nil {
		return RoleResult{}, ErrUnavailable
	}
	newID, err := s.NewID()
	if err != nil {
		return RoleResult{}, ErrUnavailable
	}
	if _, err := kernel.ParseOrgID(newID); err != nil {
		return RoleResult{}, ErrUnavailable
	}
	return s.Repository.AssignRole(ctx, scope, a, decision.Subject().MembershipID(), newID, input)
}

func (s RoleService) End(ctx context.Context, scope kernel.OrgScope, a CompanyActor, decision Decision, input RoleEndInput) (RoleResult, error) {
	if !s.Enabled {
		return RoleResult{}, ErrRoleDisabled
	}
	if err := input.Validate(); err != nil {
		return RoleResult{}, err
	}
	if err := s.check(ctx, scope, a, decision, input.AssignmentID, RoleEndAction); err != nil {
		return RoleResult{}, err
	}
	return s.Repository.EndRole(ctx, scope, a, decision.Subject().MembershipID(), input)
}

func (s RoleService) Change(ctx context.Context, scope kernel.OrgScope, a CompanyActor, decision Decision, input RoleChangeInput) (RoleChangeResult, error) {
	if !s.Enabled {
		return RoleChangeResult{}, ErrRoleDisabled
	}
	if err := input.Validate(scope); err != nil {
		return RoleChangeResult{}, err
	}
	if err := s.check(ctx, scope, a, decision, input.AssignmentID, RoleChangeAction); err != nil {
		return RoleChangeResult{}, err
	}
	if s.NewID == nil {
		return RoleChangeResult{}, ErrUnavailable
	}
	newID, err := s.NewID()
	if err != nil {
		return RoleChangeResult{}, ErrUnavailable
	}
	if _, err := kernel.ParseOrgID(newID); err != nil {
		return RoleChangeResult{}, ErrUnavailable
	}
	return s.Repository.ChangeRole(ctx, scope, a, decision.Subject().MembershipID(), newID, input)
}

func (s RoleService) Read(ctx context.Context, scope kernel.OrgScope, a CompanyActor, decision Decision, assignmentID string) (d.Assignment, error) {
	if !s.Enabled {
		return d.Assignment{}, ErrRoleDisabled
	}
	if _, err := kernel.ParseOrgID(assignmentID); err != nil {
		return d.Assignment{}, ErrRoleInvalid
	}
	if err := s.check(ctx, scope, a, decision, assignmentID, RoleReadAction); err != nil {
		return d.Assignment{}, err
	}
	return s.Repository.ReadRole(ctx, scope, a, decision.Subject().MembershipID(), assignmentID)
}

func (s RoleService) List(ctx context.Context, scope kernel.OrgScope, a CompanyActor, decision Decision, filter RoleListFilter) (RolePage, error) {
	if !s.Enabled {
		return RolePage{}, ErrRoleDisabled
	}
	if err := filter.Validate(); err != nil {
		return RolePage{}, err
	}
	if err := s.check(ctx, scope, a, decision, scope.OrgID().String(), RoleListAction); err != nil {
		return RolePage{}, err
	}
	return s.Repository.ListRoles(ctx, scope, a, decision.Subject().MembershipID(), filter)
}

// ValidateRoleReason applies the same bounded private reason contract to
// incoming commands and their immutable persisted acknowledgements.
func ValidateRoleReason(reason string) error {
	trimmed := strings.TrimSpace(reason)
	if !utf8.ValidString(reason) || trimmed == "" || utf8.RuneCountInString(reason) > 280 {
		return ErrRoleInvalid
	}
	for _, r := range reason {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ErrRoleInvalid
		}
	}
	return nil
}
