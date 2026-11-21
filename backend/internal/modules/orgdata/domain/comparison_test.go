package domain

import (
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

func TestMarketComparisonPreservesCoverageAndExactStationMean(t *testing.T) {
	base := MarketBasis{Region: "MT", Method: "survey-v1", Start: "2026-10-01", End: "2026-10-07", MinimumStations: 5,
		Sample: Sample{Fuel: kernel.GasolineRegular, Unit: "L", Stations: 5, Observations: 7,
			SumMilliBRL: 30000, MinimumMilliBRL: 5000, MaximumMilliBRL: 7000}}
	ref := base
	ref.Region = "PR"
	ref.Sample.Stations, ref.Sample.Observations, ref.Sample.SumMilliBRL = 6, 12, 36001
	if got := CompareMarket(base, ref); got != (MarketDifference{Readiness: ComparisonAvailable, MilliBRLPerUnit: "1/6"}) {
		t.Fatalf("geographic comparison lost the exact station basis: %+v", got)
	}
	ref.Region, ref.Start, ref.End = "MT", "2026-10-08", "2026-10-14"
	ref.Sample.SumMilliBRL -= 2
	if got := CompareMarket(base, ref); got != (MarketDifference{Readiness: ComparisonAvailable, MilliBRLPerUnit: "-1/6"}) {
		t.Fatalf("temporal comparison lost the signed change: %+v", got)
	}
	for _, tc := range []struct {
		edit func(*MarketBasis)
		want ComparisonReadiness
	}{
		{func(v *MarketBasis) { v.Region = "PR" }, ComparisonIncomparable},
		{func(v *MarketBasis) { v.Start = "2026-10-07" }, ComparisonIncomparable},
		{func(v *MarketBasis) { v.Method = "survey-v2" }, ComparisonIncomparable},
		{func(v *MarketBasis) { v.Sample.Fuel = kernel.Ethanol }, ComparisonIncomparable},
		{func(v *MarketBasis) { v.MinimumStations = 7 }, ComparisonInsufficient},
		{func(v *MarketBasis) { v.Sample = Sample{} }, ComparisonInsufficient},
	} {
		candidate := ref
		tc.edit(&candidate)
		if got := CompareMarket(base, candidate); got.Readiness != tc.want || got.MilliBRLPerUnit != "" {
			t.Fatalf("unsupported comparison disclosed a difference: got %+v, want %s", got, tc.want)
		}
	}
}
