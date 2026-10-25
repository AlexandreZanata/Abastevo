// Package analytics computes deterministic Abastevo metrics over a frozen
// enterprise export. It is pure: no database, network, clock or filesystem.
// Every result states its period, geography, fuel/unit, numerator,
// denominator, exclusions and method.
package analytics

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

const MetricsVersion = "abastevo-metrics-v1"

const (
	MetricCheapest   = "cheapest-by-amount-v1"
	MetricSummary    = "geography-summary-v1"
	MetricCoverage   = "coverage-v1"
	MetricHistory    = "station-history-v1"
	MetricComparison = "summary-change-v1"
)

const (
	LevelNational     = "national"
	LevelState        = "state"
	LevelMunicipality = "municipality"
)

var ErrInvalidInput = errors.New("analytics: invalid input")

type Geography struct {
	Level string `json:"level"`
	Code  string `json:"code"`
}

type Exclusions map[string]int

type FuelUnitCoverage struct {
	Fuel         string `json:"fuel"`
	Unit         string `json:"unit"`
	Stations     int    `json:"stations"`
	Observations int    `json:"observations"`
}

type Coverage struct {
	Metric                string             `json:"metric"`
	RevisionID            string             `json:"revision_id"`
	SurveyStart           string             `json:"survey_start"`
	SurveyEnd             string             `json:"survey_end"`
	PublishedAt           string             `json:"published_at"`
	SourceURL             string             `json:"source_url"`
	SourceChecksum        string             `json:"source_checksum"`
	ParserVersion         string             `json:"parser_version"`
	DirectoryStations     int                `json:"directory_stations"`
	UnpricedStations      int                `json:"unpriced_stations"`
	SourceObservations    int                `json:"source_observations"`
	IncludedObservations  int                `json:"included_observations"`
	StationCount          int                `json:"station_count"`
	UnlocatedObservations int                `json:"unlocated_observations"`
	Excluded              Exclusions         `json:"excluded"`
	FuelUnits             []FuelUnitCoverage `json:"fuel_units"`
}

type Summary struct {
	Metric              string    `json:"metric"`
	RevisionID          string    `json:"revision_id"`
	SurveyStart         string    `json:"survey_start"`
	SurveyEnd           string    `json:"survey_end"`
	Fuel                string    `json:"fuel"`
	Unit                string    `json:"unit"`
	Condition           string    `json:"condition"`
	Geography           Geography `json:"geography"`
	SourceObservations  int       `json:"source_observations"`
	StationCount        int       `json:"station_count"`
	StationSampleSHA256 string    `json:"station_sample_sha256"`
	SumMilliBRL         int64     `json:"sum_milli_brl"`
	MinMilliBRL         int64     `json:"min_milli_brl"`
	MaxMilliBRL         int64     `json:"max_milli_brl"`
	SpreadMilliBRL      int64     `json:"spread_milli_brl"`
	Method              string    `json:"method"`
}

type RankingEntry struct {
	Position         int    `json:"position"`
	StationID        string `json:"station_id"`
	AmountMilliBRL   int64  `json:"amount_milli_brl"`
	CollectedOn      string `json:"collected_on"`
	OperationStatus  string `json:"operation_status"`
	TieCount         int    `json:"tie_count"`
	TiedWithPrevious bool   `json:"tied_with_previous"`
}

type Ranking struct {
	Metric             string         `json:"metric"`
	RevisionID         string         `json:"revision_id"`
	SurveyStart        string         `json:"survey_start"`
	SurveyEnd          string         `json:"survey_end"`
	Fuel               string         `json:"fuel"`
	Unit               string         `json:"unit"`
	Condition          string         `json:"condition"`
	Geography          Geography      `json:"geography"`
	PopulationStations int            `json:"population_stations"`
	Entries            []RankingEntry `json:"entries"`
	Truncated          bool           `json:"truncated"`
	TiesAtCutoff       int            `json:"ties_at_cutoff"`
	Method             string         `json:"method"`
}

type HistoryPoint struct {
	RevisionID     string `json:"revision_id"`
	SurveyStart    string `json:"survey_start"`
	SurveyEnd      string `json:"survey_end"`
	CollectedOn    string `json:"collected_on,omitempty"`
	AmountMilliBRL *int64 `json:"amount_milli_brl,omitempty"`
	Missing        bool   `json:"missing"`
}

