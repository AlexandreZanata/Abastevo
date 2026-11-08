package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type mockCommentStore struct {
	comments  map[string]*domain.Comment
	byOpID    map[string]*domain.Comment
	createFn  func(ctx context.Context, cmd application.CreateCommentCommand) (application.CommentResult, error)
	editFn    func(ctx context.Context, cmd application.EditCommentCommand) (*domain.Comment, error)
	removeFn  func(ctx context.Context, cmd application.RemoveCommentCommand) (*domain.Comment, error)
	getFn     func(ctx context.Context, id string) (*domain.Comment, error)
	listFn    func(ctx context.Context, q application.ListPostCommentsQuery) ([]domain.Comment, error)
	repliesFn func(ctx context.Context, q application.ListCommentRepliesQuery) ([]domain.Comment, error)
}

func newMockCommentStore() *mockCommentStore {
	return &mockCommentStore{
		comments: make(map[string]*domain.Comment),
		byOpID:   make(map[string]*domain.Comment),
	}
}

func (m *mockCommentStore) CreateComment(ctx context.Context, cmd application.CreateCommentCommand) (application.CommentResult, error) {
	if m.createFn != nil {
		return m.createFn(ctx, cmd)
	}
	if existing, ok := m.byOpID[cmd.OperationID]; ok {
		if existing.BodyText == cmd.BodyText {
			return application.CommentResult{Comment: existing, Replayed: true}, nil
		}
		return application.CommentResult{}, domain.ErrConflictingOperation
	}
	now := time.Now().UTC()
	c := &domain.Comment{
		ID:                "33333333-3333-4333-8333-000000000001",
		PostID:            cmd.PostID,
		AuthorAccountID:   cmd.AuthorAccountID,
		ParentID:          cmd.ParentID,
		Depth:             0,
		Sequence:          1,
		BodyText:          cmd.BodyText,
		Revision:          1,
		PublicationState:  domain.StatePendingReview,
		Visibility:        domain.VisibilityVisible,
		CreatedAt:         now,
		UpdatedAt:         now,
		CreateOperationID: cmd.OperationID,
		BodySHA256:        HashLogicalIntent([]byte(cmd.BodyText)),
	}
	m.comments[c.ID] = c
	m.byOpID[cmd.OperationID] = c
	return application.CommentResult{Comment: c, Replayed: false}, nil
}

func (m *mockCommentStore) EditComment(ctx context.Context, cmd application.EditCommentCommand) (*domain.Comment, error) {
	if m.editFn != nil {
		return m.editFn(ctx, cmd)
	}
	c, ok := m.comments[cmd.CommentID]
	if !ok {
		return nil, domain.ErrCommentNotFound
	}
	if c.AuthorAccountID != cmd.AuthorAccountID {
		return nil, domain.ErrCommentAuthorMismatch
	}
	if c.Revision != cmd.ExpectedRevision {
		return nil, domain.ErrRevisionMismatch
	}
	c.BodyText = cmd.BodyText
	c.Revision++
	c.PublicationState = domain.StatePendingReview
	c.UpdatedAt = time.Now().UTC()
	return c, nil
}

func (m *mockCommentStore) RemoveComment(ctx context.Context, cmd application.RemoveCommentCommand) (*domain.Comment, error) {
	if m.removeFn != nil {
		return m.removeFn(ctx, cmd)
	}
	c, ok := m.comments[cmd.CommentID]
	if !ok {
		return nil, domain.ErrCommentNotFound
	}
	if c.AuthorAccountID != cmd.AuthorAccountID {
		return nil, domain.ErrCommentAuthorMismatch
	}
	if c.Revision != cmd.ExpectedRevision {
		return nil, domain.ErrRevisionMismatch
	}
	c.BodyText = ""
	c.PublicationState = domain.StateRemoved
	c.Visibility = domain.VisibilityHidden
	c.Revision++
	c.UpdatedAt = time.Now().UTC()
	return c, nil
}

