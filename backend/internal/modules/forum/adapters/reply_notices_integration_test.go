//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	notificationsadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/adapters"
	notificationsapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/application"
)

// replyNotices composes the real notifications producer exactly like the
// API composition root (cmd/api forumReplyNotices).
func replyNotices(pool *pgxpool.Pool) application.ReplyNotifier {
	producers := notificationsapp.Producers{Notifier: notificationsadapters.NewStore(pool), Clock: time.Now}
	return func(ctx context.Context, tx pgx.Tx, n application.ReplyNotice) error {
		return producers.ForumReply(ctx, tx, notificationsapp.ForumReplyFacts{
			TopicID: n.PostID, TopicTitle: n.PostTitle, TopicAuthorAccountID: n.PostAuthorAccountID,
			CommentID: n.CommentID, CommentAuthorAccountID: n.CommentAuthorAccountID, AuthorName: n.AuthorName,
		})
	}
}

func createTestTopic(t *testing.T, pool *pgxpool.Pool, id, authorID, title string) string {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO forum_posts (
			id, kind, category, title, body_text, author_account_id, public_alias,
			publication_state, visibility, first_publication_seq, first_published_at,
			comments_enabled, revision
		) VALUES ($1, 'TOPIC', 'FUELS', $2, 'Corpo do tópico', $3, 'Autor do tópico',
			'PUBLISHED', 'VISIBLE', 1, now(), true, 1)
	`, id, title, authorID); err != nil {
		t.Fatalf("createTestTopic: %v", err)
	}
	return id
}

func review(t *testing.T, store *Store, pool *pgxpool.Pool, commentID string, approve bool) error {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := store.ReviewCommentTx(ctx, tx, commentID, 0, approve, "test-review"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func comment(t *testing.T, store *Store, postID, authorID, alias, op string, parent *string) string {
	t.Helper()
	res, err := store.CreateComment(context.Background(), application.CreateCommentCommand{
		PostID: postID, AuthorAccountID: authorID, PublicAlias: alias, ParentID: parent,
		BodyText: "Resposta sintética", OperationID: op,
	})
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}
	return res.Comment.ID
}

func notices(t *testing.T, pool *pgxpool.Pool, accountID string) []map[string]any {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT kind, event_key, payload FROM account_notifications WHERE account_id=$1 ORDER BY created_at`, accountID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var kind, key string
		var raw []byte
		if err := rows.Scan(&kind, &key, &raw); err != nil {
			t.Fatal(err)
		}
		payload := map[string]any{}
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatal(err)
		}
		payload["$kind"], payload["$key"] = kind, key
		out = append(out, payload)
	}
	return out
}

func TestReplyNotices_FirstPublicationNotifiesTopicAuthorOnly(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	store.Replies = replyNotices(pool)
	author := createTestAccount(t, pool, "11111111-1111-4111-8111-00000000a001", "autor-topico")
	replier := createTestAccount(t, pool, "11111111-1111-4111-8111-00000000a002", "respondente")
	topic := createTestTopic(t, pool, "22222222-2222-4222-8222-00000000a001", author, "Etanol subiu em Cuiabá?")

	reply := comment(t, store, topic, replier, "Motorista MT", "33333333-3333-4333-8333-00000000a001", nil)
	if got := notices(t, pool, author); len(got) != 0 {
		t.Fatalf("a pending reply is not visible yet: %v", got)
	}
	if err := review(t, store, pool, reply, true); err != nil {
		t.Fatal(err)
	}
	got := notices(t, pool, author)
	if len(got) != 1 {
		t.Fatalf("topic author notices = %v", got)
	}
	want := map[string]any{
		"$kind": "forum.reply", "$key": "forum.reply:" + reply, "topic_id": topic,
		"topic_title": "Etanol subiu em Cuiabá?", "comment_id": reply, "author_name": "Motorista MT",
	}
	for k, v := range want {
		if got[0][k] != v {
			t.Fatalf("payload %s = %v, want %v (%v)", k, got[0][k], v, got[0])
		}
	}
	if len(got[0]) != len(want) {
		t.Fatalf("payload carries extra fields: %v", got[0])
	}
	if n := notices(t, pool, replier); len(n) != 0 {
		t.Fatalf("the replier was notified: %v", n)
	}

	// An owner edit returns the reply to review; the re-publication is silent.
	if _, err := store.EditComment(context.Background(), application.EditCommentCommand{
		CommentID: reply, AuthorAccountID: replier, ExpectedRevision: 2, BodyText: "Resposta editada",
	}); err != nil {
		t.Fatal(err)
	}
	if err := review(t, store, pool, reply, true); err != nil {
		t.Fatal(err)
	}
	if n := notices(t, pool, author); len(n) != 1 {
		t.Fatalf("re-publication after an edit notified again: %v", n)
	}
	// Even once the first notice expired and was purged, a later
	// re-publication is not a new reply.
	if _, err := pool.Exec(context.Background(), "DELETE FROM account_notifications"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EditComment(context.Background(), application.EditCommentCommand{
		CommentID: reply, AuthorAccountID: replier, ExpectedRevision: 4, BodyText: "Segunda edição",
	}); err != nil {
		t.Fatal(err)
	}
	if err := review(t, store, pool, reply, true); err != nil {
		t.Fatal(err)
	}
	if n := notices(t, pool, author); len(n) != 0 {
		t.Fatalf("a re-published reply counted as new: %v", n)
	}
}

func TestReplyNotices_ActorAndAuthorlessPostsAreNeverNotified(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	store.Replies = replyNotices(pool)
	author := createTestAccount(t, pool, "11111111-1111-4111-8111-00000000b001", "autor-b")
	replier := createTestAccount(t, pool, "11111111-1111-4111-8111-00000000b002", "respondente-b")
	topic := createTestTopic(t, pool, "22222222-2222-4222-8222-00000000b001", author, "Diesel S10 em Sorriso")

	own := comment(t, store, topic, author, "Autor do tópico", "33333333-3333-4333-8333-00000000b001", nil)
	if err := review(t, store, pool, own, true); err != nil {
		t.Fatal(err)
	}
	if n := notices(t, pool, author); len(n) != 0 {
		t.Fatalf("the topic author was notified about their own reply: %v", n)
	}

	editorial := createTestPost(t, pool, "22222222-2222-4222-8222-00000000b002", "Editorial sem autor", string(domain.StatePublished), string(domain.VisibilityVisible), true)
	onEditorial := comment(t, store, editorial, replier, "Motorista", "33333333-3333-4333-8333-00000000b002", nil)
	if err := review(t, store, pool, onEditorial, true); err != nil {
		t.Fatal(err)
	}
	var total int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM account_notifications").Scan(&total); err != nil || total != 0 {
		t.Fatalf("a post without an account author produced %d notices (%v)", total, err)
	}

	rejected := comment(t, store, topic, replier, "Motorista", "33333333-3333-4333-8333-00000000b003", nil)
	if err := review(t, store, pool, rejected, false); err != nil {
		t.Fatal(err)
	}
	if n := notices(t, pool, author); len(n) != 0 {
		t.Fatalf("a rejected reply notified: %v", n)
	}
}

