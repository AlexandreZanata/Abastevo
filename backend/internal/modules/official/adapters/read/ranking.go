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

// ResolveRankingRevision resolves the current published revision for one ANP
// survey week. This is the cheap freshness check the HTTP cache performs on
// every request before serving a cached page.
func (r *Reader) ResolveRankingRevision(ctx context.Context, surveyWeekStart string) (application.RankingRevision, error) {
	week, err := time.Parse("2006-01-02", surveyWeekStart)
	if err != nil {
		return application.RankingRevision{}, application.ErrInvalidFilter
	}
	revision, err := official.New(r.pool).CurrentRankingRevision(ctx, pgtype.Date{Time: week, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return application.RankingRevision{}, application.ErrRankingUnavailable
	}
	if err != nil {
		return application.RankingRevision{}, err
	}
	return application.RankingRevision{
		RevisionID:      uuidString(revision.ID),
		SurveyWeekStart: dateString(revision.SurveyStart),
		SurveyWeekEnd:   dateString(revision.SurveyEnd),
		PublishedAt:     rankingPublishedAt(revision.PublishedAt),
		SourceURL:       revision.SourceUrl,
		SourceChecksum:  revision.SourceChecksum,
	}, nil
}

// ReadRankingAt reads only the resolved revision's latest observation per
// station. It never writes an aggregate/projection: a corrected revision is
// visible as soon as the current pointer switches. q.After is an optional
// keyset position already decoded from a sealed cursor.
func (r *Reader) ReadRankingAt(ctx context.Context, q application.RankingQuery, revision application.RankingRevision) (application.RankingData, error) {
	revUUID, err := mustUUID(revision.RevisionID)
	if err != nil {
		return application.RankingData{}, application.ErrInvalidFilter
	}
	queries := official.New(r.pool)
	uf, municipality := rankingGeographyFilters(q)
	aggregates, err := queries.RankingMunicipalityAggregates(ctx, official.RankingMunicipalityAggregatesParams{
		Uf:               uf,
		MunicipalityCode: municipality,
		RevisionID:       revUUID,
		FuelProduct:      q.Product.Fuel,
		Unit:             q.Product.Unit,
	})
	if err != nil {
		return application.RankingData{}, err
	}
	sourceStations, err := queries.RankingSourceStations(ctx, official.RankingSourceStationsParams{
		RevisionID:       revUUID,
		Uf:               uf,
		MunicipalityCode: municipality,
	})
	if err != nil {
		return application.RankingData{}, err
	}
	sourceObservations, err := queries.RankingSourceObservations(ctx, official.RankingSourceObservationsParams{
		RevisionID:       revUUID,
		FuelProduct:      q.Product.Fuel,
		Unit:             q.Product.Unit,
		Uf:               uf,
		MunicipalityCode: municipality,
	})
	if err != nil {
		return application.RankingData{}, err
	}
	data := application.RankingData{
		Revision:           revision,
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
			RevisionID:  revUUID,
			FuelProduct: q.Product.Fuel,
			Unit:        q.Product.Unit,
		})
		if err != nil {
			return application.RankingData{}, err
		}
	}
	if q.Metric.Kind == "OBSERVATION_PRICE" {
		hasAfter := q.After != nil
		var afterAmount int64
		var afterID pgtype.UUID
		if hasAfter {
			afterAmount = q.After.AmountMilliBrl
			afterID, err = mustUUID(q.After.StationID)
			if err != nil {
				return application.RankingData{}, application.ErrInvalidFilter
			}
		}
		rows, err := queries.RankingStationObservations(ctx, official.RankingStationObservationsParams{
			Uf:               uf,
			MunicipalityCode: municipality,
			HasAfter:         hasAfter,
			SortAsc:          q.Metric.Sort == "ASC",
			AfterAmount:      afterAmount,
			AfterID:          afterID,
			LimitPlusOne:     int32(q.Limit + 1),
			RevisionID:       revUUID,
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