type History struct {
	Metric    string         `json:"metric"`
	StationID string         `json:"station_id"`
	Fuel      string         `json:"fuel"`
	Unit      string         `json:"unit"`
	Condition string         `json:"condition"`
	Points    []HistoryPoint `json:"points"`
	Method    string         `json:"method"`
}

type SummaryChange struct {
	Metric               string    `json:"metric"`
	PreviousRevisionID   string    `json:"previous_revision_id"`
	CurrentRevisionID    string    `json:"current_revision_id"`
	Fuel                 string    `json:"fuel"`
	Unit                 string    `json:"unit"`
	Geography            Geography `json:"geography"`
	PreviousStations     int       `json:"previous_stations"`
	CurrentStations      int       `json:"current_stations"`
	SampleChanged        bool      `json:"sample_changed"`
	Comparable           bool      `json:"comparable"`
	ChangeSumMilliBRL    *int64    `json:"change_sum_milli_brl,omitempty"`
	ChangeMinMilliBRL    *int64    `json:"change_min_milli_brl,omitempty"`
	ChangeMaxMilliBRL    *int64    `json:"change_max_milli_brl,omitempty"`
	ChangeSpreadMilliBRL *int64    `json:"change_spread_milli_brl,omitempty"`
	Method               string    `json:"method"`
}

type Definition struct {
	ID      string `json:"id"`
	Formula string `json:"formula"`
	Basis   string `json:"basis"`
}

type Input struct {
	ContentSHA256 string
	Stations      []enterpriseexport.Station
	Revisions     []enterpriseexport.Revision
	Prices        []enterpriseexport.Price
}

type Options struct {
	MaxRankingRows            int
	OmitStateRankings         bool
	OmitMunicipalitySummaries bool
}

type Projection struct {
	MetricsVersion     string       `json:"metrics_version"`
	InputContentSHA256 string       `json:"input_content_sha256"`
	Coverage           []Coverage   `json:"coverage"`
	Summaries          []Summary    `json:"summaries"`
	Rankings           []Ranking    `json:"rankings"`
	Definitions        []Definition `json:"definitions"`
}

type latestKey struct {
	station string
	fuel    string
	unit    string
}

type summaryKey struct {
	revision string
	fuel     string
	unit     string
	level    string
	code     string
}

type includedPrice struct {
	price   enterpriseexport.Price
	station enterpriseexport.Station
}

type summaryAccumulator struct {
	summary  Summary
	stations map[string]bool
}

type rankItem struct {
	station enterpriseexport.Station
	price   enterpriseexport.Price
}

type revisionInfo struct {
	revision enterpriseexport.Revision
	start    time.Time
	end      time.Time
}

