//go:build integration

package adapters_test

import (
	"context"
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
	feedbackadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/adapters"
	forumadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/adapters"
	moderationadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/adapters"
	moderationdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/safety/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/safety/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/safety/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

const (
	accA       = "aaaaaaaa-0000-4000-8000-00000000000a"
	accB       = "bbbbbbbb-0000-4000-8000-00000000000b"
	accC       = "cccccccc-0000-4000-8000-00000000000c"
	accD       = "dddddddd-0000-4000-8000-00000000000d"
	seedSource = "00000000-0000-0000-0000-000000000001"
	station    = "51515151-0000-4000-8000-000000000001"
	editorial  = "e0e0e0e0-0000-4000-8000-000000000001"
	topicB     = "70707070-0000-4000-8000-00000000000b"
	pendingB   = "70707070-0000-4000-8000-0000000000bb"
	topicA     = "70707070-0000-4000-8000-00000000000a"
	commentB   = "c0c0c0c0-0000-4000-8000-00000000000b"
	hiddenB    = "c0c0c0c0-0000-4000-8000-0000000000bb"
	commentA   = "c0c0c0c0-0000-4000-8000-00000000000a"
	replyC     = "c0c0c0c0-0000-4000-8000-00000000000c"
	replyOnHid = "c0c0c0c0-0000-4000-8000-0000000000cc"
	stationB   = "5c5c5c5c-0000-4000-8000-00000000000b"
	deletedB   = "5c5c5c5c-0000-4000-8000-0000000000bb"
	unknownID  = "99999999-0000-4000-8000-000000000099"
)

type env struct {
	pool  *pgxpool.Pool
	store *adapters.Store
	quota *fakeQuota
}

type fakeQuota struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (f *fakeQuota) check(context.Context, pgx.Tx, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.err
}

func testDSN() string {
	if dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL"); dsn != "" {
		return dsn
	}
	return "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
}

func setup(t *testing.T) env {
	t.Helper()
	adminDSN := testDSN()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("safety_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		c, err := pgx.Connect(ctx, adminDSN)
		if err != nil {
			t.Errorf("drop connect: %v", err)
			return
		}
		defer c.Close(ctx)
		if _, err := c.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Errorf("drop database: %v", err)
		}
	})
	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	if _, err := migrate.Apply(ctx, u.String(), dbmigrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	seed(t, pool)

	forum := forumadapters.NewStore(pool)
	feedback := feedbackadapters.NewPGStore(pool)
	moderation := moderationadapters.NewStore(pool)
	quota := &fakeQuota{}
	store := adapters.NewStore(pool)
	store.Sources = map[domain.ContentKind]adapters.AuthorSource{
		domain.KindForumPost:      {PublicAuthorTx: forum.SafetyTopicAuthorTx, Authors: forum.SafetyTopicAuthors},
		domain.KindForumComment:   {PublicAuthorTx: forum.SafetyCommentAuthorTx, Authors: forum.SafetyCommentAuthors},
		domain.KindStationComment: {PublicAuthorTx: feedback.SafetyCommentAuthorTx, Authors: feedback.SafetyCommentAuthors},
	}
	store.BlockQuota = quota.check
	store.OpenCase = func(ctx context.Context, tx pgx.Tx, targetType, targetID, reason, detail string) (string, bool, error) {
		c, _, err := moderationdomain.NewCase(moderationdomain.CaseParams{
			ID: newID(t), TargetType: targetType, TargetID: targetID,
			Priority: moderationdomain.PriorityP2, Reason: reason, Detail: detail,
			OpenedAt: time.Now(), PolicyVersion: moderationdomain.PolicyV1,
		})
		if err != nil {
			return "", false, err
		}
		return moderation.OpenCaseTx(ctx, tx, c)
	}
	return env{pool: pool, store: store, quota: quota}
}

var idSeq struct {
	sync.Mutex
	n int
}

func newID(t *testing.T) string {
	t.Helper()
	idSeq.Lock()
	defer idSeq.Unlock()
	idSeq.n++
	return fmt.Sprintf("12345678-0000-4000-8000-%012d", idSeq.n)
}

func exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", strings.SplitN(strings.TrimSpace(sql), "\n", 2)[0], err)
	}
}

