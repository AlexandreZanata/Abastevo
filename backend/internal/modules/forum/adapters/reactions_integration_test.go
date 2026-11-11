//go:build integration

package adapters

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

func reactionUUID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// publishedComment creates and publishes a comment (or reply) through the store.
func publishedComment(t *testing.T, store *Store, postID, authorID string, parentID *string, body string) *domain.Comment {
	t.Helper()
	ctx := context.Background()
	res, err := store.CreateComment(ctx, application.CreateCommentCommand{
		PostID: postID, AuthorAccountID: authorID, ParentID: parentID,
		BodyText: body, OperationID: reactionUUID(t), PublicAlias: "Motorista",
	})
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}
	c, err := store.PublishComment(ctx, res.Comment.ID)
	if err != nil {
		t.Fatalf("publish comment: %v", err)
	}
	return c
}

func react(t *testing.T, store *Store, target domain.ReactionTargetType, targetID, accountID string, r domain.Reaction) application.ReactionResult {
	t.Helper()
	res, err := store.SetReaction(context.Background(), application.SetReactionCommand{
		TargetType: target, TargetID: targetID, AccountID: accountID, Reaction: r,
	})
	if err != nil {
		t.Fatalf("set reaction %v on %s: %v", r, targetID, err)
	}
	return res
}

func assertCounts(t *testing.T, got domain.ReactionCounts, likes, dislikes int64) {
	t.Helper()
	if got.Likes != likes || got.Dislikes != dislikes {
		t.Fatalf("counts = %d/%d; want %d/%d", got.Likes, got.Dislikes, likes, dislikes)
	}
}

func reactionFixture(t *testing.T) (*Store, *pgxpool.Pool, string, string) {
	t.Helper()
	store, pool, _ := freshForumStore(t)
	author := createTestAccount(t, pool, reactionUUID(t), "autor")
	post := createTestPost(t, pool, reactionUUID(t), "Etanol ou gasolina", string(domain.StatePublished), string(domain.VisibilityVisible), true)
	return store, pool, author, post
}

func TestReactions_SetSwitchAndClear(t *testing.T) {
	store, pool, author, post := reactionFixture(t)
	reader := createTestAccount(t, pool, reactionUUID(t), "leitor")
	comment := publishedComment(t, store, post, author, nil, "Etanol compensa aqui.")

	assertCounts(t, react(t, store, domain.ReactionTargetComment, comment.ID, reader, domain.ReactionLike).Counts, 1, 0)
	// Repeating the same value is a no-op.
	assertCounts(t, react(t, store, domain.ReactionTargetComment, comment.ID, reader, domain.ReactionLike).Counts, 1, 0)
	// Switching moves the vote instead of adding a second one.
	assertCounts(t, react(t, store, domain.ReactionTargetComment, comment.ID, reader, domain.ReactionDislike).Counts, 0, 1)
	assertCounts(t, react(t, store, domain.ReactionTargetComment, comment.ID, author, domain.ReactionLike).Counts, 1, 1)
	assertCounts(t, react(t, store, domain.ReactionTargetComment, comment.ID, reader, domain.ReactionNone).Counts, 1, 0)
	assertCounts(t, react(t, store, domain.ReactionTargetComment, comment.ID, reader, domain.ReactionNone).Counts, 1, 0)

	res := react(t, store, domain.ReactionTargetPost, post, reader, domain.ReactionLike)
	assertCounts(t, res.Counts, 1, 0)
	if res.PostID != post {
		t.Fatalf("post reaction PostID = %q; want %q", res.PostID, post)
	}
	assertCounts(t, react(t, store, domain.ReactionTargetPost, post, reader, domain.ReactionDislike).Counts, 0, 1)

	detail, err := PublicPosts{store}.GetPostByID(context.Background(), post)
	if err != nil {
		t.Fatalf("get public post: %v", err)
	}
	if detail.LikeCount != 0 || detail.DislikeCount != 1 {
		t.Fatalf("detail counts = %d/%d; want 0/1", detail.LikeCount, detail.DislikeCount)
	}
}

