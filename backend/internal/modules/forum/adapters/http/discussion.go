package http

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	identityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	identitydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// SessionEnvelope carries client session credentials in request payloads.
type SessionEnvelope struct {
	FamilyID    string `json:"family_id"`
	AccessToken string `json:"access_token"`
}

// CreateCommentRequest is the wire request body to submit a comment or reply.
type CreateCommentRequest struct {
	SessionEnvelope
	OperationID string  `json:"operation_id"`
	ParentID    *string `json:"parent_id,omitempty"`
	BodyText    string  `json:"body_text"`
}

// EditCommentRequest is the wire request body to CAS-edit an authored comment.
type EditCommentRequest struct {
	SessionEnvelope
	OperationID      string `json:"operation_id"`
	ExpectedRevision int    `json:"expected_revision"`
	BodyText         string `json:"body_text"`
}

// RemoveCommentRequest is the wire request body to tombstone an authored comment.
type RemoveCommentRequest struct {
	SessionEnvelope
	OperationID      string `json:"operation_id"`
	ExpectedRevision int    `json:"expected_revision"`
}

// OwnerReadRequest is the wire request body to read private authored comments.
type OwnerReadRequest struct {
	SessionEnvelope
	CommentID *string `json:"comment_id,omitempty"`
	PostID    *string `json:"post_id,omitempty"`
}