// seed builds every public and non-public content shape the rules name.
func seed(t *testing.T, pool *pgxpool.Pool) {
	for _, a := range []struct{ id, alias string }{{accA, "aaaa-aaaa-aaaa"}, {accB, "bbbb-bbbb-bbbb"}, {accC, "cccc-cccc-cccc"}, {accD, "dddd-dddd-dddd"}} {
		exec(t, pool, `INSERT INTO accounts (id, alias, status) VALUES ($1, $2, 'active')`, a.id, a.alias)
	}
	exec(t, pool, `INSERT INTO forum_posts (id, kind, category, title, excerpt, body_text, source_id,
		publication_state, visibility, first_publication_seq, first_published_at, comments_enabled, revision)
		VALUES ($1, 'EDITORIAL', 'FUELS', 'Editorial', 'Resumo', 'Texto', $2, 'PUBLISHED', 'VISIBLE', 1, now(), true, 1)`, editorial, seedSource)
	topic := `INSERT INTO forum_posts (id, kind, category, title, body_text, author_account_id, public_alias,
		publication_state, visibility, first_published_at, first_publication_seq)
		VALUES ($1, 'TOPIC', 'FUELS', 'Tópico', 'Texto', $2, $3, $4, 'VISIBLE', now(), $5)`
	exec(t, pool, topic, topicB, accB, "Motorista B", "PUBLISHED", 2)
	exec(t, pool, `INSERT INTO forum_posts (id, kind, category, title, body_text, author_account_id,
		publication_state, visibility) VALUES ($1, 'TOPIC', 'FUELS', 'Pendente', 'Texto', $2, 'PENDING_REVIEW', 'VISIBLE')`, pendingB, accB)
	exec(t, pool, topic, topicA, accA, nil, "PUBLISHED", 3)
	comment := `INSERT INTO forum_comments (id, post_id, author_account_id, public_alias, parent_id, depth, sequence,
		body_text, revision, publication_state, visibility, published_at, create_operation_id, body_sha256)
		VALUES ($1, $2, $3, '', $4, $5, $6, 'Comentário', 1, 'PUBLISHED', $7, now(), $8, 'sha')`
	exec(t, pool, comment, commentB, editorial, accB, nil, 0, 1, "VISIBLE", newID(t))
	exec(t, pool, comment, hiddenB, editorial, accB, nil, 0, 2, "HIDDEN", newID(t))
	exec(t, pool, comment, commentA, editorial, accA, nil, 0, 3, "VISIBLE", newID(t))
	exec(t, pool, comment, replyC, editorial, accC, commentB, 1, 4, "VISIBLE", newID(t))
	exec(t, pool, comment, replyOnHid, editorial, accC, hiddenB, 1, 5, "VISIBLE", newID(t))
	exec(t, pool, `INSERT INTO directory_stations (id, display_name) VALUES ($1, 'Posto Teste')`, station)
	exec(t, pool, `INSERT INTO feedback_comments (id, account_id, station_id, product, depth, text)
		VALUES ($1, $2, $3, 'GASOLINA_COMUM', 0, 'Bom posto')`, stationB, accB, station)
	exec(t, pool, `INSERT INTO feedback_comments (id, account_id, station_id, product, depth, text, deleted_at)
		VALUES ($1, $2, $3, 'GASOLINA_COMUM', 0, 'Apagado', now())`, deletedB, accB, station)
}

func ref(kind domain.ContentKind, id string) domain.ContentRef {
	return domain.ContentRef{Kind: kind, ID: id}
}

func block(e env, actor string, r domain.ContentRef) (domain.BlockResult, error) {
	return e.store.Block(context.Background(), application.BlockCommand{AccountID: actor, Ref: r})
}

func count(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", sql, err)
	}
	return n
}