func TestReactions_RefuseNonPublicTargets(t *testing.T) {
	store, pool, author, post := reactionFixture(t)
	ctx := context.Background()
	reader := createTestAccount(t, pool, reactionUUID(t), "leitor")

	pending, err := store.CreateComment(ctx, application.CreateCommentCommand{
		PostID: post, AuthorAccountID: author, BodyText: "Em revisão", OperationID: reactionUUID(t), PublicAlias: "A",
	})
	if err != nil {
		t.Fatalf("create pending: %v", err)
	}
	_, err = store.SetReaction(ctx, application.SetReactionCommand{
		TargetType: domain.ReactionTargetComment, TargetID: pending.Comment.ID, AccountID: reader, Reaction: domain.ReactionLike,
	})
	if !errors.Is(err, domain.ErrCommentNotFound) {
		t.Fatalf("pending comment reaction error = %v; want ErrCommentNotFound", err)
	}

	visible := publishedComment(t, store, post, author, nil, "Visível")
	hiddenPost := createTestPost(t, pool, reactionUUID(t), "Oculto", string(domain.StatePublished), string(domain.VisibilityHidden), true)
	_, err = store.SetReaction(ctx, application.SetReactionCommand{
		TargetType: domain.ReactionTargetPost, TargetID: hiddenPost, AccountID: reader, Reaction: domain.ReactionLike,
	})
	if !errors.Is(err, domain.ErrPostNotFound) {
		t.Fatalf("hidden post reaction error = %v; want ErrPostNotFound", err)
	}

	// Hiding the post takes its comments out of reach too.
	if _, err := pool.Exec(ctx, `UPDATE forum_posts SET visibility = 'HIDDEN' WHERE id = $1`, post); err != nil {
		t.Fatalf("hide post: %v", err)
	}
	_, err = store.SetReaction(ctx, application.SetReactionCommand{
		TargetType: domain.ReactionTargetComment, TargetID: visible.ID, AccountID: reader, Reaction: domain.ReactionLike,
	})
	if !errors.Is(err, domain.ErrCommentNotFound) {
		t.Fatalf("comment on hidden post error = %v; want ErrCommentNotFound", err)
	}
	_, err = store.SetReaction(ctx, application.SetReactionCommand{
		TargetType: domain.ReactionTargetComment, TargetID: reactionUUID(t), AccountID: reader, Reaction: domain.ReactionLike,
	})
	if !errors.Is(err, domain.ErrCommentNotFound) {
		t.Fatalf("missing comment error = %v; want ErrCommentNotFound", err)
	}
}

