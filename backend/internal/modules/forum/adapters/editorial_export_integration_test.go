//go:build integration

package adapters

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

// TestEditorialExportProjectsOnlyPublishedVisibleAbastevoEditorials proves
// the B-BR-FOR-19 predicate on real PostGIS: drafts, hidden, rejected and
// removed editorials, news, topics, other-source editorials and disabled
// sources stay out; the fingerprint follows publish, edit and hide; pages are
// keyset-bounded and run inside a read-only repeatable-read transaction.
func TestEditorialExportProjectsOnlyPublishedVisibleAbastevoEditorials(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	ctx := context.Background()
	const abastevoSource = "00000000-0000-0000-0000-000000000001"
	const publisherSource = "00000000-0000-4000-8000-0000000000e1"
	station := "33333333-3333-4333-8333-333333333333"
	if _, err := pool.Exec(ctx, `INSERT INTO directory_stations (id, display_name, municipality_code, state)
		VALUES ($1, 'Posto Export', '5103403', 'MT')`, station); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO forum_sources (id, code, name, kind, origin_url, publication_enabled, rights_status)
		VALUES ($1, 'publisher-x', 'Publisher X', 'PUBLISHER', 'https://publisher.example', true, 'APPROVED')`, publisherSource); err != nil {
		t.Fatal(err)
	}
	reader := NewEditorialExportReader(pool)
	empty, err := reader.EditorialFingerprint(ctx)
	if err != nil || empty.Posts != 0 || empty.Hash != "md5:" {
		t.Fatalf("empty fingerprint = %+v, %v", empty, err)
	}

	cuiaba := 5103403
	gasoline := "GASOLINE_REGULAR"
	create := func(title, source string, state domain.PublicationState) *domain.Post {
		t.Helper()
		post := &domain.Post{
			ID: randomUUID(), Kind: domain.KindEditorial, Category: domain.CategoryFuels,
			Title: title, Excerpt: "Resumo " + title, BodyText: "Corpo de " + title + ".",
			SourceID: source, PublicationState: domain.StateDraft, Visibility: domain.VisibilityVisible,
		}
		if err := store.CreateEditorialPost(ctx, post); err != nil {
			t.Fatalf("create %q: %v", title, err)
		}
		if state == domain.StatePublished {
			if _, err := store.PublishEditorialPost(ctx, post.ID, time.Date(2026, 11, 10, 15, 0, 0, 0, time.UTC)); err != nil {
				t.Fatalf("publish %q: %v", title, err)
			}
			post.PublicationState = domain.StatePublished
		}
		return post
	}
	regional := &domain.Post{
		ID: randomUUID(), Kind: domain.KindEditorial, Category: domain.CategoryDriverEconomy,
		Title: "Gasolina em Cuiabá", Excerpt: "Resumo regional", BodyText: "Linha um\n\n- item",
		SourceID: abastevoSource, PublicationState: domain.StateDraft, Visibility: domain.VisibilityVisible,
		Region: &domain.Region{UF: "MT", MunicipalityIBGE: &cuiaba}, FuelProduct: &gasoline, StationID: &station,
	}
	if err := store.CreateEditorialPost(ctx, regional); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PublishEditorialPost(ctx, regional.ID, time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	regional.PublicationState = domain.StatePublished
	published := create("Publicada", abastevoSource, domain.StatePublished)
	create("Rascunho", abastevoSource, domain.StateDraft)
	hidden := create("Oculta", abastevoSource, domain.StatePublished)
	if err := store.SetPostVisibility(ctx, hidden.ID, domain.VisibilityHidden); err != nil {
		t.Fatal(err)
	}
	rejected := create("Rejeitada", abastevoSource, domain.StateDraft)
	removed := create("Removida", abastevoSource, domain.StatePublished)
	if _, err := pool.Exec(ctx, `UPDATE forum_posts SET publication_state = 'REJECTED' WHERE id = $1`, rejected.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE forum_posts SET publication_state = 'REMOVED' WHERE id = $1`, removed.ID); err != nil {
		t.Fatal(err)
	}
	create("Editorial de terceiro", publisherSource, domain.StatePublished)
	if _, err := pool.Exec(ctx, `INSERT INTO forum_posts (id, kind, category, title, body_text, source_id, source_published_at,
		publication_state, first_published_at, first_publication_seq)
		VALUES ($1, 'NEWS', 'FUELS', 'Notícia', 'Texto', $2, now(), 'PUBLISHED', now(), 900)`, randomUUID(), abastevoSource); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO forum_posts (id, kind, category, title, body_text, source_id, bulletin_key,
		bulletin_group_key, bulletin_provenance, publication_state, first_published_at, first_publication_seq)
		VALUES ($1, 'BULLETIN', 'ABASTEVO_DATA', 'Boletim', 'Texto', $2, 'k', 'g', '{}'::jsonb, 'PUBLISHED', now(), 901)`, randomUUID(), abastevoSource); err != nil {
		t.Fatal(err)
	}
	author := "44444444-4444-4444-8444-444444444444"
	if _, err := pool.Exec(ctx, `INSERT INTO accounts (id, alias, status) VALUES ($1, 'autor', 'active')`, author); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO forum_posts (id, kind, category, title, body_text, author_account_id,
		publication_state, first_published_at, first_publication_seq)
		VALUES ($1, 'TOPIC', 'FUELS', 'Tópico', 'Texto', $2, 'PUBLISHED', now(), 902)`, randomUUID(), author); err != nil {
		t.Fatal(err)
	}

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	inTx := NewEditorialExportReader(tx)
	before, err := inTx.EditorialFingerprint(ctx)
	if err != nil || before.Posts != 2 || before.Hash == empty.Hash {
		t.Fatalf("fingerprint = %+v, %v", before, err)
	}
	var all []application.EditorialExportPost
	after := ""
	for pages := 0; ; pages++ {
		page, next, err := inTx.ExportEditorialPage(ctx, after, 1)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, page...)
		if next == "" {
			break
		}
		if next <= after || pages > 3 {
			t.Fatalf("cursor did not advance: %q after %q", next, after)
		}
		after = next
	}
	if len(all) != 2 || all[0].ID >= all[1].ID {
		t.Fatalf("exported posts = %+v", all)
	}
	byID := map[string]application.EditorialExportPost{}
	for _, post := range all {
		byID[post.ID] = post
	}
	got, ok := byID[regional.ID]
	if !ok || got.Category != domain.CategoryDriverEconomy || got.CategoryName != "Economia do Motorista" ||
		got.Title != regional.Title || got.Excerpt != "Resumo regional" || got.BodyText != "Linha um\n\n- item" ||
		got.SourceName != "Equipe Abastevo" || got.Region == nil || got.Region.UF != "MT" ||
		got.Region.MunicipalityIBGE == nil || *got.Region.MunicipalityIBGE != cuiaba ||
		got.FuelProduct == nil || *got.FuelProduct != gasoline || got.StationID == nil || *got.StationID != station ||
		!got.PublishedAt.Equal(time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)) || got.UpdatedAt.IsZero() {
		t.Fatalf("regional projection = %+v", got)
	}
	if plain := byID[published.ID]; plain.Region != nil || plain.FuelProduct != nil || plain.StationID != nil || plain.CategoryName != "Combustíveis" {
		t.Fatalf("plain projection = %+v", plain)
	}
	if _, _, err := inTx.ExportEditorialPage(ctx, "not-a-uuid", 10); !errors.Is(err, application.ErrInvalidEditorialExportPage) {
		t.Fatalf("bad cursor = %v", err)
	}
	if _, _, err := inTx.ExportEditorialPage(ctx, "", application.MaxEditorialExportLimit+1); !errors.Is(err, application.ErrInvalidEditorialExportPage) {
		t.Fatalf("oversized page = %v", err)
	}

	// The read-only snapshot keeps its view while the post changes outside.
	edited := *published
	edited.Title = "Publicada e editada"
	var revision int
	if err := pool.QueryRow(ctx, `SELECT revision FROM forum_posts WHERE id = $1`, published.ID).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateEditorialPost(ctx, &edited, revision); err != nil {
		t.Fatal(err)
	}
	if again, err := inTx.EditorialFingerprint(ctx); err != nil || again != before {
		t.Fatalf("snapshot fingerprint moved: %+v, %v", again, err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	afterEdit, err := reader.EditorialFingerprint(ctx)
	if err != nil || afterEdit.Posts != 2 || afterEdit.Hash == before.Hash {
		t.Fatalf("edit did not move the fingerprint: %+v, %v", afterEdit, err)
	}
	if err := store.SetPostVisibility(ctx, regional.ID, domain.VisibilityHidden); err != nil {
		t.Fatal(err)
	}
	afterHide, err := reader.EditorialFingerprint(ctx)
	if err != nil || afterHide.Posts != 1 || afterHide.Hash == afterEdit.Hash {
		t.Fatalf("hide did not move the fingerprint: %+v, %v", afterHide, err)
	}
	page, _, err := reader.ExportEditorialPage(ctx, "", 10)
	if err != nil || len(page) != 1 || page[0].ID != published.ID || page[0].Title != "Publicada e editada" {
		t.Fatalf("page after hide = %+v, %v", page, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE forum_sources SET publication_enabled = false WHERE id = $1`, abastevoSource); err != nil {
		t.Fatal(err)
	}
	if disabled, err := reader.EditorialFingerprint(ctx); err != nil || disabled.Posts != 0 {
		t.Fatalf("disabled source still exported: %+v, %v", disabled, err)
	}
}
