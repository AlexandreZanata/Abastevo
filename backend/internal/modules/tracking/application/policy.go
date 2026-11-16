package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
	"github.com/jackc/pgx/v5"
)

// The tracking policy command and read (coordinator-assigned prerequisite of
// the EMP-GPS-002 activation; B-BR-EMP-467..469, EDR-302/303).
const (
	// PolicyReadPermission and PolicyManagePermission are the catalog keys
	// the specification gives the tracking policy (gps-navigation §7).
	PolicyReadPermission   = "org.settings.read"
	PolicyManagePermission = "org.settings.manage"
	// PolicyAuditAction and PolicyTargetType name the audit vocabulary.
	PolicyAuditAction = "gps.policy.changed"
	PolicyTargetType  = "tracking_policy"
	// MaxPolicyRevision bounds the expected revision.
	MaxPolicyRevision int64 = 1 << 53
)

// PolicyAction is the closed set of policy actions.
type PolicyAction uint8

const (
	PolicyRead PolicyAction = iota + 1
	PolicyManage
)

// PolicySettingsPolicy returns the fixed access policy of an action: the
// read is org.settings.read; the change a fresh configuration write under
// org.settings.manage (B-BR-EMP-467/469).
func PolicySettingsPolicy(action PolicyAction) access.Policy {
	switch action {
	case PolicyRead:
		return access.Policy{Permission: PolicyReadPermission, Capability: "organizations", Effect: access.Read}
	case PolicyManage:
		return access.Policy{Permission: PolicyManagePermission, Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
	}
	return access.Policy{}
}

var (
	ErrPolicyDisabled     = errors.New("tracking: policy surface disabled")
	ErrPolicyUnauthorized = errors.New("tracking: company proof required")
	// ErrPolicyConflict is a stale or premature expected revision.
	ErrPolicyConflict       = errors.New("tracking: policy revision conflict")
	ErrPolicyOperationReuse = errors.New("tracking: policy operation identifier reused")
)

// PolicyField is a refused field of a policy change with its frozen code.
type PolicyField struct {
	Field, Code string
}

func (f *PolicyField) Error() string { return "tracking: invalid policy field " + f.Field }

// Frozen field codes of a refused policy change (B-BR-EMP-468).
const (
	CodeRequiredWhenEnabled = "REQUIRED_WHEN_ENABLED"
	CodeAbsentWhenOff       = "ABSENT_WHEN_OFF"
	CodeNoticeVersionFormat = "NOTICE_VERSION_FORMAT"
	CodeNoticeNotInForce    = "NOT_IN_FORCE"
	CodePolicyValue         = "OFF_OPTIONAL_OR_EXPECTED"
)

func fieldError(field, code string) error { return &PolicyField{Field: field, Code: code} }

var noticeVersion = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// PolicyActor is the verified company proof of the caller.
type PolicyActor struct {
	private
	AccountID, SessionID string
}

func (a PolicyActor) Validate() error {
	if !canonicalID(a.AccountID) || !canonicalID(a.SessionID) {
		return ErrPolicyUnauthorized
	}
	return nil
}

// PolicyCommandInput is one policy change. ExpectedRevision 0 means the
// organization has no recorded version yet.
type PolicyCommandInput struct {
	private
	OperationID      string
	ExpectedRevision int64
	Policy           string
	NoticeVersion    *string
	NoticeInForceAt  *time.Time
}

// PolicyCommand is a validated, immutable change.
type PolicyCommand struct {
	private
	value PolicyCommandInput
	valid bool
}

func validPolicyInstant(at time.Time) bool {
	year := at.UTC().Year()
	return !at.IsZero() && year >= 1 && year <= 9999
}

// NewPolicyCommand validates the shape of a change (B-BR-EMP-467/468):
// OFF carries no notice; OPTIONAL and EXPECTED carry a notice version and
// its in-force instant. Whether the notice is in force is decided against
// the database clock when the version is written.
func NewPolicyCommand(in PolicyCommandInput) (PolicyCommand, error) {
	if !canonicalID(in.OperationID) || in.ExpectedRevision < 0 || in.ExpectedRevision > MaxPolicyRevision {
		return PolicyCommand{}, ErrMalformed
	}
	policy, err := domain.ParsePolicy(in.Policy)
	if err != nil {
		return PolicyCommand{}, fieldError("policy", CodePolicyValue)
	}
	if in.NoticeInForceAt != nil && !validPolicyInstant(*in.NoticeInForceAt) {
		return PolicyCommand{}, ErrMalformed
	}
	if policy == domain.PolicyOff {
		switch {
		case in.NoticeVersion != nil:
			return PolicyCommand{}, fieldError("notice_version", CodeAbsentWhenOff)
		case in.NoticeInForceAt != nil:
			return PolicyCommand{}, fieldError("notice_in_force_at", CodeAbsentWhenOff)
		}
	} else {
		switch {
		case in.NoticeVersion == nil:
			return PolicyCommand{}, fieldError("notice_version", CodeRequiredWhenEnabled)
		case !noticeVersion.MatchString(*in.NoticeVersion):
			return PolicyCommand{}, fieldError("notice_version", CodeNoticeVersionFormat)
		case in.NoticeInForceAt == nil:
			return PolicyCommand{}, fieldError("notice_in_force_at", CodeRequiredWhenEnabled)
		}
	}
	return PolicyCommand{value: in.copy(), valid: true}, nil
}

func (in PolicyCommandInput) copy() PolicyCommandInput {
	in.NoticeVersion = copyPointer(in.NoticeVersion)
	if in.NoticeInForceAt != nil {
		at := in.NoticeInForceAt.UTC()
		in.NoticeInForceAt = &at
	}
	return in
}

// Snapshot returns an independent copy of a validated change.
func (c PolicyCommand) Snapshot() (PolicyCommandInput, error) {
	if !c.valid {
		return PolicyCommandInput{}, ErrUnavailable
	}
	return c.value.copy(), nil
}

// Enabled reports whether the change turns tracking on.
func (c PolicyCommand) Enabled() bool { return c.valid && c.value.Policy != string(domain.PolicyOff) }

// Digest is the request digest bound to the organization: the canonical
// JSON of every field.
func (c PolicyCommand) Digest(scope kernel.OrgScope) string {
	if !c.valid || scope.Validate() != nil {
		return ""
	}
	v := c.value
	var at any
	if v.NoticeInForceAt != nil {
		at = v.NoticeInForceAt.UTC().Format(time.RFC3339Nano)
	}
	raw, err := json.Marshal([]any{"tracking.policy.change.v1", scope.OrgID().String(), v.OperationID, v.ExpectedRevision, v.Policy, v.NoticeVersion, at})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// PolicyAck is the result of one accepted change.
type PolicyAck struct {
	private
	Revision      int64
	Policy        domain.Policy
	NoticeVersion string
	NoticeInForce *time.Time
	EffectiveAt   time.Time
}

// PolicyView is the stored and effective tracking policy.
type PolicyView struct {
	private
	Revision        int64
	Policy          domain.Policy
	NoticeVersion   string
	NoticeInForce   *time.Time
	EffectiveAt     *time.Time
	InForce         domain.Policy
	InForceRevision int64
}

// PolicyAuthority re-proves the company authority of an action over the
// organization itself inside the caller's transaction.
type PolicyAuthority interface {
	CheckPolicyTx(context.Context, pgx.Tx, kernel.OrgScope, PolicyActor, string, PolicyAction) error
}

// PolicyRepository changes and reads the policy under the request decision.
type PolicyRepository interface {
	ChangePolicy(context.Context, kernel.OrgScope, PolicyActor, string, PolicyCommand) (PolicyAck, bool, error)
	ReadPolicy(context.Context, kernel.OrgScope, PolicyActor, string) (PolicyView, error)
}

// PolicyService exposes the change and the read when enabled.
type PolicyService struct {
	Enabled    bool
	Repository PolicyRepository
}

func (s PolicyService) bound(ctx context.Context, scope kernel.OrgScope, actor PolicyActor, decision access.Decision, action PolicyAction) error {
	if ctx == nil {
		return ErrPolicyUnauthorized
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	policy := PolicySettingsPolicy(action)
	if scope.Validate() != nil || actor.Validate() != nil || decision.Subject().Channel() != "web" || decision.Policy() != policy ||
		decision.Check(ctx, scope, actor.AccountID, policy.Permission, scope.OrgID().String()) != nil {
		return ErrPolicyUnauthorized
	}
	return nil
}

// guarded binds the completed decision before and after the repository call.
func (s PolicyService) guarded(ctx context.Context, scope kernel.OrgScope, actor PolicyActor, decision access.Decision, action PolicyAction, call func() error) error {
	if !s.Enabled {
		return ErrPolicyDisabled
	}
	if err := s.bound(ctx, scope, actor, decision, action); err != nil {
		return err
	}
	if s.Repository == nil {
		return ErrUnavailable
	}
	err := call()
	if guard := s.bound(ctx, scope, actor, decision, action); guard != nil {
		return guard
	}
	return err
}

func sameInstant(a, b *time.Time) bool {
	return a == nil && b == nil || a != nil && b != nil && a.Equal(*b)
}

func validAck(v PolicyAck, in PolicyCommandInput) bool {
	version := ""
	if in.NoticeVersion != nil {
		version = *in.NoticeVersion
	}
	return v.Revision == in.ExpectedRevision+1 && string(v.Policy) == in.Policy && v.NoticeVersion == version && sameInstant(v.NoticeInForce, in.NoticeInForceAt) &&
		validPolicyInstant(v.EffectiveAt) && (v.NoticeInForce == nil || !v.NoticeInForce.After(v.EffectiveAt))
}

// Change runs one policy change under fresh org.settings.manage.
func (s PolicyService) Change(ctx context.Context, scope kernel.OrgScope, actor PolicyActor, decision access.Decision, command PolicyCommand) (PolicyAck, bool, error) {
	in, err := command.Snapshot()
	if err != nil {
		return PolicyAck{}, false, err
	}
	var ack PolicyAck
	replayed := false
	err = s.guarded(ctx, scope, actor, decision, PolicyManage, func() error {
		var err error
		ack, replayed, err = s.Repository.ChangePolicy(ctx, scope, actor, decision.Subject().MembershipID(), command)
		return err
	})
	if err != nil {
		return PolicyAck{}, false, err
	}
	if !validAck(ack, in) {
		return PolicyAck{}, false, ErrUnavailable
	}
	return ack, replayed, nil
}

func validView(v PolicyView) bool {
	if _, err := domain.ParsePolicy(string(v.Policy)); err != nil {
		return false
	}
	if _, err := domain.ParsePolicy(string(v.InForce)); err != nil {
		return false
	}
	enabled := v.Policy != domain.PolicyOff
	if v.Revision == 0 {
		return !enabled && v.NoticeVersion == "" && v.NoticeInForce == nil && v.EffectiveAt == nil && v.InForce == domain.PolicyOff && v.InForceRevision == 0
	}
	if v.Revision < 1 || v.EffectiveAt == nil || !validPolicyInstant(*v.EffectiveAt) || enabled != (v.NoticeVersion != "") || enabled != (v.NoticeInForce != nil) ||
		v.InForceRevision < 1 || v.InForceRevision > v.Revision {
		return false
	}
	// The newest version governs once effective; before that an older one.
	if v.InForceRevision == v.Revision {
		return !enabled && v.InForce == domain.PolicyOff || enabled && (v.InForce == v.Policy || v.InForce == domain.PolicyOff)
	}
	return true
}

// Read reads the policy under org.settings.read.
func (s PolicyService) Read(ctx context.Context, scope kernel.OrgScope, actor PolicyActor, decision access.Decision) (PolicyView, error) {
	var v PolicyView
	err := s.guarded(ctx, scope, actor, decision, PolicyRead, func() error {
		var err error
		v, err = s.Repository.ReadPolicy(ctx, scope, actor, decision.Subject().MembershipID())
		return err
	})
	if err != nil {
		return PolicyView{}, err
	}
	if !validView(v) {
		return PolicyView{}, ErrUnavailable
	}
	return v, nil
}
