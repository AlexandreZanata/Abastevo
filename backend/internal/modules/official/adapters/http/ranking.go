package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

const rankingPolicyVersion = "mex-exploration-v1"

// RankingHandler serves the bounded anonymous ANP weekly ranking. The cache
// is optional; nil (or maxEntries=0) runs the exact bounded query uncached.
type RankingHandler struct {
	Read    application.RankingReader
	Enabled bool
	Secrets []byte
	Cache   *RankingCache
	Now     func() time.Time
}

func (h RankingHandler) RegisterRoutes(r chi.Router) {
	r.Get("/v1/prices/ranking", h.ranking)
}

var rankingQueryKeys = map[string]bool{
	"source": true, "survey_week_start": true, "fuel": true, "unit": true,
	"condition": true, "geography": true, "uf": true, "ibge_code": true,
	"metric": true, "group_by": true, "sort": true, "limit": true, "cursor": true,
}

var (
	rankingUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	rankingUF   = regexp.MustCompile(`^[A-Z]{2}$`)
	rankingIBGE = regexp.MustCompile(`^[0-9]{7}$`)
)

func (h RankingHandler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func (h RankingHandler) ranking(w http.ResponseWriter, r *http.Request) {
	now := h.now()
	if !h.Enabled {
		httpapi.WriteError(w, r, http.StatusNotFound, "prices.ranking-disabled", "ranking unavailable", nil)
		return
	}
	if h.Read == nil {
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "prices.unavailable", "try again later", nil)
		return
	}
	if r.Context().Err() != nil {
		return
	}
	q, err := parseRankingQuery(r)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "prices.invalid-query", "invalid ranking query", nil)
		return
	}
	revision, err := h.Read.ResolveRankingRevision(r.Context(), q.Source.SurveyWeekStart)
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		h.writeReadError(w, r, err)
		return
	}
	if r.Context().Err() != nil {
		return
	}
	filterHash := rankingFilterHash(q, revision)
	positionKey, err := httpapi.ParseCursor(h.Secrets, r.URL.Query().Get("cursor"), filterHash, now)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "prices.invalid-cursor", "invalid or expired cursor", nil)
		return
	}
	if positionKey != "" {
		after, decodeErr := decodeRankingPosition(q, positionKey)
		if decodeErr != nil {
			httpapi.WriteError(w, r, http.StatusBadRequest, "prices.invalid-cursor", "invalid or expired cursor", nil)
			return
		}
		q.After = &after
	}
	cacheKey := filterHash + "|" + positionKey
	result, cached := h.Cache.Get(cacheKey)
	if !cached {
		data, readErr := h.Read.ReadRankingAt(r.Context(), q, revision)
		if r.Context().Err() != nil {
			return
		}
		if readErr != nil {
			h.writeReadError(w, r, readErr)
			return
		}
		result, readErr = application.BuildRanking(q, data)
		if readErr != nil {
			h.writeReadError(w, r, readErr)
			return
		}
		h.Cache.Put(cacheKey, result)
	}
	if r.Context().Err() != nil {
		return
	}
	response, stable := wireRanking(q, result)
	if result.Next != nil {
		nextKey := encodeRankingPosition(q, *result.Next)
		sealed := httpapi.Seal(h.Secrets, filterHash, nextKey, now)
		response.NextCursor = &sealed
		stable.NextKey = nextKey
	}
	response.GeneratedAt = now.UTC().Format(time.RFC3339)
	body, err := json.Marshal(response)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "prices.unavailable", "try again later", nil)
		return
	}
	stableBody, err := json.Marshal(stable)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "prices.unavailable", "try again later", nil)
		return
	}
	httpapi.WriteJSONWithETag(w, r, http.StatusOK, "public, max-age=0, must-revalidate", body, httpapi.ETag(stableBody))
}