func TestReactions_ConcurrentWritersKeepExactCounts(t *testing.T) {
	store, pool, author, post := reactionFixture(t)
	ctx := context.Background()
	comment := publishedComment(t, store, post, author, nil, "Disputa")

	const readers = 24
	accounts := make([]string, readers)
	for i := range accounts {
		accounts[i] = createTestAccount(t, pool, reactionUUID(t), fmt.Sprintf("leitor-%d", i))
	}

	var wg sync.WaitGroup
	errs := make(chan error, readers*8)
	for i, account := range accounts {
		wg.Add(1)
		go func(i int, account string) {
			defer wg.Done()
			// Each account flips several times; the last value is LIKE for even, DISLIKE for odd.
			sequence := []domain.Reaction{domain.ReactionLike, domain.ReactionDislike, domain.ReactionNone, domain.ReactionLike}
			if i%2 == 1 {
				sequence = append(sequence, domain.ReactionDislike)
			}
			for _, r := range sequence {
				if _, err := store.SetReaction(ctx, application.SetReactionCommand{
					TargetType: domain.ReactionTargetComment, TargetID: comment.ID, AccountID: account, Reaction: r,
				}); err != nil {
					errs <- err
				}
			}
		}(i, account)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent reaction: %v", err)
	}

	var likes, dislikes, rowLikes, rowDislikes int64
	if err := pool.QueryRow(ctx, `SELECT like_count, dislike_count FROM forum_comments WHERE id = $1`, comment.ID).Scan(&likes, &dislikes); err != nil {
		t.Fatalf("read counts: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE value = 1), count(*) FILTER (WHERE value = -1)
		FROM forum_comment_reactions WHERE comment_id = $1`, comment.ID).Scan(&rowLikes, &rowDislikes); err != nil {
		t.Fatalf("aggregate reactions: %v", err)
	}
	if likes != readers/2 || dislikes != readers/2 || likes != rowLikes || dislikes != rowDislikes {
		t.Fatalf("counts %d/%d, rows %d/%d; want %d/%d", likes, dislikes, rowLikes, rowDislikes, readers/2, readers/2)
	}
}

func TestReactions_AccountErasureRemovesEffect(t *testing.T) {
	store, pool, author, post := reactionFixture(t)
	ctx := context.Background()
	reader := createTestAccount(t, pool, reactionUUID(t), "leitor")
	comment := publishedComment(t, store, post, author, nil, "Obrigado")
	react(t, store, domain.ReactionTargetComment, comment.ID, reader, domain.ReactionLike)
	react(t, store, domain.ReactionTargetPost, post, reader, domain.ReactionDislike)

	if _, err := pool.Exec(ctx, `UPDATE accounts SET status = 'deleted' WHERE id = $1`, reader); err != nil {
		t.Fatalf("erase account: %v", err)
	}
	var commentLikes, postDislikes, rows int64
	if err := pool.QueryRow(ctx, `SELECT like_count FROM forum_comments WHERE id = $1`, comment.ID).Scan(&commentLikes); err != nil {
		t.Fatalf("comment counts: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT dislike_count FROM forum_posts WHERE id = $1`, post).Scan(&postDislikes); err != nil {
		t.Fatalf("post counts: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM forum_comment_reactions WHERE account_id = $1)
		     + (SELECT count(*) FROM forum_post_reactions WHERE account_id = $1)`, reader).Scan(&rows); err != nil {
		t.Fatalf("reaction rows: %v", err)
	}
	if commentLikes != 0 || postDislikes != 0 || rows != 0 {
		t.Fatalf("after erasure: comment likes %d, post dislikes %d, rows %d; want 0", commentLikes, postDislikes, rows)
	}
}

func TestReactions_OwnReadAndExport(t *testing.T) {
	store, pool, author, post := reactionFixture(t)
	ctx := context.Background()
	reader := createTestAccount(t, pool, reactionUUID(t), "leitor")
	first := publishedComment(t, store, post, author, nil, "Um")
	second := publishedComment(t, store, post, author, nil, "Dois")
	react(t, store, domain.ReactionTargetComment, first.ID, reader, domain.ReactionLike)
	react(t, store, domain.ReactionTargetComment, second.ID, reader, domain.ReactionDislike)
	react(t, store, domain.ReactionTargetPost, post, reader, domain.ReactionLike)

	own, err := store.OwnReactions(ctx, reader, post)
	if err != nil {
		t.Fatalf("own reactions: %v", err)
	}
	if own.Post != domain.ReactionLike || own.Comments[first.ID] != domain.ReactionLike ||
		own.Comments[second.ID] != domain.ReactionDislike || len(own.Comments) != 2 {
		t.Fatalf("own reactions = %+v", own)
	}
	other, err := store.OwnReactions(ctx, author, post)
	if err != nil || other.Post != domain.ReactionNone || len(other.Comments) != 0 {
		t.Fatalf("author sees reader reactions: %+v, %v", other, err)
	}
	export, err := store.ListReactionsByAccount(ctx, reader)
	if err != nil || len(export) != 3 {
		t.Fatalf("export = %+v, %v; want 3 records", export, err)
	}
}

func TestComments_RankedListingKeysetAndTopReplies(t *testing.T) {
	store, pool, author, post := reactionFixture(t)
	ctx := context.Background()
	voters := make([]string, 4)
	for i := range voters {
		voters[i] = createTestAccount(t, pool, reactionUUID(t), fmt.Sprintf("v%d", i))
	}
	// Sequence order: a, b, c, d. Likes: a 1, b 3, c 3 (and 2 dislikes), d 0.
	a := publishedComment(t, store, post, author, nil, "A")
	b := publishedComment(t, store, post, author, nil, "B")
	c := publishedComment(t, store, post, author, nil, "C")
	d := publishedComment(t, store, post, author, nil, "D")
	react(t, store, domain.ReactionTargetComment, a.ID, voters[0], domain.ReactionLike)
	for _, v := range voters[:3] {
		react(t, store, domain.ReactionTargetComment, b.ID, v, domain.ReactionLike)
		react(t, store, domain.ReactionTargetComment, c.ID, v, domain.ReactionLike)
	}
	_ = d
	// c gets 2 extra dislikes from accounts that did not like it.
	extra := []string{createTestAccount(t, pool, reactionUUID(t), "x1"), createTestAccount(t, pool, reactionUUID(t), "x2")}
	for _, v := range extra {
		react(t, store, domain.ReactionTargetComment, c.ID, v, domain.ReactionDislike)
	}

	replyLow := publishedComment(t, store, post, author, &b.ID, "resposta fraca")
	replyHigh := publishedComment(t, store, post, author, &b.ID, "resposta forte")
	react(t, store, domain.ReactionTargetComment, replyHigh.ID, voters[0], domain.ReactionLike)
	_ = replyLow

	expect := map[domain.ReplySort][]string{
		domain.ReplySortOldest:   {a.ID, b.ID, c.ID, d.ID},
		domain.ReplySortRecent:   {d.ID, c.ID, b.ID, a.ID},
		domain.ReplySortLiked:    {c.ID, b.ID, a.ID, d.ID}, // likes 3,3 -> newest first
		domain.ReplySortRelevant: {b.ID, c.ID, a.ID, d.ID}, // net 3,1,1,0 -> c newer than a
	}
	for sort, want := range expect {
		// Page size 1 walks the whole keyset; no duplicates, no gaps.
		var got []string
		var cursorSeq, cursorScore *int64
		for i := 0; i < 10; i++ {
			page, err := store.ListCommentsByPost(ctx, application.ListPostCommentsQuery{
				PostID: post, Limit: 1, Sort: sort, CursorSequence: cursorSeq, CursorScore: cursorScore, WithTopReplies: true,
			})
			if err != nil {
				t.Fatalf("%s page: %v", sort, err)
			}
			if len(page) == 0 {
				break
			}
			got = append(got, page[0].ID)
			cursor, err := domain.ParseReplyCursor(sort, domain.FormatReplyCursor(sort, page[0]))
			if err != nil {
				t.Fatalf("%s cursor: %v", sort, err)
			}
			cursorSeq, cursorScore = &cursor.Sequence, cursor.Score
			if page[0].ID == b.ID && (page[0].TopReply == nil || page[0].TopReply.ID != replyHigh.ID) {
				t.Fatalf("%s: top reply of b = %+v; want the most liked reply", sort, page[0].TopReply)
			}
			if page[0].ID == b.ID && page[0].ReplyCount != 2 {
				t.Fatalf("%s: reply count of b = %d; want 2", sort, page[0].ReplyCount)
			}
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("%s order = %v; want %v", sort, got, want)
		}
	}
}

type forumEvent struct {
	Type     string  `json:"type"`
	Post     string  `json:"post"`
	Comment  string  `json:"comment"`
	Parent   *string `json:"parent"`
	Likes    int64   `json:"likes"`
	Dislikes int64   `json:"dislikes"`
}

func TestForumEvents_NotifyOnlyCommittedPublicChanges(t *testing.T) {
	store, pool, author, post := reactionFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	reader := createTestAccount(t, pool, reactionUUID(t), "leitor")

	listener, err := pgx.Connect(ctx, pool.Config().ConnString())
	if err != nil {
		t.Fatalf("listener connect: %v", err)
	}
	defer listener.Close(context.Background())
	if _, err := listener.Exec(ctx, "LISTEN forum_events"); err != nil {
		t.Fatalf("listen: %v", err)
	}
	next := func() forumEvent {
		t.Helper()
		wait, stop := context.WithTimeout(ctx, 5*time.Second)
		defer stop()
		n, err := listener.WaitForNotification(wait)
		if err != nil {
			t.Fatalf("wait notification: %v", err)
		}
		var ev forumEvent
		if err := json.Unmarshal([]byte(n.Payload), &ev); err != nil {
			t.Fatalf("payload %q: %v", n.Payload, err)
		}
		return ev
	}

	// A pending comment is not public: nothing is announced until publication.
	res, err := store.CreateComment(ctx, application.CreateCommentCommand{
		PostID: post, AuthorAccountID: author, BodyText: "Novo", OperationID: reactionUUID(t), PublicAlias: "A",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := store.PublishComment(ctx, res.Comment.ID); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if ev := next(); ev.Type != "comment" || ev.Post != post || ev.Comment != res.Comment.ID {
		t.Fatalf("publish event = %+v", ev)
	}

	// A rolled back reaction announces nothing; the next event is the committed one.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO forum_comment_reactions (comment_id, account_id, post_id, value) VALUES ($1, $2, $3, 1)`,
		res.Comment.ID, author, post); err != nil {
		t.Fatalf("insert in tx: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	react(t, store, domain.ReactionTargetComment, res.Comment.ID, reader, domain.ReactionDislike)
	if ev := next(); ev.Type != "comment_reactions" || ev.Likes != 0 || ev.Dislikes != 1 {
		t.Fatalf("reaction event = %+v", ev)
	}

	react(t, store, domain.ReactionTargetPost, post, reader, domain.ReactionLike)
	if ev := next(); ev.Type != "post_reactions" || ev.Post != post || ev.Likes != 1 {
		t.Fatalf("post reaction event = %+v", ev)
	}

	if _, err := pool.Exec(ctx, `UPDATE forum_comments SET visibility = 'HIDDEN' WHERE id = $1`, res.Comment.ID); err != nil {
		t.Fatalf("hide: %v", err)
	}
	if ev := next(); ev.Type != "comment_gone" || ev.Comment != res.Comment.ID {
		t.Fatalf("hide event = %+v", ev)
	}
}