// B-BR-SAFE-05/06/07, BUC-SAFE-03/04: block converges per account pair,
// hides that account's content of every kind for the blocker only, and
// unblocking restores it.
func TestSafetyBlockLifecycle(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	first, err := block(e, accA, ref(domain.KindForumPost, topicB))
	if err != nil || first.Replayed {
		t.Fatalf("first block = %+v, %v", first, err)
	}
	if first.Block.Label != "Motorista B" || first.Block.SourceKind != domain.KindForumPost {
		t.Fatalf("block view = %+v", first.Block)
	}
	again, err := block(e, accA, ref(domain.KindStationComment, stationB))
	if err != nil || !again.Replayed || again.Block.ID != first.Block.ID {
		t.Fatalf("same author via station comment = %+v, %v; want replay of %s", again, err, first.Block.ID)
	}
	if e.quota.calls != 1 {
		t.Fatalf("quota consumed %d times, want once (replays are free)", e.quota.calls)
	}

	list, err := e.store.ListBlocks(ctx, accA)
	if err != nil || len(list) != 1 || list[0].ID != first.Block.ID {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if other, _ := e.store.ListBlocks(ctx, accC); len(other) != 0 {
		t.Fatalf("foreign list leaked: %+v", other)
	}

	refs := []domain.ContentRef{
		ref(domain.KindForumPost, topicB), ref(domain.KindForumPost, pendingB), ref(domain.KindForumPost, topicA),
		ref(domain.KindForumComment, commentB), ref(domain.KindForumComment, hiddenB), ref(domain.KindForumComment, replyC),
		ref(domain.KindStationComment, stationB), ref(domain.KindStationComment, deletedB),
		ref(domain.KindForumComment, unknownID), ref(domain.KindForumPost, editorial),
	}
	hidden, err := e.store.Hidden(ctx, accA, refs)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.ContentRef{refs[0], refs[1], refs[3], refs[4], refs[6], refs[7]}
	if fmt.Sprint(hidden) != fmt.Sprint(want) {
		t.Fatalf("hidden = %v, want %v", hidden, want)
	}
	if other, err := e.store.Hidden(ctx, accC, refs); err != nil || len(other) != 0 {
		t.Fatalf("non-blocker hidden = %v, %v", other, err)
	}

	// Foreign and unknown block ids refuse alike and change nothing.
	if err := e.store.Unblock(ctx, application.UnblockCommand{AccountID: accC, BlockID: first.Block.ID}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign unblock = %v, want ErrNotFound", err)
	}
	if err := e.store.Unblock(ctx, application.UnblockCommand{AccountID: accA, BlockID: unknownID}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown unblock = %v", err)
	}
	if err := e.store.Unblock(ctx, application.UnblockCommand{AccountID: accA, BlockID: first.Block.ID}); err != nil {
		t.Fatalf("unblock: %v", err)
	}
	if err := e.store.Unblock(ctx, application.UnblockCommand{AccountID: accA, BlockID: first.Block.ID}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second unblock = %v, want ErrNotFound", err)
	}
	if hidden, err := e.store.Hidden(ctx, accA, refs); err != nil || len(hidden) != 0 {
		t.Fatalf("hidden after unblock = %v, %v", hidden, err)
	}
}

// B-BR-SAFE-02: only content public reads serve names an author; own
// content is a self target; malformed sources never reach SQL.
func TestSafetyTargetRefusals(t *testing.T) {
	e := setup(t)
	cases := map[string]struct {
		r    domain.ContentRef
		want error
	}{
		"own topic":             {ref(domain.KindForumPost, topicA), domain.ErrSelfTarget},
		"own comment":           {ref(domain.KindForumComment, commentA), domain.ErrSelfTarget},
		"pending topic":         {ref(domain.KindForumPost, pendingB), domain.ErrNotFound},
		"editorial post":        {ref(domain.KindForumPost, editorial), domain.ErrNotFound},
		"comment id as topic":   {ref(domain.KindForumPost, commentB), domain.ErrNotFound},
		"hidden comment":        {ref(domain.KindForumComment, hiddenB), domain.ErrNotFound},
		"reply on hidden":       {ref(domain.KindForumComment, replyOnHid), domain.ErrNotFound},
		"deleted station":       {ref(domain.KindStationComment, deletedB), domain.ErrNotFound},
		"unknown comment":       {ref(domain.KindForumComment, unknownID), domain.ErrNotFound},
		"forum id as station":   {ref(domain.KindStationComment, commentB), domain.ErrNotFound},
		"station id as comment": {ref(domain.KindForumComment, stationB), domain.ErrNotFound},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := block(e, accA, tc.r); !errors.Is(err, tc.want) {
				t.Fatalf("block err = %v, want %v", err, tc.want)
			}
			_, err := e.store.ReportUser(context.Background(), application.ReportCommand{AccountID: accA, Ref: tc.r, Reason: domain.ReasonSpam})
			if !errors.Is(err, tc.want) {
				t.Fatalf("report err = %v, want %v", err, tc.want)
			}
		})
	}
	if n := count(t, e.pool, `SELECT count(*) FROM community_user_blocks`); n != 0 {
		t.Fatalf("refusals stored %d blocks", n)
	}
	if n := count(t, e.pool, `SELECT count(*) FROM community_user_reports`); n != 0 {
		t.Fatalf("refusals stored %d reports", n)
	}
	if n := count(t, e.pool, `SELECT count(*) FROM moderation_cases`); n != 0 {
		t.Fatalf("refusals opened %d cases", n)
	}
	// A reply by another account to a visible comment is public.
	if _, err := block(e, accA, ref(domain.KindForumComment, replyC)); err != nil {
		t.Fatalf("visible reply refused: %v", err)
	}
}

