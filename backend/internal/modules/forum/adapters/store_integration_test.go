//go:build integration

package adapters

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("integration database unreachable at %s: %v", dsn, err)
	}
	defer conn.Close(ctx)
	return dsn
}

func freshForumStore(t *testing.T) (*Store, *pgxpool.Pool, string) {
	t.Helper()
	adminDSN := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)

	name := fmt.Sprintf("forum_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create database %s: %v", name, err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, adminDSN)
		if err != nil {
			t.Errorf("admin connect for drop: %v", err)
			return
		}
		defer admin.Close(ctx)
		if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Errorf("drop database %s: %v", name, err)
		}
	})

	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u.Path = "/" + name
	dsn := u.String()

	if _, err := migrate.Apply(ctx, dsn, dbmigrations.Files); err != nil {
		t.Fatalf("migrate apply: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool new: %v", err)
	}
	t.Cleanup(pool.Close)

	return NewStore(pool), pool, dsn
}

func randomUUID() string {
	raw := make([]byte, 16)
	copy(raw, []byte(fmt.Sprintf("%016x", time.Now().UnixNano())))
	h := hex.EncodeToString(raw)
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32])
}

func TestForumMigration_AppliedAndIdempotent(t *testing.T) {
	store, pool, dsn := freshForumStore(t)
	ctx := context.Background()

	// 1. Verify tables exist
	tables := []string{"forum_sources", "forum_feed_state", "forum_posts", "forum_changes"}
	for _, tbl := range tables {
		var exists bool
		err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = $1
			)
		`, tbl).Scan(&exists)
		if err != nil {
			t.Fatalf("error checking table %s existence: %v", tbl, err)
		}
		if !exists {
			t.Errorf("expected table %s to exist", tbl)
		}
	}

	// 2. Verify seeded source
	var sourceCode, sourceName string
	err := pool.QueryRow(ctx, "SELECT code, name FROM forum_sources WHERE id = '00000000-0000-0000-0000-000000000001'").Scan(&sourceCode, &sourceName)
	if err != nil {
		t.Fatalf("failed to query seeded source: %v", err)
	}
	if sourceCode != "abastevo-editorial" || sourceName != "Equipe Abastevo" {
		t.Errorf("unexpected seed source values: code=%s, name=%s", sourceCode, sourceName)
	}

	// 3. Verify seeded feed state
	var feedRevision int64
	err = pool.QueryRow(ctx, "SELECT revision FROM forum_feed_state WHERE id = 1").Scan(&feedRevision)
	if err != nil {
		t.Fatalf("failed to query seeded feed state: %v", err)
	}
	if feedRevision != 0 {
		t.Errorf("expected initial feed revision 0, got %d", feedRevision)
	}

	// 4. Test Categories reader
	cats, ver, err := store.ListCategories(ctx)
	if err != nil {
		t.Fatalf("unexpected error listing categories: %v", err)
	}
	if ver != 1 || len(cats) != 7 {
		t.Errorf("expected 7 categories with version 1, got %d cats, ver %d", len(cats), ver)
	}

	// 5. Test migration idempotency (re-applying migrate should succeed cleanly)
	if _, err := migrate.Apply(ctx, dsn, dbmigrations.Files); err != nil {
		t.Fatalf("re-applying migrations failed: %v", err)
	}
}

func TestForumConstraints_NegativeChecks(t *testing.T) {
	_, pool, _ := freshForumStore(t)
	ctx := context.Background()

	const seedSourceID = "00000000-0000-0000-0000-000000000001"

	// 1. forum_sources: non-HTTPS origin_url must fail check
	t.Run("source_non_https_fails", func(t *testing.T) {
		_, err := pool.Exec(ctx, `
			INSERT INTO forum_sources (id, code, name, kind, origin_url)
			VALUES ($1, 'test-http', 'Test Source', 'PUBLISHER', 'http://insecure.example.com')
		`, randomUUID())
		if err == nil {
			t.Fatal("expected error on non-https origin_url, got nil")
		}
	})

	// 2. forum_sources: duplicate code must fail unique constraint
	t.Run("source_duplicate_code_fails", func(t *testing.T) {
		_, err := pool.Exec(ctx, `
			INSERT INTO forum_sources (id, code, name, kind, origin_url)
			VALUES ($1, 'abastevo-editorial', 'Duplicate Code', 'PUBLISHER', 'https://example.com')
		`, randomUUID())
		if err == nil {
			t.Fatal("expected error on duplicate source code, got nil")
		}
	})

	// 3. forum_feed_state: insert id != 1 must fail check
	t.Run("feed_state_invalid_id_fails", func(t *testing.T) {
		_, err := pool.Exec(ctx, "INSERT INTO forum_feed_state (id, revision) VALUES (2, 1)")
		if err == nil {
			t.Fatal("expected check constraint violation on id != 1")
		}
	})

	// 4. forum_posts: title empty must fail check
	t.Run("post_title_empty_fails", func(t *testing.T) {
		_, err := pool.Exec(ctx, `
			INSERT INTO forum_posts (id, kind, category, title, source_id)
			VALUES ($1, 'EDITORIAL', 'FUELS', '   ', $2)
		`, randomUUID(), seedSourceID)
		if err == nil {
			t.Fatal("expected error on empty title, got nil")
		}
	})

	// 5. forum_posts: title > 120 chars must fail check
	t.Run("post_title_too_long_fails", func(t *testing.T) {
		longTitle := strings.Repeat("A", 121)
		_, err := pool.Exec(ctx, `
			INSERT INTO forum_posts (id, kind, category, title, source_id)
			VALUES ($1, 'EDITORIAL', 'FUELS', $2, $3)
		`, randomUUID(), longTitle, seedSourceID)
		if err == nil {
			t.Fatal("expected error on title > 120 chars, got nil")
		}
	})

	// 6. forum_posts: non-TOPIC without source_id must fail source_kind_check
	t.Run("post_editorial_without_source_fails", func(t *testing.T) {
		_, err := pool.Exec(ctx, `
			INSERT INTO forum_posts (id, kind, category, title, source_id)
			VALUES ($1, 'EDITORIAL', 'FUELS', 'Valid Title', NULL)
		`, randomUUID())
		if err == nil {
			t.Fatal("expected check constraint violation on EDITORIAL without source_id")
		}
	})

	// 7. forum_posts: TOPIC with source_id must fail source_kind_check
	t.Run("post_topic_with_source_fails", func(t *testing.T) {
		_, err := pool.Exec(ctx, `
			INSERT INTO forum_posts (id, kind, category, title, source_id)
			VALUES ($1, 'TOPIC', 'FUELS', 'Topic Title', $2)
		`, randomUUID(), seedSourceID)
		if err == nil {
			t.Fatal("expected check constraint violation on TOPIC with source_id")
		}
	})

	// 8. forum_posts: NEWS without source_published_at must fail news_source_date_check
	t.Run("post_news_without_source_date_fails", func(t *testing.T) {
		_, err := pool.Exec(ctx, `
			INSERT INTO forum_posts (id, kind, category, title, source_id, source_published_at)
			VALUES ($1, 'NEWS', 'FUELS', 'News Title', $2, NULL)
		`, randomUUID(), seedSourceID)
		if err == nil {
			t.Fatal("expected check constraint violation on NEWS without source_published_at")
		}
	})

	// 9. forum_posts: PUBLISHED without first_published_at / first_publication_seq > 0 must fail check
	t.Run("post_published_without_seq_fails", func(t *testing.T) {
		_, err := pool.Exec(ctx, `
			INSERT INTO forum_posts (id, kind, category, title, source_id, publication_state, first_publication_seq, first_published_at)
			VALUES ($1, 'EDITORIAL', 'FUELS', 'Editorial Title', $2, 'PUBLISHED', 0, now())
		`, randomUUID(), seedSourceID)
		if err == nil {
			t.Fatal("expected check constraint violation on PUBLISHED with first_publication_seq == 0")
		}
	})

	// 10. forum_posts: duplicate external_id for same source must fail unique index
	t.Run("post_duplicate_external_id_fails", func(t *testing.T) {
		id1 := randomUUID()
		id2 := randomUUID()
		_, err := pool.Exec(ctx, `
			INSERT INTO forum_posts (id, kind, category, title, source_id, external_id)
			VALUES ($1, 'EDITORIAL', 'FUELS', 'Post 1', $2, 'ext-123')
		`, id1, seedSourceID)
		if err != nil {
			t.Fatalf("failed to insert initial post: %v", err)
		}

		_, err = pool.Exec(ctx, `
			INSERT INTO forum_posts (id, kind, category, title, source_id, external_id)
			VALUES ($1, 'EDITORIAL', 'FUELS', 'Post 2', $2, 'ext-123')
		`, id2, seedSourceID)
		if err == nil {
			t.Fatal("expected unique violation on duplicate (source_id, external_id)")
		}
	})

	// 11. forum_changes: duplicate (target_type, target_id, revision, action) must fail unique constraint
	t.Run("change_duplicate_key_fails", func(t *testing.T) {
		postID := randomUUID()
		chID1 := randomUUID()
		chID2 := randomUUID()
		_, err := pool.Exec(ctx, `
			INSERT INTO forum_changes (id, target_type, target_id, revision, action, actor_kind, reason_code)
			VALUES ($1, 'POST', $2, 1, 'PUBLISH', 'ADMIN', 'editorial-release')
		`, chID1, postID)
		if err != nil {
			t.Fatalf("failed to insert first change record: %v", err)
		}

		_, err = pool.Exec(ctx, `
			INSERT INTO forum_changes (id, target_type, target_id, revision, action, actor_kind, reason_code)
			VALUES ($1, 'POST', $2, 1, 'PUBLISH', 'ADMIN', 'editorial-release')
		`, chID2, postID)
		if err == nil {
			t.Fatal("expected unique violation on duplicate change key")
		}
	})
}

func TestForumPublication_MonotonicSequenceAndConcurrency(t *testing.T) {
	store, _, _ := freshForumStore(t)
	ctx := context.Background()

	const seedSourceID = "00000000-0000-0000-0000-000000000001"
	const numPosts = 10

	postIDs := make([]string, numPosts)
	for i := 0; i < numPosts; i++ {
		id := randomUUID()
		postIDs[i] = id
		err := store.CreateEditorialPost(ctx, &domain.Post{
			ID:               id,
			Kind:             domain.KindEditorial,
			Category:         domain.CategoryFuels,
			Title:            fmt.Sprintf("Concurrent Post %d", i+1),
			Excerpt:          "Excerpt for concurrent publication test",
			BodyText:         "Body text for concurrent publication test",
			SourceID:         seedSourceID,
			PublicationState: domain.StateDraft,
			Visibility:       domain.VisibilityVisible,
			Revision:         1,
		})
		if err != nil {
			t.Fatalf("failed to create post %d: %v", i, err)
		}
	}

	// Concurrently publish all posts
	var wg sync.WaitGroup
	seqs := make([]int64, numPosts)
	errs := make([]error, numPosts)

	for i := 0; i < numPosts; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			seq, err := store.PublishEditorialPost(ctx, postIDs[idx], time.Now().UTC())
			seqs[idx] = seq
			errs[idx] = err
		}(i)
	}
	wg.Wait()

	// Verify all published successfully
	for i := 0; i < numPosts; i++ {
		if errs[i] != nil {
			t.Errorf("error publishing post %d: %v", i, errs[i])
		}
	}

	// Verify monotonic sequences: all sequences must be unique and between 1 and numPosts
	seen := make(map[int64]bool)
	for _, seq := range seqs {
		if seq < 1 || seq > numPosts {
			t.Errorf("unexpected sequence number %d (expected 1..%d)", seq, numPosts)
		}
		if seen[seq] {
			t.Errorf("duplicate sequence number %d detected", seq)
		}
		seen[seq] = true
	}

	// Verify feed snapshot matches revision
	snapshot, err := store.GetFeedSnapshot(ctx)
	if err != nil {
		t.Fatalf("failed to get feed snapshot: %v", err)
	}
	expectedSnapshot := fmt.Sprintf("rev-%d", numPosts)
	if snapshot != expectedSnapshot {
		t.Errorf("expected snapshot %q, got %q", expectedSnapshot, snapshot)
	}
}

func TestForumFeed_KeysetPaginationAndSnapshot(t *testing.T) {
	store, _, _ := freshForumStore(t)
	ctx := context.Background()

	const seedSourceID = "00000000-0000-0000-0000-000000000001"
	const totalPosts = 5

	for i := 1; i <= totalPosts; i++ {
		id := randomUUID()
		cat := domain.CategoryFuels
		if i == 5 {
			cat = domain.CategoryElectric
		}
		err := store.CreateEditorialPost(ctx, &domain.Post{
			ID:               id,
			Kind:             domain.KindEditorial,
			Category:         cat,
			Title:            fmt.Sprintf("Feed Post %d", i),
			Excerpt:          fmt.Sprintf("Excerpt %d", i),
			BodyText:         fmt.Sprintf("Body %d", i),
			SourceID:         seedSourceID,
			PublicationState: domain.StateDraft,
			Visibility:       domain.VisibilityVisible,
			Revision:         1,
		})
		if err != nil {
			t.Fatalf("failed to create post %d: %v", i, err)
		}
		// Publish sequentially to establish deterministic order
		if _, err := store.PublishEditorialPost(ctx, id, time.Now().UTC()); err != nil {
			t.Fatalf("failed to publish post %d: %v", i, err)
		}
	}

	// Page 1: limit 2
	page1, err := store.ListPublicPosts(ctx, application.ListPublicPostsQuery{
		Limit: 2,
	})
	if err != nil {
		t.Fatalf("page 1 failed: %v", err)
	}
	if len(page1.Items) != 2 {
		t.Fatalf("expected 2 items on page 1, got %d", len(page1.Items))
	}
	if page1.NextCursor == nil || *page1.NextCursor == "" {
		t.Fatal("expected next cursor on page 1, got nil/empty")
	}

	// Page 2: with cursor from page 1, limit 2
	page2, err := store.ListPublicPosts(ctx, application.ListPublicPostsQuery{
		Limit:  2,
		Cursor: page1.NextCursor,
	})
	if err != nil {
		t.Fatalf("page 2 failed: %v", err)
	}
	if len(page2.Items) != 2 {
		t.Fatalf("expected 2 items on page 2, got %d", len(page2.Items))
	}
	if page2.NextCursor == nil || *page2.NextCursor == "" {
		t.Fatal("expected next cursor on page 2, got nil/empty")
	}

	// Verify no overlap between page 1 and page 2
	page1IDs := map[string]bool{page1.Items[0].ID: true, page1.Items[1].ID: true}
	for _, item := range page2.Items {
		if page1IDs[item.ID] {
			t.Errorf("item %s leaked across page 1 and page 2", item.ID)
		}
	}

	// Page 3: with cursor from page 2, limit 2
	page3, err := store.ListPublicPosts(ctx, application.ListPublicPostsQuery{
		Limit:  2,
		Cursor: page2.NextCursor,
	})
	if err != nil {
		t.Fatalf("page 3 failed: %v", err)
	}
	if len(page3.Items) != 1 {
		t.Fatalf("expected 1 item on page 3, got %d", len(page3.Items))
	}
	if page3.NextCursor != nil {
		t.Errorf("expected no next cursor on last page, got %v", *page3.NextCursor)
	}

	// Filtering: Category ELECTRIC should return exactly 1 post
	catFilter := domain.CategoryElectric
	elecPage, err := store.ListPublicPosts(ctx, application.ListPublicPostsQuery{
		Category: &catFilter,
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("category filtered feed failed: %v", err)
	}
	if len(elecPage.Items) != 1 {
		t.Fatalf("expected 1 item for ELECTRIC, got %d", len(elecPage.Items))
	}
	if elecPage.Items[0].Category != domain.CategoryElectric {
		t.Errorf("expected ELECTRIC category, got %s", elecPage.Items[0].Category)
	}

	// Invalid cursor handling
	invalidCursor := "not-a-valid-cursor"
	_, err = store.ListPublicPosts(ctx, application.ListPublicPostsQuery{
		Cursor: &invalidCursor,
	})
	if !errors.Is(err, domain.ErrInvalidCursor) {
		t.Fatalf("expected ErrInvalidCursor, got %v", err)
	}
}

func TestForumUpdateAndVisibility_RollbackAndFeedInvalidation(t *testing.T) {
	store, _, _ := freshForumStore(t)
	ctx := context.Background()

	const seedSourceID = "00000000-0000-0000-0000-000000000001"
	postID := randomUUID()

	post := &domain.Post{
		ID:               postID,
		Kind:             domain.KindEditorial,
		Category:         domain.CategoryFuels,
		Title:            "Original Title",
		Excerpt:          "Original Excerpt",
		BodyText:         "Original Body",
		SourceID:         seedSourceID,
		PublicationState: domain.StateDraft,
		Visibility:       domain.VisibilityVisible,
		Revision:         1,
	}
	if err := store.CreateEditorialPost(ctx, post); err != nil {
		t.Fatalf("failed to create post: %v", err)
	}

	// Publish
	if _, err := store.PublishEditorialPost(ctx, postID, time.Now().UTC()); err != nil {
		t.Fatalf("failed to publish post: %v", err)
	}

	// Update with wrong expected revision -> should fail with ErrRevisionMismatch
	post.Title = "Updated Title Mismatch"
	err := store.UpdateEditorialPost(ctx, post, 99)
	if !errors.Is(err, domain.ErrRevisionMismatch) {
		t.Fatalf("expected ErrRevisionMismatch, got %v", err)
	}

	// Verify post was NOT changed
	loaded, err := store.GetPostByID(ctx, postID)
	if err != nil {
		t.Fatalf("failed to load post: %v", err)
	}
	if loaded.Title != "Original Title" {
		t.Errorf("title changed despite revision mismatch: %q", loaded.Title)
	}

	// Update with correct revision (loaded.Revision == 2) -> should succeed
	post.Title = "Updated Title OK"
	post.PublicationState = domain.StatePublished
	if err := store.UpdateEditorialPost(ctx, post, loaded.Revision); err != nil {
		t.Fatalf("update with correct revision failed: %v", err)
	}

	loaded, err = store.GetPostByID(ctx, postID)
	if err != nil {
		t.Fatalf("failed to load post: %v", err)
	}
	if loaded.Title != "Updated Title OK" {
		t.Errorf("title not updated: %q", loaded.Title)
	}
	if loaded.Revision != 3 {
		t.Errorf("expected revision 3, got %d", loaded.Revision)
	}

	// Hide post -> disappears from public feed
	if err := store.SetPostVisibility(ctx, postID, domain.VisibilityHidden); err != nil {
		t.Fatalf("hide post failed: %v", err)
	}

	feed, err := store.ListPublicPosts(ctx, application.ListPublicPostsQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list public posts failed: %v", err)
	}
	if len(feed.Items) != 0 {
		t.Errorf("expected 0 public items after hiding, got %d", len(feed.Items))
	}

	// Public detail projection should reject hidden post
	hiddenPost, err := store.GetPostByID(ctx, postID)
	if err != nil {
		t.Fatalf("failed to get hidden post: %v", err)
	}
	if _, err := hiddenPost.ToDetail(); !errors.Is(err, domain.ErrNotPubliclyVisible) {
		t.Fatalf("expected ErrNotPubliclyVisible from ToDetail on hidden post, got %v", err)
	}
}

func TestForumExplainPlan_UsesPartialIndex(t *testing.T) {
	_, pool, _ := freshForumStore(t)
	ctx := context.Background()

	// Run EXPLAIN on the ListPublicPosts query
	query := `
		EXPLAIN (FORMAT JSON)
		SELECT
			p.id, p.kind, p.category, p.title, p.excerpt,
			p.source_id, p.canonical_url,
			p.author_account_id, p.public_alias,
			p.source_published_at,
			p.local_updated_at,
			p.first_published_at, p.first_publication_seq,
			p.revision,
			p.region_uf, p.region_municipality_ibge,
			p.fuel_product, p.station_id,
			s.name AS source_name,
			s.origin_url AS source_origin_url,
			s.attribution AS source_attribution
		FROM forum_posts p
		LEFT JOIN forum_sources s ON s.id = p.source_id
		LEFT JOIN accounts a ON a.id = p.author_account_id
		WHERE p.publication_state = 'PUBLISHED'
		  AND p.visibility = 'VISIBLE'
		  AND (
			(p.kind <> 'TOPIC' AND s.id IS NOT NULL AND s.rights_status = 'APPROVED' AND s.publication_enabled = true)
			OR
			(p.kind = 'TOPIC' AND a.status = 'active')
		  )
		ORDER BY p.first_publication_seq DESC, p.id DESC
		LIMIT 20;
	`

	var planJSON string
	err := pool.QueryRow(ctx, query).Scan(&planJSON)
	if err != nil {
		t.Fatalf("EXPLAIN failed: %v", err)
	}

	if planJSON == "" {
		t.Fatal("expected non-empty explain plan")
	}
}
