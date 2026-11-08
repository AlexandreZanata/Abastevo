package read

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	official "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/official"
	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/application"
)

// LatestTwoCurrentPublishedRevisions returns the current published revisions
// of the latest and the latest strictly earlier survey window.
func (r *Reader) LatestTwoCurrentPublishedRevisions(ctx context.Context) (application.BulletinRevision, application.BulletinRevision, error) {
	row, err := official.New(r.pool).LatestTwoCurrentPublishedRevisions(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.BulletinRevision{}, application.BulletinRevision{}, application.ErrBulletinUnavailable
	}
	if err != nil {
		return application.BulletinRevision{}, application.BulletinRevision{}, err
	}
	return application.BulletinRevision{
		RevisionID:     uuidString(row.CurrentID),
		SurveyStart:    dateString(row.CurrentSurveyStart),
		SurveyEnd:      dateString(row.CurrentSurveyEnd),
		PublishedAt:    rankingPublishedAt(row.CurrentPublishedAt),
		SourceURL:      row.CurrentSourceUrl,
		SourceChecksum: row.CurrentSourceChecksum,
		ParserVersion:  row.CurrentParserVersion,
	}, application.BulletinRevision{
		RevisionID:     uuidString(row.PreviousID),
		SurveyStart:    dateString(row.PreviousSurveyStart),
		SurveyEnd:      dateString(row.PreviousSurveyEnd),
		PublishedAt:    rankingPublishedAt(row.PreviousPublishedAt),
		SourceURL:      row.PreviousSourceUrl,
		SourceChecksum: row.PreviousSourceChecksum,
		ParserVersion:  row.PreviousParserVersion,
	}, nil
}

// ReadBulletinSamples returns the exact UF aggregates for one revision.
func (r *Reader) ReadBulletinSamples(ctx context.Context, revisionID, fuel, unit string) ([]application.BulletinSample, error) {
	revUUID, err := mustUUID(revisionID)
	if err != nil {
		return nil, application.ErrBulletinInvalid
	}
	rows, err := official.New(r.pool).BulletinRevisionSamples(ctx, official.BulletinRevisionSamplesParams{
		RevisionID:  revUUID,
		FuelProduct: fuel,
		Unit:        unit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]application.BulletinSample, 0, len(rows))
	for _, row := range rows {
		if !row.Uf.Valid {
			return nil, application.ErrBulletinInvalidAggregate
		}
		sum, err := application.ParseMilliSum(row.SumMilliBrl)
		if err != nil {
			return nil, err
		}
		out = append(out, application.BulletinSample{
			UF:               row.Uf.String,
			SumMilliBrl:      sum,
			ObservationCount: row.ObservationCount,
			Coverage: application.BulletinCoverage{
				SourceObservations: row.SourceObservations,
				SampledStations:    row.SampledStations,
				SourceStations:     row.SourceStations,
			},
		})
	}
	return out, nil
}
