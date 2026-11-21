package adapters

import (
	"context"
	"errors"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgbilling"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// CatalogStore reads platform-owned plan metadata. Effective access is
// evaluated from the independently scoped subscription and membership.
type CatalogStore struct {
	DB           db.DBTX
	Capabilities capability.Catalog
}

func (store CatalogStore) GetPlan(ctx context.Context, key domain.PlanKey) (domain.Plan, error) {
	if err := key.Validate(); err != nil {
		return domain.Plan{}, err
	}
	row, err := db.New(store.DB).GetPlanVersion(ctx, db.GetPlanVersionParams{
		ProductCode: key.Product, PlanCode: key.Plan, Revision: key.Revision,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Plan{}, domain.ErrPlanNotFound
	}
	if err != nil {
		return domain.Plan{}, err
	}

	limits := make(map[domain.Meter]int64)
	for meter, value := range map[domain.Meter]pgtype.Int8{
		domain.ActiveVehicles: row.ActiveVehicles,
		domain.ActiveStations: row.ActiveStations,
		domain.StorageBytes:   row.StorageBytes,
		domain.APICalls:       row.ApiCalls,
	} {
		if value.Valid {
			limits[meter] = value.Int64
		}
	}
	storedKey := domain.PlanKey{Product: row.ProductCode, Plan: row.PlanCode, Revision: row.Revision}
	return domain.NewPlan(storedKey, row.DisplayName, row.Capabilities, limits, store.Capabilities)
}
