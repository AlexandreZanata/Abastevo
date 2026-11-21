package adapters

import (
	"context"
	"errors"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgdata"
	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/domain"
	"github.com/jackc/pgx/v5"
)

var ErrDatasetNotFound = errors.New("orgdata: dataset version not found")

type Catalog struct{ queries *db.Queries }

// OpenCatalog retains the source decision through a READ COMMITTED transaction.
// Review writers take the exclusive side of the lock before inserting a decision.
func OpenCatalog(ctx context.Context, tx pgx.Tx) (Catalog, error) {
	if tx == nil {
		return Catalog{}, domain.ErrInvalidDataset
	}
	q := db.New(tx)
	if err := q.LockDataSourceRights(ctx); err != nil {
		return Catalog{}, err
	}
	return Catalog{queries: q}, nil
}

type AdmittedDataset struct {
	Definition domain.Definition
	Reviews    [2]domain.Review
}

// ReadAdmitted checks both current source reviews in the same statement.
// The serving path must repeat that check for each delivered result.
func (c Catalog) ReadAdmitted(ctx context.Context, key string, revision int64) (AdmittedDataset, error) {
	if c.queries == nil || key == "" || len(key) > 64 || revision < 1 {
		return AdmittedDataset{}, domain.ErrInvalidDataset
	}
	row, err := c.queries.ReadDatasetVersion(ctx, db.ReadDatasetVersionParams{DatasetKey: key, Revision: revision})
	if errors.Is(err, pgx.ErrNoRows) {
		return AdmittedDataset{}, ErrDatasetNotFound
	}
	if err != nil {
		return AdmittedDataset{}, err
	}
	registry := domain.SourcePin{Source: domain.PublicRegistry, Digest: row.RegistryDigest}
	prices := domain.SourcePin{Source: domain.OfficialPrices, Digest: row.PriceDigest}
	admitted := AdmittedDataset{
		Definition: domain.Definition{
			Key: row.DatasetKey, DisplayName: row.DisplayName, Revision: row.Revision, MarketState: row.StateCode,
			MinimumStations: int(row.MinimumStations), Method: row.MethodVersion, Snapshot: row.SnapshotDigest,
			OfficialRevision: row.OfficialRevision, Registry: registry, Prices: prices,
		},
		Reviews: [2]domain.Review{
			{Pin: registry, State: domain.RightsState(row.RegistryStatus), LicenseRef: row.RegistryLicense, Attribution: row.RegistryAttribution},
			{Pin: prices, State: domain.RightsState(row.PriceStatus), LicenseRef: row.PriceLicense, Attribution: row.PriceAttribution},
		},
	}
	admitted.Definition.Registry.ReviewRevision = row.RegistryReview
	admitted.Definition.Prices.ReviewRevision = row.PriceReview
	admitted.Reviews[0].Pin.ReviewRevision = row.RegistryCurrentRevision
	admitted.Reviews[1].Pin.ReviewRevision = row.PriceCurrentRevision
	if err := admitted.Definition.CheckAdmission(admitted.Reviews[:]); err != nil {
		return AdmittedDataset{}, err
	}
	return admitted, nil
}

func (c Catalog) readScopedDataset(ctx context.Context, plan billing.PlanKey, key string, revision int64) (AdmittedDataset, error) {
	allowed, err := c.queries.CheckPlanDataset(ctx, db.CheckPlanDatasetParams{
		ProductCode: plan.Product, PlanCode: plan.Plan, PlanRevision: plan.Revision, DatasetKey: key, DatasetRevision: revision,
	})
	if err != nil {
		return AdmittedDataset{}, err
	}
	if !allowed {
		return AdmittedDataset{}, ErrDatasetNotFound
	}
	return c.ReadAdmitted(ctx, key, revision)
}