func (h RankingHandler) writeReadError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, application.ErrInvalidFilter),
		errors.Is(err, application.ErrUnsupportedSource),
		errors.Is(err, application.ErrUnsupportedCondition),
		errors.Is(err, application.ErrUnsupportedMetric):
		httpapi.WriteError(w, r, http.StatusBadRequest, "prices.invalid-query", "invalid ranking query", nil)
	case errors.Is(err, application.ErrRankingUnavailable):
		httpapi.WriteError(w, r, http.StatusNotFound, "prices.ranking-unavailable", "no published ranking for this week", nil)
	default:
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "prices.unavailable", "try again later", nil)
	}
}

func parseRankingQuery(r *http.Request) (application.RankingQuery, error) {
	values := r.URL.Query()
	for key, list := range values {
		if !rankingQueryKeys[key] || len(list) != 1 {
			return application.RankingQuery{}, fmt.Errorf("invalid query parameter")
		}
	}
	if cursor, present := values["cursor"]; present && strings.TrimSpace(cursor[0]) == "" {
		return application.RankingQuery{}, fmt.Errorf("empty cursor")
	}
	limit, err := httpapi.ParseLimit(values.Get("limit"))
	if err != nil {
		return application.RankingQuery{}, err
	}
	condition := "STANDARD"
	if _, present := values["condition"]; present {
		condition = values.Get("condition")
	}
	metric := values.Get("metric")
	weighting := ""
	if metric == "AREA_MEAN" {
		weighting = "ELIGIBLE_SOURCE_OBSERVATIONS"
	}
	q := application.RankingQuery{
		Version: 1,
		Kind:    "PRICE_RANKING",
		Source: application.RankingSourceQuery{
			Kind: values.Get("source"), SurveyWeekStart: values.Get("survey_week_start"),
		},
		Product: application.RankingProduct{
			Fuel: values.Get("fuel"), Unit: values.Get("unit"),
		},
		Condition: application.RankingCondition{Kind: condition},
		Geography: application.RankingGeography{
			Level: values.Get("geography"), Country: "BR",
			UF: values.Get("uf"), IBGECode: values.Get("ibge_code"),
		},
		Metric: application.RankingMetric{
			Kind: metric, GroupBy: values.Get("group_by"),
			Sort: values.Get("sort"), Weighting: weighting,
		},
		Limit:         limit,
		PolicyVersion: rankingPolicyVersion,
	}
	return application.ValidateRanking(q)
}

func rankingFilterHash(q application.RankingQuery, revision application.RankingRevision) string {
	return httpapi.FilterHash(
		"mex-ranking-v1",
		q.Source.Kind, q.Source.SurveyWeekStart,
		q.Product.Fuel, q.Product.Unit, q.Condition.Kind,
		q.Geography.Level, q.Geography.Country, q.Geography.UF, q.Geography.IBGECode,
		q.Metric.Kind, q.Metric.GroupBy, q.Metric.Sort, q.Metric.Weighting,
		strconv.Itoa(q.Limit), revision.RevisionID,
	)
}

func encodeRankingPosition(q application.RankingQuery, position application.RankingPosition) string {
	if q.Metric.Kind == "OBSERVATION_PRICE" {
		return "s|" + strconv.FormatInt(position.AmountMilliBrl, 10) + "|" + position.StationID
	}
	return "a|" + strconv.FormatInt(position.MeanNumerator, 10) + "|" +
		strconv.FormatInt(position.MeanDenominator, 10) + "|" +
		position.State + "|" + position.MunicipalityCode
}

