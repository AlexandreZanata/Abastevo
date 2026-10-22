package application

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strings"
	"time"
)

// MEX-28 typed ANP weekly price exploration. The frozen MEX-02 query shapes
// are narrowed to the source/conditions this module can prove: ANP weekly
// observations, STANDARD prices, OBSERVATION_PRICE by station and AREA_MEAN
// by municipality/state. Community prices, other conditions, and HTTP
// cursors/ETags belong to later units and are refused rather than guessed.

var (
	ErrUnsupportedSource    = errors.New("official: unsupported ranking source")
	ErrUnsupportedCondition = errors.New("official: unsupported ranking condition")
	ErrUnsupportedMetric    = errors.New("official: unsupported ranking metric")
	ErrRankingUnavailable   = errors.New("official: ranking source unavailable")
)

var rankingUnits = map[string]string{
	"ETHANOL":                "L",
	"GASOLINE_REGULAR":       "L",
	"GASOLINE_ADDITIVED":     "L",
	"GASOLINE_PREMIUM_GRADE": "L",
	"DIESEL_S500":            "L",
	"DIESEL_S10":             "L",
	"CNG":                    "M3",
	"LPG_P13":                "KG_13",
}

var rankingStates = map[string]bool{
	"AC": true, "AL": true, "AP": true, "AM": true, "BA": true, "CE": true,
	"DF": true, "ES": true, "GO": true, "MA": true, "MT": true, "MS": true,
	"MG": true, "PA": true, "PB": true, "PR": true, "PE": true, "PI": true,
	"RJ": true, "RN": true, "RS": true, "RO": true, "RR": true, "SC": true,
	"SP": true, "SE": true, "TO": true,
}

var ibgeMunicipality = regexp.MustCompile(`^[0-9]{7}$`)

type RankingSourceQuery struct {
	Kind            string
	SurveyWeekStart string
}

type RankingProduct struct {
	Fuel string
	Unit string
}

type RankingCondition struct {
	Kind        string
	QualifierID string
}

type RankingGeography struct {
	Level    string
	Country  string
	UF       string
	IBGECode string
}

type RankingMetric struct {
	Kind      string
	GroupBy   string
	Sort      string
	Weighting string
}

// RankingQuery is the validated typed intent. No SQL, remote URL or free-form
// expression is accepted.
type RankingQuery struct {
	Version       int
	Kind          string
	Source        RankingSourceQuery
	Product       RankingProduct
	Condition     RankingCondition
	Geography     RankingGeography
	Metric        RankingMetric
	Limit         int
	PolicyVersion string
}

type RankingRevision struct {
	RevisionID      string
	SurveyWeekStart string
	SurveyWeekEnd   string
	PublishedAt     string
	SourceURL       string
	SourceChecksum  string
}

// MunicipalityAggregate is one latest observation per station, summed for a
// located municipality. ObservationCount is the exact mean denominator.
type MunicipalityAggregate struct {
	State            string
	MunicipalityCode string
	SumMilliBrl      int64
	ObservationCount int64
	MinMilliBrl      int64
	MaxMilliBrl      int64
}

type StationObservationRow struct {
	StationID        string
	AmountMilliBrl   int64
	CollectedOn      string
	State            *string
	MunicipalityCode *string
}

// RankingData is the raw adapter output. The pure builder below owns exact
// means, stable ties, state roll-up and coverage.
type RankingData struct {
	Revision              RankingRevision
	Municipalities        []MunicipalityAggregate
	Stations              []StationObservationRow
	SourceStations        int64
	SourceObservations    int64
	UnlocatedObservations int64
}

type RankingReader interface {
	ReadRanking(ctx context.Context, q RankingQuery) (RankingData, error)
}

type RankingSource struct {
	Kind            string
	SurveyWeekStart string
	SurveyWeekEnd   string
	RevisionID      string
	PublishedAt     string
	SourceURL       string
	SourceChecksum  string
}

