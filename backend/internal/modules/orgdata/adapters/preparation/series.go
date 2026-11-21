// Package preparation owns offline preparation of approved aggregate artifacts.
package preparation

import (
	"context"
	"errors"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgdata"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Prepare(ctx context.Context, pool *pgxpool.Pool, key string, revision int64, directory string) (string, error) {
	if pool == nil || directory == "" {
		return "", app.ErrUnavailable
	}
	var admitted adapters.AdmittedDataset
	if err := database.WithOrgBootstrapTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		admitted, err = (adapters.Catalog{DB: tx}).ReadAdmitted(ctx, key, revision)
		return err
	}); err != nil {
		return "", err
	}
	definition := admitted.Definition
	manifest, snapshot, err := enterpriseexport.ReadVerifiedCore(ctx, directory, definition.Snapshot, definition.MatchManifest)
	if err != nil {
		return "", err
	}
	result, err := domain.PrepareSeries(definition, manifest, snapshot)
	if err != nil {
		return "", err
	}
	digest, err := result.Digest()
	if err != nil {
		return "", err
	}
	err = database.WithOrgBootstrapTx(ctx, pool, func(tx pgx.Tx) error {
		current, err := (adapters.Catalog{DB: tx}).ReadAdmitted(ctx, key, revision)
		if err != nil {
			return err
		}
		if current != admitted {
			return domain.ErrRightsPending
		}
		return installSeries(ctx, tx, definition, result, digest)
	})
	if err != nil {
		return "", err
	}
	return digest, nil
}

func installSeries(ctx context.Context, tx pgx.Tx, definition domain.Definition, series domain.Series, digest string) error {
	result, err := tx.Exec(ctx, `INSERT INTO enterprise_regional_series
(dataset_key,dataset_revision,survey_start,survey_end,published_at,source_url,result_sha256)
VALUES($1,$2,$3::text::date,$4::text::date,$5,$6,$7)
ON CONFLICT(dataset_key,dataset_revision) DO NOTHING`, definition.Key, definition.Revision,
		series.SurveyStart, series.SurveyEnd, series.PublishedAt, series.SourceURL, digest)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		previous, err := db.New(tx).ReadSeriesBasis(ctx, db.ReadSeriesBasisParams{DatasetKey: definition.Key, DatasetRevision: definition.Revision})
		if errors.Is(err, pgx.ErrNoRows) || err == nil && previous.ResultSha256 != digest {
			return domain.ErrSourceBinding
		}
		return err
	}
	for _, sample := range series.Samples {
		if _, err := tx.Exec(ctx, `INSERT INTO enterprise_regional_series_samples
(dataset_key,dataset_revision,fuel_product,unit_code,stations,observations,sum_milli_brl,minimum_milli_brl,maximum_milli_brl)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, definition.Key, definition.Revision, sample.Fuel, sample.Unit,
			sample.Stations, sample.Observations, sample.SumMilliBRL, sample.MinimumMilliBRL, sample.MaximumMilliBRL); err != nil {
			return err
		}
	}
	return nil
}
