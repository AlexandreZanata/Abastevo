//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func freshDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("explicit disposable PostGIS DSN required")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Hostname() != "127.0.0.1" {
		t.Fatal("literal loopback test DSN required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("disposable database unavailable")
	}
	name := fmt.Sprintf("notifications_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	admin.Close(ctx)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, dsn)
		if err != nil {
			return
		}
		defer admin.Close(ctx)
		_, _ = admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
	})
	u.Path = "/" + name
	if _, err = migrate.Apply(ctx, u.String(), migrations.Files); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 24
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func newAccount(t *testing.T, pool *pgxpool.Pool, status string) string {
	t.Helper()
	id, err := newID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), "INSERT INTO accounts(id,alias,status) VALUES($1,$2,$3)", id, "synthetic-"+id, status); err != nil {
		t.Fatal(err)
	}
	return id
}

func uuid(t *testing.T) string {
	t.Helper()
	id, err := newID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func replyEvent(t *testing.T, recipient, comment string, at time.Time) domain.Event {
	t.Helper()
	e, err := domain.NewForumReply(recipient, uuid(t), domain.ForumReply{
		TopicID: "33333333-3333-4333-8333-333333333333", TopicTitle: "Synthetic topic", CommentID: comment, AuthorName: "Synthetic",
	}, at)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// notify runs one producer transaction; commit=false rolls it back.
func notify(t *testing.T, pool *pgxpool.Pool, s *Store, e domain.Event, commit bool) error {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.Notify(ctx, tx, e); err != nil {
		return err
	}
	if !commit {
		return tx.Rollback(ctx)
	}
	return tx.Commit(ctx)
}

func count(t *testing.T, pool *pgxpool.Pool, where string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM account_notifications WHERE "+where, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestNotifyIsIdempotentPerAccountAndEventKey(t *testing.T) {
	pool := freshDB(t)
	s := NewStore(pool)
	a, b := newAccount(t, pool, "active"), newAccount(t, pool, "active")
	comment := uuid(t)
	now := time.Now()
	for range 3 {
		if err := notify(t, pool, s, replyEvent(t, a, comment, now), true); err != nil {
			t.Fatal(err)
		}
	}
	if err := notify(t, pool, s, replyEvent(t, b, comment, now), true); err != nil {
		t.Fatal(err)
	}
	if count(t, pool, "account_id=$1", a) != 1 || count(t, pool, "account_id=$1", b) != 1 {
		t.Fatal("a repeated event key must notify each account exactly once")
	}
}

func TestNotifyJoinsTheProducerTransaction(t *testing.T) {
	pool := freshDB(t)
	s := NewStore(pool)
	a := newAccount(t, pool, "active")
	if err := notify(t, pool, s, replyEvent(t, a, uuid(t), time.Now()), false); err != nil {
		t.Fatal(err)
	}
	if count(t, pool, "account_id=$1", a) != 0 {
		t.Fatal("a rolled-back producer left a notice behind")
	}
	if err := s.Notify(context.Background(), nil, replyEvent(t, a, uuid(t), time.Now())); err == nil {
		t.Fatal("a notice outside a producer transaction was accepted")
	}
}

func TestNotifySkipsAccountsThatAreNotActive(t *testing.T) {
	pool := freshDB(t)
	s := NewStore(pool)
	for _, status := range []string{"suspended", "deleted"} {
		id := newAccount(t, pool, status)
		if err := notify(t, pool, s, replyEvent(t, id, uuid(t), time.Now()), true); err != nil {
			t.Fatalf("%s: %v", status, err)
		}
		if count(t, pool, "account_id=$1", id) != 0 {
			t.Fatalf("%s account notified", status)
		}
	}
	if err := notify(t, pool, s, replyEvent(t, uuid(t), uuid(t), time.Now()), true); err != nil {
		t.Fatalf("unknown account must be skipped, not fail the producer: %v", err)
	}
}

func TestConcurrentNotifySameEventKeyConverges(t *testing.T) {
	pool := freshDB(t)
	s := NewStore(pool)
	a := newAccount(t, pool, "active")
	comment := uuid(t)
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- notify(t, pool, s, replyEvent(t, a, comment, time.Now()), true)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent producer failed: %v", err)
		}
	}
	if n := count(t, pool, "account_id=$1", a); n != 1 {
		t.Fatalf("concurrent same-key inserts left %d rows", n)
	}
}