type RankingCoverage struct {
	SourceObservations          int64
	StationsWithPrice           int64
	SourceStations              int64
	Municipalities              int64
	States                      int64
	UnlocatedObservations       int64
	Sampled                     bool
	NationalCompletenessClaimed bool
}

type StationObservation struct {
	StationID        string
	AmountMilliBrl   int64
	CollectedOn      string
	State            *string
	MunicipalityCode *string
}

// AreaAggregate is an exact mean (MeanNumerator/MeanDenominator) over one
// latest observation per station. Variation is the exact max-min range, not a
// floating-point standard deviation.
type AreaAggregate struct {
	Level             string
	State             string
	MunicipalityCode  string
	MeanNumerator     int64
	MeanDenominator   int64
	Mean              string
	MinMilliBrl       int64
	MaxMilliBrl       int64
	VariationMilliBrl int64
	Observations      int64
}

type RankingResult struct {
	Source   RankingSource
	Coverage RankingCoverage
	Stations []StationObservation
	Areas    []AreaAggregate
}

// ValidateRanking accepts only the frozen typed combinations this backend can
// prove. It never substitutes a missing source, condition or weighting.
func ValidateRanking(q RankingQuery) (RankingQuery, error) {
	q = normalizeRanking(q)
	if q.Version != 1 || q.Kind != "PRICE_RANKING" || q.PolicyVersion != "mex-exploration-v1" {
		return RankingQuery{}, fmt.Errorf("%w: envelope", ErrInvalidFilter)
	}
	if q.Source.Kind != "ANP_WEEKLY" {
		return RankingQuery{}, fmt.Errorf("%w: %s", ErrUnsupportedSource, q.Source.Kind)
	}
	if _, err := time.Parse("2006-01-02", q.Source.SurveyWeekStart); err != nil {
		return RankingQuery{}, fmt.Errorf("%w: survey_week_start", ErrInvalidFilter)
	}
	if unit, ok := rankingUnits[q.Product.Fuel]; !ok || unit != q.Product.Unit {
		return RankingQuery{}, fmt.Errorf("%w: product", ErrInvalidFilter)
	}
	if q.Condition.Kind != "STANDARD" || q.Condition.QualifierID != "" {
		return RankingQuery{}, fmt.Errorf("%w: %s", ErrUnsupportedCondition, q.Condition.Kind)
	}
	if q.Geography.Country != "BR" {
		return RankingQuery{}, fmt.Errorf("%w: country", ErrInvalidFilter)
	}
	switch q.Geography.Level {
	case "COUNTRY":
		if q.Geography.UF != "" || q.Geography.IBGECode != "" {
			return RankingQuery{}, fmt.Errorf("%w: country geography", ErrInvalidFilter)
		}
	case "STATE":
		if !rankingStates[q.Geography.UF] || q.Geography.IBGECode != "" {
			return RankingQuery{}, fmt.Errorf("%w: state geography", ErrInvalidFilter)
		}
	case "MUNICIPALITY":
		if !rankingStates[q.Geography.UF] || !ibgeMunicipality.MatchString(q.Geography.IBGECode) {
			return RankingQuery{}, fmt.Errorf("%w: municipality geography", ErrInvalidFilter)
		}
	default:
		return RankingQuery{}, fmt.Errorf("%w: geography", ErrInvalidFilter)
	}
	switch q.Metric.Kind {
	case "OBSERVATION_PRICE":
		if q.Metric.GroupBy != "STATION" || (q.Metric.Sort != "ASC" && q.Metric.Sort != "DESC") || q.Metric.Weighting != "" {
			return RankingQuery{}, fmt.Errorf("%w: observation metric", ErrUnsupportedMetric)
		}
	case "AREA_MEAN":
		if (q.Metric.GroupBy != "MUNICIPALITY" && q.Metric.GroupBy != "STATE") ||
			(q.Metric.Sort != "ASC" && q.Metric.Sort != "DESC") ||
			q.Metric.Weighting != "ELIGIBLE_SOURCE_OBSERVATIONS" {
			return RankingQuery{}, fmt.Errorf("%w: area metric", ErrUnsupportedMetric)
		}
	default:
		return RankingQuery{}, fmt.Errorf("%w: %s", ErrUnsupportedMetric, q.Metric.Kind)
	}
	if q.Limit < 1 || q.Limit > 100 {
		return RankingQuery{}, fmt.Errorf("%w: limit", ErrInvalidFilter)
	}
	return q, nil
}

