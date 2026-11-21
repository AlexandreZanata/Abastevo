package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"time"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgdata"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/jackc/pgx/v5"
)

var (
	ErrSeriesUnavailable = errors.New("orgdata: regional series unavailable")
	ErrSeriesCursor      = errors.New("orgdata: invalid regional series cursor")
)

type SeriesInput struct {
	Dataset   string
	Revisions []int64
	Limit     int
	Cursor    string
}

type SeriesSource struct {
	Code        domain.Source `json:"code"`
	Digest      string        `json:"digest"`
	License     string        `json:"license"`
	Attribution string        `json:"attribution"`
}

type SeriesPeriod struct {
	CatalogRevision  string          `json:"catalog_revision"`
	OfficialRevision string          `json:"official_revision"`
	Snapshot         string          `json:"snapshot"`
	Readiness        string          `json:"readiness"`
	MinimumStations  int             `json:"minimum_stations"`
	SurveyStart      string          `json:"survey_start"`
	SurveyEnd        string          `json:"survey_end"`
	PublishedAt      string          `json:"published_at"`
	SourceURL        string          `json:"source_url"`
	Sources          [2]SeriesSource `json:"sources"`
	Samples          []domain.Sample `json:"samples"`
}

type SeriesPage struct {
	Dataset    string         `json:"dataset"`
	Region     string         `json:"region"`
	Method     string         `json:"method"`
	Condition  string         `json:"condition"`
	NextCursor string         `json:"next_cursor"`
	Periods    []SeriesPeriod `json:"periods"`
}

type RegionalSeries struct {
	Meter        UsageMeter
	CursorSecret []byte
}

func (s RegionalSeries) List(ctx context.Context, org kernel.OrgScope, key domain.Bearer, operationID string, input SeriesInput) (SeriesPage, error) {
	if len(s.CursorSecret) < 32 {
		return SeriesPage{}, ErrSeriesUnavailable
	}
	if !domain.ValidDatasetKey(input.Dataset) || len(input.Revisions) == 0 || len(input.Revisions) > 32 || len(input.Cursor) > 2048 {
		return SeriesPage{}, domain.ErrInvalidDataset
	}
	if input.Limit == 0 {
		input.Limit = httpapi.DefaultLimit
	}
	if input.Limit < 1 || input.Limit > httpapi.MaxLimit {
		return SeriesPage{}, domain.ErrInvalidDataset
	}
	input.Revisions = slices.Clone(input.Revisions)
	slices.Sort(input.Revisions)
	for i, revision := range input.Revisions {
		if revision < 1 || i > 0 && input.Revisions[i-1] == revision {
			return SeriesPage{}, domain.ErrInvalidDataset
		}
	}
	encoded, _ := json.Marshal(input)
	call := UsageCall{ID: operationID, Hash: sha256.Sum256(encoded)}
	var result SeriesPage
	err := s.Meter.Read(ctx, org, key, domain.ReadRegional, call, func(ctx context.Context, tx pgx.Tx, authority UsageAuthority) error {
		basis, _ := json.Marshal([]any{"regional-series.v1", org.OrgID().String(), authority.FamilyID,
			input.Dataset, input.Revisions, input.Limit})
		binding := httpapi.FilterHash(string(basis))
		start := 0
		if input.Cursor != "" {
			offset, err := httpapi.Open(s.CursorSecret, input.Cursor, binding, authority.At)
			if err != nil {
				return ErrSeriesCursor
			}
			start, err = strconv.Atoi(offset)
			if err != nil || start < 1 || start >= len(input.Revisions) {
				return ErrSeriesCursor
			}
		}
		stop := min(start+input.Limit, len(input.Revisions))
		result = SeriesPage{Dataset: input.Dataset, Condition: "STANDARD", Periods: make([]SeriesPeriod, 0, stop-start)}
		for i, revision := range input.Revisions {
			admitted, err := readScopedDataset(ctx, tx, authority, input.Dataset, revision)
			if err != nil {
				return err
			}
			definition := admitted.Definition
			if i == 0 {
				result.Region, result.Method = definition.MarketState, definition.Method
			} else if result.Region != definition.MarketState || result.Method != definition.Method {
				return domain.ErrInvalidDataset
			}
			if i >= start && i < stop {
				period, err := readPeriod(ctx, tx, admitted)
				if err != nil {
					return err
				}
				result.Periods = append(result.Periods, period)
			}
		}
		if stop < len(input.Revisions) {
			result.NextCursor = httpapi.Seal(s.CursorSecret, binding, strconv.Itoa(stop), authority.At)
		}
		return nil
	})
	if err != nil {
		return SeriesPage{}, err
	}
	return result, nil
}

