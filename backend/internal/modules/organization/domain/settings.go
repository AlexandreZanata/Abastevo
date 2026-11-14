package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

type privateSettings struct{}

func (privateSettings) Format(f fmt.State, _ rune)   { redact(f, "Settings(redacted)") }
func (privateSettings) MarshalJSON() ([]byte, error) { return json.Marshal("Settings(redacted)") }
func (privateSettings) LogValue() slog.Value         { return slog.StringValue("Settings(redacted)") }

type settingsFieldError struct {
	privateSettings
	field string
}

func (settingsFieldError) Error() string   { return "organization: invalid settings field" }
func (settingsFieldError) Unwrap() error   { return ErrInvalid }
func (e settingsFieldError) Field() string { return e.field }
func settingsField(field string) error     { return settingsFieldError{field: field} }

// SettingsField returns only a source-controlled field key, never its value.
func SettingsField(err error) string {
	var e settingsFieldError
	if errors.As(err, &e) {
		return e.field
	}
	return ""
}

type ConsumptionDisplay string

const (
	ConsumptionKMPerL    ConsumptionDisplay = "KM_PER_L"
	ConsumptionLPer100KM ConsumptionDisplay = "L_PER_100KM"
)

type ReviewRequired struct{ Trips, Fuelings, Expenses bool }
type SourcePolicies struct {
	RequireInvitationConfirmation bool
	privateSettings
	IntakeWindowDays                                                       int
	ClockSkewLimitSeconds                                                  int64
	DelegationMaximumDays, InvitationValidityDays, ActivationValidityHours int
}
type ResponsibilityPair struct {
	privateSettings
	PrimaryID, SubstituteID string
}
type Responsibilities struct {
	privateSettings
	ReviewFallback, PlatformNotices, DataProtection, Billing ResponsibilityPair
}
type Responsibility string

const (
	ReviewResponsibility   Responsibility = "REVIEW_FALLBACK"
	PlatformResponsibility Responsibility = "PLATFORM_NOTICES"
	PrivacyResponsibility  Responsibility = "DATA_PROTECTION"
	BillingResponsibility  Responsibility = "BILLING"
)

func (r Responsibilities) Pair(kind Responsibility) (ResponsibilityPair, error) {
	switch kind {
	case ReviewResponsibility:
		return r.ReviewFallback, nil
	case PlatformResponsibility:
		return r.PlatformNotices, nil
	case PrivacyResponsibility:
		return r.DataProtection, nil
	case BillingResponsibility:
		return r.Billing, nil
	}
	return ResponsibilityPair{}, ErrInvalid
}
func (r Responsibilities) pairs() []struct {
	name  string
	value ResponsibilityPair
} {
	return []struct {
		name  string
		value ResponsibilityPair
	}{{"review_fallback", r.ReviewFallback}, {"platform_notices", r.PlatformNotices}, {"data_protection", r.DataProtection}, {"billing", r.Billing}}
}

type SettingsValues struct {
	privateSettings
	TimeZone                                 string
	ConsumptionDisplay                       ConsumptionDisplay
	ReviewRequired                           ReviewRequired
	ApprovalLevelsDefault, ClosurePeriodDays int
	Policies                                 SourcePolicies
	Responsibilities                         Responsibilities
}