// RankPrices validates, reads and builds one typed ranking. A missing current
// published revision is an unavailable source, never an empty national result.
func RankPrices(ctx context.Context, reader RankingReader, q RankingQuery) (RankingResult, error) {
	validated, err := ValidateRanking(q)
	if err != nil {
		return RankingResult{}, err
	}
	data, err := reader.ReadRanking(ctx, validated)
	if err != nil {
		return RankingResult{}, err
	}
	return BuildRanking(validated, data)
}

// BuildRanking is pure: exact rational means, stable ties, bounded output.
func BuildRanking(q RankingQuery, data RankingData) (RankingResult, error) {
	result := RankingResult{
		Source: RankingSource{
			Kind:            "ANP_WEEKLY",
			SurveyWeekStart: data.Revision.SurveyWeekStart,
			SurveyWeekEnd:   data.Revision.SurveyWeekEnd,
			RevisionID:      data.Revision.RevisionID,
			PublishedAt:     data.Revision.PublishedAt,
			SourceURL:       data.Revision.SourceURL,
			SourceChecksum:  data.Revision.SourceChecksum,
		},
		Coverage: buildRankingCoverage(q, data),
	}
	switch q.Metric.Kind {
	case "OBSERVATION_PRICE":
		stations := make([]StationObservation, 0, len(data.Stations))
		for _, row := range data.Stations {
			stations = append(stations, StationObservation(row))
		}
		sort.SliceStable(stations, func(i, j int) bool {
			if stations[i].AmountMilliBrl != stations[j].AmountMilliBrl {
				if q.Metric.Sort == "DESC" {
					return stations[i].AmountMilliBrl > stations[j].AmountMilliBrl
				}
				return stations[i].AmountMilliBrl < stations[j].AmountMilliBrl
			}
			return stations[i].StationID < stations[j].StationID
		})
		if len(stations) > q.Limit {
			stations = stations[:q.Limit]
		}
		result.Stations = stations
	case "AREA_MEAN":
		areas, err := buildAreaAggregates(q, data.Municipalities)
		if err != nil {
			return RankingResult{}, err
		}
		if len(areas) > q.Limit {
			areas = areas[:q.Limit]
		}
		result.Areas = areas
	}
	return result, nil
}

func buildRankingCoverage(q RankingQuery, data RankingData) RankingCoverage {
	var stationsWithPrice int64
	states := map[string]bool{}
	for _, row := range data.Municipalities {
		stationsWithPrice += row.ObservationCount
		states[row.State] = true
	}
	unlocated := int64(0)
	if q.Geography.Level == "COUNTRY" {
		unlocated = data.UnlocatedObservations
		stationsWithPrice += unlocated
	}
	municipalities := int64(len(data.Municipalities))
	if q.Geography.Level == "MUNICIPALITY" && municipalities > 1 {
		municipalities = 1
	}
	return RankingCoverage{
		SourceObservations:          data.SourceObservations,
		StationsWithPrice:           stationsWithPrice,
		SourceStations:              data.SourceStations,
		Municipalities:              municipalities,
		States:                      int64(len(states)),
		UnlocatedObservations:       unlocated,
		Sampled:                     true,
		NationalCompletenessClaimed: false,
	}
}

type areaWithMean struct {
	area AreaAggregate
	mean *big.Rat
}

