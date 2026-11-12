package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/jackc/pgx/v5"
)

func SettingsPolicy(write bool) access.Policy {
	if write {
		return access.Policy{Permission: "org.settings.manage", Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
	}
	return access.Policy{Permission: "org.settings.read", Capability: "organizations", Effect: access.Read}
}

type SettingsWriteInput struct {
	privateOnboarding
	OperationID      string
	ExpectedRevision int64
	Values           d.SettingsValues
	Reason           string
}
type SettingsWrite struct {
	privateOnboarding
	value SettingsWriteInput
}

func NewSettingsWrite(v SettingsWriteInput) (SettingsWrite, error) {
	if !canonicalID(v.OperationID) || v.ExpectedRevision < 1 || v.ExpectedRevision == math.MaxInt64 {
		return SettingsWrite{}, d.ErrInvalid
	}
	if e := v.Values.Validate(); e != nil {
		return SettingsWrite{}, e
	}
	reason, e := d.CanonicalSettingsReason(v.Reason)
	if e != nil {
		return SettingsWrite{}, e
	}
	v.Reason = reason
	return SettingsWrite{value: v}, nil
}
func (c SettingsWrite) Snapshot() (SettingsWriteInput, error) {
	v, e := NewSettingsWrite(c.value)
	return v.value, e
}
func (c SettingsWrite) Digest(scope kernel.OrgScope) string {
	v, e := c.Snapshot()
	if e != nil || scope.Validate() != nil {
		return ""
	}
	p := v.Values.Policies
	r := v.Values.Responsibilities
	raw, _ := json.Marshal([]any{1, "org.settings.command", scope.OrgID().String(), v.OperationID, v.ExpectedRevision, v.Values.TimeZone, v.Values.ConsumptionDisplay, v.Values.ReviewRequired.Trips, v.Values.ReviewRequired.Fuelings, v.Values.ReviewRequired.Expenses, v.Values.ApprovalLevelsDefault, v.Values.ClosurePeriodDays, p.IntakeWindowDays, p.ClockSkewLimitSeconds, p.DelegationMaximumDays, p.InvitationValidityDays, p.ActivationValidityHours, r.ReviewFallback.PrimaryID, r.ReviewFallback.SubstituteID, r.PlatformNotices.PrimaryID, r.PlatformNotices.SubstituteID, r.DataProtection.PrimaryID, r.DataProtection.SubstituteID, r.Billing.PrimaryID, r.Billing.SubstituteID, v.Reason})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

type SettingsResult struct {
	Revision, RootRevision int64
	EffectiveAt            time.Time
}
type SettingsFilter struct {
	BeforeRevision int64
	Limit          int
}

func (f SettingsFilter) Validate() error {
	if f.BeforeRevision < 0 || f.Limit < 1 || f.Limit > 100 {
		return d.ErrInvalid
	}
	return nil
}

type SettingsRead struct {
	privateOnboarding
	Settings     d.Settings
	RootRevision int64
}

type SettingsPage struct {
	privateOnboarding
	Versions     []d.Settings
	NextRevision int64
}
type SettingsRepository interface {
	ChangeSettings(context.Context, kernel.OrgScope, CompanyActor, string, SettingsWrite) (SettingsResult, bool, error)
	ReadSettings(context.Context, kernel.OrgScope, CompanyActor, string, int64) (SettingsRead, error)
	ListSettings(context.Context, kernel.OrgScope, CompanyActor, string, SettingsFilter) (SettingsPage, error)
}
type SettingsIntake struct {
	privateOnboarding
	Settings   d.Settings
	ReceivedAt time.Time
}

// SettingsIntakeRepository is for an owning, already verified business command.
// Capture selects its server receipt/revision under the root lock in that same
// transaction. Device occurrence time and requested revisions are not inputs.
type SettingsIntakeRepository interface {
	CaptureSettingsTx(context.Context, pgx.Tx, kernel.OrgScope) (SettingsIntake, error)
	SettingsRevisionTx(context.Context, pgx.Tx, kernel.OrgScope, int64) (d.Settings, error)
}
type SettingsService struct {
	Enabled    bool
	Repository SettingsRepository
}

func (s SettingsService) check(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, decision access.Decision, write bool) error {
	if !s.Enabled {
		return ErrSurfaceDisabled
	}
	if scope.Validate() != nil {
		return d.ErrScope
	}
	p := SettingsPolicy(write)
	if actor.Validate() != nil || decision.Policy() != p || decision.Check(ctx, scope, actor.AccountID, p.Permission, scope.OrgID().String()) != nil {
		return ErrUnauthenticated
	}
	if s.Repository == nil {
		return ErrUnavailable
	}
	return nil
}
func (s SettingsService) Change(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, decision access.Decision, c SettingsWrite) (SettingsResult, bool, error) {
	if !s.Enabled {
		return SettingsResult{}, false, ErrSurfaceDisabled
	}
	if _, e := c.Snapshot(); e != nil {
		return SettingsResult{}, false, e
	}
	if e := s.check(ctx, scope, actor, decision, true); e != nil {
		return SettingsResult{}, false, e
	}
	return s.Repository.ChangeSettings(ctx, scope, actor, decision.Subject().MembershipID(), c)
}
func (s SettingsService) Read(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, decision access.Decision, revision int64) (SettingsRead, error) {
	if !s.Enabled {
		return SettingsRead{}, ErrSurfaceDisabled
	}
	if revision < 0 {
		return SettingsRead{}, d.ErrInvalid
	}
	if e := s.check(ctx, scope, actor, decision, false); e != nil {
		return SettingsRead{}, e
	}
	return s.Repository.ReadSettings(ctx, scope, actor, decision.Subject().MembershipID(), revision)
}
func (s SettingsService) List(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, decision access.Decision, f SettingsFilter) (SettingsPage, error) {
	if !s.Enabled {
		return SettingsPage{}, ErrSurfaceDisabled
	}
	if e := f.Validate(); e != nil {
		return SettingsPage{}, e
	}
	if e := s.check(ctx, scope, actor, decision, false); e != nil {
		return SettingsPage{}, e
	}
	return s.Repository.ListSettings(ctx, scope, actor, decision.Subject().MembershipID(), f)
}
