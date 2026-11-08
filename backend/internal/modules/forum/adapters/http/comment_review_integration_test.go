//go:build integration

package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

func TestCommentReview_AdminQueuePublishAndReject(t *testing.T) {
	env := setupFOR13TestEnv(t)
	env.adminSvc.Comments = env.store
	ctx := context.Background()

	const (
		author = "11111111-2222-4333-8444-000000000001"
		post   = "99999999-1111-4999-8999-000000000001"
		c1     = "cccccccc-0001-4ccc-8ccc-000000000001"
		c2     = "cccccccc-0002-4ccc-8ccc-000000000002"
	)
	now := time.Now().UTC()
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := env.pool.Exec(ctx, q, args...); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	mustExec("INSERT INTO accounts(id,alias,status) VALUES($1,'reviewed-author','active')", author)
	mustExec(`INSERT INTO forum_posts(id, kind, category, title, excerpt, body_text, publication_state, visibility,
		first_publication_seq, source_id, comments_enabled, local_created_at, local_updated_at, first_published_at, next_comment_sequence)
		VALUES ($1, 'EDITORIAL', 'FUELS', 'Post com fila de revisão', 'Resumo', 'Corpo', 'PUBLISHED', 'VISIBLE',
		1, '00000000-0000-0000-0000-000000000001', true, $2, $2, $2, 3)`, post, now)
	for i, id := range []string{c1, c2} {
		mustExec(`INSERT INTO forum_comments(id, post_id, author_account_id, public_alias, depth, sequence, body_text,
			revision, publication_state, visibility, created_at, updated_at, create_operation_id, body_sha256)
			VALUES ($1, $2, $3, 'Motorista', 0, $4, $5, 1, 'PENDING_REVIEW', 'VISIBLE', $6, $6, $7, 'sha')`,
			id, post, author, i+1, "Comentário aguardando revisão", now.Add(time.Duration(i)*time.Second),
			"55555555-0000-4555-8555-00000000000"+string(rune('1'+i)))
	}

	serve := func(req *http.Request) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		env.adminRouter.ServeHTTP(w, req)
		return w
	}
	type queue struct {
		Items      []application.AdminCommentView `json:"items"`
		NextCursor *string                        `json:"next_cursor"`
	}
	list := func(path string) queue {
		t.Helper()
		w := serve(env.newAdminReq(http.MethodGet, path, nil, ""))
		if w.Code != http.StatusOK {
			t.Fatalf("list %s: %d %s", path, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), author) {
			t.Fatal("review queue leaked the author account id")
		}
		var q queue
		if err := json.Unmarshal(w.Body.Bytes(), &q); err != nil {
			t.Fatalf("decode queue: %v", err)
		}
		return q
	}

	// Oldest first, keyset paginated.
	page1 := list("/v1/admin/forum/comments?limit=1")
	if len(page1.Items) != 1 || page1.Items[0].ID != c1 || page1.NextCursor == nil {
		t.Fatalf("page1 = %+v", page1)
	}
	page2 := list("/v1/admin/forum/comments?limit=1&cursor=" + *page1.NextCursor)
	if len(page2.Items) != 1 || page2.Items[0].ID != c2 {
		t.Fatalf("page2 = %+v", page2)
	}
	if w := serve(env.newAdminReq(http.MethodGet, "/v1/admin/forum/comments?limit=51", nil, "")); w.Code != http.StatusBadRequest {
		t.Fatalf("limit 51: expected 400, got %d", w.Code)
	}

	review := func(id, verb string, body map[string]any, key string) *httptest.ResponseRecorder {
		return serve(env.newAdminReq(http.MethodPost, "/v1/admin/forum/comments/"+id+"/"+verb, body, key))
	}
	reason := "comentário revisado pela moderação editorial"
	if w := review(c1, "publish", map[string]any{"reason": reason}, "77777777-0000-4777-8777-000000000001"); w.Code != http.StatusBadRequest {
		t.Fatalf("missing revision: expected 400, got %d %s", w.Code, w.Body.String())
	}
	if w := review(c1, "publish", map[string]any{"expected_revision": 2, "reason": reason}, "77777777-0000-4777-8777-000000000002"); w.Code != http.StatusConflict {
		t.Fatalf("stale revision: expected 409, got %d %s", w.Code, w.Body.String())
	}
	w := review(c1, "publish", map[string]any{"expected_revision": 1, "reason": reason}, "77777777-0000-4777-8777-000000000003")
	if w.Code != http.StatusOK {
		t.Fatalf("publish: %d %s", w.Code, w.Body.String())
	}
	var published application.AdminCommentView
	if err := json.Unmarshal(w.Body.Bytes(), &published); err != nil {
		t.Fatalf("decode publish: %v", err)
	}
	if published.PublicationState != domain.StatePublished || published.Revision != 2 {
		t.Fatalf("published = %+v", published)
	}
	if w := review(c1, "reject", map[string]any{"expected_revision": 2, "reason": reason}, "77777777-0000-4777-8777-000000000004"); w.Code != http.StatusBadRequest {
		t.Fatalf("reviewing a published comment: expected 400, got %d", w.Code)
	}
	if w := review("cccccccc-0009-4ccc-8ccc-000000000009", "publish", map[string]any{"expected_revision": 1, "reason": reason}, "77777777-0000-4777-8777-000000000005"); w.Code != http.StatusNotFound {
		t.Fatalf("missing comment: expected 404, got %d", w.Code)
	}

	w = review(c2, "reject", map[string]any{"expected_revision": 1, "reason": reason}, "77777777-0000-4777-8777-000000000006")
	if w.Code != http.StatusOK {
		t.Fatalf("reject: %d %s", w.Code, w.Body.String())
	}
	var rejected application.AdminCommentView
	if err := json.Unmarshal(w.Body.Bytes(), &rejected); err != nil {
		t.Fatalf("decode reject: %v", err)
	}
	if rejected.PublicationState != domain.StateRejected || rejected.Visibility != domain.VisibilityHidden {
		t.Fatalf("rejected = %+v", rejected)
	}

	// The approved comment is public; the rejected one and the queue are gone.
	public, err := env.store.ListCommentsByPost(ctx, application.ListPostCommentsQuery{PostID: post, Limit: 10})
	if err != nil {
		t.Fatalf("public comments: %v", err)
	}
	if len(public) != 1 || public[0].ID != c1 {
		t.Fatalf("public comments = %+v", public)
	}
	if q := list("/v1/admin/forum/comments"); len(q.Items) != 0 {
		t.Fatalf("queue should be empty, got %d", len(q.Items))
	}
	var changes int
	if err := env.pool.QueryRow(ctx, "SELECT count(*) FROM forum_changes WHERE target_type='COMMENT' AND actor_kind='ADMIN'").Scan(&changes); err != nil || changes != 2 {
		t.Fatalf("expected 2 admin comment change records, got %d (%v)", changes, err)
	}
}
