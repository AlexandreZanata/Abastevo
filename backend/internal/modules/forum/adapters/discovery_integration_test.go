//go:build integration

package adapters

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

func TestForumDiscovery_FiltersAndPagination(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	ctx := context.Background()

	const sourceID = "00000000-0000-0000-0000-000000000001"
	stationA := "11111111-1111-4111-8111-111111111111"
	stationB := "22222222-2222-4222-8222-222222222222"
	if _, err := pool.Exec(ctx, `
		INSERT INTO directory_stations (id, display_name, municipality_code, state)
		VALUES ($1, 'Posto A', '3550308', 'SP'), ($2, 'Posto B', '3550308', 'SP')
	`, stationA, stationB); err != nil {
		t.Fatalf("seed directory stations: %v", err)
	}

	spIBGE := 3550308
	rjIBGE := 3304557
	mgIBGE := 3106200
	ethanol := "ETHANOL"
	gasoline := "GASOLINE_REGULAR"

	posts := []*domain.Post{
		{
			ID: randomUUID(), Kind: domain.KindEditorial, Category: domain.CategoryFuels,
			Title: "Etanol em São Paulo", Excerpt: "Preço do etanol na capital.",
			BodyText: "O etanol hidratado recuou nas bombas paulistas.",
			SourceID: sourceID, PublicationState: domain.StateDraft, Visibility: domain.VisibilityVisible,
			Region:      &domain.Region{UF: "SP", MunicipalityIBGE: &spIBGE},
			FuelProduct: &ethanol, StationID: &stationA,
		},
		{
			ID: randomUUID(), Kind: domain.KindEditorial, Category: domain.CategoryFuels,
			Title: "Gasolina em São Paulo", Excerpt: "Preço da gasolina comum.",
			BodyText: "A gasolina comum segue estável na região metropolitana.",
			SourceID: sourceID, PublicationState: domain.StateDraft, Visibility: domain.VisibilityVisible,
			Region:      &domain.Region{UF: "SP", MunicipalityIBGE: &spIBGE},
			FuelProduct: &gasoline, StationID: &stationB,
		},
		{
			ID: randomUUID(), Kind: domain.KindEditorial, Category: domain.CategoryDriverEconomy,
			Title: "Etanol no interior paulista", Excerpt: "Safra e preço do etanol.",
			BodyText: "Usinas do interior ampliam a oferta de etanol.",
			SourceID: sourceID, PublicationState: domain.StateDraft, Visibility: domain.VisibilityVisible,
			Region: &domain.Region{UF: "SP"}, FuelProduct: &ethanol,
		},
		{
			ID: randomUUID(), Kind: domain.KindEditorial, Category: domain.CategoryRoadsMobility,
			Title: "Gasolina no Rio de Janeiro", Excerpt: "Preço na capital fluminense.",
			BodyText: "A gasolina comum teve leve alta no Rio.",
			SourceID: sourceID, PublicationState: domain.StateDraft, Visibility: domain.VisibilityVisible,
			Region: &domain.Region{UF: "RJ", MunicipalityIBGE: &rjIBGE}, FuelProduct: &gasoline,
		},
		{
			ID: randomUUID(), Kind: domain.KindEditorial, Category: domain.CategoryFuels,
			Title: "Etanol em Minas Gerais", Excerpt: "Preço do etanol no Triângulo.",
			BodyText: "O etanol mineiro acompanha a safra do Sudeste.",
			SourceID: sourceID, PublicationState: domain.StateDraft, Visibility: domain.VisibilityVisible,
			Region: &domain.Region{UF: "MG", MunicipalityIBGE: &mgIBGE}, FuelProduct: &ethanol,
		},
	}
	for _, post := range posts {
		if err := store.CreateEditorialPost(ctx, post); err != nil {
			t.Fatalf("create post %q: %v", post.Title, err)
		}
		if _, err := store.PublishEditorialPost(ctx, post.ID, time.Now().UTC()); err != nil {
			t.Fatalf("publish post %q: %v", post.Title, err)
		}
	}

	feed := func(query application.ListPublicPostsQuery) domain.FeedPage {
		t.Helper()
		page, err := store.ListPublicPosts(ctx, query)
		if err != nil {
			t.Fatalf("list public posts: %v", err)
		}
		return page
	}

	sp := "SP"
	if page := feed(application.ListPublicPostsQuery{UF: &sp, Limit: 20}); len(page.Items) != 3 {
		t.Errorf("uf=SP: expected 3, got %d", len(page.Items))
	}

	spMunicipality := 3550308
	if page := feed(application.ListPublicPostsQuery{UF: &sp, MunicipalityIBGE: &spMunicipality, Limit: 20}); len(page.Items) != 2 {
		t.Errorf("uf=SP municipality=3550308: expected 2, got %d", len(page.Items))
	}

	if page := feed(application.ListPublicPostsQuery{MunicipalityIBGE: &mgIBGE, Limit: 20}); len(page.Items) != 1 {
		t.Errorf("municipality=3106200: expected 1, got %d", len(page.Items))
	}

	if page := feed(application.ListPublicPostsQuery{FuelProduct: &ethanol, Limit: 20}); len(page.Items) != 3 {
		t.Errorf("fuel=ETHANOL: expected 3, got %d", len(page.Items))
	}

	if page := feed(application.ListPublicPostsQuery{FuelProduct: &gasoline, UF: &sp, Limit: 20}); len(page.Items) != 1 {
		t.Errorf("fuel=GASOLINE_REGULAR uf=SP: expected 1, got %d", len(page.Items))
	}

	if page := feed(application.ListPublicPostsQuery{StationID: &stationA, Limit: 20}); len(page.Items) != 1 {
		t.Errorf("station=A: expected 1, got %d", len(page.Items))
	}

	page1 := feed(application.ListPublicPostsQuery{UF: &sp, Limit: 2})
	if len(page1.Items) != 2 || page1.NextCursor == nil {
		t.Fatalf("expected 2 items and cursor, got %d items, cursor %v", len(page1.Items), page1.NextCursor)
	}
	page2 := feed(application.ListPublicPostsQuery{UF: &sp, Limit: 2, Cursor: page1.NextCursor})
	if len(page2.Items) != 1 || page2.NextCursor != nil {
		t.Fatalf("expected final 1 item without cursor, got %d items, cursor %v", len(page2.Items), page2.NextCursor)
	}
	seen := map[string]bool{}
	for _, item := range append(page1.Items, page2.Items...) {
		if seen[item.ID] {
			t.Errorf("duplicate item %s across filtered pages", item.ID)
		}
		seen[item.ID] = true
		if item.Region == nil || item.Region.UF != "SP" {
			t.Errorf("filtered page leaked non-SP item: %+v", item.Region)
		}
	}
}