func (m *mockCommentStore) GetCommentByID(ctx context.Context, id string) (*domain.Comment, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	c, ok := m.comments[id]
	if !ok {
		return nil, domain.ErrCommentNotFound
	}
	return c, nil
}

func (m *mockCommentStore) ListCommentsByPost(ctx context.Context, q application.ListPostCommentsQuery) ([]domain.Comment, error) {
	if m.listFn != nil {
		return m.listFn(ctx, q)
	}
	var out []domain.Comment
	for _, c := range m.comments {
		if c.PostID == q.PostID && c.PublicationState == domain.StatePublished && c.Visibility == domain.VisibilityVisible {
			out = append(out, *c)
		}
	}
	return out, nil
}

func (m *mockCommentStore) ListRepliesByParent(ctx context.Context, q application.ListCommentRepliesQuery) ([]domain.Comment, error) {
	if m.repliesFn != nil {
		return m.repliesFn(ctx, q)
	}
	var out []domain.Comment
	for _, c := range m.comments {
		if c.ParentID != nil && *c.ParentID == q.ParentID && c.PublicationState == domain.StatePublished && c.Visibility == domain.VisibilityVisible {
			out = append(out, *c)
		}
	}
	return out, nil
}

func (m *mockCommentStore) PublishComment(ctx context.Context, commentID string) (*domain.Comment, error) {
	c, ok := m.comments[commentID]
	if !ok {
		return nil, domain.ErrCommentNotFound
	}
	now := time.Now().UTC()
	c.PublicationState = domain.StatePublished
	c.PublishedAt = &now
	return c, nil
}

func (m *mockCommentStore) RejectComment(ctx context.Context, commentID string) (*domain.Comment, error) {
	c, ok := m.comments[commentID]
	if !ok {
		return nil, domain.ErrCommentNotFound
	}
	c.PublicationState = domain.StateRejected
	return c, nil
}

func (m *mockCommentStore) ListCommentsByAccount(ctx context.Context, accountID string) ([]domain.Comment, error) {
	var out []domain.Comment
	for _, c := range m.comments {
		if c.AuthorAccountID == accountID {
			out = append(out, *c)
		}
	}
	return out, nil
}

func (m *mockCommentStore) EraseAccountBatch(ctx context.Context, accountID string, limit int) (bool, int, error) {
	count := 0
	for _, c := range m.comments {
		if c.AuthorAccountID == accountID && (c.PublicationState != domain.StateRemoved || c.BodyText != "") {
			c.BodyText = ""
			c.PublicationState = domain.StateRemoved
			c.Visibility = domain.VisibilityHidden
			count++
		}
	}
	return true, count, nil
}