// Compute builds the deterministic projection. Input order is irrelevant;
// duplicate station/revision identities and orphan revision rows are errors.
func Compute(in Input, opts Options) (Projection, error) {
	if in.ContentSHA256 == "" {
		return Projection{}, fmt.Errorf("%w: missing input content hash", ErrInvalidInput)
	}
	if opts.MaxRankingRows <= 0 {
		opts.MaxRankingRows = 1000
	}
	stations := make(map[string]enterpriseexport.Station, len(in.Stations))
	for _, station := range in.Stations {
		if station.StationID == "" {
			return Projection{}, fmt.Errorf("%w: station without id", ErrInvalidInput)
		}
		if _, exists := stations[station.StationID]; exists {
			return Projection{}, fmt.Errorf("%w: duplicate station %s", ErrInvalidInput, station.StationID)
		}
		stations[station.StationID] = station
	}
	revisions := make(map[string]revisionInfo, len(in.Revisions))
	ordered := make([]revisionInfo, 0, len(in.Revisions))
	for _, revision := range in.Revisions {
		if revision.RevisionID == "" {
			return Projection{}, fmt.Errorf("%w: revision without id", ErrInvalidInput)
		}
		if _, exists := revisions[revision.RevisionID]; exists {
			return Projection{}, fmt.Errorf("%w: duplicate revision %s", ErrInvalidInput, revision.RevisionID)
		}
		start, err := time.Parse("2006-01-02", revision.SurveyStart)
		if err != nil {
			return Projection{}, fmt.Errorf("%w: revision %s survey_start", ErrInvalidInput, revision.RevisionID)
		}
		end, err := time.Parse("2006-01-02", revision.SurveyEnd)
		if err != nil || end.Before(start) {
			return Projection{}, fmt.Errorf("%w: revision %s survey_end", ErrInvalidInput, revision.RevisionID)
		}
		info := revisionInfo{revision: revision, start: start, end: end}
		revisions[revision.RevisionID] = info
		ordered = append(ordered, info)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].revision.SurveyStart != ordered[j].revision.SurveyStart {
			return ordered[i].revision.SurveyStart < ordered[j].revision.SurveyStart
		}
		return ordered[i].revision.RevisionID < ordered[j].revision.RevisionID
	})
	pricesByRevision := make(map[string][]enterpriseexport.Price, len(in.Revisions))
	for _, price := range in.Prices {
		if _, ok := revisions[price.RevisionID]; !ok {
			return Projection{}, fmt.Errorf("%w: price %s has unknown revision %s", ErrInvalidInput, price.PriceID, price.RevisionID)
		}
		pricesByRevision[price.RevisionID] = append(pricesByRevision[price.RevisionID], price)
	}

	projection := Projection{
		MetricsVersion:     MetricsVersion,
		InputContentSHA256: in.ContentSHA256,
		Definitions:        definitions(),
	}
	for _, info := range ordered {
		coverage, summaries, rankings := computeRevision(info, stations, pricesByRevision[info.revision.RevisionID], opts)
		projection.Coverage = append(projection.Coverage, coverage)
		projection.Summaries = append(projection.Summaries, summaries...)
		projection.Rankings = append(projection.Rankings, rankings...)
	}
	sort.Slice(projection.Summaries, func(i, j int) bool {
		a, b := projection.Summaries[i], projection.Summaries[j]
		if a.SurveyStart != b.SurveyStart {
			return a.SurveyStart < b.SurveyStart
		}
		if a.RevisionID != b.RevisionID {
			return a.RevisionID < b.RevisionID
		}
		if a.Fuel != b.Fuel {
			return a.Fuel < b.Fuel
		}
		if a.Unit != b.Unit {
			return a.Unit < b.Unit
		}
		if a.Geography.Level != b.Geography.Level {
			return levelOrder(a.Geography.Level) < levelOrder(b.Geography.Level)
		}
		return a.Geography.Code < b.Geography.Code
	})
	sort.Slice(projection.Rankings, func(i, j int) bool {
		a, b := projection.Rankings[i], projection.Rankings[j]
		if a.SurveyStart != b.SurveyStart {
			return a.SurveyStart < b.SurveyStart
		}
		if a.RevisionID != b.RevisionID {
			return a.RevisionID < b.RevisionID
		}
		if a.Fuel != b.Fuel {
			return a.Fuel < b.Fuel
		}
		if a.Unit != b.Unit {
			return a.Unit < b.Unit
		}
		if a.Geography.Level != b.Geography.Level {
			return levelOrder(a.Geography.Level) < levelOrder(b.Geography.Level)
		}
		return a.Geography.Code < b.Geography.Code
	})
	return projection, nil
}

