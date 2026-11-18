package domain

import (
	"math/big"
	"sort"
)

// Route station price suggestions and detour cost evaluation (EMP-ROUTE-008,
// BR-ROUTE-020..022, §10.7). Uses exact integer arithmetic and refuses to mix
// incomparable price sources or payment conditions.
type PriceSourceKind string

const (
	PriceSourceCommunity   PriceSourceKind = "COMMUNITY"
	PriceSourceOfficialANP PriceSourceKind = "OFFICIAL_ANP"
	PriceSourceStation     PriceSourceKind = "STATION_ANNOUNCED"
)

type FuelPriceReference struct {
	SourceKind       PriceSourceKind `json:"source_kind"`
	Product          string          `json:"product"`
	Unit             string          `json:"unit"`
	PaymentCondition string          `json:"payment_condition"`
	PriceMilliBRL    int64           `json:"price_milli_brl"`
	AgeSeconds       int64           `json:"age_seconds"`
}

type AssetConsumptionRef struct {
	DistanceMetres int64
	VolumeMilli    int64
}

type StationCandidate struct {
	StationID          string              `json:"station_id"`
	StationName        string              `json:"station_name"`
	Price              *FuelPriceReference `json:"price,omitempty"`
	RouteTotalMetres   int64               `json:"route_total_metres"`
	RouteTotalSeconds  int64               `json:"route_total_seconds"`
	HasPaidPlan        bool                `json:"has_paid_plan"`
	HasActiveAgreement bool                `json:"has_active_agreement"`
}

type SuggestionEvaluation struct {
	StationID          string              `json:"station_id"`
	StationName        string              `json:"station_name"`
	Price              *FuelPriceReference `json:"price,omitempty"`
	DetourMetres       int64               `json:"detour_metres"`
	DetourSeconds      int64               `json:"detour_seconds"`
	DetourFuelMilli    *int64              `json:"detour_fuel_ml,omitempty"`
	DetourCostCentavos *int64              `json:"detour_cost_centavos,omitempty"`
	ReferenceCentavos  *int64              `json:"reference_cost_centavos,omitempty"`
	CandidateCentavos  *int64              `json:"candidate_cost_centavos,omitempty"`
	GrossDiffCentavos  *int64              `json:"gross_diff_centavos,omitempty"`
	NetDiffCentavos    *int64              `json:"net_diff_centavos,omitempty"`
	Comparable         bool                `json:"comparable"`
	ComparisonReason   string              `json:"comparison_reason,omitempty"`
	DetourExceedsDiff  bool                `json:"detour_exceeds_diff"`
}

func priceCostCentavos(volumeMilli, priceMilliBRL int64) (int64, error) {
	p := new(big.Int).Mul(big.NewInt(volumeMilli), big.NewInt(priceMilliBRL))
	p.Add(p, big.NewInt(5000))
	p.Quo(p, big.NewInt(10000))
	if !p.IsInt64() {
		return 0, ErrInvalid
	}
	return p.Int64(), nil
}

