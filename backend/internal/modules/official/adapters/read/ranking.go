package read

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	official "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/official"
	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/application"
)

// ReadRanking resolves the current published revision for the requested ANP
// survey week and reads only that revision's latest observation per station.
// It never writes an aggregate/projection: a corrected revision is visible as
// soon as the current pointer switches.
func (r *Reader) ReadRanking(ctx context.Context, q application.RankingQuery) (application.RankingData, error) {
	week, err := time.Parse("2006-01-02", q.Source.SurveyWeekStart)
	if err != nil {
		return application.RankingData{}, application.ErrInvalidFilter
	}
	queries := official.New(r.pool)
	revision, err := queries.CurrentRankingRevision(ctx, pgtype.Date{Time: week, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return application.RankingData{}, application.ErrRankingUnavailable
	}
	if err != nil {
		return application.RankingData{}, err
	}
	uf, municipality := rankingGeographyFilters(q)
	aggregates, err := queries.RankingMunicipalityAggregates(ctx, official.RankingMunicipalityAggregatesParams{
		Uf:               uf,
		MunicipalityCode: municipality,
		RevisionID:       revision.ID,
		FuelProduct:      q.Product.Fuel,
		Unit:             q.Product.Unit,
	})
	if err != nil {
		return application.RankingData{}, err
	}
	sourceStations, err := queries.RankingSourceStations(ctx, official.RankingSourceStationsParams{
		RevisionID:       revision.ID,
		Uf:               uf,
		MunicipalityCode: municipality,
	})
	if err != nil {
		return application.RankingData{}, err
	}
	sourceObservations, err := queries.RankingSourceObservations(ctx, official.RankingSourceObservationsParams{
		RevisionID:       revision.ID,
		FuelProduct:      q.Product.Fuel,
		Unit:             q.Product.Unit,
		Uf:               uf,
		MunicipalityCode: municipality,
	})
	if err != nil {
		return application.RankingData{}, err
	}
	data := application.RankingData{
		Revision: application.RankingRevision{
			RevisionID:      uuidString(revision.ID),
			SurveyWeekStart: dateString(revision.SurveyStart),
			SurveyWeekEnd:   dateString(revision.SurveyEnd),
			PublishedAt:     rankingPublishedAt(revision.PublishedAt),
			SourceURL:       revision.SourceUrl,
			SourceChecksum:  revision.SourceChecksum,
		},
		Municipalities:     make([]application.MunicipalityAggregate, 0, len(aggregates)),
		SourceStations:     sourceStations,
		SourceObservations: sourceObservations,
	}
	for _, row := range aggregates {
		data.Municipalities = append(data.Municipalities, application.MunicipalityAggregate{
			State:            row.State.String,
			MunicipalityCode: row.MunicipalityCode.String,
			SumMilliBrl:      row.SumMilliBrl,
			ObservationCount: row.ObservationCount,
			MinMilliBrl:      row.MinMilliBrl,
			MaxMilliBrl:      row.MaxMilliBrl,
		})
	}
	if q.Geography.Level == "COUNTRY" {
		data.UnlocatedObservations, err = queries.RankingUnlocatedObservations(ctx, official.RankingUnlocatedObservationsParams{
			RevisionID:  revision.ID,
			FuelProduct: q.Product.Fuel,
			Unit:        q.Product.Unit,
		})
		if err != nil {
			return application.RankingData{}, err
		}
	}
	if q.Metric.Kind == "OBSERVATION_PRICE" {
		rows, err := queries.RankingStationObservations(ctx, official.RankingStationObservationsParams{
			Uf:               uf,
			MunicipalityCode: municipality,
			SortAsc:          q.Metric.Sort == "ASC",
			LimitPlusOne:     int32(q.Limit + 1),
			RevisionID:       revision.ID,
			FuelProduct:      q.Product.Fuel,
			Unit:             q.Product.Unit,
		})
		if err != nil {
			return application.RankingData{}, err
		}
		data.Stations = make([]application.StationObservationRow, 0, len(rows))
		for _, row := range rows {
			data.Stations = append(data.Stations, application.StationObservationRow{
				StationID:        uuidString(row.StationID),
				AmountMilliBrl:   row.AmountMilliBrl,
				CollectedOn:      dateString(row.CollectedOn),
				State:            optionalText(row.State),
				MunicipalityCode: optionalText(row.MunicipalityCode),
			})
		}
	}
	return data, nil
}

func rankingGeographyFilters(q application.RankingQuery) (uf, municipality string) {
	switch q.Geography.Level {
	case "STATE":
		return q.Geography.UF, ""
	case "MUNICIPALITY":
		return q.Geography.UF, q.Geography.IBGECode
	default:
		return "", ""
	}
}

func rankingPublishedAt(value pgtype.Timestamptz) string {
	if !value.Valid {
		return ""
	}
	return value.Time.UTC().Format(time.RFC3339)
}

func optionalText(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}
