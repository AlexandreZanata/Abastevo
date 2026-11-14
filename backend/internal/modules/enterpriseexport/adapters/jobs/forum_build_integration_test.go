//go:build integration

package jobs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	parentdirectory "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	forumadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/adapters"
	forumapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	forumdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

const forumEditorialSource = "00000000-0000-0000-0000-000000000001"

// forumEnv seeds one published price week and returns a builder and a
// reconcile job wired to the real forum export port, as the worker does.
type forumEnv struct {
	pool      *pgxpool.Pool
	store     *forumadapters.Store
	root      string
	build     Build
	reconcile Reconcile
	enqueued  []string
}

func newForumEnv(t *testing.T) *forumEnv {
	t.Helper()
	pool := freshJobsPool(t)
	ctx := context.Background()
	station, err := parentdirectory.NewRepository(pool).ResolveCNPJ(ctx, "04218406000104", "Posto Fórum", nil)
	if err != nil {
		t.Fatal(err)
	}
	seedJobsWeek(t, pool, station.ID, "forum-w1",
		time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 12, 0, 0, 0, 0, time.UTC), 5999)
	env := &forumEnv{pool: pool, store: forumadapters.NewStore(pool), root: t.TempDir()}
	env.build = Build{
		Pool: pool, Root: env.root, WindowWeeks: 1, PageSize: 100,
		ForumEditorial: func(tx pgx.Tx) forumapp.EditorialExportReader { return forumadapters.NewEditorialExportReader(tx) },
	}
	env.reconcile = Reconcile{
		Pool: pool, Root: env.root, WindowWeeks: 1, PageSize: 100,
		ForumEditorial: func(p *pgxpool.Pool) forumapp.EditorialExportReader { return forumadapters.NewEditorialExportReader(p) },
		Enqueue: func(_ context.Context, _ BuildPayload, dedupe string) (platformjobs.EnqueueResult, error) {
			env.enqueued = append(env.enqueued, dedupe)
			return platformjobs.EnqueueResult{}, nil
		},
	}
	return env
}

func (e *forumEnv) publish(t *testing.T, id, title string, at time.Time) *forumdomain.Post {
	t.Helper()
	ctx := context.Background()
	post := &forumdomain.Post{
		ID: id, Kind: forumdomain.KindEditorial, Category: forumdomain.CategoryFuels,
		Title: title, Excerpt: "Resumo de " + title, BodyText: "Introdução de " + title + ".\n\nSubtítulo\n\n- item um",
		SourceID: forumEditorialSource, PublicationState: forumdomain.StateDraft, Visibility: forumdomain.VisibilityVisible,
	}
	if err := e.store.CreateEditorialPost(ctx, post); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.PublishEditorialPost(ctx, id, at); err != nil {
		t.Fatal(err)
	}
	post.PublicationState = forumdomain.StatePublished
	return post
}

func (e *forumEnv) handle(t *testing.T) State {
	t.Helper()
	if err := e.build.Handle(context.Background(), platformjobs.Job{}); err != nil {
		t.Fatal(err)
	}
	state, ok := LoadState(e.root)
	if !ok || !state.Valid() {
		t.Fatalf("state = %+v, %v", state, ok)
	}
	return state
}

func (e *forumEnv) reconcileOnce(t *testing.T) int {
	t.Helper()
	before := len(e.enqueued)
	if err := e.reconcile.Handle(context.Background(), platformjobs.Job{}); err != nil {
		t.Fatal(err)
	}
	return len(e.enqueued) - before
}