// CommentReceiptDTO represents the private owner projection returned upon
// command execution, idempotent replay, or private owner read.
type CommentReceiptDTO struct {
	ID               string  `json:"id"`
	PostID           string  `json:"post_id"`
	ParentID         *string `json:"parent_id,omitempty"`
	Depth            int16   `json:"depth"`
	Sequence         int64   `json:"sequence"`
	Revision         int     `json:"revision"`
	PublicationState string  `json:"publication_state"`
	Visibility       string  `json:"visibility"`
	BodyText         string  `json:"body_text"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
	PublishedAt      *string `json:"published_at,omitempty"`
	Replayed         bool    `json:"replayed"`
}

// OwnerCommentResponseDTO wraps a single private owner comment receipt.
type OwnerCommentResponseDTO struct {
	Comment CommentReceiptDTO `json:"comment"`
}

// CommentItemDTO represents a public approved comment item.
type CommentItemDTO struct {
	ID          string  `json:"id"`
	PostID      string  `json:"post_id"`
	ParentID    *string `json:"parent_id,omitempty"`
	Depth       int16   `json:"depth"`
	Sequence    int64   `json:"sequence"`
	Revision    int     `json:"revision"`
	PublicAlias string  `json:"public_alias"`
	BodyText    string  `json:"body_text"`
	CreatedAt   string  `json:"created_at"`
	ReplyCount  int     `json:"reply_count"`
}

// CommentsResponseDTO is the envelope for top-level post comments.
type CommentsResponseDTO struct {
	Items      []CommentItemDTO `json:"items"`
	NextCursor *string          `json:"next_cursor,omitempty"`
}

// RepliesResponseDTO is the envelope for single-level replies.
type RepliesResponseDTO struct {
	Items      []CommentItemDTO `json:"items"`
	NextCursor *string          `json:"next_cursor,omitempty"`
}

// Canonical intent structures for stable logical intent hashing.
// Rotating credentials (family_id, access_token) and signature nonces/headers
// are strictly excluded, so session refresh prior to retry yields an identical hash.

type canonicalCreatePayload struct {
	Action      string  `json:"action"`
	OperationID string  `json:"operation_id"`
	PostID      string  `json:"post_id"`
	ParentID    *string `json:"parent_id,omitempty"`
	BodyText    string  `json:"body_text"`
}

type canonicalEditPayload struct {
	Action           string `json:"action"`
	OperationID      string `json:"operation_id"`
	CommentID        string `json:"comment_id"`
	ExpectedRevision int    `json:"expected_revision"`
	BodyText         string `json:"body_text"`
}

type canonicalRemovePayload struct {
	Action           string `json:"action"`
	OperationID      string `json:"operation_id"`
	CommentID        string `json:"comment_id"`
	ExpectedRevision int    `json:"expected_revision"`
}

type canonicalOwnerReadPayload struct {
	Action    string  `json:"action"`
	AccountID string  `json:"account_id"`
	CommentID *string `json:"comment_id,omitempty"`
	PostID    *string `json:"post_id,omitempty"`
}

// CanonicalCreateIntent serializes the stable logical intent of a create comment command.
func CanonicalCreateIntent(operationID, postID string, parentID *string, bodyText string) ([]byte, error) {
	return json.Marshal(canonicalCreatePayload{
		Action:      "create_comment",
		OperationID: strings.TrimSpace(operationID),
		PostID:      strings.TrimSpace(postID),
		ParentID:    parentID,
		BodyText:    bodyText,
	})
}

// CanonicalEditIntent serializes the stable logical intent of an edit comment command.
func CanonicalEditIntent(operationID, commentID string, expectedRevision int, bodyText string) ([]byte, error) {
	return json.Marshal(canonicalEditPayload{
		Action:           "edit_comment",
		OperationID:      strings.TrimSpace(operationID),
		CommentID:        strings.TrimSpace(commentID),
		ExpectedRevision: expectedRevision,
		BodyText:         bodyText,
	})
}

// CanonicalRemoveIntent serializes the stable logical intent of a remove comment command.
func CanonicalRemoveIntent(operationID, commentID string, expectedRevision int) ([]byte, error) {
	return json.Marshal(canonicalRemovePayload{
		Action:           "remove_comment",
		OperationID:      strings.TrimSpace(operationID),
		CommentID:        strings.TrimSpace(commentID),
		ExpectedRevision: expectedRevision,
	})
}

// CanonicalOwnerReadIntent serializes the stable logical intent of a private owner read.
func CanonicalOwnerReadIntent(accountID string, commentID, postID *string) ([]byte, error) {
	return json.Marshal(canonicalOwnerReadPayload{
		Action:    "owner_read",
		AccountID: strings.TrimSpace(accountID),
		CommentID: commentID,
		PostID:    postID,
	})
}

// HashLogicalIntent computes SHA-256 hex digest of canonical logical intent bytes.
func HashLogicalIntent(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// DiscussionHandler implements signed owner admission and discussion endpoints.
type DiscussionHandler struct {
	Comments    application.CommentStore
	Sessions    func(ctx context.Context, familyID, accessToken string) (accountID string, err error)
	SignedOwner func(r *http.Request, accountID string) (contributorID string, keyID string, err error)
	Guard       func(ctx context.Context, tx pgx.Tx, accountID, familyID, accessToken, contributorID, keyID string) error
	Runner      *identityadapters.Runner
	Clock       func() time.Time
}

func (h *DiscussionHandler) now() time.Time {
	if h.Clock != nil {
		return h.Clock().UTC()
	}
	return time.Now().UTC()
}

// RegisterRoutes registers discussion endpoints on a chi router.
func (h *DiscussionHandler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/forum/posts/{post_id}/comments", h.CreateComment)
	r.Get("/v1/forum/posts/{post_id}/comments", h.ListComments)
	r.Post("/v1/forum/comments/{comment_id}/edit", h.EditComment)
	r.Post("/v1/forum/comments/{comment_id}/remove", h.RemoveComment)
	r.Get("/v1/forum/comments/{comment_id}/replies", h.ListReplies)
	r.Post("/v1/forum/owner/read", h.OwnerRead)
}

func readJSONBody(r *http.Request, v any) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return domain.ErrCommentBodyEmpty
	}
	bodyBytes, err := httpapi.ReadBody(r, 16<<10)
	if err != nil || httpapi.DecodeJSON(bodyBytes, v) != nil {
		return domain.ErrCommentBodyEmpty
	}
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	r.ContentLength = int64(len(bodyBytes))
	return nil
}

type discussionPrincipal struct {
	AccountID     string
	ContributorID string
	KeyID         string
	FamilyID      string
	AccessToken   string
}

func (h *DiscussionHandler) authenticatePrincipal(r *http.Request, env SessionEnvelope) (discussionPrincipal, int, string, string) {
	if h.Sessions == nil || h.SignedOwner == nil {
		return discussionPrincipal{}, http.StatusUnauthorized, "forum.auth-required", "authentication required"
	}
	if !isUUID(env.FamilyID) || env.AccessToken == "" || len(env.AccessToken) > 512 {
		return discussionPrincipal{}, http.StatusUnauthorized, "forum.auth-required", "invalid session credentials"
	}

	accountID, err := h.Sessions(r.Context(), env.FamilyID, env.AccessToken)
	if err != nil {
		return discussionPrincipal{}, http.StatusUnauthorized, "forum.auth-required", "session expired or invalid"
	}
	if !isUUID(accountID) {
		return discussionPrincipal{}, http.StatusUnauthorized, "forum.auth-required", "invalid account authority"
	}

	contributorID, keyID, err := h.SignedOwner(r, accountID)
	if err != nil {
		return discussionPrincipal{}, http.StatusUnauthorized, "forum.auth-required", "request signature proof rejected or unbound"
	}
	if !isUUID(contributorID) || !isUUID(keyID) {
		return discussionPrincipal{}, http.StatusUnauthorized, "forum.auth-required", "unbound signing key"
	}

	return discussionPrincipal{
		AccountID:     accountID,
		ContributorID: contributorID,
		KeyID:         keyID,
		FamilyID:      env.FamilyID,
		AccessToken:   env.AccessToken,
	}, 0, "", ""
}

func (h *DiscussionHandler) makeTxGuard(p discussionPrincipal) func(context.Context, pgx.Tx) error {
	if h.Guard == nil {
		return nil
	}
	return func(ctx context.Context, tx pgx.Tx) error {
		return h.Guard(ctx, tx, p.AccountID, p.FamilyID, p.AccessToken, p.ContributorID, p.KeyID)
	}
}

func mapCommentToReceiptDTO(c *domain.Comment, replayed bool) CommentReceiptDTO {
	var pubAt *string
	if c.PublishedAt != nil {
		s := c.PublishedAt.UTC().Format(time.RFC3339)
		pubAt = &s
	}
	return CommentReceiptDTO{
		ID:               c.ID,
		PostID:           c.PostID,
		ParentID:         c.ParentID,
		Depth:            c.Depth,
		Sequence:         c.Sequence,
		Revision:         c.Revision,
		PublicationState: string(c.PublicationState),
		Visibility:       string(c.Visibility),
		BodyText:         c.BodyText,
		CreatedAt:        c.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:        c.UpdatedAt.UTC().Format(time.RFC3339),
		PublishedAt:      pubAt,
		Replayed:         replayed,
	}
}

func (h *DiscussionHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusServiceUnavailable
	code := "forum.service-unavailable"
	msg := "service unavailable"

	var quotaErr *identityadapters.QuotaError
	switch {
	case errors.As(err, &quotaErr):
		status = http.StatusTooManyRequests
		code = "forum.rate-limited"
		msg = "comment rate quota exceeded"
		if quotaErr.RetryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(quotaErr.RetryAfter.Seconds())))
		}
	case errors.Is(err, domain.ErrCommentLiveQuotaExceeded):
		status = http.StatusTooManyRequests
		code = "forum.budget-exceeded"
		msg = "live comment budget exceeded across forum"
		w.Header().Set("Retry-After", "3600")
	case errors.Is(err, domain.ErrCommentBodyEmpty),
		errors.Is(err, domain.ErrCommentBodyTooLong),
		errors.Is(err, domain.ErrInvalidCommentDepth),
		errors.Is(err, domain.ErrCrossPostParent):
		status = http.StatusBadRequest
		code = "forum.bad-request"
		msg = err.Error()
	case errors.Is(err, domain.ErrPostNotFound),
		errors.Is(err, domain.ErrCommentNotFound),
		errors.Is(err, domain.ErrParentCommentNotFound),
		errors.Is(err, domain.ErrCommentAuthorMismatch):
		// Per B-BR-FOR-04: IDOR protection. Author mismatch or missing resource returns 404.
		status = http.StatusNotFound
		code = "forum.not-found"
		msg = "resource not found"
	case errors.Is(err, domain.ErrRevisionMismatch),
		errors.Is(err, domain.ErrConflictingOperation),
		errors.Is(err, domain.ErrRemovedIsTerminal),
		errors.Is(err, identitydomain.ErrIdempotentConflict):
		status = http.StatusConflict
		code = "forum.conflict"
		msg = "conflicting operation or revision mismatch"
	case errors.Is(err, domain.ErrPostNotCommentable),
		errors.Is(err, domain.ErrPostCommentsDisabled):
		status = http.StatusBadRequest
		code = "forum.comments-disabled"
		msg = "comments are disabled or post is not published"
	}

	w.Header().Set("Cache-Control", "no-store")
	httpapi.WriteError(w, r, status, code, msg, nil)
}

// CreateComment handles signed owner submission of comments and replies.
func (h *DiscussionHandler) CreateComment(w http.ResponseWriter, r *http.Request) {
	postID := chi.URLParam(r, "post_id")
	if !isUUID(postID) {
		httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", "invalid post id", nil)
		return
	}

	var req CreateCommentRequest
	if err := readJSONBody(r, &req); err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", "malformed request payload", nil)
		return
	}

	principal, status, code, msg := h.authenticatePrincipal(r, req.SessionEnvelope)
	if status != 0 {
		w.Header().Set("Cache-Control", "no-store")
		httpapi.WriteError(w, r, status, code, msg, nil)
		return
	}

	if !isUUID(req.OperationID) {
		httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", "invalid operation id", nil)
		return
	}

	if req.ParentID != nil && *req.ParentID != "" && !isUUID(*req.ParentID) {
		httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", "invalid parent id", nil)
		return
	}

	cleanedBody, err := domain.ValidateCommentBody(req.BodyText)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", err.Error(), nil)
		return
	}

	// Canonical logical intent bytes strictly excluding session credentials and headers
	intentBytes, err := CanonicalCreateIntent(req.OperationID, postID, req.ParentID, cleanedBody)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", "serialization error", nil)
		return
	}

	guard := h.makeTxGuard(principal)

	runFn := func(ctx context.Context) (CommentReceiptDTO, error) {
		cmd := application.CreateCommentCommand{
			PostID:          postID,
			AuthorAccountID: principal.AccountID,
			ParentID:        req.ParentID,
			BodyText:        cleanedBody,
			OperationID:     req.OperationID,
			Guard:           guard,
		}
		res, err := h.Comments.CreateComment(ctx, cmd)
		if err != nil {
			return CommentReceiptDTO{}, err
		}
		return mapCommentToReceiptDTO(res.Comment, res.Replayed), nil
	}

	if h.Runner != nil {
		key := identitydomain.IdempotencyKey{
			ContributorID: principal.ContributorID,
			Method:        r.Method,
			Route:         r.URL.Path,
			Key:           req.OperationID,
		}
		outcome, err := h.Runner.Do(r.Context(), key, intentBytes, func(ctx context.Context, _ pgx.Tx) (int, []byte, error) {
			receipt, err := runFn(ctx)
			if err != nil {
				return 0, nil, err
			}
			bodyBytes, err := json.Marshal(receipt)
			if err != nil {
				return 0, nil, err
			}
			return http.StatusOK, bodyBytes, nil
		})
		if err != nil {
			h.writeError(w, r, err)
			return
		}

		w.Header().Set("Cache-Control", "no-store")
		var receipt CommentReceiptDTO
		if err := json.Unmarshal(outcome.Response, &receipt); err == nil {
			receipt.Replayed = receipt.Replayed || outcome.Replayed
			bodyBytes, _ := json.Marshal(receipt)
			httpapi.WriteJSON(w, r, http.StatusOK, "no-store", bodyBytes)
			return
		}
		httpapi.WriteJSON(w, r, outcome.StatusCode, "no-store", outcome.Response)
		return
	}

	receipt, err := runFn(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	respBytes, _ := json.Marshal(receipt)
	w.Header().Set("Cache-Control", "no-store")
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", respBytes)
}

// EditComment handles signed owner CAS editing of an existing comment.
func (h *DiscussionHandler) EditComment(w http.ResponseWriter, r *http.Request) {
	commentID := chi.URLParam(r, "comment_id")
	if !isUUID(commentID) {
		httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", "invalid comment id", nil)
		return
	}

	var req EditCommentRequest
	if err := readJSONBody(r, &req); err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", "malformed request payload", nil)
		return
	}

	principal, status, code, msg := h.authenticatePrincipal(r, req.SessionEnvelope)
	if status != 0 {
		w.Header().Set("Cache-Control", "no-store")
		httpapi.WriteError(w, r, status, code, msg, nil)
		return
	}

	if !isUUID(req.OperationID) {
		httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", "invalid operation id", nil)
		return
	}

	if req.ExpectedRevision < 1 {
		httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", "expected_revision must be >= 1", nil)
		return
	}

	cleanedBody, err := domain.ValidateCommentBody(req.BodyText)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", err.Error(), nil)
		return
	}

	intentBytes, err := CanonicalEditIntent(req.OperationID, commentID, req.ExpectedRevision, cleanedBody)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", "serialization error", nil)
		return
	}

	guard := h.makeTxGuard(principal)

	runFn := func(ctx context.Context) (CommentReceiptDTO, error) {
		cmd := application.EditCommentCommand{
			CommentID:        commentID,
			AuthorAccountID:  principal.AccountID,
			ExpectedRevision: req.ExpectedRevision,
			BodyText:         cleanedBody,
			Guard:            guard,
		}
		updated, err := h.Comments.EditComment(ctx, cmd)
		if err != nil {
			return CommentReceiptDTO{}, err
		}
		return mapCommentToReceiptDTO(updated, false), nil
	}

	if h.Runner != nil {
		key := identitydomain.IdempotencyKey{
			ContributorID: principal.ContributorID,
			Method:        r.Method,
			Route:         r.URL.Path,
			Key:           req.OperationID,
		}
		outcome, err := h.Runner.Do(r.Context(), key, intentBytes, func(ctx context.Context, _ pgx.Tx) (int, []byte, error) {
			receipt, err := runFn(ctx)
			if err != nil {
				return 0, nil, err
			}
			bodyBytes, err := json.Marshal(receipt)
			if err != nil {
				return 0, nil, err
			}
			return http.StatusOK, bodyBytes, nil
		})
		if err != nil {
			h.writeError(w, r, err)
			return
		}

		w.Header().Set("Cache-Control", "no-store")
		var receipt CommentReceiptDTO
		if err := json.Unmarshal(outcome.Response, &receipt); err == nil {
			receipt.Replayed = receipt.Replayed || outcome.Replayed
			bodyBytes, _ := json.Marshal(receipt)
			httpapi.WriteJSON(w, r, http.StatusOK, "no-store", bodyBytes)
			return
		}
		httpapi.WriteJSON(w, r, outcome.StatusCode, "no-store", outcome.Response)
		return
	}

	receipt, err := runFn(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	respBytes, _ := json.Marshal(receipt)
	w.Header().Set("Cache-Control", "no-store")
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", respBytes)
}

// RemoveComment handles signed owner tombstoning of an existing comment.
func (h *DiscussionHandler) RemoveComment(w http.ResponseWriter, r *http.Request) {
	commentID := chi.URLParam(r, "comment_id")
	if !isUUID(commentID) {
		httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", "invalid comment id", nil)
		return
	}

	var req RemoveCommentRequest
	if err := readJSONBody(r, &req); err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", "malformed request payload", nil)
		return
	}

	principal, status, code, msg := h.authenticatePrincipal(r, req.SessionEnvelope)
	if status != 0 {
		w.Header().Set("Cache-Control", "no-store")
		httpapi.WriteError(w, r, status, code, msg, nil)
		return
	}

	if !isUUID(req.OperationID) {
		httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", "invalid operation id", nil)
		return
	}

	if req.ExpectedRevision < 1 {
		httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", "expected_revision must be >= 1", nil)
		return
	}

	intentBytes, err := CanonicalRemoveIntent(req.OperationID, commentID, req.ExpectedRevision)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", "serialization error", nil)
		return
	}

	guard := h.makeTxGuard(principal)

	runFn := func(ctx context.Context) (CommentReceiptDTO, error) {
		cmd := application.RemoveCommentCommand{
			CommentID:        commentID,
			AuthorAccountID:  principal.AccountID,
			ExpectedRevision: req.ExpectedRevision,
			Guard:            guard,
		}
		removed, err := h.Comments.RemoveComment(ctx, cmd)
		if err != nil {
			return CommentReceiptDTO{}, err
		}
		return mapCommentToReceiptDTO(removed, false), nil
	}

	if h.Runner != nil {
		key := identitydomain.IdempotencyKey{
			ContributorID: principal.ContributorID,
			Method:        r.Method,
			Route:         r.URL.Path,
			Key:           req.OperationID,
		}
		outcome, err := h.Runner.Do(r.Context(), key, intentBytes, func(ctx context.Context, _ pgx.Tx) (int, []byte, error) {
			receipt, err := runFn(ctx)
			if err != nil {
				return 0, nil, err
			}
			bodyBytes, err := json.Marshal(receipt)
			if err != nil {
				return 0, nil, err
			}
			return http.StatusOK, bodyBytes, nil
		})
		if err != nil {
			h.writeError(w, r, err)
			return
		}

		w.Header().Set("Cache-Control", "no-store")
		var receipt CommentReceiptDTO
		if err := json.Unmarshal(outcome.Response, &receipt); err == nil {
			receipt.Replayed = receipt.Replayed || outcome.Replayed
			bodyBytes, _ := json.Marshal(receipt)
			httpapi.WriteJSON(w, r, http.StatusOK, "no-store", bodyBytes)
			return
		}
		httpapi.WriteJSON(w, r, outcome.StatusCode, "no-store", outcome.Response)
		return
	}

	receipt, err := runFn(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	respBytes, _ := json.Marshal(receipt)
	w.Header().Set("Cache-Control", "no-store")
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", respBytes)
}

// OwnerRead handles private owner projection reads of authored comments.
func (h *DiscussionHandler) OwnerRead(w http.ResponseWriter, r *http.Request) {
	var req OwnerReadRequest
	if err := readJSONBody(r, &req); err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", "malformed request payload", nil)
		return
	}

	principal, status, code, msg := h.authenticatePrincipal(r, req.SessionEnvelope)
	if status != 0 {
		w.Header().Set("Cache-Control", "no-store")
		httpapi.WriteError(w, r, status, code, msg, nil)
		return
	}

	if req.CommentID == nil || *req.CommentID == "" || !isUUID(*req.CommentID) {
		httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", "comment_id required", nil)
		return
	}

	comment, err := h.Comments.GetCommentByID(r.Context(), *req.CommentID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	// B-BR-FOR-04 IDOR check: author must match authenticated session account.
	// Foreign owner returns 404 (indistinguishable from missing resource).
	if comment.AuthorAccountID != principal.AccountID {
		httpapi.WriteError(w, r, http.StatusNotFound, "forum.not-found", "comment not found", nil)
		return
	}

	receipt := mapCommentToReceiptDTO(comment, false)
	resp := OwnerCommentResponseDTO{Comment: receipt}
	bodyBytes, _ := json.Marshal(resp)
	w.Header().Set("Cache-Control", "no-store")
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", bodyBytes)
}

// ListComments handles anonymous paginated reading of published visible top-level comments.
func (h *DiscussionHandler) ListComments(w http.ResponseWriter, r *http.Request) {
	postID := chi.URLParam(r, "post_id")
	if !isUUID(postID) {
		httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", "invalid post id", nil)
		return
	}

	limit := 20
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		if n, err := strconv.Atoi(rawLimit); err == nil && n >= 1 && n <= 50 {
			limit = n
		} else {
			httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", "limit must be 1..50", nil)
			return
		}
	}

	var cursorSeq *int64
	if rawCursor := strings.TrimSpace(r.URL.Query().Get("cursor")); rawCursor != "" {
		if n, err := strconv.ParseInt(rawCursor, 10, 64); err == nil && n > 0 {
			cursorSeq = &n
		} else {
			httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", "invalid cursor", nil)
			return
		}
	}

	query := application.ListPostCommentsQuery{
		PostID:         postID,
		Limit:          limit,
		CursorSequence: cursorSeq,
	}

	comments, err := h.Comments.ListCommentsByPost(r.Context(), query)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	items := make([]CommentItemDTO, 0, len(comments))
	var nextCursor *string
	for i, c := range comments {
		if i == limit {
			// next cursor available
			break
		}
		items = append(items, CommentItemDTO{
			ID:          c.ID,
			PostID:      c.PostID,
			ParentID:    c.ParentID,
			Depth:       c.Depth,
			Sequence:    c.Sequence,
			Revision:    c.Revision,
			PublicAlias: c.PublicAlias,
			BodyText:    c.BodyText,
			CreatedAt:   c.CreatedAt.UTC().Format(time.RFC3339),
			ReplyCount:  0,
		})
	}
	if len(items) > 0 && len(comments) > limit {
		lastSeq := strconv.FormatInt(items[len(items)-1].Sequence, 10)
		nextCursor = &lastSeq
	}

	resp := CommentsResponseDTO{
		Items:      items,
		NextCursor: nextCursor,
	}
	bodyBytes, _ := json.Marshal(resp)
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", bodyBytes)
}

// ListReplies handles anonymous paginated reading of published visible replies.
func (h *DiscussionHandler) ListReplies(w http.ResponseWriter, r *http.Request) {
	parentID := chi.URLParam(r, "comment_id")
	if !isUUID(parentID) {
		httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", "invalid comment id", nil)
		return
	}

	limit := 20
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		if n, err := strconv.Atoi(rawLimit); err == nil && n >= 1 && n <= 50 {
			limit = n
		} else {
			httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", "limit must be 1..50", nil)
			return
		}
	}

	var cursorSeq *int64
	if rawCursor := strings.TrimSpace(r.URL.Query().Get("cursor")); rawCursor != "" {
		if n, err := strconv.ParseInt(rawCursor, 10, 64); err == nil && n > 0 {
			cursorSeq = &n
		} else {
			httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", "invalid cursor", nil)
			return
		}
	}

	query := application.ListCommentRepliesQuery{
		ParentID:       parentID,
		Limit:          limit,
		CursorSequence: cursorSeq,
	}

	replies, err := h.Comments.ListRepliesByParent(r.Context(), query)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	items := make([]CommentItemDTO, 0, len(replies))
	var nextCursor *string
	for i, c := range replies {
		if i == limit {
			break
		}
		items = append(items, CommentItemDTO{
			ID:          c.ID,
			PostID:      c.PostID,
			ParentID:    c.ParentID,
			Depth:       c.Depth,
			Sequence:    c.Sequence,
			Revision:    c.Revision,
			PublicAlias: c.PublicAlias,
			BodyText:    c.BodyText,
			CreatedAt:   c.CreatedAt.UTC().Format(time.RFC3339),
			ReplyCount:  0,
		})
	}
	if len(items) > 0 && len(replies) > limit {
		lastSeq := strconv.FormatInt(items[len(items)-1].Sequence, 10)
		nextCursor = &lastSeq
	}

	resp := RepliesResponseDTO{
		Items:      items,
		NextCursor: nextCursor,
	}
	bodyBytes, _ := json.Marshal(resp)
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", bodyBytes)
}
