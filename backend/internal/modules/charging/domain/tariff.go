// Package charging owns the pure electric-charging tariff domain (EE-11).
//
// Exact tariff + declared scenario + known eligibility -> a priced scenario or
// an explicit UNKNOWN carrying a frozen reason. No network, SQL, entitlement,
// provider call or float arithmetic happens here. Rates are exact rational
// centavos per base unit; quantities are rounded up to their declared billing
// step only after the applicable grace, and the single final rounding is the
// tariff's declared half-up-centavo boundary.
package charging

import (
	"math/big"
	"regexp"
	"time"
	"unicode/utf8"
)

const (
	TariffVersion = 1

	MaxEnergyWh    int64 = 1_000_000_000
	MaxSeconds     int64 = 31_622_400
	MaxCentavos    int64 = 1_000_000_000_000
	MaxBillingStep int64 = 1_000_000_000
	MaxComponents        = 32
)

// Rational is the frozen canonical reduced nonnegative wire value.
type Rational struct {
	Numerator   string `json:"numerator"`
	Denominator string `json:"denominator"`
}

// TariffComponent mirrors TariffComponent in the frozen EV schema.
type TariffComponent struct {
	Dimension    string   `json:"dimension"`
	RateCentavos Rational `json:"rate_centavos"`
	BillingStep  int64    `json:"billing_step"`
	GraceSeconds int64    `json:"grace_seconds"`
}

// Tariff mirrors the frozen Tariff schema. Source eligibility, steps, grace,
// tax, minimum/maximum and the actual paid amount stay distinct inputs.
type Tariff struct {
	Version                     int               `json:"version"`
	ID                          string            `json:"id"`
	Revision                    string            `json:"revision"`
	TargetKind                  string            `json:"target_kind"`
	TargetID                    string            `json:"target_id"`
	Currency                    string            `json:"currency"`
	Condition                   string            `json:"condition"`
	QualifierID                 *string           `json:"qualifier_id"`
	Timezone                    string            `json:"timezone"`
	ValidFrom                   time.Time         `json:"valid_from"`
	ValidUntil                  *time.Time        `json:"valid_until"`
	TaxBasis                    string            `json:"tax_basis"`
	MandatoryComponentsComplete bool              `json:"mandatory_components_complete"`
	Components                  []TariffComponent `json:"components"`
	MinimumCentavos             *int64            `json:"minimum_centavos"`
	MaximumCentavos             *int64            `json:"maximum_centavos"`
	BillingRounding             string            `json:"billing_rounding"`
	Provenance                  string            `json:"provenance"`
}

// Scenario is the declared session being priced. Nil quantities mean the
// caller does not know the amount; the engine refuses instead of using zero.
type Scenario struct {
	Condition       string    `json:"condition"`
	QualifierID     *string   `json:"qualifier_id"`
	WindowStart     time.Time `json:"window_start"`
	WindowEnd       time.Time `json:"window_end"`
	EnergyWh        *int64    `json:"energy_wh"`
	ChargingSeconds *int64    `json:"charging_seconds"`
	IdleSeconds     *int64    `json:"idle_seconds"`
	SessionCount    *int64    `json:"session_count"`
	ParkingSeconds  *int64    `json:"parking_seconds"`
}

// Estimate is a known CENTAVOS total or an explicit UNKNOWN with null value.
type Estimate struct {
	State  string    `json:"state"`
	Method string    `json:"method,omitempty"`
	Unit   string    `json:"unit,omitempty"`
	Reason string    `json:"reason,omitempty"`
	Value  *Rational `json:"value"`
}

// ValidationError names a refused input shape. It is never a priced result.
type ValidationError struct {
	Code string
}

func (e *ValidationError) Error() string { return e.Code }

