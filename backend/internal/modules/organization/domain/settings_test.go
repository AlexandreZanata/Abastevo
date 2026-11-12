package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestSettingsDefaultsImmutableCaptureAndSubstituteCleanup(t *testing.T) {
	scope := teamScope(t, "7b0e2a54-0000-4000-8000-000000000301")
	creator := "7b0e2a54-0000-4000-8000-000000000601"
	account := "7b0e2a54-0000-4000-8000-000000000401"
	op := "7b0e2a54-0000-4000-8000-000000001008"
	at := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	first, e := NewOnboardingSettings(scope, "America/Cuiaba", creator, account, op, at)
	if e != nil {
		t.Fatal(e)
	}
	v, _ := first.Snapshot(scope)
	if !v.Values.ReviewRequired.Trips || !v.Values.ReviewRequired.Fuelings || !v.Values.ReviewRequired.Expenses || v.Values.ApprovalLevelsDefault != 1 || v.Values.ConsumptionDisplay != ConsumptionKMPerL || v.Values.Responsibilities.Billing.PrimaryID != creator || v.Values.Policies.IntakeWindowDays != 15 || v.Values.Policies.DelegationMaximumDays != 90 {
		t.Fatal("unsafe defaults")
	}
	substitute := "7b0e2a54-0000-4000-8000-000000000602"
	values := v.Values
	values.TimeZone = "Pacific/Auckland"
	values.ConsumptionDisplay = ConsumptionLPer100KM
	values.Responsibilities.ReviewFallback = ResponsibilityPair{PrimaryID: creator, SubstituteID: substitute}
	changed, e := first.Change(scope, 1, values, SettingsAuthor{AccountID: account, MembershipID: creator}, "Synthetic private reason", op, at.Add(time.Second))
	if e != nil {
		t.Fatal(e)
	}
	if _, e := changed.Change(scope, 1, values, SettingsAuthor{AccountID: account, MembershipID: creator}, "stale", op, at); e != ErrConflict {
		t.Fatal("stale", e)
	}
	if _, e := changed.Change(scope, 2, values, SettingsAuthor{AccountID: account, MembershipID: creator}, "clock", op, at); e != ErrConditions {
		t.Fatal("regressed effective time", e)
	}
	old, _ := first.Snapshot(scope)
	if old.Values.TimeZone != "America/Cuiaba" || old.Revision != 1 {
		t.Fatal("old settings changed")
	}
	cleared, changedSlots, e := changed.ClearMember(scope, substitute, 3, at.Add(2*time.Second))
	if e != nil || changedSlots != 1 {
		t.Fatal("substitute cleanup", changedSlots, e)
	}
	after, _ := cleared.Snapshot(scope)
	if after.Values.Responsibilities.ReviewFallback.PrimaryID != creator || after.Values.Responsibilities.ReviewFallback.SubstituteID != "" || after.Source != SettingsMembershipReduction || after.SourceMembershipID != substitute || after.Author.AccountID != "" || after.Revision != 3 {
		t.Fatal("cleanup rewrote unaffected slot or fabricated actor")
	}
	if _, count, e := cleared.ClearMember(scope, substitute, 4, at.Add(3*time.Second)); e != nil || count != 0 {
		t.Fatal("cleanup repeated", e)
	}
	for _, value := range []any{changed, v, values, after} {
		raw, _ := json.Marshal(value)
		if strings.Contains(fmt.Sprintf("%+v %s", value, raw), "Synthetic private") {
			t.Fatal("private settings formatting")
		}
	}
}
func TestSettingsRejectUnregisteredPolicyBoundsAndDuplicateRouting(t *testing.T) {
	values, e := DefaultSettingsValues("UTC", "")
	if e != nil {
		t.Fatal(e)
	}
	cases := []struct {
		field  string
		change func(*SettingsValues)
	}{
		{"time_zone", func(v *SettingsValues) { v.TimeZone = "GMT" }},
		{"consumption_display", func(v *SettingsValues) { v.ConsumptionDisplay = "L" }},
		{"approval_levels_default", func(v *SettingsValues) { v.ApprovalLevelsDefault = 4 }},
		{"closure_period_days", func(v *SettingsValues) { v.ClosurePeriodDays = 6 }},
		{"policies.intake_window_days", func(v *SettingsValues) { v.Policies.IntakeWindowDays = 61 }},
		{"policies.clock_skew_limit_seconds", func(v *SettingsValues) { v.Policies.ClockSkewLimitSeconds = -1 }},
		{"policies.delegation_maximum_days", func(v *SettingsValues) { v.Policies.DelegationMaximumDays = 181 }},
		{"policies.invitation_validity_days", func(v *SettingsValues) { v.Policies.InvitationValidityDays = 0 }},
		{"policies.activation_validity_hours", func(v *SettingsValues) { v.Policies.ActivationValidityHours = 169 }},
		{"responsibilities.billing.substitute", func(v *SettingsValues) {
			v.Responsibilities.Billing = ResponsibilityPair{PrimaryID: "7b0e2a54-0000-4000-8000-000000000601", SubstituteID: "7b0e2a54-0000-4000-8000-000000000601"}
		}},
	}
	for _, c := range cases {
		v := values
		c.change(&v)
		e := v.Validate()
		var field interface{ Field() string }
		if e == nil {
			t.Fatal("invalid settings accepted", c.field)
		}
		field, _ = e.(interface{ Field() string })
		if field == nil || field.Field() != c.field {
			t.Fatal("unbounded or wrong field refusal", c.field)
		}
	}
}

