//go:build integration

package read

// RST-09 handoff integration on real disposable PostGIS: staged official
// rows project through existing ports with source-separated provenance,
// freshness, honest unknown locations and stable municipality pagination.
// Vectors are synthetic with owned [RST09-TEST] markers.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
	parent "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/registry"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
)

const handoffCSV = `CNPJ;RAZAO_SOCIAL;COD_IBGE;UF;SITUACAO;ATO_AUTORIZACAO
04218406000104;[RST09-TEST] ALFA;3550308;SP;ATIVA;PRC-1
00428184000195;[RST09-TEST] PÃO DE AÇÚCAR;3550308;SP;ATIVA;PRC-3
12ABC34501DE35;[RST09-TEST] GAMA;3550308;SP;DESCONHECIDA;
`

func stageHandoff(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	store := &registry.PGStore{Q: directory.New(pool)}
	staged, err := registry.StageCSV(ctx, store, "handoff-1", strings.NewReader(handoffCSV), registry.Limits{MaxBytes: 1 << 20, MaxRows: 100, BatchSize: 10})
	if err != nil || staged.State != "complete" {
		t.Fatalf("stage = %+v, err = %v", staged, err)
	}
	repo := parent.NewRepository(pool)
	canon := parent.RegistryCanonicalizer{Repo: repo}
	if _, err := registry.ReconcileRun(ctx, store, canon, registry.SourceCSV, "handoff-1"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	// Test setup only: project the municipality columns ResolveCNPJ
	// leaves null (a separate projection concern). Reads filter by
	// code; this pins the read contract, not the projection path.
	if _, err := pool.Exec(ctx, `UPDATE directory_stations SET municipality_code = '3550308', state = 'SP'`); err != nil {
		t.Fatalf("project municipality: %v", err)
	}
}

func searchCity(t *testing.T, reader *Reader, limit int, after string) ([]application.Station, string) {
	t.Helper()
	filter, err := application.ValidateSearch("SP", "3550308", "", limit, after)
	if err != nil {
		t.Fatal(err)
	}
	items, cursor, err := reader.Search(context.Background(), filter)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	return items, cursor
}

func TestHandoffOfficialAnchorAndFreshness(t *testing.T) {
	pool := freshPool(t)
	stageHandoff(t, pool)
	reader := NewReader(pool)

	items, _ := searchCity(t, reader, 10, "")
	if len(items) != 3 {
		t.Fatalf("stations = %d, want 3", len(items))
	}
	for _, item := range items {
		if item.Official == nil {
			t.Fatalf("%q has no official anchor", item.DisplayName)
		}
		if item.Official.Source != registry.SourceCSV {
			t.Fatalf("anchor source = %q", item.Official.Source)
		}
		if item.Official.FinishedAt == nil || time.Since(*item.Official.FinishedAt) > time.Hour {
			t.Fatalf("anchor freshness = %+v", item.Official.FinishedAt)
		}
		if item.Official.DisplayName == "" {
			t.Fatal("anchor without display name")
		}
	}
}

func TestHandoffCommunityOnlyStaysDistinct(t *testing.T) {
	pool := freshPool(t)
	stageHandoff(t, pool)
	reader := NewReader(pool)

	repo := parent.NewRepository(pool)
	if _, err := repo.ResolveCNPJ(context.Background(), "11222333000181", "[RST09-TEST] COMUNITARIA", nil); err != nil {
		t.Fatalf("community resolve: %v", err)
	}
	station, err := reader.ByCNPJ(context.Background(), "11222333000181")
	if err != nil {
		t.Fatalf("bycnpj: %v", err)
	}
	if station.Official != nil {
		t.Fatalf("community-only station borrows official anchor: %+v", station.Official)
	}
	if station.CNPJNormalized == nil || *station.CNPJNormalized != "11222333000181" {
		t.Fatalf("community CNPJ = %+v", station.CNPJNormalized)
	}
}

func TestHandoffUnknownLocationsStayHonest(t *testing.T) {
	pool := freshPool(t)
	stageHandoff(t, pool)
	reader := NewReader(pool)

	items, _ := searchCity(t, reader, 10, "")
	for _, item := range items {
		if item.Coordinates != nil {
			t.Fatalf("%q fabricates coordinates", item.DisplayName)
		}
		if item.LocationQuality != "unknown" {
			t.Fatalf("quality = %q, want unknown without a point", item.LocationQuality)
		}
	}
	nearby, err := application.ValidateNearby(-23.55, -46.633, 5000, 20, "")
	if err != nil {
		t.Fatal(err)
	}
	found, _, err := reader.Nearby(context.Background(), nearby)
	if err != nil {
		t.Fatalf("nearby: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("nearby returned %d point-less stations", len(found))
	}
}

func TestHandoffMunicipalityPaginationStable(t *testing.T) {
	pool := freshPool(t)
	stageHandoff(t, pool)
	reader := NewReader(pool)

	first, cursor := searchCity(t, reader, 2, "")
	if len(first) != 2 || cursor == "" {
		t.Fatalf("page one = %d items cursor %q", len(first), cursor)
	}
	second, _ := searchCity(t, reader, 2, cursor)
	if len(second) != 1 {
		t.Fatalf("page two = %d items, want 1", len(second))
	}
	seen := map[string]bool{}
	for _, item := range append(first, second...) {
		if seen[item.ID] {
			t.Fatalf("station %s on both pages", item.ID)
		}
		seen[item.ID] = true
	}
	// Municipality filtering is code-exact: display-name accents never
	// affect it, and unaccented name queries do not match accented names.
	rows, _, err := reader.Search(context.Background(), mustSearch(t, "SP", "3550308", "pao", 10, ""))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("unaccented name query matched %d accented rows", len(rows))
	}
}

func mustSearch(t *testing.T, state, municipality, q string, limit int, after string) application.SearchFilter {
	t.Helper()
	filter, err := application.ValidateSearch(state, municipality, q, limit, after)
	if err != nil {
		t.Fatal(err)
	}
	return filter
}