var (
	numeratorPattern   = regexp.MustCompile(`^(0|[1-9][0-9]{0,11}|1000000000000)$`)
	denominatorPattern = regexp.MustCompile(`^([1-9][0-9]{0,8}|1000000000)$`)
	currencyPattern    = regexp.MustCompile(`^[A-Z]{3}$`)
	uuidPattern        = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

var dimensions = map[string]bool{
	"ENERGY_WH":       true,
	"CHARGING_SECOND": true,
	"IDLE_SECOND":     true,
	"SESSION":         true,
	"PARKING_SECOND":  true,
}

var timeDimensions = map[string]bool{
	"CHARGING_SECOND": true,
	"IDLE_SECOND":     true,
	"PARKING_SECOND":  true,
}

func invalid(code string) error { return &ValidationError{Code: code} }

// Calculate prices scenario against tariff. The precedence is fixed so that
// shared vectors are deterministic: invalid-tariff, invalid-scenario,
// ineligible-condition, UNSUPPORTED_CURRENCY, UNKNOWN_FEES,
// MISMATCHED_WINDOW, missing-scenario-quantity, then exact arithmetic and
// OVERFLOW at the money boundary.
func Calculate(t Tariff, s Scenario) (Estimate, error) {
	if err := validateTariff(t); err != nil {
		return Estimate{}, err
	}
	if err := validateScenario(s); err != nil {
		return Estimate{}, err
	}
	if t.Condition == "UNKNOWN" || s.Condition == "UNKNOWN" ||
		t.Condition != s.Condition || !sameQualifier(t.QualifierID, s.QualifierID) {
		return Estimate{}, invalid("ineligible-condition")
	}
	if t.Currency != "BRL" {
		return unknown("UNSUPPORTED_CURRENCY"), nil
	}
	// Only INCLUDED tax, complete mandatory components, no undeclared
	// minimum/maximum stage and the supported final rounding yield a total.
	if t.TaxBasis != "INCLUDED" || !t.MandatoryComponentsComplete ||
		t.MinimumCentavos != nil || t.MaximumCentavos != nil ||
		t.BillingRounding != "FINAL_HALF_UP_CENTAVO" {
		return unknown("UNKNOWN_FEES"), nil
	}
	if s.WindowStart.Before(t.ValidFrom) ||
		(t.ValidUntil != nil && s.WindowEnd.After(*t.ValidUntil)) {
		return unknown("MISMATCHED_WINDOW"), nil
	}

	// Validate every declared quantity before deciding completeness so a
	// malformed scenario is refused rather than reported as unknown fees.
	quantities := make([]int64, len(t.Components))
	hasEnergy := false
	for index, component := range t.Components {
		quantity, err := chargedQuantity(component, s)
		if err != nil {
			return Estimate{}, err
		}
		quantities[index] = quantity
		if component.Dimension == "ENERGY_WH" {
			hasEnergy = true
		}
	}
	// An absent energy price is not a zero price.
	if !hasEnergy {
		return unknown("UNKNOWN_FEES"), nil
	}

	sum := new(big.Rat)
	for index, component := range t.Components {
		rate, err := parseRate(component.RateCentavos)
		if err != nil {
			return Estimate{}, err
		}
		cost := new(big.Rat).Mul(rate, new(big.Rat).SetInt64(quantities[index]))
		sum.Add(sum, cost)
	}

	rounded := halfUpCentavos(sum)
	if rounded.Cmp(big.NewInt(MaxCentavos)) > 0 {
		return unknown("OVERFLOW"), nil
	}
	return Estimate{
		State:  "KNOWN",
		Method: "TARIFF_SCENARIO",
		Unit:   "CENTAVOS",
		Value: &Rational{
			Numerator:   rounded.String(),
			Denominator: "1",
		},
	}, nil
}

func unknown(reason string) Estimate {
	return Estimate{State: "UNKNOWN", Reason: reason, Value: nil}
}

func sameQualifier(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func validateTariff(t Tariff) error {
	if t.Version != TariffVersion || !uuidPattern.MatchString(t.ID) ||
		!uuidPattern.MatchString(t.TargetID) || !boundedText(t.Revision) ||
		!boundedText(t.Timezone) || t.ValidFrom.IsZero() {
		return invalid("invalid-tariff")
	}
	switch t.TargetKind {
	case "LOCATION", "EVSE", "CONNECTOR":
	default:
		return invalid("invalid-tariff")
	}
	if !currencyPattern.MatchString(t.Currency) {
		return invalid("invalid-tariff")
	}
	if err := validateCondition(t.Condition, t.QualifierID, "invalid-tariff"); err != nil {
		return err
	}
	if t.ValidUntil != nil && t.ValidUntil.Before(t.ValidFrom) {
		return invalid("invalid-tariff")
	}
	switch t.TaxBasis {
	case "INCLUDED", "EXCLUDED", "UNKNOWN":
	default:
		return invalid("invalid-tariff")
	}
	switch t.Provenance {
	case "LICENSED_OPERATOR", "DATED_COMMUNITY_REPORT", "USER_ESTIMATE":
	default:
		return invalid("invalid-tariff")
	}
	switch t.BillingRounding {
	case "FINAL_HALF_UP_CENTAVO", "SOURCE_SPECIFIC_UNSUPPORTED":
	default:
		return invalid("invalid-tariff")
	}
	if len(t.Components) < 1 || len(t.Components) > MaxComponents {
		return invalid("invalid-tariff")
	}
	seen := make(map[string]bool, len(t.Components))
	for _, component := range t.Components {
		if !dimensions[component.Dimension] || seen[component.Dimension] {
			return invalid("invalid-tariff")
		}
		seen[component.Dimension] = true
		if _, err := parseRate(component.RateCentavos); err != nil {
			return err
		}
		if component.BillingStep < 1 || component.BillingStep > MaxBillingStep ||
			component.GraceSeconds < 0 || component.GraceSeconds > MaxSeconds {
			return invalid("invalid-tariff")
		}
		if component.GraceSeconds > 0 && !timeDimensions[component.Dimension] {
			return invalid("invalid-tariff")
		}
		if component.Dimension == "SESSION" && component.BillingStep != 1 {
			return invalid("invalid-tariff")
		}
	}
	if t.MinimumCentavos != nil && (*t.MinimumCentavos < 0 || *t.MinimumCentavos > MaxCentavos) {
		return invalid("invalid-tariff")
	}
	if t.MaximumCentavos != nil && (*t.MaximumCentavos < 0 || *t.MaximumCentavos > MaxCentavos) {
		return invalid("invalid-tariff")
	}
	if t.MinimumCentavos != nil && t.MaximumCentavos != nil &&
		*t.MinimumCentavos > *t.MaximumCentavos {
		return invalid("invalid-tariff")
	}
	return nil
}

func validateScenario(s Scenario) error {
	if s.WindowStart.IsZero() || s.WindowEnd.IsZero() || !s.WindowEnd.After(s.WindowStart) {
		return invalid("invalid-scenario")
	}
	for _, declared := range []struct {
		dimension string
		quantity  *int64
	}{
		{"ENERGY_WH", s.EnergyWh}, {"CHARGING_SECOND", s.ChargingSeconds},
		{"IDLE_SECOND", s.IdleSeconds}, {"SESSION", s.SessionCount}, {"PARKING_SECOND", s.ParkingSeconds},
	} {
		if declared.quantity != nil {
			if err := validateQuantity(declared.dimension, *declared.quantity); err != nil {
				return err
			}
		}
	}
	return validateCondition(s.Condition, s.QualifierID, "invalid-scenario")
}

// OpenAPI maxLength counts Unicode scalar values, not UTF-8 bytes.
func boundedText(s string) bool {
	return s != "" && utf8.ValidString(s) && utf8.RuneCountInString(s) <= 128
}

func validateCondition(condition string, qualifier *string, code string) error {
	switch condition {
	case "STANDARD":
		if qualifier != nil {
			return invalid(code)
		}
	case "APP", "MEMBER", "EMPLOYER":
		if qualifier == nil || !boundedText(*qualifier) {
			return invalid(code)
		}
	case "UNKNOWN":
		if qualifier != nil {
			return invalid(code)
		}
	default:
		return invalid(code)
	}
	return nil
}

func parseRate(value Rational) (*big.Rat, error) {
	if !numeratorPattern.MatchString(value.Numerator) ||
		!denominatorPattern.MatchString(value.Denominator) {
		return nil, invalid("invalid-tariff")
	}
	numerator, ok := new(big.Int).SetString(value.Numerator, 10)
	if !ok {
		return nil, invalid("invalid-tariff")
	}
	denominator, ok := new(big.Int).SetString(value.Denominator, 10)
	if !ok || denominator.Sign() <= 0 {
		return nil, invalid("invalid-tariff")
	}
	// Canonical reduced values only; 0/1 is the explicit zero price.
	if new(big.Int).GCD(nil, nil, numerator, denominator).Cmp(big.NewInt(1)) != 0 {
		return nil, invalid("invalid-tariff")
	}
	return new(big.Rat).SetFrac(numerator, denominator), nil
}

func chargedQuantity(component TariffComponent, s Scenario) (int64, error) {
	var quantity *int64
	switch component.Dimension {
	case "ENERGY_WH":
		quantity = s.EnergyWh
	case "CHARGING_SECOND":
		quantity = s.ChargingSeconds
	case "IDLE_SECOND":
		quantity = s.IdleSeconds
	case "SESSION":
		quantity = s.SessionCount
	case "PARKING_SECOND":
		quantity = s.ParkingSeconds
	}
	if quantity == nil {
		return 0, invalid("missing-scenario-quantity")
	}
	value := *quantity
	if err := validateQuantity(component.Dimension, value); err != nil {
		return 0, err
	}
	if component.Dimension == "SESSION" {
		return 1, nil
	}
	if timeDimensions[component.Dimension] && component.GraceSeconds > 0 {
		if value <= component.GraceSeconds {
			return 0, nil
		}
		value -= component.GraceSeconds
	}
	if value == 0 {
		return 0, nil
	}
	if remainder := value % component.BillingStep; remainder != 0 {
		value = (value/component.BillingStep + 1) * component.BillingStep
	}
	return value, nil
}

func validateQuantity(dimension string, value int64) error {
	switch dimension {
	case "ENERGY_WH":
		if value < 0 || value > MaxEnergyWh {
			return invalid("invalid-scenario")
		}
	case "SESSION":
		if value != 1 {
			return invalid("invalid-scenario")
		}
	default:
		if value < 0 || value > MaxSeconds {
			return invalid("invalid-scenario")
		}
	}
	return nil
}

// halfUpCentavos is floor(sum + 1/2) for a nonnegative exact sum.
func halfUpCentavos(sum *big.Rat) *big.Int {
	numerator := new(big.Int).Lsh(sum.Num(), 1)
	numerator.Add(numerator, sum.Denom())
	denominator := new(big.Int).Lsh(sum.Denom(), 1)
	return new(big.Int).Quo(numerator, denominator)
}
