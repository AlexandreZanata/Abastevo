package domain

import (
	"math/big"
	"strings"
)

const PriceMethodologyVersion = "price-comparison-v1"

type PriceComparisonInput struct {
	FuelingProduct      string
	FuelingUnit         string
	PlaceKnown          bool
	PlaceRule           string // SAME_STATION or SAME_MUNICIPALITY
	FuelingCondition    string // STANDARD, CASH, DEBIT, CREDIT, APP, LOYALTY, OTHER
	QuantityMilli       int64
	PaidCents           int64
	HasReference        bool
	ReferenceSource     string
	ReferenceProduct    string
	ReferenceUnit       string
	ReferenceCondition  string
	ReferencePriceMilli int64
	ReferenceAgeDays    int
	MaxAgeDays          int
	CoverageStations    int
	MinCoverageStations int
}

type PriceComparisonResult struct {
	Status                 string
	Reason                 InsufficiencyReason
	MethodologyVersion     string
	ReferenceSource        string
	PlaceRule              string
	EffectiveUnitMilli     int64
	ReferenceUnitMilli     int64
	ReferenceCents         int64
	DifferenceCents        int64
	DifferencePerUnitMilli int64
	DisplayNote            string
}

func validCondition(c string) bool {
	switch c {
	case "STANDARD", "CASH", "DEBIT", "CREDIT", "APP", "LOYALTY", "OTHER":
		return true
	}
	return false
}

// EvaluatePriceComparison implements §10.6 price-comparison-v1 without mixing
// reference sources or claiming proven loss or savings (VAL-15, EMP-KPI-005).
func EvaluatePriceComparison(in PriceComparisonInput) PriceComparisonResult {
	effUnit := int64(0)
	if in.QuantityMilli > 0 && in.PaidCents >= 0 {
		effUnit = roundHalfUpBig(new(big.Int).Mul(big.NewInt(in.PaidCents), big.NewInt(10000)), big.NewInt(in.QuantityMilli))
	}
	base := PriceComparisonResult{
		MethodologyVersion: PriceMethodologyVersion,
		ReferenceSource:    in.ReferenceSource,
		PlaceRule:          in.PlaceRule,
		EffectiveUnitMilli: effUnit,
	}
	if !in.HasReference || in.ReferencePriceMilli <= 0 || in.ReferenceProduct != in.FuelingProduct || in.ReferenceUnit != in.FuelingUnit {
		base.Status = "INSUFFICIENT_DATA"
		base.Reason = ReasonNoReference
		return base
	}
	if !in.PlaceKnown {
		base.Status = "INSUFFICIENT_DATA"
		base.Reason = ReasonPlaceUnknown
		return base
	}
	if in.ReferenceAgeDays > in.MaxAgeDays {
		base.Status = "INSUFFICIENT_DATA"
		base.Reason = ReasonReferenceStale
		return base
	}
	if in.PlaceRule == "SAME_MUNICIPALITY" && in.CoverageStations < in.MinCoverageStations {
		base.Status = "INSUFFICIENT_DATA"
		base.Reason = ReasonLowCoverage
		return base
	}
	if !validCondition(in.FuelingCondition) || !validCondition(in.ReferenceCondition) {
		base.Status = "INSUFFICIENT_DATA"
		base.Reason = ReasonConditionUnknown
		return base
	}
	if strings.TrimSpace(in.FuelingCondition) != strings.TrimSpace(in.ReferenceCondition) {
		base.Status = "INSUFFICIENT_DATA"
		base.Reason = ReasonConditionMismatch
		return base
	}
	refCents := roundHalfUpBig(new(big.Int).Mul(big.NewInt(in.QuantityMilli), big.NewInt(in.ReferencePriceMilli)), big.NewInt(10000))
	diffCents := in.PaidCents - refCents
	diffUnit := effUnit - in.ReferencePriceMilli
	base.Status = "COMPUTED"
	base.ReferenceUnitMilli = in.ReferencePriceMilli
	base.ReferenceCents = refCents
	base.DifferenceCents = diffCents
	base.DifferencePerUnitMilli = diffUnit
	base.DisplayNote = "Diferença em relação à referência para revisão operacional"
	return base
}
