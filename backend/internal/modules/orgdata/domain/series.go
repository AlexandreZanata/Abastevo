package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"sort"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

const surveyDate = "2006-01-02"

type Sample struct {
	Fuel            kernel.Product `json:"fuel"`
	Unit            kernel.Unit    `json:"unit"`
	Stations        int32          `json:"stations"`
	Observations    int32          `json:"observations"`
	SumMilliBRL     int64          `json:"sum_milli_brl"`
	MinimumMilliBRL int64          `json:"min_milli_brl"`
	MaximumMilliBRL int64          `json:"max_milli_brl"`
}

func (s Sample) Valid(threshold int) bool {
	unit, err := s.Fuel.Unit()
	return err == nil && unit == s.Unit && int(s.Stations) >= threshold && s.Observations >= s.Stations &&
		s.MinimumMilliBRL >= kernel.MinMilliBRL && s.MaximumMilliBRL <= kernel.MaxMilliBRL &&
		s.MinimumMilliBRL <= s.MaximumMilliBRL && s.SumMilliBRL >= int64(s.Stations)*s.MinimumMilliBRL &&
		s.SumMilliBRL <= int64(s.Stations)*s.MaximumMilliBRL
}

type Series struct {
	SurveyStart string
	SurveyEnd   string
	PublishedAt time.Time
	SourceURL   string
	Samples     []Sample
}

func (s Series) Digest() (string, error) {
	record, err := json.Marshal(s)
	if err != nil {
		return "", ErrSourceBinding
	}
	digest := sha256.Sum256(record)
	return hex.EncodeToString(digest[:]), nil
}

func (s Series) Validate(threshold int) error {
	start, startErr := time.Parse(surveyDate, s.SurveyStart)
	end, endErr := time.Parse(surveyDate, s.SurveyEnd)
	source, urlErr := url.Parse(s.SourceURL)
	if startErr != nil || endErr != nil || end.Before(start) || s.PublishedAt.IsZero() ||
		s.PublishedAt.Year() < 1 || s.PublishedAt.Year() > 9999 || !publicText(s.SourceURL, 2048) ||
		urlErr != nil || source.Scheme != "https" || source.Hostname() == "" || source.User != nil {
		return ErrSourceBinding
	}
	seen := make(map[kernel.Product]bool, len(s.Samples))
	for _, sample := range s.Samples {
		if seen[sample.Fuel] || !sample.Valid(threshold) {
			return ErrSourceBinding
		}
		seen[sample.Fuel] = true
	}
	return nil
}

// PrepareSeries accepts the already verified Core export only through the
// preparer. It takes the admitted region and revision before computing metrics
// and keeps only approved aggregate fields.
func PrepareSeries(definition Definition, manifest enterpriseexport.Manifest, snapshot enterpriseexport.Snapshot) (Series, error) {
	if err := definition.MatchManifest(manifest); err != nil {
		return Series{}, err
	}
	var revision enterpriseexport.Revision
	var pin enterpriseexport.RevisionPin
	for _, candidate := range snapshot.Revisions {
		if candidate.RevisionID == definition.OfficialRevision {
			if revision.RevisionID != "" {
				return Series{}, ErrSourceBinding
			}
			revision = candidate
		}
	}
	for _, candidate := range manifest.Sources.Official {
		if candidate.RevisionID == definition.OfficialRevision {
			pin = candidate
			break
		}
	}
	if revision.RevisionID == "" || enterpriseexport.RevisionPin(revision) != pin {
		return Series{}, ErrSourceBinding
	}
	published, err := time.Parse(time.RFC3339, revision.PublishedAt)
	if err != nil {
		return Series{}, ErrSourceBinding
	}
	output := Series{SurveyStart: revision.SurveyStart, SurveyEnd: revision.SurveyEnd,
		PublishedAt: published.UTC().Truncate(time.Microsecond), SourceURL: revision.SourceURL, Samples: []Sample{}}
	input := analytics.Input{ContentSHA256: definition.Snapshot, Revisions: []enterpriseexport.Revision{revision}}
	stations := make(map[string]bool)
	for _, station := range snapshot.Stations {
		if station.State == definition.MarketState {
			stations[station.StationID] = true
			input.Stations = append(input.Stations, station)
		}
	}
	for _, price := range snapshot.Prices {
		if price.RevisionID != definition.OfficialRevision || !stations[price.StationID] {
			continue
		}
		if price.AmountMilliBRL < kernel.MinMilliBRL || price.AmountMilliBRL > kernel.MaxMilliBRL {
			return Series{}, ErrSourceBinding
		}
		input.Prices = append(input.Prices, price)
	}
	projection, err := analytics.Compute(input, analytics.Options{OmitStateRankings: true, OmitMunicipalitySummaries: true})
	if err != nil {
		return Series{}, ErrSourceBinding
	}
	for _, summary := range projection.Summaries {
		if summary.Geography.Level != analytics.LevelState || summary.Geography.Code != definition.MarketState ||
			summary.StationCount < definition.MinimumStations {
			continue
		}
		output.Samples = append(output.Samples, Sample{
			Fuel: kernel.Product(summary.Fuel), Unit: kernel.Unit(summary.Unit),
			Stations: int32(summary.StationCount), Observations: int32(summary.SourceObservations),
			SumMilliBRL: summary.SumMilliBRL, MinimumMilliBRL: summary.MinMilliBRL, MaximumMilliBRL: summary.MaxMilliBRL,
		})
	}
	sort.Slice(output.Samples, func(i, j int) bool { return output.Samples[i].Fuel < output.Samples[j].Fuel })
	if err := output.Validate(definition.MinimumStations); err != nil {
		return Series{}, err
	}
	return output, nil
}