// TestBuildCarriesEditorialForumAndReconcilesChanges proves on real PostGIS
// that published editorial posts enter the snapshot in the build's read-only
// transaction, that publish/edit/hide change the export content hash and
// enqueue a reconcile build, and that an unchanged forum stays a no-op.
func TestBuildCarriesEditorialForumAndReconcilesChanges(t *testing.T) {
	env := newForumEnv(t)
	ctx := context.Background()
	first := env.publish(t, "aaaaaaaa-0000-4000-8000-000000000001", "Etanol ou gasolina", time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC))
	draft := &forumdomain.Post{
		ID: "aaaaaaaa-0000-4000-8000-000000000009", Kind: forumdomain.KindEditorial, Category: forumdomain.CategoryFuels,
		Title: "Rascunho", BodyText: "Texto", SourceID: forumEditorialSource,
		PublicationState: forumdomain.StateDraft, Visibility: forumdomain.VisibilityVisible,
	}
	if err := env.store.CreateEditorialPost(ctx, draft); err != nil {
		t.Fatal(err)
	}

	state := env.handle(t)
	if state.Forum == nil || state.Forum.Posts != 1 || state.Forum.Fingerprint == "" {
		t.Fatalf("state forum = %+v", state.Forum)
	}
	snapshot, err := enterpriseexport.ReadSnapshot(state.CandidateDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.ForumPosts) != 1 || snapshot.ForumPosts[0].PostID != first.ID ||
		snapshot.ForumPosts[0].Title != "Etanol ou gasolina" || snapshot.ForumPosts[0].SourceName != "Equipe Abastevo" ||
		snapshot.ForumPosts[0].PublishedAt != "2026-11-10T12:00:00Z" || !strings.Contains(snapshot.ForumPosts[0].Body, "- item um") {
		t.Fatalf("snapshot forum = %+v", snapshot.ForumPosts)
	}
	if n := env.reconcileOnce(t); n != 0 {
		t.Fatalf("unchanged forum enqueued %d builds", n)
	}
	builtAt := state.CandidateDir
	if again := env.handle(t); again.CandidateDir != builtAt {
		t.Fatal("unchanged forum rebuilt the snapshot")
	}

	hashes := []string{state.ContentSHA256}
	steps := []struct {
		name   string
		change func(t *testing.T)
		posts  int
	}{
		{"publish", func(t *testing.T) {
			env.publish(t, "aaaaaaaa-0000-4000-8000-000000000002", "Recarga em rodovias", time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC))
		}, 2},
		{"edit", func(t *testing.T) {
			var revision int
			if err := env.pool.QueryRow(ctx, `SELECT revision FROM forum_posts WHERE id = $1`, first.ID).Scan(&revision); err != nil {
				t.Fatal(err)
			}
			edited := *first
			edited.Title = "Etanol ou gasolina: atualizado"
			if err := env.store.UpdateEditorialPost(ctx, &edited, revision); err != nil {
				t.Fatal(err)
			}
		}, 2},
		{"hide", func(t *testing.T) {
			if err := env.store.SetPostVisibility(ctx, first.ID, forumdomain.VisibilityHidden); err != nil {
				t.Fatal(err)
			}
		}, 1},
	}
	for _, step := range steps {
		step.change(t)
		if n := env.reconcileOnce(t); n != 1 {
			t.Fatalf("%s enqueued %d reconcile builds", step.name, n)
		}
		state = env.handle(t)
		if state.ContentSHA256 == hashes[len(hashes)-1] || state.Forum == nil || state.Forum.Posts != int64(step.posts) {
			t.Fatalf("%s state = %+v (previous hash %s)", step.name, state, hashes[len(hashes)-1])
		}
		hashes = append(hashes, state.ContentSHA256)
	}
	snapshot, err = enterpriseexport.ReadSnapshot(state.CandidateDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.ForumPosts) != 1 || snapshot.ForumPosts[0].PostID == first.ID {
		t.Fatalf("hidden post still exported: %+v", snapshot.ForumPosts)
	}
	if len(env.enqueued) != 3 || env.enqueued[0] == env.enqueued[1] || env.enqueued[1] == env.enqueued[2] {
		t.Fatalf("reconcile dedupe keys = %v", env.enqueued)
	}

	// Forum read switch off: the next build drops the forum section.
	env.build.ForumEditorial = nil
	env.reconcile.ForumEditorial = nil
	if n := env.reconcileOnce(t); n != 1 {
		t.Fatalf("switching the forum off enqueued %d builds", n)
	}
	state = env.handle(t)
	snapshot, err = enterpriseexport.ReadSnapshot(state.CandidateDir)
	if err != nil {
		t.Fatal(err)
	}
	if state.Forum != nil || snapshot.ForumPosts != nil {
		t.Fatalf("forum survived the read switch: %+v %+v", state.Forum, snapshot.ForumPosts)
	}
}

// TestRefreshPublishesAndWithdrawsEditorialPostPages runs the worker
// pipeline end to end on real PostGIS: publish → build → refresh serves the
// post page and lists it in the forum sitemap; hide → reconcile → build →
// refresh releases a portal without the page (404) or any forum route.
func TestRefreshPublishesAndWithdrawsEditorialPostPages(t *testing.T) {
	env := newForumEnv(t)
	ctx := context.Background()
	post := env.publish(t, "bbbbbbbb-0000-4000-8000-000000000001", "Etanol na estrada", time.Date(2026, 11, 12, 15, 0, 0, 0, time.UTC))
	env.handle(t)
	portalRoot := filepath.Join(t.TempDir(), "portal")
	refresh := Refresh{ExportRoot: env.root, PortalRoot: portalRoot, SiteURL: "https://portal.example", Indexable: true}
	if err := refresh.Handle(ctx, platformjobs.Job{}); err != nil {
		t.Fatal(err)
	}
	route := filepath.Join("forum", "combustiveis", "etanol-na-estrada-bbbbbbbb", "index.html")
	page, err := os.ReadFile(filepath.Join(portalRoot, "current", route))
	if err != nil {
		t.Fatalf("post page not released: %v", err)
	}
	for _, want := range []string{"<h1>Etanol na estrada</h1>", `"@type":"DiscussionForumPosting"`, "<h2>Subtítulo</h2>", "<li>item um</li>", `<meta name="robots" content="index,follow">`} {
		if !strings.Contains(string(page), want) {
			t.Errorf("released post page lacks %s", want)
		}
	}
	sitemap, err := os.ReadFile(filepath.Join(portalRoot, "current", "sitemaps", "portal-forum-1.xml"))
	if err != nil || !strings.Contains(string(sitemap), "https://portal.example/forum/combustiveis/etanol-na-estrada-bbbbbbbb/") {
		t.Fatalf("forum sitemap = %s, %v", sitemap, err)
	}
	if err := env.store.SetPostVisibility(ctx, post.ID, forumdomain.VisibilityHidden); err != nil {
		t.Fatal(err)
	}
	if n := env.reconcileOnce(t); n != 1 {
		t.Fatalf("hide enqueued %d builds", n)
	}
	env.handle(t)
	if err := refresh.Handle(ctx, platformjobs.Job{}); err != nil {
		t.Fatal(err)
	}
	if state, ok := loadRefreshState(portalRoot); !ok || state.LastOutcome != "released" {
		t.Fatalf("refresh after hide = %+v", state)
	}
	if _, err := os.Stat(filepath.Join(portalRoot, "current", route)); !os.IsNotExist(err) {
		t.Fatalf("hidden post still served: %v", err)
	}
	for _, gone := range []string{"forum", filepath.Join("sitemaps", "portal-forum-1.xml")} {
		if _, err := os.Stat(filepath.Join(portalRoot, "current", gone)); !os.IsNotExist(err) {
			t.Errorf("%s survived the last hidden post: %v", gone, err)
		}
	}
}