func computeRevision(info revisionInfo, stations map[string]enterpriseexport.Station, prices []enterpriseexport.Price, opts Options) (Coverage, []Summary, []Ranking) {
	revision := info.revision
	coverage := Coverage{
		Metric: MetricCoverage, RevisionID: revision.RevisionID,
		SurveyStart: revision.SurveyStart, SurveyEnd: revision.SurveyEnd,
		PublishedAt: revision.PublishedAt, SourceURL: revision.SourceURL,
		SourceChecksum: revision.SourceChecksum, ParserVersion: revision.ParserVersion,
		DirectoryStations: len(stations), Excluded: Exclusions{},
	}
	latest := make(map[latestKey]enterpriseexport.Price, len(prices))
	sourceCounts := make(map[summaryKey]int, len(prices)*3)
	for _, price := range prices {
		coverage.SourceObservations++
		station, ok := stations[price.StationID]
		if !ok {
			coverage.Excluded["unknown_station"]++
			continue
		}
		unit, err := kernel.Product(price.FuelProduct).Unit()
		if err != nil {
			coverage.Excluded["unknown_fuel"]++
			continue
		}
		if price.Unit != string(unit) {
			coverage.Excluded["unit_mismatch"]++
			continue
		}
		if price.Currency != "BRL" {
			coverage.Excluded["unsupported_currency"]++
			continue
		}
		collected, err := time.Parse("2006-01-02", price.CollectedOn)
		if err != nil {
			coverage.Excluded["invalid_date"]++
			continue
		}
		if collected.Before(info.start) || collected.After(info.end) {
			coverage.Excluded["period_mismatch"]++
			continue
		}
		for _, key := range summaryKeys(revision.RevisionID, price.FuelProduct, price.Unit, station, opts) {
			sourceCounts[key]++
		}
		key := latestKey{station: price.StationID, fuel: price.FuelProduct, unit: price.Unit}
		if previous, exists := latest[key]; exists {
			if newer(price, previous) {
				latest[key] = price
			}
			coverage.Excluded["older_observation"]++
			continue
		}
		latest[key] = price
	}

	included := make([]includedPrice, 0, len(latest))
	for _, price := range latest {
		included = append(included, includedPrice{price: price, station: stations[price.StationID]})
	}
	sort.Slice(included, func(i, j int) bool {
		if included[i].price.StationID != included[j].price.StationID {
			return included[i].price.StationID < included[j].price.StationID
		}
		if included[i].price.FuelProduct != included[j].price.FuelProduct {
			return included[i].price.FuelProduct < included[j].price.FuelProduct
		}
		return included[i].price.Unit < included[j].price.Unit
	})
	coverage.IncludedObservations = len(included)
	stationSet := map[string]bool{}
	fuelUnits := map[string]*FuelUnitCoverage{}
	for _, entry := range included {
		stationSet[entry.station.StationID] = true
		if entry.station.State == "" || entry.station.MunicipalityCode == "" {
			coverage.UnlocatedObservations++
		}
		key := entry.price.FuelProduct + "\x00" + entry.price.Unit
		item := fuelUnits[key]
		if item == nil {
			item = &FuelUnitCoverage{Fuel: entry.price.FuelProduct, Unit: entry.price.Unit}
			fuelUnits[key] = item
		}
		item.Stations++
		item.Observations++
	}
	coverage.StationCount = len(stationSet)
	coverage.UnpricedStations = coverage.DirectoryStations - coverage.StationCount
	if coverage.UnpricedStations < 0 {
		coverage.UnpricedStations = 0
	}
	for _, item := range fuelUnits {
		coverage.FuelUnits = append(coverage.FuelUnits, *item)
	}
	sort.Slice(coverage.FuelUnits, func(i, j int) bool {
		if coverage.FuelUnits[i].Fuel != coverage.FuelUnits[j].Fuel {
			return coverage.FuelUnits[i].Fuel < coverage.FuelUnits[j].Fuel
		}
		return coverage.FuelUnits[i].Unit < coverage.FuelUnits[j].Unit
	})

	summaries := buildSummaries(info, included, sourceCounts, opts)
	rankings := buildRankings(info, included, opts)
	return coverage, summaries, rankings
}

func newer(candidate, existing enterpriseexport.Price) bool {
	if candidate.CollectedOn != existing.CollectedOn {
		return candidate.CollectedOn > existing.CollectedOn
	}
	return candidate.PriceID > existing.PriceID
}

func summaryKeys(revisionID, fuel, unit string, station enterpriseexport.Station, opts Options) []summaryKey {
	keys := []summaryKey{{
		revision: revisionID, fuel: fuel, unit: unit,
		level: LevelNational, code: "BR",
	}}
	if station.State != "" {
		keys = append(keys, summaryKey{
			revision: revisionID, fuel: fuel, unit: unit,
			level: LevelState, code: station.State,
		})
		if !opts.OmitMunicipalitySummaries && station.MunicipalityCode != "" {
			keys = append(keys, summaryKey{
				revision: revisionID, fuel: fuel, unit: unit,
				level: LevelMunicipality, code: station.MunicipalityCode,
			})
		}
	}
	return keys
}