func TestReplyNotices_InvisibleTargetsStaySilent(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	store.Replies = replyNotices(pool)
	author := createTestAccount(t, pool, "11111111-1111-4111-8111-00000000c001", "autor-c")
	replier := createTestAccount(t, pool, "11111111-1111-4111-8111-00000000c002", "respondente-c")
	topic := createTestTopic(t, pool, "22222222-2222-4222-8222-00000000c001", author, "GNV em Campinas")

	parent := comment(t, store, topic, replier, "Pai", "33333333-3333-4333-8333-00000000c001", nil)
	if err := review(t, store, pool, parent, true); err != nil {
		t.Fatal(err)
	}
	answer := comment(t, store, topic, replier, "Resposta", "33333333-3333-4333-8333-00000000c002", &parent)
	if _, err := pool.Exec(context.Background(), "UPDATE forum_comments SET visibility='HIDDEN' WHERE id=$1", parent); err != nil {
		t.Fatal(err)
	}
	if err := review(t, store, pool, answer, true); err != nil {
		t.Fatal(err)
	}
	if n := notices(t, pool, author); len(n) != 1 {
		t.Fatalf("an answer under a hidden parent must stay silent: %v", n)
	}

	late := comment(t, store, topic, replier, "Tarde", "33333333-3333-4333-8333-00000000c003", nil)
	if _, err := pool.Exec(context.Background(), "UPDATE forum_posts SET visibility='HIDDEN' WHERE id=$1", topic); err != nil {
		t.Fatal(err)
	}
	if err := review(t, store, pool, late, true); err != nil {
		t.Fatal(err)
	}
	if n := notices(t, pool, author); len(n) != 1 {
		t.Fatalf("a reply on a hidden topic must stay silent: %v", n)
	}
}

func TestReplyNotices_NotifierFailureRollsPublicationBack(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	author := createTestAccount(t, pool, "11111111-1111-4111-8111-00000000d001", "autor-d")
	replier := createTestAccount(t, pool, "11111111-1111-4111-8111-00000000d002", "respondente-d")
	topic := createTestTopic(t, pool, "22222222-2222-4222-8222-00000000d001", author, "Gasolina aditivada")
	reply := comment(t, store, topic, replier, "Motorista", "33333333-3333-4333-8333-00000000d001", nil)
	boom := errors.New("inbox unavailable")
	store.Replies = func(context.Context, pgx.Tx, application.ReplyNotice) error { return boom }
	if err := review(t, store, pool, reply, true); !errors.Is(err, boom) {
		t.Fatalf("review error = %v", err)
	}
	var state string
	if err := pool.QueryRow(context.Background(), "SELECT publication_state FROM forum_comments WHERE id=$1", reply).Scan(&state); err != nil || state != string(domain.StatePendingReview) {
		t.Fatalf("publication must roll back with its notice: %s %v", state, err)
	}
	store.Replies = replyNotices(pool)
	if err := review(t, store, pool, reply, true); err != nil {
		t.Fatal(err)
	}
	if n := notices(t, pool, author); len(n) != 1 {
		t.Fatalf("retry after rollback notices = %v", n)
	}
}