func readScopedDataset(ctx context.Context, tx pgx.Tx, authority UsageAuthority, key string, revision int64) (AdmittedDataset, error) {
	plan := authority.Plan
	allowed, err := db.New(tx).CheckPlanDataset(ctx, db.CheckPlanDatasetParams{
		ProductCode: plan.Product, PlanCode: plan.Plan, PlanRevision: plan.Revision, DatasetKey: key, DatasetRevision: revision,
	})
	if err != nil {
		return AdmittedDataset{}, err
	}
	if !allowed {
		return AdmittedDataset{}, ErrDatasetNotFound
	}
	return (Catalog{DB: tx}).ReadAdmitted(ctx, key, revision)
}

func readPeriod(ctx context.Context, tx pgx.Tx, admitted AdmittedDataset) (SeriesPeriod, error) {
	definition := admitted.Definition
	q := db.New(tx)
	row, err := q.ReadSeriesBasis(ctx, db.ReadSeriesBasisParams{DatasetKey: definition.Key, DatasetRevision: definition.Revision})
	if errors.Is(err, pgx.ErrNoRows) {
		return SeriesPeriod{}, ErrSeriesUnavailable
	}
	if err != nil {
		return SeriesPeriod{}, err
	}
	samples, err := q.ListSeriesSamples(ctx, db.ListSeriesSamplesParams{DatasetKey: definition.Key, DatasetRevision: definition.Revision})
	if err != nil {
		return SeriesPeriod{}, err
	}
	series := domain.Series{
		SurveyStart: row.SurveyStart.Time.Format("2006-01-02"), SurveyEnd: row.SurveyEnd.Time.Format("2006-01-02"),
		PublishedAt: row.PublishedAt.Time.UTC(), SourceURL: row.SourceUrl,
		Samples: make([]domain.Sample, 0, len(samples)),
	}
	for _, sample := range samples {
		series.Samples = append(series.Samples, domain.Sample{Fuel: kernel.Product(sample.FuelProduct),
			Unit: kernel.Unit(sample.UnitCode), Stations: sample.Stations, Observations: sample.Observations,
			SumMilliBRL: sample.SumMilliBrl, MinimumMilliBRL: sample.MinimumMilliBrl, MaximumMilliBRL: sample.MaximumMilliBrl})
	}
	if series.Validate(definition.MinimumStations) != nil {
		return SeriesPeriod{}, ErrSeriesUnavailable
	}
	digest, err := series.Digest()
	if err != nil || digest != row.ResultSha256 {
		return SeriesPeriod{}, ErrSeriesUnavailable
	}
	period := SeriesPeriod{CatalogRevision: strconv.FormatInt(definition.Revision, 10), OfficialRevision: definition.OfficialRevision,
		Snapshot: definition.Snapshot, Readiness: "AVAILABLE", MinimumStations: definition.MinimumStations,
		SurveyStart: series.SurveyStart, SurveyEnd: series.SurveyEnd, PublishedAt: series.PublishedAt.Format(time.RFC3339Nano),
		SourceURL: series.SourceURL, Samples: series.Samples}
	if len(series.Samples) == 0 {
		period.Readiness = "INSUFFICIENT_DATA"
	}
	for i, review := range admitted.Reviews {
		period.Sources[i] = SeriesSource{Code: review.Pin.Source, Digest: review.Pin.Digest, License: review.LicenseRef, Attribution: review.Attribution}
	}
	return period, nil
}