func buildSummaries(info revisionInfo, included []includedPrice, sourceCounts map[summaryKey]int, opts Options) []Summary {
	summaries := map[summaryKey]*summaryAccumulator{}
	for _, entry := range included {
		for _, key := range summaryKeys(info.revision.RevisionID, entry.price.FuelProduct, entry.price.Unit, entry.station, opts) {
			acc := summaries[key]
			if acc == nil {
				acc = &summaryAccumulator{
					summary: Summary{
						Metric: MetricSummary, RevisionID: info.revision.RevisionID,
						SurveyStart: info.revision.SurveyStart, SurveyEnd: info.revision.SurveyEnd,
						Fuel: entry.price.FuelProduct, Unit: entry.price.Unit,
						Condition:          "STANDARD",
						Geography:          Geography{Level: key.level, Code: key.code},
						SourceObservations: sourceCounts[key],
						MinMilliBRL:        entry.price.AmountMilliBRL,
						MaxMilliBRL:        entry.price.AmountMilliBRL,
						Method:             "Abastevo-derived from ANP station facts; station-level latest observation; not an ANP official summary",
					},
					stations: map[string]bool{},
				}
				summaries[key] = acc
			}
			acc.stations[entry.station.StationID] = true
			acc.summary.SumMilliBRL += entry.price.AmountMilliBRL
			if entry.price.AmountMilliBRL < acc.summary.MinMilliBRL {
				acc.summary.MinMilliBRL = entry.price.AmountMilliBRL
			}
			if entry.price.AmountMilliBRL > acc.summary.MaxMilliBRL {
				acc.summary.MaxMilliBRL = entry.price.AmountMilliBRL
			}
		}
	}
	out := make([]Summary, 0, len(summaries))
	for _, acc := range summaries {
		acc.summary.StationCount = len(acc.stations)
		acc.summary.StationSampleSHA256 = sampleHash(acc.stations)
		acc.summary.SpreadMilliBRL = acc.summary.MaxMilliBRL - acc.summary.MinMilliBRL
		out = append(out, acc.summary)
	}
	return out
}

