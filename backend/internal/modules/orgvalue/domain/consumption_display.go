package domain

import (
	"math/big"
	"regexp"
)

var consumptionRatio = regexp.MustCompile(`^[0-9]{1,32}(/[1-9][0-9]{0,31})?$`)

type ConsumptionPresentation struct{ Value, Unit, Unknown string }

// PresentConsumption renders an already eligible owner-computed ratio only.
// It neither determines full-to-full/hour eligibility nor changes recorded
// facts. Unknown results remain unknown, and hour-based machines stay L/hour.
func PresentConsumption(base, basis, preference, unknown string) (ConsumptionPresentation, error) {
	if preference != "KM_PER_L" && preference != "L_PER_100KM" || basis != "KM_PER_L" && basis != "L_PER_H" {
		return ConsumptionPresentation{}, ErrInvalid
	}
	if unknown != "" {
		return ConsumptionPresentation{Unknown: unknown}, nil
	}
	if len(base) > 65 || !consumptionRatio.MatchString(base) {
		return ConsumptionPresentation{}, ErrInvalid
	}
	value, ok := new(big.Rat).SetString(base)
	if !ok || value.Sign() <= 0 {
		return ConsumptionPresentation{}, ErrInvalid
	}
	unit := basis
	if basis == "KM_PER_L" && preference == "L_PER_100KM" {
		value.Quo(big.NewRat(100, 1), value)
		unit = preference
	}
	return ConsumptionPresentation{Value: value.RatString(), Unit: unit}, nil
}