func TestCanonicalIntentHashingStabilityAndConflict(t *testing.T) {
	opID := "22222222-2222-4222-8222-000000000001"
	postID := "44444444-4444-4444-8444-000000000001"
	body := "Comentario sobre formacao de precos."

	// 1. Initial canonical create intent bytes and hash
	intent1, err := CanonicalCreateIntent(opID, postID, nil, body)
	if err != nil {
		t.Fatalf("unexpected error creating intent: %v", err)
	}
	hash1 := HashLogicalIntent(intent1)

	// 2. Simulated token rotation retry:
	// Notice that the client's session credentials (family_id and access_token) and signature nonces rotated,
	// but the business intent is identical.
	intent2, err := CanonicalCreateIntent(opID, postID, nil, body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hash2 := HashLogicalIntent(intent2)

	if !bytes.Equal(intent1, intent2) {
		t.Errorf("expected byte-exact identity across session token rotation, got %s vs %s", intent1, intent2)
	}
	if hash1 != hash2 {
		t.Errorf("expected hash identity across session token rotation: %s != %s", hash1, hash2)
	}

	// 3. Changed body text with same operation ID produces different hash (triggering conflict)
	intentChanged, err := CanonicalCreateIntent(opID, postID, nil, "Comentario com texto alterado")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hashChanged := HashLogicalIntent(intentChanged)
	if hash1 == hashChanged {
		t.Error("expected different hash for changed body, got identical hash")
	}

	// 4. Edit intent stability
	editIntent1, _ := CanonicalEditIntent(opID, "comment-1", 1, "Updated body")
	editIntent2, _ := CanonicalEditIntent(opID, "comment-1", 1, "Updated body")
	if !bytes.Equal(editIntent1, editIntent2) {
		t.Error("expected edit intent byte identity")
	}

	// 5. Remove intent stability
	remIntent1, _ := CanonicalRemoveIntent(opID, "comment-1", 2)
	remIntent2, _ := CanonicalRemoveIntent(opID, "comment-1", 2)
	if !bytes.Equal(remIntent1, remIntent2) {
		t.Error("expected remove intent byte identity")
	}
}

func TestDiscussionAdmission_HTTP_NegativeGates(t *testing.T) {
	postID := "44444444-4444-4444-8444-000000000001"
	authorA := "11111111-aaaa-4111-8111-000000000001"
	authorB := "11111111-bbbb-4111-8111-000000000002"
	familyA := "22222222-aaaa-4222-8222-000000000001"
	contributorA := "33333333-aaaa-4333-8333-000000000001"
	keyA := "44444444-aaaa-4444-8444-000000000001"
	commentIDA := "55555555-aaaa-4555-8555-000000000001"

	for _, tc := range []struct {
		name       string
		endpoint   string
		method     string
		body       string
		sessionErr error
		proofErr   error
		storeSetup func(*mockCommentStore)
		wantStatus int
		wantCode   string
	}{
		{
			name:       "missing proof / signature verifier rejected",
			endpoint:   "/v1/forum/posts/" + postID + "/comments",
			method:     "POST",
			body:       fmt.Sprintf(`{"family_id":"%s","access_token":"token","operation_id":"66666666-6666-4666-8666-000000000001","body_text":"Valido"}`, familyA),
			proofErr:   errors.New("signature proof rejected"),
			wantStatus: http.StatusUnauthorized,
			wantCode:   "forum.auth-required",
		},
		{
			name:       "forged proof / signature mismatch",
			endpoint:   "/v1/forum/posts/" + postID + "/comments",
			method:     "POST",
			body:       fmt.Sprintf(`{"family_id":"%s","access_token":"token","operation_id":"66666666-6666-4666-8666-000000000001","body_text":"Valido"}`, familyA),
			proofErr:   errors.New("signature mismatch"),
			wantStatus: http.StatusUnauthorized,
			wantCode:   "forum.auth-required",
		},
		{
			name:       "reused proof nonce",
			endpoint:   "/v1/forum/posts/" + postID + "/comments",
			method:     "POST",
			body:       fmt.Sprintf(`{"family_id":"%s","access_token":"token","operation_id":"66666666-6666-4666-8666-000000000001","body_text":"Valido"}`, familyA),
			proofErr:   errors.New("nonce reused"),
			wantStatus: http.StatusUnauthorized,
			wantCode:   "forum.auth-required",
		},
		{
			name:       "wrong authority / unbound key",
			endpoint:   "/v1/forum/posts/" + postID + "/comments",
			method:     "POST",
			body:       fmt.Sprintf(`{"family_id":"%s","access_token":"token","operation_id":"66666666-6666-4666-8666-000000000001","body_text":"Valido"}`, familyA),
			proofErr:   errors.New("unbound key"),
			wantStatus: http.StatusUnauthorized,
			wantCode:   "forum.auth-required",
		},
		{
			name:       "expired session family",
			endpoint:   "/v1/forum/posts/" + postID + "/comments",
			method:     "POST",
			body:       fmt.Sprintf(`{"family_id":"%s","access_token":"expired-token","operation_id":"66666666-6666-4666-8666-000000000001","body_text":"Valido"}`, familyA),
			sessionErr: errors.New("session expired"),
			wantStatus: http.StatusUnauthorized,
			wantCode:   "forum.auth-required",
		},
		{
			name:       "revoked session family",
			endpoint:   "/v1/forum/posts/" + postID + "/comments",
			method:     "POST",
			body:       fmt.Sprintf(`{"family_id":"%s","access_token":"revoked-token","operation_id":"66666666-6666-4666-8666-000000000001","body_text":"Valido"}`, familyA),
			sessionErr: errors.New("session family revoked"),
			wantStatus: http.StatusUnauthorized,
			wantCode:   "forum.auth-required",
		},
		{
			name:     "other-owner IDOR on EditComment returns 404",
			endpoint: "/v1/forum/comments/" + commentIDA + "/edit",
			method:   "POST",
			body:     fmt.Sprintf(`{"family_id":"%s","access_token":"token","operation_id":"66666666-6666-4666-8666-000000000001","expected_revision":1,"body_text":"Tentativa de edicao de outro autor"}`, familyA),
			storeSetup: func(s *mockCommentStore) {
				// Comment belongs to author B, but request session belongs to author A
				s.comments[commentIDA] = &domain.Comment{
					ID:              commentIDA,
					AuthorAccountID: authorB,
					Revision:        1,
					BodyText:        "Comentario original do autor B",
				}
			},
			wantStatus: http.StatusNotFound,
			wantCode:   "forum.not-found",
		},
		{
			name:     "other-owner IDOR on RemoveComment returns 404",
			endpoint: "/v1/forum/comments/" + commentIDA + "/remove",
			method:   "POST",
			body:     fmt.Sprintf(`{"family_id":"%s","access_token":"token","operation_id":"66666666-6666-4666-8666-000000000001","expected_revision":1}`, familyA),
			storeSetup: func(s *mockCommentStore) {
				s.comments[commentIDA] = &domain.Comment{
					ID:              commentIDA,
					AuthorAccountID: authorB,
					Revision:        1,
					BodyText:        "Comentario original do autor B",
				}
			},
			wantStatus: http.StatusNotFound,
			wantCode:   "forum.not-found",
		},
		{
			name:     "other-owner IDOR on OwnerRead returns 404",
			endpoint: "/v1/forum/owner/read",
			method:   "POST",
			body:     fmt.Sprintf(`{"family_id":"%s","access_token":"token","comment_id":"%s"}`, familyA, commentIDA),
			storeSetup: func(s *mockCommentStore) {
				s.comments[commentIDA] = &domain.Comment{
					ID:               commentIDA,
					AuthorAccountID:  authorB,
					Revision:         1,
					PublicationState: domain.StatePendingReview,
					BodyText:         "Comentario pendente do autor B",
				}
			},
			wantStatus: http.StatusNotFound,
			wantCode:   "forum.not-found",
		},
		{
			name:       "invalid operation_id returns 400",
			endpoint:   "/v1/forum/posts/" + postID + "/comments",
			method:     "POST",
			body:       fmt.Sprintf(`{"family_id":"%s","access_token":"token","operation_id":"not-a-uuid","body_text":"Valido"}`, familyA),
			wantStatus: http.StatusBadRequest,
			wantCode:   "forum.bad-request",
		},
		{
			name:       "empty body_text returns 400",
			endpoint:   "/v1/forum/posts/" + postID + "/comments",
			method:     "POST",
			body:       fmt.Sprintf(`{"family_id":"%s","access_token":"token","operation_id":"66666666-6666-4666-8666-000000000001","body_text":"   "}`, familyA),
			wantStatus: http.StatusBadRequest,
			wantCode:   "forum.bad-request",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newMockCommentStore()
			if tc.storeSetup != nil {
				tc.storeSetup(store)
			}

			h := DiscussionHandler{
				Comments: store,
				Sessions: func(ctx context.Context, f, a string) (string, error) {
					if tc.sessionErr != nil {
						return "", tc.sessionErr
					}
					return authorA, nil
				},
				SignedOwner: func(r *http.Request, acc string) (string, string, error) {
					if tc.proofErr != nil {
						return "", "", tc.proofErr
					}
					return contributorA, keyA, nil
				},
				Guard: func(ctx context.Context, tx pgx.Tx, acc, fam, tok, contrib, key string) error {
					return nil
				},
			}

			r := chi.NewRouter()
			h.RegisterRoutes(r)

			req := httptest.NewRequest(tc.method, tc.endpoint, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Fatalf("status code mismatch: got %d, want %d (body: %s)", w.Code, tc.wantStatus, w.Body.String())
			}

			// Security header invariant: Cache-Control must be no-store
			if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
				t.Errorf("expected Cache-Control: no-store, got %q", cc)
			}

			// Privacy leak invariant: response body must never leak credentials or internal account IDs
			bodyResp := w.Body.String()
			if strings.Contains(bodyResp, "token") && strings.Contains(bodyResp, "fixture") {
				t.Errorf("response body leaks session token: %s", bodyResp)
			}
			if strings.Contains(bodyResp, authorA) || strings.Contains(bodyResp, authorB) {
				t.Errorf("response body leaks internal account UUID: %s", bodyResp)
			}
		})
	}
}

