//go:build integration

package adapters_test

import (
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"testing"
)

func TestFleetSchemaOwnsIsolatedPersistenceAndFuelProjection(t *testing.T) {
	f := orgtest.New(t)
	var count int
	if err := f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM pg_class WHERE oid=ANY(ARRAY['org_asset_categories'::regclass,'org_assets'::regclass,'org_asset_tanks'::regclass,'org_asset_tank_products'::regclass,'org_asset_expected'::regclass,'org_asset_unit_allocations'::regclass,'org_asset_revisions'::regclass,'org_asset_commands'::regclass]) AND relrowsecurity AND relforcerowsecurity`).Scan(&count); err != nil || count != 8 {
		t.Fatal("actual forced-RLS fleet tables unavailable", err)
	}
	rows, err := f.Admin.Query(t.Context(), `SELECT code,unit,storage_unit,category,family,version FROM org_fuel_products_v1 ORDER BY code`)
	if err != nil {
		t.Fatal("owned fuel projection unavailable", err)
	}
	defer rows.Close()
	products := fuel.Products()
	at := 0
	for rows.Next() {
		var code, unit, storage, category, family, version string
		if err := rows.Scan(&code, &unit, &storage, &category, &family, &version); err != nil {
			t.Fatal(err)
		}
		if at >= len(products) {
			t.Fatal("extra SQL fuel descriptor")
		}
		p := products[at]
		if code != string(p.Code()) || unit != string(p.Unit()) || storage != p.StorageUnit() || category != string(p.Category()) || family != p.Family() || version != fuel.CatalogVersion {
			t.Fatal("SQL descriptor differs from owning fuel-products-v1")
		}
		at++
	}
	if rows.Err() != nil || at != len(products) {
		t.Fatal("fuel projection incomplete", rows.Err())
	}
	var liveFK bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='org_asset_commands'::regclass AND confrelid='org_assets'::regclass AND contype='f')`).Scan(&liveFK); err != nil || liveFK {
		t.Fatal("discard replay depends on live asset", err)
	}
}