func TestForumDiscovery_ExplainPlans10kAnd100k(t *testing.T) {
	for _, volume := range []int{10000, 100000} {
		t.Run(fmt.Sprintf("posts_%d", volume), func(t *testing.T) {
			_, pool, _ := freshForumStore(t)
			ctx := context.Background()
			const sourceID = "00000000-0000-0000-0000-000000000001"
			const stationID = "11111111-1111-4111-8111-111111111111"

			if _, err := pool.Exec(ctx, `
				INSERT INTO directory_stations (id, display_name, municipality_code, state)
				VALUES ($1, 'Posto sintético', '3550308', 'SP')
			`, stationID); err != nil {
				t.Fatalf("seed station: %v", err)
			}

			if _, err := pool.Exec(ctx, `
				INSERT INTO forum_posts (
					id, kind, category, title, excerpt, body_text, source_id,
					local_created_at, local_updated_at, first_published_at,
					first_publication_seq, publication_state, visibility, revision,
					region_uf, region_municipality_ibge, fuel_product, station_id
				)
				SELECT
					md5(g::text)::uuid,
					'EDITORIAL',
					'FUELS',
					'Preço da gasolina',
					'Boletim sintético',
					'Preço da gasolina comum no estado.',
					$1,
					now(), now(), now(), g, 'PUBLISHED', 'VISIBLE', 1,
					CASE WHEN g % 10000 = 0 THEN 'SP' ELSE 'RJ' END,
					CASE WHEN g % 10000 = 0 THEN 3550308 ELSE 3304557 END,
					CASE WHEN g % 10000 = 0 THEN 'ETHANOL' ELSE 'GASOLINE_REGULAR' END,
					CASE WHEN g % 10000 = 0 THEN $2::uuid ELSE NULL END
				FROM generate_series(1, $3) AS g
			`, sourceID, stationID, volume); err != nil {
				t.Fatalf("seed %d posts: %v", volume, err)
			}
			if _, err := pool.Exec(ctx, `ANALYZE forum_posts`); err != nil {
				t.Fatalf("analyze: %v", err)
			}

			explain := func(sql string, args ...any) string {
				t.Helper()
				rows, err := pool.Query(ctx, "EXPLAIN (FORMAT TEXT) "+sql, args...)
				if err != nil {
					t.Fatalf("explain: %v", err)
				}
				defer rows.Close()
				var lines []string
				for rows.Next() {
					var line string
					if err := rows.Scan(&line); err != nil {
						t.Fatalf("scan explain: %v", err)
					}
					lines = append(lines, line)
				}
				return strings.Join(lines, "\n")
			}

			base := `
				SELECT p.id, p.first_publication_seq
				FROM forum_posts p
				WHERE p.publication_state = 'PUBLISHED'
				  AND p.visibility = 'VISIBLE'
				  AND %s
				ORDER BY p.first_publication_seq DESC, p.id DESC
				LIMIT 20`
			cases := []struct {
				name  string
				where string
				args  []any
				index string
			}{
				{name: "uf", where: "p.region_uf = $1", args: []any{"SP"}, index: "forum_posts_feed_uf_idx"},
				{name: "municipality", where: "p.region_municipality_ibge = $1", args: []any{3550308}, index: "forum_posts_feed_municipality_idx"},
				{name: "fuel", where: "p.fuel_product = $1", args: []any{"ETHANOL"}, index: "forum_posts_feed_fuel_idx"},
				{name: "station", where: "p.station_id = $1", args: []any{stationID}, index: "forum_posts_feed_station_idx"},
			}
			for _, c := range cases {
				plan := explain(fmt.Sprintf(base, c.where), c.args...)
				if !strings.Contains(plan, c.index) {
					t.Errorf("%s plan must use %s:\n%s", c.name, c.index, plan)
				}
			}
		})
	}
}