// B-BR-SAFE-04, BUC-SAFE-02/05: one report per pair, one open case per
// reported account, fixed PII-free case detail, private free text.
func TestSafetyReportUser(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	first, err := e.store.ReportUser(ctx, application.ReportCommand{AccountID: accA, Ref: ref(domain.KindStationComment, stationB), Reason: domain.ReasonHarassment, Detail: "insultos repetidos"})
	if err != nil || first.Replayed || first.Reason != domain.ReasonHarassment {
		t.Fatalf("first report = %+v, %v", first, err)
	}
	again, err := e.store.ReportUser(ctx, application.ReportCommand{AccountID: accA, Ref: ref(domain.KindForumComment, commentB), Reason: domain.ReasonSpam})
	if err != nil || !again.Replayed || again.ID != first.ID || again.Reason != domain.ReasonHarassment {
		t.Fatalf("replay = %+v, %v; want first receipt", again, err)
	}
	if _, err := e.store.ReportUser(ctx, application.ReportCommand{AccountID: accC, Ref: ref(domain.KindForumPost, topicB), Reason: domain.ReasonSpam}); err != nil {
		t.Fatalf("second reporter: %v", err)
	}

	var target, detail, reason string
	var cases int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) OVER (), target_id::text, detail, reason FROM moderation_cases WHERE target_type = 'COMMUNITY_ACCOUNT'`).Scan(&cases, &target, &detail, &reason); err != nil {
		t.Fatal(err)
	}
	if cases != 1 || target != accB {
		t.Fatalf("cases = %d target = %s, want one case on %s", cases, target, accB)
	}
	if detail != "community user report via STATION_COMMENT "+stationB || strings.Contains(detail, "insultos") || strings.Contains(detail, accA) {
		t.Fatalf("case detail leaks or drifts: %q", detail)
	}
	if n := count(t, e.pool, `SELECT count(*) FROM community_user_reports WHERE moderation_case_id IS NOT NULL`); n != 2 {
		t.Fatalf("linked reports = %d, want 2", n)
	}
	if n := count(t, e.pool, `SELECT count(*) FROM community_user_reports WHERE detail = 'insultos repetidos' AND reporter_account_id = $1`, accA); n != 1 {
		t.Fatalf("private detail not kept with the report")
	}
}

// B-BR-SAFE-04: the 11th new user report in 24 hours refuses with a
// retry delay and stores nothing; replays stay free.
func TestSafetyReportQuota(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	exec(t, e.pool, `INSERT INTO accounts (id, alias, status)
		SELECT ('eeeeeeee-0000-4000-8000-' || lpad(i::text, 12, '0'))::uuid, 'quota-' || i, 'active' FROM generate_series(1, 10) i`)
	exec(t, e.pool, `INSERT INTO community_user_reports (id, reporter_account_id, reported_account_id, source_kind, source_id, reason, created_at)
		SELECT gen_random_uuid(), $1, ('eeeeeeee-0000-4000-8000-' || lpad(i::text, 12, '0'))::uuid, 'FORUM_COMMENT', gen_random_uuid(), 'SPAM', now() - interval '1 hour'
		FROM generate_series(1, 10) i`, accD)
	_, err := e.store.ReportUser(ctx, application.ReportCommand{AccountID: accD, Ref: ref(domain.KindForumComment, commentB), Reason: domain.ReasonSpam})
	var quota *domain.QuotaError
	if !errors.As(err, &quota) || quota.RetryAfterSeconds <= 0 {
		t.Fatalf("11th report err = %v, want QuotaError", err)
	}
	if n := count(t, e.pool, `SELECT count(*) FROM moderation_cases`); n != 0 {
		t.Fatalf("quota refusal opened %d cases", n)
	}
	// Reports older than the window no longer count.
	exec(t, e.pool, `UPDATE community_user_reports SET created_at = now() - interval '25 hours' WHERE reporter_account_id = $1`, accD)
	if _, err := e.store.ReportUser(ctx, application.ReportCommand{AccountID: accD, Ref: ref(domain.KindForumComment, commentB), Reason: domain.ReasonSpam}); err != nil {
		t.Fatalf("report after window: %v", err)
	}
}

// B-BR-SAFE-05: active cap answers 409 before the daily window; a window
// refusal rolls the whole transaction back.
func TestSafetyBlockLimits(t *testing.T) {
	e := setup(t)
	exec(t, e.pool, `INSERT INTO accounts (id, alias, status)
		SELECT ('ffffffff-0000-4000-8000-' || lpad(i::text, 12, '0'))::uuid, 'cap-' || i, 'active' FROM generate_series(1, 500) i`)

	e.quota.err = &domain.QuotaError{RetryAfterSeconds: 120}
	if _, err := block(e, accA, ref(domain.KindForumComment, commentB)); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("window refusal = %v, want ErrRateLimited", err)
	}
	if n := count(t, e.pool, `SELECT count(*) FROM community_user_blocks`); n != 0 {
		t.Fatalf("window refusal stored %d blocks", n)
	}

	e.quota.err = nil
	exec(t, e.pool, `INSERT INTO community_user_blocks (id, blocker_account_id, blocked_account_id, source_kind, created_at)
		SELECT gen_random_uuid(), $1, ('ffffffff-0000-4000-8000-' || lpad(i::text, 12, '0'))::uuid, 'FORUM_COMMENT', now()
		FROM generate_series(1, 500) i`, accA)
	calls := e.quota.calls
	if _, err := block(e, accA, ref(domain.KindForumComment, commentB)); !errors.Is(err, domain.ErrBlockLimit) {
		t.Fatalf("501st block = %v, want ErrBlockLimit", err)
	}
	if e.quota.calls != calls {
		t.Fatal("active cap refusal consumed window quota")
	}
	list, err := e.store.ListBlocks(context.Background(), accA)
	if err != nil || len(list) != domain.MaxActiveBlocks {
		t.Fatalf("list = %d, %v; want the bounded 500", len(list), err)
	}
}

// B-BR-SAFE-05: concurrent blocks of one pair converge on one row.
func TestSafetyConcurrentBlocksConverge(t *testing.T) {
	e := setup(t)
	const n = 8
	ids := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			kind, id := domain.KindForumComment, commentB
			if i%2 == 1 {
				kind, id = domain.KindStationComment, stationB
			}
			res, err := block(e, accA, ref(kind, id))
			ids[i], errs[i] = res.Block.ID, err
		}(i)
	}
	wg.Wait()
	for i := range ids {
		if errs[i] != nil || ids[i] != ids[0] {
			t.Fatalf("result %d = %s, %v; want %s", i, ids[i], errs[i], ids[0])
		}
	}
	if c := count(t, e.pool, `SELECT count(*) FROM community_user_blocks`); c != 1 {
		t.Fatalf("rows = %d, want 1", c)
	}
}

// B-BR-SAFE-03: a guard refusal inside the transaction stores nothing.
func TestSafetyGuardRefusalStoresNothing(t *testing.T) {
	e := setup(t)
	refused := errors.New("session revoked")
	guard := func(context.Context, pgx.Tx) error { return refused }
	if _, err := e.store.Block(context.Background(), application.BlockCommand{AccountID: accA, Ref: ref(domain.KindForumComment, commentB), Guard: guard}); !errors.Is(err, refused) {
		t.Fatalf("block err = %v", err)
	}
	if _, err := e.store.ReportUser(context.Background(), application.ReportCommand{AccountID: accA, Ref: ref(domain.KindForumComment, commentB), Reason: domain.ReasonSpam, Guard: guard}); !errors.Is(err, refused) {
		t.Fatalf("report err = %v", err)
	}
	if c := count(t, e.pool, `SELECT (SELECT count(*) FROM community_user_blocks) + (SELECT count(*) FROM community_user_reports) + (SELECT count(*) FROM moderation_cases)`); c != 0 {
		t.Fatalf("guard refusal stored %d rows", c)
	}
}

// B-BR-SAFE-08: erasure deletes blocks in both directions and scrubs the
// account's report text in bounded batches; others' reports remain.
func TestSafetyErasure(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if _, err := block(e, accA, ref(domain.KindForumComment, commentB)); err != nil {
		t.Fatal(err)
	}
	if _, err := block(e, accC, ref(domain.KindForumComment, commentA)); err != nil {
		t.Fatal(err)
	}
	if _, err := block(e, accC, ref(domain.KindForumComment, commentB)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.ReportUser(ctx, application.ReportCommand{AccountID: accA, Ref: ref(domain.KindForumComment, commentB), Reason: domain.ReasonOther, Detail: "texto privado"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.ReportUser(ctx, application.ReportCommand{AccountID: accB, Ref: ref(domain.KindForumComment, commentA), Reason: domain.ReasonSpam, Detail: "texto de B"}); err != nil {
		t.Fatal(err)
	}

	done, err := e.store.EraseAccountBatch(ctx, accA, 1)
	if err != nil || done {
		t.Fatalf("first bounded batch = %v, %v; want pending", done, err)
	}
	for i := 0; i < 5 && !done; i++ {
		if done, err = e.store.EraseAccountBatch(ctx, accA, 1); err != nil {
			t.Fatal(err)
		}
	}
	if !done {
		t.Fatal("erasure never converged")
	}
	if n := count(t, e.pool, `SELECT count(*) FROM community_user_blocks WHERE blocker_account_id = $1 OR blocked_account_id = $1`, accA); n != 0 {
		t.Fatalf("blocks left for erased account: %d", n)
	}
	if n := count(t, e.pool, `SELECT count(*) FROM community_user_blocks`); n != 1 {
		t.Fatalf("unrelated blocks = %d, want C's block of B kept", n)
	}
	if n := count(t, e.pool, `SELECT count(*) FROM community_user_reports WHERE reporter_account_id = $1 AND detail <> ''`, accA); n != 0 {
		t.Fatal("erased reporter detail kept")
	}
	if n := count(t, e.pool, `SELECT count(*) FROM community_user_reports WHERE reporter_account_id = $1 AND detail = 'texto de B'`, accB); n != 1 {
		t.Fatal("another reporter's evidence about the erased account was scrubbed")
	}
	if done, err := e.store.EraseAccountBatch(ctx, accA, 1); err != nil || !done {
		t.Fatalf("replayed erasure = %v, %v", done, err)
	}
}

// Migration 000239 constraints hold below the application.
func TestSafetySchemaConstraints(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	_, err := e.pool.Exec(ctx, `INSERT INTO community_user_blocks (id, blocker_account_id, blocked_account_id, source_kind, created_at)
		VALUES (gen_random_uuid(), $1, $1, 'FORUM_POST', now())`, accA)
	if err == nil {
		t.Fatal("self block accepted by the schema")
	}
	_, err = e.pool.Exec(ctx, `INSERT INTO community_user_reports (id, reporter_account_id, reported_account_id, source_kind, source_id, reason, created_at)
		VALUES (gen_random_uuid(), $1, $2, 'FORUM_POST', gen_random_uuid(), 'OFF_TOPIC', now())`, accA, accB)
	if err == nil {
		t.Fatal("unknown reason accepted by the schema")
	}
	_, err = e.pool.Exec(ctx, `INSERT INTO community_user_blocks (id, blocker_account_id, blocked_account_id, source_kind, created_at)
		VALUES (gen_random_uuid(), $1, $2, 'EDITORIAL', now())`, accA, accB)
	if err == nil {
		t.Fatal("unknown source kind accepted by the schema")
	}
}
