package domain

import (
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/metrics"
	"testing"
)

func TestConsumptionPresentationChangesNoBaseFactsOrEligibility(t *testing.T) {
	fill := metrics.Fill{QuantityMilli: 51500, TotalCentavos: 30000, Full: true, Fuel: "DIESEL_S10", Unit: "L"}
	facts, e := (metrics.Interval{StartingFillExcluded: fill, Fills: []metrics.Fill{fill}, DistanceMetres: 412000}).Compute()
	if e != nil || facts.Unknown != "" {
		t.Fatal(e)
	}
	before := facts
	km, e := PresentConsumption(facts.KmPerUnit, "KM_PER_L", "KM_PER_L", facts.Unknown)
	if e != nil || km.Value != "8" || km.Unit != "KM_PER_L" {
		t.Fatal("km/L presentation", e)
	}
	hundred, e := PresentConsumption(facts.KmPerUnit, "KM_PER_L", "L_PER_100KM", facts.Unknown)
	if e != nil || hundred.Value != "25/2" || hundred.Unit != "L_PER_100KM" || facts != before {
		t.Fatal("base values rewritten", e)
	}
	hours, e := PresentConsumption("13/2", "L_PER_H", "L_PER_100KM", "")
	if e != nil || hours.Value != "13/2" || hours.Unit != "L_PER_H" {
		t.Fatal("hours became distance", e)
	}
	unknown, e := PresentConsumption("", "KM_PER_L", "L_PER_100KM", "full-anchors-required")
	if e != nil || unknown.Value != "" || unknown.Unknown != "full-anchors-required" {
		t.Fatal("guessed consumption", e)
	}
	for _, raw := range []string{"0", "-1", "1e100000000", "1/0", "not a ratio"} {
		if _, e := PresentConsumption(raw, "KM_PER_L", "KM_PER_L", ""); e != ErrInvalid {
			t.Fatal("invalid ratio accepted")
		}
	}
}
