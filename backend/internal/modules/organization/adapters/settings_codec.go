package adapters

import (
	"encoding/json"
	"reflect"
	"strconv"

	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// Explicit boundary representation: domain objects redact generic JSON/logging.
// No field is optional; an explicit false/zero/empty reference is meaningful.
type settingsWire struct {
	TimeZone           string               `json:"time_zone"`
	ConsumptionDisplay d.ConsumptionDisplay `json:"consumption_display"`
	ReviewRequired     struct {
		Trips    bool `json:"trips"`
		Fuelings bool `json:"fuelings"`
		Expenses bool `json:"expenses"`
	} `json:"review_required"`
	ApprovalLevelsDefault int `json:"approval_levels_default"`
	ClosurePeriodDays     int `json:"closure_period_days"`
	Policies              struct {
		IntakeWindowDays        int    `json:"intake_window_days"`
		ClockSkewLimitSeconds   string `json:"clock_skew_limit_seconds"`
		DelegationMaximumDays   int    `json:"delegation_maximum_days"`
		InvitationValidityDays  int    `json:"invitation_validity_days"`
		ActivationValidityHours int    `json:"activation_validity_hours"`
	} `json:"policies"`
	Responsibilities struct {
		ReviewFallback  settingsPairWire `json:"review_fallback"`
		PlatformNotices settingsPairWire `json:"platform_notices"`
		DataProtection  settingsPairWire `json:"data_protection"`
		Billing         settingsPairWire `json:"billing"`
	} `json:"responsibilities"`
}
type settingsPairWire struct {
	Primary    string `json:"primary"`
	Substitute string `json:"substitute"`
}

func EncodeSettingsValues(v d.SettingsValues) ([]byte, error) {
	if e := v.Validate(); e != nil {
		return nil, e
	}
	w := settingsWire{TimeZone: v.TimeZone, ConsumptionDisplay: v.ConsumptionDisplay, ApprovalLevelsDefault: v.ApprovalLevelsDefault, ClosurePeriodDays: v.ClosurePeriodDays}
	w.ReviewRequired.Trips = v.ReviewRequired.Trips
	w.ReviewRequired.Fuelings = v.ReviewRequired.Fuelings
	w.ReviewRequired.Expenses = v.ReviewRequired.Expenses
	w.Policies.IntakeWindowDays = v.Policies.IntakeWindowDays
	w.Policies.ClockSkewLimitSeconds = strconv.FormatInt(v.Policies.ClockSkewLimitSeconds, 10)
	w.Policies.DelegationMaximumDays = v.Policies.DelegationMaximumDays
	w.Policies.InvitationValidityDays = v.Policies.InvitationValidityDays
	w.Policies.ActivationValidityHours = v.Policies.ActivationValidityHours
	pair := func(p d.ResponsibilityPair) settingsPairWire { return settingsPairWire{p.PrimaryID, p.SubstituteID} }
	w.Responsibilities.ReviewFallback = pair(v.Responsibilities.ReviewFallback)
	w.Responsibilities.PlatformNotices = pair(v.Responsibilities.PlatformNotices)
	w.Responsibilities.DataProtection = pair(v.Responsibilities.DataProtection)
	w.Responsibilities.Billing = pair(v.Responsibilities.Billing)
	return json.Marshal(w)
}
func DecodeSettingsValues(raw []byte) (d.SettingsValues, error) {
	var w settingsWire
	if httpapi.DecodeJSON(raw, &w) != nil {
		return d.SettingsValues{}, d.ErrInvalid
	}
	skew, e := d.ParseSettingsClockSkew(w.Policies.ClockSkewLimitSeconds)
	if e != nil {
		return d.SettingsValues{}, e
	}
	pair := func(p settingsPairWire) d.ResponsibilityPair {
		return d.ResponsibilityPair{PrimaryID: p.Primary, SubstituteID: p.Substitute}
	}
	v := d.SettingsValues{TimeZone: w.TimeZone, ConsumptionDisplay: w.ConsumptionDisplay, ReviewRequired: d.ReviewRequired{Trips: w.ReviewRequired.Trips, Fuelings: w.ReviewRequired.Fuelings, Expenses: w.ReviewRequired.Expenses}, ApprovalLevelsDefault: w.ApprovalLevelsDefault, ClosurePeriodDays: w.ClosurePeriodDays, Policies: d.SourcePolicies{IntakeWindowDays: w.Policies.IntakeWindowDays, ClockSkewLimitSeconds: skew, DelegationMaximumDays: w.Policies.DelegationMaximumDays, InvitationValidityDays: w.Policies.InvitationValidityDays, ActivationValidityHours: w.Policies.ActivationValidityHours}, Responsibilities: d.Responsibilities{ReviewFallback: pair(w.Responsibilities.ReviewFallback), PlatformNotices: pair(w.Responsibilities.PlatformNotices), DataProtection: pair(w.Responsibilities.DataProtection), Billing: pair(w.Responsibilities.Billing)}}
	canonical, e := EncodeSettingsValues(v)
	if e != nil {
		return d.SettingsValues{}, e
	}
	var input, output any
	if json.Unmarshal(raw, &input) != nil || json.Unmarshal(canonical, &output) != nil || !reflect.DeepEqual(input, output) {
		return d.SettingsValues{}, d.ErrInvalid
	}
	return v, nil
}