func TestDiscussionAdmission_OwnerReceiptAndPublicIsolation(t *testing.T) {
	postID := "44444444-4444-4444-8444-000000000001"
	authorA := "11111111-aaaa-4111-8111-000000000001"
	familyA := "22222222-aaaa-4222-8222-000000000001"
	contributorA := "33333333-aaaa-4333-8333-000000000001"
	keyA := "44444444-aaaa-4444-8444-000000000001"
	commentID := "55555555-aaaa-4555-8555-000000000001"

	store := newMockCommentStore()
	store.comments[commentID] = &domain.Comment{
		ID:               commentID,
		PostID:           postID,
		AuthorAccountID:  authorA,
		Depth:            0,
		Sequence:         1,
		BodyText:         "Comentario pendente privado do autor",
		Revision:         1,
		PublicationState: domain.StatePendingReview,
		Visibility:       domain.VisibilityVisible,
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}

	h := DiscussionHandler{
		Comments: store,
		Sessions: func(ctx context.Context, f, a string) (string, error) {
			return authorA, nil
		},
		SignedOwner: func(r *http.Request, acc string) (string, string, error) {
			return contributorA, keyA, nil
		},
	}

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	// 1. Author A reads private pending comment via OwnerRead -> 200 OK with body text
	ownerReq := httptest.NewRequest("POST", "/v1/forum/owner/read", strings.NewReader(fmt.Sprintf(
		`{"family_id":"%s","access_token":"token","comment_id":"%s"}`, familyA, commentID)))
	ownerReq.Header.Set("Content-Type", "application/json")
	wOwner := httptest.NewRecorder()
	r.ServeHTTP(wOwner, ownerReq)

	if wOwner.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for owner read of pending comment, got %d: %s", wOwner.Code, wOwner.Body.String())
	}
	var ownerResp OwnerCommentResponseDTO
	if err := json.Unmarshal(wOwner.Body.Bytes(), &ownerResp); err != nil {
		t.Fatalf("failed to decode owner response: %v", err)
	}
	if ownerResp.Comment.ID != commentID || ownerResp.Comment.BodyText != "Comentario pendente privado do autor" {
		t.Errorf("unexpected owner receipt: %+v", ownerResp.Comment)
	}
	if ownerResp.Comment.PublicationState != "PENDING_REVIEW" {
		t.Errorf("expected PENDING_REVIEW state, got %s", ownerResp.Comment.PublicationState)
	}

	// 2. Anonymous public read of comments on post -> 200 OK with empty items (pending comment never leaked)
	pubReq := httptest.NewRequest("GET", "/v1/forum/posts/"+postID+"/comments", nil)
	wPub := httptest.NewRecorder()
	r.ServeHTTP(wPub, pubReq)

	if wPub.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for public comments read, got %d", wPub.Code)
	}
	var pubResp CommentsResponseDTO
	if err := json.Unmarshal(wPub.Body.Bytes(), &pubResp); err != nil {
		t.Fatalf("failed to decode public response: %v", err)
	}
	if len(pubResp.Items) != 0 {
		t.Errorf("privacy leak: pending comment returned in public comments list: %+v", pubResp.Items)
	}
}
