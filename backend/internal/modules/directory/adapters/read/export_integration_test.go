//go:build integration

package read

import (
	"context"
	"testing"

	parent "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
)

func TestExportPageStatusIdentityAndCoordinates(t *testing.T) {
	pool := freshPool(t)
	ctx := context.Background()
	repo := parent.NewRepository(pool)
	a, err := repo.ResolveCNPJ(ctx, "04218406000104", "Posto A", map[string]string{
		"street": "Rua A", "number": "100", "raw": "drop this free text",
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := repo.ResolveCNPJ(ctx, "11222333000181", "Posto B", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		"UPDATE directory_stations SET current_point=ST_GeogFromText('POINT(-47.9 -15.8)'), current_quality='reviewed', current_revision_id=$2 WHERE id=$1",
		a.ID, "00000000-0000-4000-8000-0000000000a1"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE directory_stations SET status='suspended' WHERE id=$1", b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE directory_identifiers SET valid_to=now() WHERE station_id=$1", b.ID); err != nil {
		t.Fatal(err)
	}

	reader := NewExportReader(pool)
	first, next, err := reader.ExportPage(ctx, "", 1)
	if err != nil || len(first) != 1 || next == "" {
		t.Fatalf("first page = %+v, next=%q, err=%v", first, next, err)
	}
	second, next2, err := reader.ExportPage(ctx, next, 1)
	if err != nil || len(second) != 1 || next2 != "" {
		t.Fatalf("second page = %+v, next=%q, err=%v", second, next2, err)
	}
	all := append(first, second...)
	byID := map[string]int{}
	for i, station := range all {
		byID[station.ID] = i
	}
	active := all[byID[a.ID]]
	if active.CNPJ != "04218406000104" || active.Status != "active" ||
		active.Coordinates == nil || active.Coordinates.Lat != -15.8 ||
		active.Address["street"] != "Rua A" || active.Address["raw"] != "" {
		t.Errorf("active station = %+v", active)
	}
	suspended := all[byID[b.ID]]
	if suspended.CNPJ != "" || suspended.Status != "suspended" ||
		suspended.Coordinates != nil || suspended.LocationQuality != "unknown" {
		t.Errorf("suspended station = %+v", suspended)
	}
}