func buildAreaAggregates(q RankingQuery, municipalities []MunicipalityAggregate) ([]AreaAggregate, error) {
	rows := make([]areaWithMean, 0, len(municipalities))
	if q.Metric.GroupBy == "STATE" {
		type stateAggregate struct {
			sum, count, min, max int64
		}
		byState := map[string]*stateAggregate{}
		order := []string{}
		for _, row := range municipalities {
			agg, ok := byState[row.State]
			if !ok {
				agg = &stateAggregate{min: row.MinMilliBrl, max: row.MaxMilliBrl}
				byState[row.State] = agg
				order = append(order, row.State)
			}
			agg.sum += row.SumMilliBrl
			agg.count += row.ObservationCount
			if row.MinMilliBrl < agg.min {
				agg.min = row.MinMilliBrl
			}
			if row.MaxMilliBrl > agg.max {
				agg.max = row.MaxMilliBrl
			}
		}
		for _, state := range order {
			agg := byState[state]
			area, mean, err := newAreaAggregate("STATE", state, "", agg.sum, agg.count, agg.min, agg.max)
			if err != nil {
				return nil, err
			}
			rows = append(rows, areaWithMean{area: area, mean: mean})
		}
	} else {
		for _, row := range municipalities {
			area, mean, err := newAreaAggregate("MUNICIPALITY", row.State, row.MunicipalityCode, row.SumMilliBrl, row.ObservationCount, row.MinMilliBrl, row.MaxMilliBrl)
			if err != nil {
				return nil, err
			}
			rows = append(rows, areaWithMean{area: area, mean: mean})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		comparison := rows[i].mean.Cmp(rows[j].mean)
		if q.Metric.Sort == "DESC" {
			comparison = -comparison
		}
		if comparison != 0 {
			return comparison < 0
		}
		if rows[i].area.State != rows[j].area.State {
			return rows[i].area.State < rows[j].area.State
		}
		return rows[i].area.MunicipalityCode < rows[j].area.MunicipalityCode
	})
	out := make([]AreaAggregate, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.area)
	}
	return out, nil
}

func newAreaAggregate(level, state, municipality string, sum, count, min, max int64) (AreaAggregate, *big.Rat, error) {
	if count <= 0 || sum < 0 || min < 0 || max < min {
		return AreaAggregate{}, nil, fmt.Errorf("official: invalid ranking aggregate")
	}
	divisor := gcdInt64(sum, count)
	numerator, denominator := sum/divisor, count/divisor
	mean := big.NewRat(sum, count)
	text := fmt.Sprintf("%d", numerator)
	if denominator != 1 {
		text = fmt.Sprintf("%d/%d", numerator, denominator)
	}
	return AreaAggregate{
		Level:             level,
		State:             state,
		MunicipalityCode:  municipality,
		MeanNumerator:     numerator,
		MeanDenominator:   denominator,
		Mean:              text,
		MinMilliBrl:       min,
		MaxMilliBrl:       max,
		VariationMilliBrl: max - min,
		Observations:      count,
	}, mean, nil
}

func gcdInt64(a, b int64) int64 {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	if a == 0 {
		return 1
	}
	return a
}

// normalizeRanking trims the typed strings without changing case-sensitive
// vocabularies.
func normalizeRanking(q RankingQuery) RankingQuery {
	q.Source.Kind = strings.TrimSpace(q.Source.Kind)
	q.Source.SurveyWeekStart = strings.TrimSpace(q.Source.SurveyWeekStart)
	q.Product.Fuel = strings.TrimSpace(q.Product.Fuel)
	q.Product.Unit = strings.TrimSpace(q.Product.Unit)
	q.Condition.Kind = strings.TrimSpace(q.Condition.Kind)
	q.Geography.Level = strings.TrimSpace(q.Geography.Level)
	q.Geography.Country = strings.TrimSpace(q.Geography.Country)
	q.Geography.UF = strings.TrimSpace(q.Geography.UF)
	q.Geography.IBGECode = strings.TrimSpace(q.Geography.IBGECode)
	q.Metric.Kind = strings.TrimSpace(q.Metric.Kind)
	q.Metric.GroupBy = strings.TrimSpace(q.Metric.GroupBy)
	q.Metric.Sort = strings.TrimSpace(q.Metric.Sort)
	q.Metric.Weighting = strings.TrimSpace(q.Metric.Weighting)
	q.PolicyVersion = strings.TrimSpace(q.PolicyVersion)
	return q
}