func sampleHash(stations map[string]bool) string {
	ids := make([]string, 0, len(stations))
	for id := range stations {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	hash := sha256.New()
	for _, id := range ids {
		_, _ = hash.Write([]byte(id))
		_, _ = hash.Write([]byte("\n"))
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func buildRankings(info revisionInfo, included []includedPrice, opts Options) []Ranking {
	groups := map[string][]rankItem{}
	for _, entry := range included {
		key := entry.price.FuelProduct + "\x00" + entry.price.Unit
		groups[key] = append(groups[key], rankItem{station: entry.station, price: entry.price})
	}
	var rankings []Ranking
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		items := groups[key]
		fuel, unit := items[0].price.FuelProduct, items[0].price.Unit
		national := make([]rankItem, len(items))
		copy(national, items)
		sort.Slice(national, func(i, j int) bool {
			if national[i].price.AmountMilliBRL != national[j].price.AmountMilliBRL {
				return national[i].price.AmountMilliBRL < national[j].price.AmountMilliBRL
			}
			return national[i].station.StationID < national[j].station.StationID
		})
		rankings = append(rankings, makeRanking(info, fuel, unit, Geography{Level: LevelNational, Code: "BR"}, national, opts.MaxRankingRows))
		if opts.OmitStateRankings {
			continue
		}
		states := map[string][]rankItem{}
		for _, item := range items {
			if item.station.State == "" {
				continue
			}
			states[item.station.State] = append(states[item.station.State], item)
		}
		stateCodes := make([]string, 0, len(states))
		for code := range states {
			stateCodes = append(stateCodes, code)
		}
		sort.Strings(stateCodes)
		for _, code := range stateCodes {
			stateItems := states[code]
			sort.Slice(stateItems, func(i, j int) bool {
				if stateItems[i].price.AmountMilliBRL != stateItems[j].price.AmountMilliBRL {
					return stateItems[i].price.AmountMilliBRL < stateItems[j].price.AmountMilliBRL
				}
				return stateItems[i].station.StationID < stateItems[j].station.StationID
			})
			rankings = append(rankings, makeRanking(info, fuel, unit, Geography{Level: LevelState, Code: code}, stateItems, opts.MaxRankingRows))
		}
	}
	return rankings
}

func makeRanking(info revisionInfo, fuel, unit string, geography Geography, items []rankItem, max int) Ranking {
	counts := map[int64]int{}
	for _, item := range items {
		counts[item.price.AmountMilliBRL]++
	}
	entries := make([]RankingEntry, 0, len(items))
	for i, item := range items {
		entries = append(entries, RankingEntry{
			Position:         i + 1,
			StationID:        item.station.StationID,
			AmountMilliBRL:   item.price.AmountMilliBRL,
			CollectedOn:      item.price.CollectedOn,
			OperationStatus:  item.station.OperationStatus,
			TieCount:         counts[item.price.AmountMilliBRL],
			TiedWithPrevious: i > 0 && items[i-1].price.AmountMilliBRL == item.price.AmountMilliBRL,
		})
	}
	truncated := false
	tiesAtCutoff := 0
	if max > 0 && len(entries) > max {
		cutoff := entries[max-1].AmountMilliBRL
		end := max
		for end < len(entries) && entries[end].AmountMilliBRL == cutoff {
			end++
		}
		tiesAtCutoff = counts[cutoff]
		entries = entries[:end]
		truncated = true
	}
	return Ranking{
		Metric: MetricCheapest, RevisionID: info.revision.RevisionID,
		SurveyStart: info.revision.SurveyStart, SurveyEnd: info.revision.SurveyEnd,
		Fuel: fuel, Unit: unit, Condition: "STANDARD", Geography: geography,
		PopulationStations: len(items), Entries: entries,
		Truncated: truncated, TiesAtCutoff: tiesAtCutoff,
		Method: "ANP station facts within one survey revision; amount asc and station UUID tie-break; ties explicit",
	}
}

func levelOrder(level string) int {
	switch level {
	case LevelNational:
		return 0
	case LevelState:
		return 1
	case LevelMunicipality:
		return 2
	default:
		return 3
	}
}

func definitions() []Definition {
	return []Definition{
		{ID: MetricSummary, Formula: "count(distinct station), sum/min/max/spread of the latest station observation per fuel+unit within one published revision and geography", Basis: "ANP station facts; Abastevo-derived; never an ANP official summary or an average of municipal means"},
		{ID: MetricCheapest, Formula: "stations ordered by amount_milli_brl asc, station UUID asc; tie_count per amount; population is the stated geography", Basis: "ANP station facts within one survey revision; ties and exclusions explicit"},
		{ID: MetricCoverage, Formula: "every source row is included or counted in exactly one exclusion bucket; station_count is the included denominator", Basis: "normalized source revisions; no invented coverage"},
		{ID: MetricHistory, Formula: "one point per input revision for one station+fuel+unit; missing observation is a gap, never zero", Basis: "published station observations; period grid from the input revisions"},
		{ID: MetricComparison, Formula: "difference of summary min/max/sum/spread only when fuel+unit+geography and station_count match; otherwise sample_changed", Basis: "two compatible summaries; no percentage on changed sample composition"},
	}
}

// StationHistory returns one point per input revision for one station/fuel/unit.
// A revision without a valid observation is an explicit gap, never a zero price.
func StationHistory(prices []enterpriseexport.Price, revisions []enterpriseexport.Revision, stationID, fuel, unit string) (History, error) {
	if stationID == "" {
		return History{}, fmt.Errorf("%w: missing station id", ErrInvalidInput)
	}
	product := kernel.Product(fuel)
	expectedUnit, err := product.Unit()
	if err != nil {
		return History{}, fmt.Errorf("%w: unknown fuel %s", ErrInvalidInput, fuel)
	}
	if unit != string(expectedUnit) {
		return History{}, fmt.Errorf("%w: unit mismatch for %s", ErrInvalidInput, fuel)
	}
	ordered := make([]enterpriseexport.Revision, 0, len(revisions))
	seen := map[string]bool{}
	windows := map[string]revisionInfo{}
	for _, revision := range revisions {
		if revision.RevisionID == "" || seen[revision.RevisionID] {
			return History{}, fmt.Errorf("%w: invalid or duplicate revision", ErrInvalidInput)
		}
		seen[revision.RevisionID] = true
		start, err := time.Parse("2006-01-02", revision.SurveyStart)
		if err != nil {
			return History{}, fmt.Errorf("%w: revision %s survey_start", ErrInvalidInput, revision.RevisionID)
		}
		end, err := time.Parse("2006-01-02", revision.SurveyEnd)
		if err != nil || end.Before(start) {
			return History{}, fmt.Errorf("%w: revision %s survey_end", ErrInvalidInput, revision.RevisionID)
		}
		windows[revision.RevisionID] = revisionInfo{revision: revision, start: start, end: end}
		ordered = append(ordered, revision)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].SurveyStart != ordered[j].SurveyStart {
			return ordered[i].SurveyStart < ordered[j].SurveyStart
		}
		return ordered[i].RevisionID < ordered[j].RevisionID
	})
	latest := map[string]enterpriseexport.Price{}
	for _, price := range prices {
		if price.StationID != stationID || price.FuelProduct != fuel || price.Unit != unit || price.Currency != "BRL" {
			continue
		}
		info, ok := windows[price.RevisionID]
		if !ok {
			return History{}, fmt.Errorf("%w: price %s has unknown revision", ErrInvalidInput, price.PriceID)
		}
		collected, err := time.Parse("2006-01-02", price.CollectedOn)
		if err != nil || collected.Before(info.start) || collected.After(info.end) {
			continue
		}
		if previous, exists := latest[price.RevisionID]; !exists || newer(price, previous) {
			latest[price.RevisionID] = price
		}
	}
	history := History{
		Metric: MetricHistory, StationID: stationID, Fuel: fuel, Unit: unit,
		Condition: "STANDARD",
		Method:    "One point per input revision; latest station observation; absent weeks are gaps",
	}
	for _, revision := range ordered {
		point := HistoryPoint{
			RevisionID:  revision.RevisionID,
			SurveyStart: revision.SurveyStart, SurveyEnd: revision.SurveyEnd,
			Missing: true,
		}
		if price, ok := latest[revision.RevisionID]; ok {
			amount := price.AmountMilliBRL
			point.AmountMilliBRL = &amount
			point.CollectedOn = price.CollectedOn
			point.Missing = false
		}
		history.Points = append(history.Points, point)
	}
	return history, nil
}

