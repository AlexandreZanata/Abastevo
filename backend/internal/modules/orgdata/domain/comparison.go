package domain

import (
	"math/big"
	"time"
)

type ComparisonReadiness string

const (
	ComparisonAvailable    ComparisonReadiness = "AVAILABLE"
	ComparisonIncomparable ComparisonReadiness = "INCOMPARABLE_BASIS"
	ComparisonInsufficient ComparisonReadiness = "INSUFFICIENT_COVERAGE"
)

type MarketBasis struct {
	Region, Method, Start, End string
	MinimumStations            int
	Sample                     Sample
}

type MarketDifference struct {
	Readiness       ComparisonReadiness `json:"readiness"`
	MilliBRLPerUnit string              `json:"difference_milli_brl_per_unit,omitempty"`
}

// CompareMarket calculates the difference of equally weighted station means
// between two already admitted surveys. It reports coverage separately from
// comparability and never rounds the difference to the presentation unit.
func CompareMarket(base, reference MarketBasis) MarketDifference {
	if base.MinimumStations < 5 || reference.MinimumStations < 5 ||
		!base.Sample.Valid(base.MinimumStations) || !reference.Sample.Valid(reference.MinimumStations) {
		return MarketDifference{Readiness: ComparisonInsufficient}
	}
	if !base.comparable(reference) {
		return MarketDifference{Readiness: ComparisonIncomparable}
	}
	before := big.NewRat(base.Sample.SumMilliBRL, int64(base.Sample.Stations))
	after := big.NewRat(reference.Sample.SumMilliBRL, int64(reference.Sample.Stations))
	return MarketDifference{Readiness: ComparisonAvailable, MilliBRLPerUnit: after.Sub(after, before).RatString()}
}

func (b MarketBasis) comparable(other MarketBasis) bool {
	if !marketState(b.Region) || !marketState(other.Region) || b.Method == "" || b.Method != other.Method ||
		b.Sample.Fuel != other.Sample.Fuel || b.Sample.Unit != other.Sample.Unit ||
		!validMarketPeriod(b.Start, b.End) || !validMarketPeriod(other.Start, other.End) {
		return false
	}
	if b.Region == other.Region {
		return b.End < other.Start
	}
	return b.Start == other.Start && b.End == other.End
}

func validMarketPeriod(start, end string) bool {
	startDay, startErr := time.Parse(surveyDate, start)
	endDay, endErr := time.Parse(surveyDate, end)
	return startErr == nil && endErr == nil && !endDay.Before(startDay)
}