func TestExpiredRowsAreHiddenAndPurgedInBoundedBatches(t *testing.T) {
	pool := freshDB(t)
	s := NewStore(pool)
	svc := application.Service{Inbox: s}
	a := newAccount(t, pool, "active")
	old := time.Now().Add(-domain.TTL - time.Hour)
	expired := make([]string, 0, domain.PurgeBatch+5)
	for range domain.PurgeBatch + 5 {
		e := replyEvent(t, a, uuid(t), old)
		if _, err := pool.Exec(context.Background(), `INSERT INTO account_notifications(id,account_id,event_key,kind,payload,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`,
			uuid(t), a, e.EventKey, e.Kind, e.Payload, e.CreatedAt, e.ExpiresAt); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(context.Background(), "SELECT array_agg(id::text) FROM account_notifications").Scan(&expired); err != nil {
		t.Fatal(err)
	}
	live := uuid(t)
	if _, err := pool.Exec(context.Background(), `INSERT INTO account_notifications(id,account_id,event_key,kind,payload,created_at,expires_at) VALUES($1,$2,'forum.reply:live','forum.reply','{}',now(),now()+interval '30 days')`, live, a); err != nil {
		t.Fatal(err)
	}
	page, err := svc.List(context.Background(), a, nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].NotificationID != live || page.Unread != 1 {
		t.Fatalf("expired rows must be skipped: %d items, unread %d", len(page.Items), page.Unread)
	}
	if n, err := svc.MarkRead(context.Background(), a, domain.Selection{IDs: expired[:50]}); err != nil || n != 0 {
		t.Fatalf("expired rows must not be markable: %d %v", n, err)
	}
	// That write purged one bounded batch; the next write purges the rest.
	if n := count(t, pool, "expires_at <= now()"); n != 5 {
		t.Fatalf("first write left %d expired rows, want 5", n)
	}
	if err := notify(t, pool, s, replyEvent(t, a, uuid(t), time.Now()), true); err != nil {
		t.Fatal(err)
	}
	if n := count(t, pool, "expires_at <= now()"); n != 0 {
		t.Fatalf("producer write left %d expired rows", n)
	}
	if count(t, pool, "id=$1", live) != 1 {
		t.Fatal("purge removed a live row")
	}
}

func TestAccountsNeverSeeOrMarkEachOthersRows(t *testing.T) {
	pool := freshDB(t)
	s := NewStore(pool)
	svc := application.Service{Inbox: s}
	a, b := newAccount(t, pool, "active"), newAccount(t, pool, "active")
	now := time.Now()
	for range 3 {
		if err := notify(t, pool, s, replyEvent(t, b, uuid(t), now), true); err != nil {
			t.Fatal(err)
		}
	}
	if err := notify(t, pool, s, replyEvent(t, a, uuid(t), now), true); err != nil {
		t.Fatal(err)
	}
	var bIDs []string
	if err := pool.QueryRow(context.Background(), "SELECT array_agg(id::text) FROM account_notifications WHERE account_id=$1", b).Scan(&bIDs); err != nil {
		t.Fatal(err)
	}
	page, err := svc.List(context.Background(), a, nil, 50)
	if err != nil || len(page.Items) != 1 || page.Unread != 1 {
		t.Fatalf("A lists %d items, unread %d, %v", len(page.Items), page.Unread, err)
	}
	for _, id := range bIDs {
		if page.Items[0].NotificationID == id {
			t.Fatal("A listed a row of B")
		}
	}
	if n, err := svc.MarkRead(context.Background(), a, domain.Selection{IDs: bIDs}); err != nil || n != 0 {
		t.Fatalf("A marking B's IDs answered %d %v", n, err)
	}
	if n, err := svc.MarkRead(context.Background(), a, domain.Selection{All: true}); err != nil || n != 1 {
		t.Fatalf("A mark all answered %d %v", n, err)
	}
	if count(t, pool, "account_id=$1 AND read_at IS NULL", b) != 3 {
		t.Fatal("A changed the read state of B")
	}
	// A cursor positioned on B's newest row still only pages A's rows.
	var bNewest time.Time
	if err := pool.QueryRow(context.Background(), "SELECT max(created_at) FROM account_notifications WHERE account_id=$1", b).Scan(&bNewest); err != nil {
		t.Fatal(err)
	}
	cross, err := svc.List(context.Background(), a, &domain.Position{CreatedAt: bNewest.Add(time.Hour), ID: bIDs[0]}, 50)
	if err != nil || len(cross.Items) != 1 || cross.Items[0].Read != true {
		t.Fatalf("foreign position leaked rows: %+v %v", cross, err)
	}
}

func TestPagingWalksEveryRowOnceNewestFirst(t *testing.T) {
	pool := freshDB(t)
	s := NewStore(pool)
	svc := application.Service{Inbox: s}
	a := newAccount(t, pool, "active")
	base := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	want := 7
	for i := range want {
		at := base.Add(time.Duration(i) * time.Minute)
		if i >= 5 {
			at = base.Add(5 * time.Minute) // two rows share created_at: id breaks the tie
		}
		if err := notify(t, pool, s, replyEvent(t, a, uuid(t), at), true); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	var before *domain.Position
	var last application.Item
	pages := 0
	for {
		page, err := svc.List(context.Background(), a, before, 2)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		if page.Unread != int64(want) {
			t.Fatalf("unread %d on page %d", page.Unread, pages)
		}
		for _, item := range page.Items {
			if seen[item.NotificationID] {
				t.Fatalf("row %s listed twice", item.NotificationID)
			}
			seen[item.NotificationID] = true
			if last.CreatedAt != "" && item.CreatedAt > last.CreatedAt {
				t.Fatalf("not newest first: %s after %s", item.CreatedAt, last.CreatedAt)
			}
			var p domain.ForumReply
			if err := json.Unmarshal(item.Payload, &p); err != nil || p.TopicTitle != "Synthetic topic" {
				t.Fatalf("payload %s", item.Payload)
			}
			last = item
		}
		if page.Next == nil {
			break
		}
		before = page.Next
		if pages > want {
			t.Fatal("paging does not terminate")
		}
	}
	if len(seen) != want || pages != 4 {
		t.Fatalf("walked %d rows in %d pages", len(seen), pages)
	}
}

func TestMarkReadIsReplayStable(t *testing.T) {
	pool := freshDB(t)
	s := NewStore(pool)
	svc := application.Service{Inbox: s}
	a := newAccount(t, pool, "active")
	for range 3 {
		if err := notify(t, pool, s, replyEvent(t, a, uuid(t), time.Now()), true); err != nil {
			t.Fatal(err)
		}
	}
	var ids []string
	if err := pool.QueryRow(context.Background(), "SELECT array_agg(id::text ORDER BY id) FROM account_notifications").Scan(&ids); err != nil {
		t.Fatal(err)
	}
	sel := domain.Selection{IDs: ids[:2]}
	for attempt := range 2 {
		if n, err := svc.MarkRead(context.Background(), a, sel); err != nil || n != 2 {
			t.Fatalf("attempt %d marked %d %v", attempt, n, err)
		}
	}
	var firstRead time.Time
	if err := pool.QueryRow(context.Background(), "SELECT read_at FROM account_notifications WHERE id=$1", ids[0]).Scan(&firstRead); err != nil {
		t.Fatal(err)
	}
	page, err := svc.List(context.Background(), a, nil, 10)
	if err != nil || page.Unread != 1 {
		t.Fatalf("unread %d %v", page.Unread, err)
	}
	if n, err := svc.MarkRead(context.Background(), a, domain.Selection{All: true}); err != nil || n != 3 {
		t.Fatalf("all %d %v", n, err)
	}
	var again time.Time
	if err := pool.QueryRow(context.Background(), "SELECT read_at FROM account_notifications WHERE id=$1", ids[0]).Scan(&again); err != nil || !again.Equal(firstRead) {
		t.Fatal("a replay must not move an existing read time")
	}
}

func TestConcurrentMarkReadConverges(t *testing.T) {
	pool := freshDB(t)
	s := NewStore(pool)
	svc := application.Service{Inbox: s}
	a := newAccount(t, pool, "active")
	for range 5 {
		if err := notify(t, pool, s, replyEvent(t, a, uuid(t), time.Now()), true); err != nil {
			t.Fatal(err)
		}
	}
	var ids []string
	if err := pool.QueryRow(context.Background(), "SELECT array_agg(id::text) FROM account_notifications").Scan(&ids); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan int64, 8)
	errs := make(chan error, 8)
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			sel := domain.Selection{IDs: ids}
			if i%2 == 0 {
				sel = domain.Selection{All: true}
			}
			n, err := svc.MarkRead(context.Background(), a, sel)
			results <- n
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent mark failed: %v", err)
		}
	}
	for n := range results {
		if n != 5 {
			t.Fatalf("a concurrent mark answered %d, want 5", n)
		}
	}
	if count(t, pool, "account_id=$1 AND read_at IS NULL", a) != 0 {
		t.Fatal("concurrent marks left unread rows")
	}
}