func DefaultSettingsValues(zone, creator string) (SettingsValues, error) {
	v := SettingsValues{TimeZone: zone, ConsumptionDisplay: ConsumptionKMPerL, ReviewRequired: ReviewRequired{true, true, true}, ApprovalLevelsDefault: 1, ClosurePeriodDays: 30, Policies: SourcePolicies{IntakeWindowDays: 15, ClockSkewLimitSeconds: 300, DelegationMaximumDays: 90, InvitationValidityDays: 7, ActivationValidityHours: 48}}
	pair := ResponsibilityPair{PrimaryID: creator}
	v.Responsibilities = Responsibilities{PlatformNotices: pair, DataProtection: pair, Billing: pair}
	return v, v.Validate()
}
func (v SettingsValues) Validate() error {
	if _, ok := kernel.LoadTimeZone(v.TimeZone); !ok {
		return settingsField("time_zone")
	}
	if v.ConsumptionDisplay != ConsumptionKMPerL && v.ConsumptionDisplay != ConsumptionLPer100KM {
		return settingsField("consumption_display")
	}
	if v.ApprovalLevelsDefault < 1 || v.ApprovalLevelsDefault > 3 {
		return settingsField("approval_levels_default")
	}
	if v.ClosurePeriodDays < 7 || v.ClosurePeriodDays > 90 {
		return settingsField("closure_period_days")
	}
	p := v.Policies
	if p.IntakeWindowDays < 0 || p.IntakeWindowDays > 60 {
		return settingsField("policies.intake_window_days")
	}
	if p.ClockSkewLimitSeconds < 0 {
		return settingsField("policies.clock_skew_limit_seconds")
	}
	if p.DelegationMaximumDays < 1 || p.DelegationMaximumDays > 180 {
		return settingsField("policies.delegation_maximum_days")
	}
	if p.InvitationValidityDays < 1 || p.InvitationValidityDays > 30 {
		return settingsField("policies.invitation_validity_days")
	}
	if p.ActivationValidityHours < 1 || p.ActivationValidityHours > 168 {
		return settingsField("policies.activation_validity_hours")
	}
	for _, pair := range v.Responsibilities.pairs() {
		if pair.value.PrimaryID != "" && !canonicalLegalID(pair.value.PrimaryID) {
			return settingsField("responsibilities." + pair.name + ".primary")
		}
		if pair.value.SubstituteID != "" && (!canonicalLegalID(pair.value.SubstituteID) || pair.value.PrimaryID == pair.value.SubstituteID) {
			return settingsField("responsibilities." + pair.name + ".substitute")
		}
	}
	return nil
}

const OwnershipTransferValidityDays = 7

// Source policies are validated here from the published owner contracts, not
// enforced as invitations/delegations/intake or legal retention by this module.
type SettingsSource string

const (
	SettingsOnboarding          SettingsSource = "ONBOARDING"
	SettingsBootstrap           SettingsSource = "BOOTSTRAP"
	SettingsCommand             SettingsSource = "COMMAND"
	SettingsMembershipReduction SettingsSource = "MEMBERSHIP_REDUCTION"
)

type SettingsAuthor struct {
	privateSettings
	AccountID, MembershipID string
}
type SettingsSnapshot struct {
	privateSettings
	OrgID                                   kernel.OrgID
	Revision                                int64
	EffectiveAt                             time.Time
	Values                                  SettingsValues
	Source                                  SettingsSource
	Author                                  SettingsAuthor
	Reason, OperationID, SourceMembershipID string
	SourceMembershipRevision                int64
}
type Settings struct {
	privateSettings
	value SettingsSnapshot
}