// EvaluateSuggestion computes detour fuel, detour cost and comparable price
// difference per §10.7 and BR-ROUTE-020..022.
func EvaluateSuggestion(
	product string,
	plannedVolumeMilli int64,
	baseMetres int64,
	baseSeconds int64,
	consumption *AssetConsumptionRef,
	freshnessLimitSeconds int64,
	reference *FuelPriceReference,
	candidate StationCandidate,
) (SuggestionEvaluation, error) {
	if product == "" || plannedVolumeMilli <= 0 || baseMetres < 0 || baseSeconds < 0 || freshnessLimitSeconds <= 0 ||
		candidate.RouteTotalMetres < baseMetres || candidate.RouteTotalSeconds < baseSeconds {
		return SuggestionEvaluation{}, ErrInvalid
	}
	if consumption != nil && (consumption.DistanceMetres <= 0 || consumption.VolumeMilli <= 0) {
		return SuggestionEvaluation{}, ErrInvalid
	}
	detourM := candidate.RouteTotalMetres - baseMetres
	detourS := candidate.RouteTotalSeconds - baseSeconds

	out := SuggestionEvaluation{
		StationID:     candidate.StationID,
		StationName:   candidate.StationName,
		Price:         candidate.Price,
		DetourMetres:  detourM,
		DetourSeconds: detourS,
	}

	if candidate.Price == nil {
		out.ComparisonReason = "NO_CANDIDATE_PRICE"
		return out, nil
	}
	if candidate.Price.AgeSeconds < 0 || candidate.Price.PriceMilliBRL <= 0 {
		return SuggestionEvaluation{}, ErrInvalid
	}

	var detourFuelPtr, detourCostPtr *int64
	if consumption != nil {
		mul := new(big.Int).Mul(big.NewInt(detourM), big.NewInt(consumption.VolumeMilli))
		half := new(big.Int).Quo(big.NewInt(consumption.DistanceMetres), big.NewInt(2))
		mul.Add(mul, half)
		mul.Quo(mul, big.NewInt(consumption.DistanceMetres))
		if !mul.IsInt64() {
			return SuggestionEvaluation{}, ErrInvalid
		}
		df := mul.Int64()
		dc, err := priceCostCentavos(df, candidate.Price.PriceMilliBRL)
		if err != nil {
			return SuggestionEvaluation{}, err
		}
		detourFuelPtr = &df
		detourCostPtr = &dc
		out.DetourFuelMilli = detourFuelPtr
		out.DetourCostCentavos = detourCostPtr
	}

	candCost, err := priceCostCentavos(plannedVolumeMilli, candidate.Price.PriceMilliBRL)
	if err != nil {
		return SuggestionEvaluation{}, err
	}
	out.CandidateCentavos = &candCost

	if reference == nil {
		out.ComparisonReason = "NO_REFERENCE_PRICE"
		return out, nil
	}
	if reference.AgeSeconds < 0 || reference.PriceMilliBRL <= 0 {
		return SuggestionEvaluation{}, ErrInvalid
	}
	refCost, err := priceCostCentavos(plannedVolumeMilli, reference.PriceMilliBRL)
	if err != nil {
		return SuggestionEvaluation{}, err
	}
	out.ReferenceCentavos = &refCost

	// BR-ROUTE-020: strict comparability gate.
	switch {
	case reference.SourceKind != candidate.Price.SourceKind:
		out.ComparisonReason = "SOURCE_KIND_MISMATCH"
		return out, nil
	case reference.Product != candidate.Price.Product || candidate.Price.Product != product:
		out.ComparisonReason = "PRODUCT_MISMATCH"
		return out, nil
	case reference.Unit != candidate.Price.Unit:
		out.ComparisonReason = "UNIT_MISMATCH"
		return out, nil
	case reference.PaymentCondition != candidate.Price.PaymentCondition:
		out.ComparisonReason = "PAYMENT_CONDITION_MISMATCH"
		return out, nil
	case reference.AgeSeconds > freshnessLimitSeconds || candidate.Price.AgeSeconds > freshnessLimitSeconds:
		out.ComparisonReason = "STALE_PRICE"
		return out, nil
	}

	out.Comparable = true
	gross := refCost - candCost
	out.GrossDiffCentavos = &gross
	if detourCostPtr != nil {
		net := gross - *detourCostPtr
		out.NetDiffCentavos = &net
		out.DetourExceedsDiff = *detourCostPtr > gross
	}
	return out, nil
}

// RankSuggestions orders candidates neutrally by comparable net difference,
// detour distance, price and station ID, ignoring HasPaidPlan and
// HasActiveAgreement completely (BR-ROUTE-022).
func RankSuggestions(items []SuggestionEvaluation) []SuggestionEvaluation {
	ranked := make([]SuggestionEvaluation, len(items))
	copy(ranked, items)
	sort.SliceStable(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if a.Comparable != b.Comparable {
			return a.Comparable
		}
		if a.NetDiffCentavos != nil && b.NetDiffCentavos != nil && *a.NetDiffCentavos != *b.NetDiffCentavos {
			return *a.NetDiffCentavos > *b.NetDiffCentavos
		}
		if a.Comparable && b.Comparable && a.GrossDiffCentavos != nil && b.GrossDiffCentavos != nil && *a.GrossDiffCentavos != *b.GrossDiffCentavos {
			return *a.GrossDiffCentavos > *b.GrossDiffCentavos
		}
		if a.DetourMetres != b.DetourMetres {
			return a.DetourMetres < b.DetourMetres
		}
		if a.Price != nil && b.Price != nil && a.Price.PriceMilliBRL != b.Price.PriceMilliBRL {
			return a.Price.PriceMilliBRL < b.Price.PriceMilliBRL
		}
		return a.StationID < b.StationID
	})
	return ranked
}