func TestEraseAccountBatchDrainsOnlyThatAccount(t *testing.T) {
	pool := freshDB(t)
	s := NewStore(pool)
	a, b := newAccount(t, pool, "active"), newAccount(t, pool, "active")
	for range 5 {
		if err := notify(t, pool, s, replyEvent(t, a, uuid(t), time.Now()), true); err != nil {
			t.Fatal(err)
		}
	}
	if err := notify(t, pool, s, replyEvent(t, b, uuid(t), time.Now()), true); err != nil {
		t.Fatal(err)
	}
	done, err := s.EraseAccountBatch(context.Background(), a, 3)
	if err != nil || done {
		t.Fatalf("first batch done=%v %v", done, err)
	}
	done, err = s.EraseAccountBatch(context.Background(), a, 3)
	if err != nil || !done || count(t, pool, "account_id=$1", a) != 0 || count(t, pool, "account_id=$1", b) != 1 {
		t.Fatalf("second batch done=%v %v", done, err)
	}
	if _, err := pool.Exec(context.Background(), "DELETE FROM accounts WHERE id=$1", b); err != nil {
		t.Fatal(err)
	}
	if count(t, pool, "account_id=$1", b) != 0 {
		t.Fatal("deleting the account row must cascade to its inbox")
	}
}