func settingsInstant(at time.Time) bool {
	return !at.IsZero() && at.UTC().Year() >= 1 && at.UTC().Year() <= 9999
}
func NewOnboardingSettings(scope kernel.OrgScope, zone, creator, account, op string, at time.Time) (Settings, error) {
	v, e := DefaultSettingsValues(zone, creator)
	if e != nil {
		return Settings{}, e
	}
	return RehydrateSettings(scope, SettingsSnapshot{OrgID: scope.OrgID(), Revision: 1, EffectiveAt: at.UTC(), Values: v, Source: SettingsOnboarding, Author: SettingsAuthor{AccountID: account, MembershipID: creator}, Reason: "Organization creation", OperationID: op})
}
func RehydrateSettings(scope kernel.OrgScope, v SettingsSnapshot) (Settings, error) {
	if scope.Validate() != nil || v.OrgID != scope.OrgID() {
		return Settings{}, ErrScope
	}
	if v.Revision < 1 || !settingsInstant(v.EffectiveAt) {
		return Settings{}, ErrInvalid
	}
	if e := v.Values.Validate(); e != nil {
		return Settings{}, e
	}
	reason, e := legalName(v.Reason, true)
	if e != nil || reason != v.Reason {
		return Settings{}, settingsField("reason")
	}
	switch v.Source {
	case SettingsOnboarding, SettingsCommand:
		if !canonicalLegalID(v.Author.AccountID) || !canonicalLegalID(v.Author.MembershipID) || !canonicalLegalID(v.OperationID) || v.SourceMembershipID != "" || v.SourceMembershipRevision != 0 {
			return Settings{}, ErrInvalid
		}
		if v.Source == SettingsOnboarding && v.Revision != 1 || v.Source == SettingsCommand && v.Revision < 2 {
			return Settings{}, ErrInvalid
		}
		if v.Source == SettingsOnboarding {
			defaults, err := DefaultSettingsValues(v.Values.TimeZone, v.Author.MembershipID)
			if err != nil || defaults != v.Values {
				return Settings{}, ErrInvalid
			}
		}

	case SettingsBootstrap:
		if v.Revision != 1 || v.Author != (SettingsAuthor{}) || v.OperationID != "" || v.SourceMembershipID != "" || v.SourceMembershipRevision != 0 {
			return Settings{}, ErrInvalid
		}
		defaults, err := DefaultSettingsValues(v.Values.TimeZone, v.Values.Responsibilities.Billing.PrimaryID)
		if err != nil || defaults != v.Values {
			return Settings{}, ErrInvalid
		}

	case SettingsMembershipReduction:
		if v.Revision < 2 || v.Author != (SettingsAuthor{}) || v.OperationID != "" || !canonicalLegalID(v.SourceMembershipID) || v.SourceMembershipRevision < 2 || v.Reason != "DESIGNATION_CLEARED" {
			return Settings{}, ErrInvalid
		}
	default:
		return Settings{}, ErrInvalid
	}
	v.EffectiveAt = v.EffectiveAt.UTC()
	return Settings{value: v}, nil
}
func (s Settings) Snapshot(scope kernel.OrgScope) (SettingsSnapshot, error) {
	v, e := RehydrateSettings(scope, s.value)
	return v.value, e
}
func (s Settings) next(scope kernel.OrgScope, expected int64, at time.Time) (SettingsSnapshot, error) {
	v, e := s.Snapshot(scope)
	if e != nil {
		return SettingsSnapshot{}, e
	}
	if expected < 1 || expected != v.Revision {
		return SettingsSnapshot{}, ErrConflict
	}
	if expected == math.MaxInt64 {
		return SettingsSnapshot{}, ErrRevision
	}
	if !settingsInstant(at) || at.Before(v.EffectiveAt) {
		return SettingsSnapshot{}, ErrConditions
	}
	v.Revision++
	v.EffectiveAt = at.UTC()
	return v, nil
}
func (s Settings) Change(scope kernel.OrgScope, expected int64, values SettingsValues, author SettingsAuthor, reason, op string, at time.Time) (Settings, error) {
	v, e := s.next(scope, expected, at)
	if e != nil {
		return Settings{}, e
	}
	v.Values = values
	v.Author = author
	v.Reason = strings.TrimSpace(reason)
	v.OperationID = op
	v.Source = SettingsCommand
	v.SourceMembershipID = ""
	v.SourceMembershipRevision = 0
	return RehydrateSettings(scope, v)
}
func (s Settings) ClearMember(scope kernel.OrgScope, member string, memberRevision int64, at time.Time) (Settings, int, error) {
	current, e := s.Snapshot(scope)
	if e != nil {
		return Settings{}, 0, e
	}
	if !canonicalLegalID(member) || memberRevision < 2 {
		return Settings{}, 0, ErrInvalid
	}
	count := 0
	for _, pair := range []*ResponsibilityPair{&current.Values.Responsibilities.ReviewFallback, &current.Values.Responsibilities.PlatformNotices, &current.Values.Responsibilities.DataProtection, &current.Values.Responsibilities.Billing} {
		if pair.PrimaryID == member {
			pair.PrimaryID = ""
			count++
		}
		if pair.SubstituteID == member {
			pair.SubstituteID = ""
			count++
		}
	}
	if count == 0 {
		return s, 0, nil
	}
	v, e := s.next(scope, current.Revision, at)
	if e != nil {
		return Settings{}, 0, e
	}
	v.Values = current.Values
	v.Source = SettingsMembershipReduction
	v.Author = SettingsAuthor{}
	v.OperationID = ""
	v.Reason = "DESIGNATION_CLEARED"
	v.SourceMembershipID = member
	v.SourceMembershipRevision = memberRevision
	result, e := RehydrateSettings(scope, v)
	return result, count, e
}

func CanonicalSettingsReason(raw string) (string, error) {
	v, e := legalName(raw, true)
	if e != nil {
		return "", settingsField("reason")
	}
	return v, nil
}

// ParseSettingsClockSkew preserves the source's exact nonnegative int64 bound
// across JSON clients, including runtimes whose numbers are IEEE-754 doubles.
func ParseSettingsClockSkew(raw string) (int64, error) {
	value, e := strconv.ParseInt(raw, 10, 64)
	if e != nil || value < 0 || strconv.FormatInt(value, 10) != raw {
		return 0, settingsField("policies.clock_skew_limit_seconds")
	}
	return value, nil
}