func decodeRankingPosition(q application.RankingQuery, raw string) (application.RankingPosition, error) {
	parts := strings.Split(raw, "|")
	if q.Metric.Kind == "OBSERVATION_PRICE" {
		if len(parts) != 3 || parts[0] != "s" || !rankingUUID.MatchString(parts[2]) {
			return application.RankingPosition{}, fmt.Errorf("invalid station position")
		}
		amount, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || amount < 0 {
			return application.RankingPosition{}, fmt.Errorf("invalid station amount")
		}
		return application.RankingPosition{AmountMilliBrl: amount, StationID: parts[2]}, nil
	}
	if len(parts) != 5 || parts[0] != "a" || !rankingUF.MatchString(parts[3]) ||
		!rankingIBGE.MatchString(parts[4]) {
		return application.RankingPosition{}, fmt.Errorf("invalid area position")
	}
	numerator, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || numerator < 0 {
		return application.RankingPosition{}, fmt.Errorf("invalid mean numerator")
	}
	denominator, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || denominator <= 0 {
		return application.RankingPosition{}, fmt.Errorf("invalid mean denominator")
	}
	return application.RankingPosition{
		MeanNumerator: numerator, MeanDenominator: denominator,
		State: parts[3], MunicipalityCode: parts[4],
	}, nil
}

type rankingInterpretationSource struct {
	Kind            string `json:"kind"`
	SurveyWeekStart string `json:"survey_week_start"`
}

type rankingInterpretationProduct struct {
	Fuel string `json:"fuel"`
	Unit string `json:"unit"`
}

type rankingInterpretationCondition struct {
	Kind string `json:"kind"`
}

type rankingInterpretationGeography struct {
	Level    string `json:"level"`
	Country  string `json:"country"`
	UF       string `json:"uf,omitempty"`
	IBGECode string `json:"ibge_code,omitempty"`
}

type rankingInterpretationMetric struct {
	Kind      string `json:"kind"`
	GroupBy   string `json:"group_by"`
	Sort      string `json:"sort"`
	Weighting string `json:"weighting,omitempty"`
}

type rankingInterpretation struct {
	Source        rankingInterpretationSource    `json:"source"`
	Product       rankingInterpretationProduct   `json:"product"`
	Condition     rankingInterpretationCondition `json:"condition"`
	Geography     rankingInterpretationGeography `json:"geography"`
	Metric        rankingInterpretationMetric    `json:"metric"`
	PolicyVersion string                         `json:"policy_version"`
}

type rankingSource struct {
	Kind            string `json:"kind"`
	SurveyWeekStart string `json:"survey_week_start"`
	SurveyWeekEnd   string `json:"survey_week_end"`
	RevisionID      string `json:"revision_id"`
	PublishedAt     string `json:"published_at"`
	SourceURL       string `json:"source_url"`
	SourceChecksum  string `json:"source_checksum"`
}

type rankingCoverage struct {
	SourceObservations          int64 `json:"source_observations"`
	StationsWithPrice           int64 `json:"stations_with_price"`
	SourceStations              int64 `json:"source_stations"`
	Municipalities              int64 `json:"municipalities"`
	States                      int64 `json:"states"`
	UnlocatedObservations       int64 `json:"unlocated_observations"`
	Sampled                     bool  `json:"sampled"`
	NationalCompletenessClaimed bool  `json:"national_completeness_claimed"`
}

type rankingRational struct {
	Numerator   string `json:"numerator"`
	Denominator string `json:"denominator"`
}

type rankingStation struct {
	StationID      string  `json:"station_id"`
	AmountMilliBrl int64   `json:"amount_milli_brl"`
	Currency       string  `json:"currency"`
	CollectedOn    string  `json:"collected_on"`
	State          *string `json:"state"`
	IBGECode       *string `json:"ibge_code"`
}

type rankingArea struct {
	Level             string          `json:"level"`
	State             string          `json:"state"`
	IBGECode          string          `json:"ibge_code"`
	Mean              rankingRational `json:"mean"`
	MinMilliBrl       int64           `json:"min_milli_brl"`
	MaxMilliBrl       int64           `json:"max_milli_brl"`
	VariationMilliBrl int64           `json:"variation_milli_brl"`
	Observations      int64           `json:"observations"`
	Currency          string          `json:"currency"`
}