func TestSettingsClockPrecisionAndRejectedVersionSources(t *testing.T) {
	for raw, want := range map[string]int64{"0": 0, "300": 300, "9223372036854775807": 9223372036854775807} {
		got, e := ParseSettingsClockSkew(raw)
		if e != nil || got != want {
			t.Fatal("exact clock bound", e)
		}
	}
	for _, raw := range []string{"", "-1", "-0", "+1", "01", "1.0", "1e3", "9223372036854775808", " 300"} {
		if _, e := ParseSettingsClockSkew(raw); SettingsField(e) != "policies.clock_skew_limit_seconds" {
			t.Fatal("noncanonical or overflowing clock admitted")
		}
	}
	scope := teamScope(t, "7b0e2a54-0000-4000-8000-000000000301")
	other := teamScope(t, "7b0e2a54-0000-4000-8000-000000000302")
	member, account, op := "7b0e2a54-0000-4000-8000-000000000601", "7b0e2a54-0000-4000-8000-000000000401", "7b0e2a54-0000-4000-8000-000000001008"
	at := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	first, e := NewOnboardingSettings(scope, "UTC", member, account, op, at)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := first.Snapshot(other); e != ErrScope {
		t.Fatal("foreign version readable", e)
	}
	v, _ := first.Snapshot(scope)
	for _, change := range []func(*SettingsSnapshot){func(v *SettingsSnapshot) { v.Revision = 0 }, func(v *SettingsSnapshot) { v.EffectiveAt = time.Time{} }, func(v *SettingsSnapshot) { v.Source = "UNKNOWN" }, func(v *SettingsSnapshot) { v.Author.AccountID = "" }, func(v *SettingsSnapshot) { v.SourceMembershipID = member }, func(v *SettingsSnapshot) { v.Values.ReviewRequired.Trips = false }, func(v *SettingsSnapshot) { v.Source = SettingsCommand }, func(v *SettingsSnapshot) { v.Source = SettingsBootstrap }, func(v *SettingsSnapshot) { v.Reason = " spaced " }} {
		bad := v
		change(&bad)
		if _, e := RehydrateSettings(scope, bad); e == nil {
			t.Fatal("invalid historical source admitted")
		}
	}
	bootstrap := v
	bootstrap.Source = SettingsBootstrap
	bootstrap.Author = SettingsAuthor{}
	bootstrap.OperationID = ""
	if _, e := RehydrateSettings(scope, bootstrap); e != nil {
		t.Fatal("honest system bootstrap rejected", e)
	}
	if _, _, e := first.ClearMember(scope, member, 1, at); e == nil {
		t.Fatal("unversioned reduction cause admitted")
	}
	equal, e := first.Change(scope, 1, v.Values, v.Author, "reason", op, at)
	if e != nil {
		t.Fatal("equal server timestamps require revision ordering", e)
	}
	max, _ := equal.Snapshot(scope)
	max.Revision = 9223372036854775807
	limit, e := RehydrateSettings(scope, max)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := limit.Change(scope, max.Revision, max.Values, max.Author, "overflow", op, at); e != ErrRevision {
		t.Fatal("version overflow", e)
	}
	for _, kind := range []Responsibility{ReviewResponsibility, PlatformResponsibility, PrivacyResponsibility, BillingResponsibility} {
		if _, e := v.Values.Responsibilities.Pair(kind); e != nil {
			t.Fatal("registered responsibility", e)
		}
	}
	if _, e := v.Values.Responsibilities.Pair("UNKNOWN"); e != ErrInvalid {
		t.Fatal("unregistered responsibility", e)
	}
}