// CompareSummaries compares two compatible summaries. It refuses to compute a
// change when the sample composition (station count) differs; the caller must
// report the sample change instead of hiding it in a percentage.
func CompareSummaries(previous, current Summary) (SummaryChange, error) {
	if previous.Metric != MetricSummary || current.Metric != MetricSummary {
		return SummaryChange{}, fmt.Errorf("%w: not summary metrics", ErrInvalidInput)
	}
	if previous.Fuel != current.Fuel || previous.Unit != current.Unit ||
		previous.Geography != current.Geography {
		return SummaryChange{}, fmt.Errorf("%w: incompatible summaries", ErrInvalidInput)
	}
	if previous.RevisionID == current.RevisionID {
		return SummaryChange{}, fmt.Errorf("%w: same revision", ErrInvalidInput)
	}
	change := SummaryChange{
		Metric:             MetricComparison,
		PreviousRevisionID: previous.RevisionID, CurrentRevisionID: current.RevisionID,
		Fuel: current.Fuel, Unit: current.Unit, Geography: current.Geography,
		PreviousStations: previous.StationCount, CurrentStations: current.StationCount,
		Method: "Comparable only with identical fuel+unit+geography and station count; otherwise sample_changed",
	}
	if previous.StationSampleSHA256 == "" || current.StationSampleSHA256 == "" {
		return SummaryChange{}, fmt.Errorf("%w: missing station sample hash", ErrInvalidInput)
	}
	if previous.StationCount != current.StationCount ||
		previous.StationSampleSHA256 != current.StationSampleSHA256 {
		change.SampleChanged = true
		return change, nil
	}
	if previous.StationCount == 0 {
		return change, nil
	}
	sum := current.SumMilliBRL - previous.SumMilliBRL
	min := current.MinMilliBRL - previous.MinMilliBRL
	max := current.MaxMilliBRL - previous.MaxMilliBRL
	spread := current.SpreadMilliBRL - previous.SpreadMilliBRL
	change.Comparable = true
	change.ChangeSumMilliBRL = &sum
	change.ChangeMinMilliBRL = &min
	change.ChangeMaxMilliBRL = &max
	change.ChangeSpreadMilliBRL = &spread
	return change, nil
}