type rankingResponse struct {
	Interpretation rankingInterpretation `json:"interpretation"`
	Source         rankingSource         `json:"source"`
	Coverage       rankingCoverage       `json:"coverage"`
	Stations       []rankingStation      `json:"stations"`
	Areas          []rankingArea         `json:"areas"`
	NextCursor     *string               `json:"next_cursor"`
	GeneratedAt    string                `json:"generated_at"`
}

type rankingStable struct {
	Interpretation rankingInterpretation `json:"interpretation"`
	Source         rankingSource         `json:"source"`
	Coverage       rankingCoverage       `json:"coverage"`
	Stations       []rankingStation      `json:"stations"`
	Areas          []rankingArea         `json:"areas"`
	NextKey        string                `json:"next_key"`
}

func wireRanking(q application.RankingQuery, result application.RankingResult) (rankingResponse, rankingStable) {
	interpretation := rankingInterpretation{
		Source: rankingInterpretationSource{
			Kind: q.Source.Kind, SurveyWeekStart: q.Source.SurveyWeekStart,
		},
		Product:   rankingInterpretationProduct{Fuel: q.Product.Fuel, Unit: q.Product.Unit},
		Condition: rankingInterpretationCondition{Kind: q.Condition.Kind},
		Geography: rankingInterpretationGeography{
			Level: q.Geography.Level, Country: q.Geography.Country,
			UF: q.Geography.UF, IBGECode: q.Geography.IBGECode,
		},
		Metric: rankingInterpretationMetric{
			Kind: q.Metric.Kind, GroupBy: q.Metric.GroupBy,
			Sort: q.Metric.Sort, Weighting: q.Metric.Weighting,
		},
		PolicyVersion: q.PolicyVersion,
	}
	source := rankingSource{
		Kind: "ANP_WEEKLY", SurveyWeekStart: result.Source.SurveyWeekStart,
		SurveyWeekEnd: result.Source.SurveyWeekEnd, RevisionID: result.Source.RevisionID,
		PublishedAt: result.Source.PublishedAt, SourceURL: result.Source.SourceURL,
		SourceChecksum: result.Source.SourceChecksum,
	}
	coverage := rankingCoverage{
		SourceObservations:          result.Coverage.SourceObservations,
		StationsWithPrice:           result.Coverage.StationsWithPrice,
		SourceStations:              result.Coverage.SourceStations,
		Municipalities:              result.Coverage.Municipalities,
		States:                      result.Coverage.States,
		UnlocatedObservations:       result.Coverage.UnlocatedObservations,
		Sampled:                     result.Coverage.Sampled,
		NationalCompletenessClaimed: result.Coverage.NationalCompletenessClaimed,
	}
	stations := make([]rankingStation, 0, len(result.Stations))
	for _, station := range result.Stations {
		stations = append(stations, rankingStation{
			StationID: station.StationID, AmountMilliBrl: station.AmountMilliBrl,
			Currency: "BRL", CollectedOn: station.CollectedOn,
			State: station.State, IBGECode: station.MunicipalityCode,
		})
	}
	areas := make([]rankingArea, 0, len(result.Areas))
	for _, area := range result.Areas {
		areas = append(areas, rankingArea{
			Level: area.Level, State: area.State, IBGECode: area.MunicipalityCode,
			Mean: rankingRational{
				Numerator:   strconv.FormatInt(area.MeanNumerator, 10),
				Denominator: strconv.FormatInt(area.MeanDenominator, 10),
			},
			MinMilliBrl: area.MinMilliBrl, MaxMilliBrl: area.MaxMilliBrl,
			VariationMilliBrl: area.VariationMilliBrl,
			Observations:      area.Observations, Currency: "BRL",
		})
	}
	stable := rankingStable{
		Interpretation: interpretation, Source: source, Coverage: coverage,
		Stations: stations, Areas: areas,
	}
	response := rankingResponse{
		Interpretation: interpretation, Source: source, Coverage: coverage,
		Stations: stations, Areas: areas,
	}
	return response, stable
}
