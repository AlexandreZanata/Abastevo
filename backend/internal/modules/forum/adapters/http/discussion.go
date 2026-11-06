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

// OwnerReadRequest is the wire request body to read private authored comments and topics.
type OwnerReadRequest struct {
	SessionEnvelope
	CommentID *string `json:"comment_id,omitempty"`
	PostID    *string `json:"post_id,omitempty"`
	TopicID   *string `json:"topic_id,omitempty"`
}

// CreateTopicRequest is the wire request body to submit an authored discussion topic.
type CreateTopicRequest struct {
	SessionEnvelope
	OperationID            string  `json:"operation_id"`
	Category               string  `json:"category"`
	Title                  string  `json:"title"`
	BodyText               string  `json:"body_text"`
	PublicAlias            *string `json:"public_alias,omitempty"`
	RegionUF               *string `json:"region_uf,omitempty"`
	RegionMunicipalityIBGE *int    `json:"region_municipality_ibge,omitempty"`
	FuelProduct            *string `json:"fuel_product,omitempty"`
	StationID              *string `json:"station_id,omitempty"`
}

// EditTopicRequest is the wire request body to CAS-edit an authored topic.
type EditTopicRequest struct {
	SessionEnvelope
	OperationID            string  `json:"operation_id"`
	ExpectedRevision       int     `json:"expected_revision"`
	Category               string  `json:"category"`
	Title                  string  `json:"title"`
	BodyText               string  `json:"body_text"`
	RegionUF               *string `json:"region_uf,omitempty"`
	RegionMunicipalityIBGE *int    `json:"region_municipality_ibge,omitempty"`
	FuelProduct            *string `json:"fuel_product,omitempty"`
	StationID              *string `json:"station_id,omitempty"`
}

// RemoveTopicRequest is the wire request body to tombstone an authored topic.
type RemoveTopicRequest struct {
	SessionEnvelope
	OperationID      string `json:"operation_id"`
	ExpectedRevision int    `json:"expected_revision"`
}

// ReportRequest is the wire request body to report a post or comment for moderation.
type ReportRequest struct {
	SessionEnvelope
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}

// ReportReceiptDTO represents the receipt returned for a filed moderation report.
type ReportReceiptDTO struct {
	ReportID   string `json:"report_id"`
	CaseID     string `json:"case_id"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Reason     string `json:"reason"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
	Replayed   bool   `json:"replayed"`
}

// ReportResponseDTO wraps a report receipt response.
type ReportResponseDTO struct {
	Receipt ReportReceiptDTO `json:"receipt"`
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

// TopicReceiptDTO represents the private owner receipt returned upon
// topic create, edit, removal, or owner read.
type TopicReceiptDTO struct {
	ID               string     `json:"id"`
	Kind             string     `json:"kind"`
	Category         string     `json:"category"`
	Title            string     `json:"title"`
	BodyText         string     `json:"body_text"`
	Revision         int        `json:"revision"`
	PublicationState string     `json:"publication_state"`
	Visibility       string     `json:"visibility"`
	Region           *RegionDTO `json:"region,omitempty"`
	FuelProduct      *string    `json:"fuel_product,omitempty"`
	StationID        *string    `json:"station_id,omitempty"`
	PublicAlias      *string    `json:"public_alias,omitempty"`
	CreatedAt        string     `json:"created_at"`
	UpdatedAt        string     `json:"updated_at"`
	PublishedAt      *string    `json:"published_at,omitempty"`
	Replayed         bool       `json:"replayed"`
}

// OwnerTopicResponseDTO wraps a single private owner topic receipt.
type OwnerTopicResponseDTO struct {
	Topic TopicReceiptDTO `json:"topic"`
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

type canonicalCreateTopicPayload struct {
	Action                 string  `json:"action"`
	OperationID            string  `json:"operation_id"`
	Category               string  `json:"category"`
	Title                  string  `json:"title"`
	BodyText               string  `json:"body_text"`
	PublicAlias            *string `json:"public_alias,omitempty"`
	RegionUF               *string `json:"region_uf,omitempty"`
	RegionMunicipalityIBGE *int    `json:"region_municipality_ibge,omitempty"`
	FuelProduct            *string `json:"fuel_product,omitempty"`
	StationID              *string `json:"station_id,omitempty"`
}

type canonicalEditTopicPayload struct {
	Action                 string  `json:"action"`
	OperationID            string  `json:"operation_id"`
	TopicID                string  `json:"topic_id"`
	ExpectedRevision       int     `json:"expected_revision"`
	Category               string  `json:"category"`
	Title                  string  `json:"title"`
	BodyText               string  `json:"body_text"`
	RegionUF               *string `json:"region_uf,omitempty"`
	RegionMunicipalityIBGE *int    `json:"region_municipality_ibge,omitempty"`
	FuelProduct            *string `json:"fuel_product,omitempty"`
	StationID              *string `json:"station_id,omitempty"`
}

type canonicalRemoveTopicPayload struct {
	Action           string `json:"action"`
	OperationID      string `json:"operation_id"`
	TopicID          string `json:"topic_id"`
	ExpectedRevision int    `json:"expected_revision"`
}

// CanonicalCreateTopicIntent serializes the stable logical intent of a create topic command.
func CanonicalCreateTopicIntent(operationID, category, title, bodyText string, publicAlias, regionUF *string, ibge *int, fuelProduct, stationID *string) ([]byte, error) {
	return json.Marshal(canonicalCreateTopicPayload{
		Action:                 "create_topic",
		OperationID:            strings.TrimSpace(operationID),
		Category:               strings.TrimSpace(category),
		Title:                  title,
		BodyText:               bodyText,
		PublicAlias:            publicAlias,
		RegionUF:               regionUF,
		RegionMunicipalityIBGE: ibge,
		FuelProduct:            fuelProduct,
		StationID:              stationID,
	})
}

// CanonicalEditTopicIntent serializes the stable logical intent of an edit topic command.
func CanonicalEditTopicIntent(operationID, topicID string, expectedRevision int, category, title, bodyText string, regionUF *string, ibge *int, fuelProduct, stationID *string) ([]byte, error) {
	return json.Marshal(canonicalEditTopicPayload{
		Action:                 "edit_topic",
		OperationID:            strings.TrimSpace(operationID),
		TopicID:                strings.TrimSpace(topicID),
		ExpectedRevision:       expectedRevision,
		Category:               strings.TrimSpace(category),
		Title:                  title,
		BodyText:               bodyText,
		RegionUF:               regionUF,
		RegionMunicipalityIBGE: ibge,
		FuelProduct:            fuelProduct,
		StationID:              stationID,
	})
}

// CanonicalRemoveTopicIntent serializes the stable logical intent of a remove topic command.
func CanonicalRemoveTopicIntent(operationID, topicID string, expectedRevision int) ([]byte, error) {
	return json.Marshal(canonicalRemoveTopicPayload{
		Action:           "remove_topic",
		OperationID:      strings.TrimSpace(operationID),
		TopicID:          strings.TrimSpace(topicID),
		ExpectedRevision: expectedRevision,
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
	Reports     application.ReportStore
	Topics      application.TopicStore
	Sessions    func(ctx context.Context, familyID, accessToken string) (accountID string, err error)
	SignedOwner func(r *http.Request, accountID string) (contributorID string, keyID string, err error)
	Guard       func(ctx context.Context, tx pgx.Tx, accountID, familyID, accessToken, contributorID, keyID string) error
	Runner      *identityadapters.Runner
	Clock       func() time.Time
	Config      *domain.Config
}

func (h *DiscussionHandler) now() time.Time {
	if h.Clock != nil {
		return h.Clock().UTC()
	}
	return time.Now().UTC()
}

func (h *DiscussionHandler) readsAllowed() bool {
	if h.Config == nil {
		return true
	}
	return h.Config.ReadsAllowed()
}

func (h *DiscussionHandler) writesAllowed() bool {
	if h.Config == nil {
		return true
	}
	return h.Config.WritesAllowed()
}

func (h *DiscussionHandler) writeBadRequest(w http.ResponseWriter, r *http.Request, msg string) {
	w.Header().Set("Cache-Control", "no-store")
	httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", msg, nil)
}

// RegisterRoutes registers discussion endpoints on a chi router.
func (h *DiscussionHandler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/forum/posts/{post_id}/comments", h.CreateComment)
	r.Get("/v1/forum/posts/{post_id}/comments", h.ListComments)
	r.Post("/v1/forum/posts/{post_id}/report", h.ReportPost)
	r.Post("/v1/forum/comments/{comment_id}/edit", h.EditComment)
	r.Post("/v1/forum/comments/{comment_id}/remove", h.RemoveComment)
	r.Get("/v1/forum/comments/{comment_id}/replies", h.ListReplies)
	r.Post("/v1/forum/comments/{comment_id}/report", h.ReportComment)
	r.Post("/v1/forum/owner/read", h.OwnerRead)
	r.Post("/v1/forum/topics", h.CreateTopic)
	r.Post("/v1/forum/topics/{topic_id}/edit", h.EditTopic)
	r.Post("/v1/forum/topics/{topic_id}/remove", h.RemoveTopic)
}

func readJSONBody(r *http.Request, v any) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return domain.ErrCommentBodyEmpty
	}
	bodyBytes, err := httpapi.ReadBody(r, 16<<10)
	if err != nil {
		return err
	}
	if err := httpapi.DecodeJSON(bodyBytes, v); err != nil {
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

func mapPostToTopicReceiptDTO(p *domain.Post, replayed bool) TopicReceiptDTO {
	var pubAt *string
	if p.FirstPublishedAt != nil {
		s := p.FirstPublishedAt.UTC().Format(time.RFC3339)
		pubAt = &s
	}
	return TopicReceiptDTO{
		ID:               p.ID,
		Kind:             string(p.Kind),
		Category:         string(p.Category),
		Title:            p.Title,
		BodyText:         p.BodyText,
		Revision:         p.Revision,
		PublicationState: string(p.PublicationState),
		Visibility:       string(p.Visibility),
		Region:           mapRegionToDTO(p.Region),
		FuelProduct:      p.FuelProduct,
		StationID:        p.StationID,
		PublicAlias:      p.PublicAlias,
		CreatedAt:        p.LocalCreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:        p.LocalUpdatedAt.UTC().Format(time.RFC3339),
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
	case errors.Is(err, domain.ErrCommentLiveQuotaExceeded),
		errors.Is(err, domain.ErrTopicLiveQuotaExceeded):
		status = http.StatusTooManyRequests
		code = "forum.budget-exceeded"
		msg = "live budget exceeded across forum"
		w.Header().Set("Retry-After", "3600")
	case errors.Is(err, domain.ErrTopicHourlyQuotaExceeded):
		status = http.StatusTooManyRequests
		code = "forum.rate-limited"
		msg = "topic hourly quota exceeded (maximum 3 topics per hour)"
		w.Header().Set("Retry-After", "3600")
	case errors.Is(err, domain.ErrTopicDailyQuotaExceeded):
		status = http.StatusTooManyRequests
		code = "forum.rate-limited"
		msg = "topic daily quota exceeded (maximum 10 topics per day)"
		w.Header().Set("Retry-After", "86400")
	case errors.Is(err, domain.ErrReportQuotaExceeded):
		status = http.StatusTooManyRequests
		code = "forum.rate-limited"
		msg = "report quota exceeded (maximum 10 reports per day)"
		w.Header().Set("Retry-After", "86400")
	case errors.Is(err, domain.ErrSelfReportForbidden):
		status = http.StatusBadRequest
		code = "forum.bad-request"
		msg = "cannot report own content"
	case errors.Is(err, domain.ErrReportInvalid):
		status = http.StatusBadRequest
		code = "forum.bad-request"
		msg = "invalid report reason or detail"
	case errors.Is(err, domain.ErrCommentBodyEmpty),
		errors.Is(err, domain.ErrCommentBodyTooLong),
		errors.Is(err, domain.ErrInvalidCommentDepth),
		errors.Is(err, domain.ErrCrossPostParent),
		errors.Is(err, domain.ErrTopicTitleEmpty),
		errors.Is(err, domain.ErrTopicTitleTooLong),
		errors.Is(err, domain.ErrTopicBodyEmpty),
		errors.Is(err, domain.ErrTopicBodyTooLong),
		errors.Is(err, domain.ErrTopicAlreadyRemoved),
		errors.Is(err, domain.ErrInvalidCategory),
		errors.Is(err, domain.ErrInvalidRegion),
		errors.Is(err, domain.ErrInvalidFuelProduct),
		errors.Is(err, domain.ErrInvalidStationID),
		errors.Is(err, domain.ErrTopicKindSpoof):
		status = http.StatusBadRequest
		code = "forum.bad-request"
		msg = err.Error()
	case errors.Is(err, domain.ErrPostNotFound),
		errors.Is(err, domain.ErrCommentNotFound),
		errors.Is(err, domain.ErrParentCommentNotFound),
		errors.Is(err, domain.ErrParentCommentNotVisible),
		errors.Is(err, domain.ErrReportTargetMissing),
		errors.Is(err, domain.ErrCommentAuthorMismatch),
		errors.Is(err, domain.ErrTopicNotFound),
		errors.Is(err, domain.ErrTopicAuthorMismatch):
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
	case errors.Is(err, domain.ErrForumDisabled):
		status = http.StatusServiceUnavailable
		code = "forum.service-unavailable"
		msg = "forum discussion is disabled"
	case errors.Is(err, httpapi.ErrBodyTooLarge):
		status = http.StatusBadRequest
		code = "forum.bad-request"
		msg = "request body exceeds 16KiB limit"
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
	if !h.writesAllowed() {
		h.writeError(w, r, domain.ErrForumDisabled)
		return
	}

	postID := chi.URLParam(r, "post_id")
	if !isUUID(postID) {
		h.writeBadRequest(w, r, "invalid post id")
		return
	}

	var req CreateCommentRequest
	if err := readJSONBody(r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}

	principal, status, code, msg := h.authenticatePrincipal(r, req.SessionEnvelope)
	if status != 0 {
		w.Header().Set("Cache-Control", "no-store")
		httpapi.WriteError(w, r, status, code, msg, nil)
		return
	}

	if !isUUID(req.OperationID) {
		h.writeBadRequest(w, r, "invalid operation id")
		return
	}

	if req.ParentID != nil && *req.ParentID != "" && !isUUID(*req.ParentID) {
		h.writeBadRequest(w, r, "invalid parent id")
		return
	}

	cleanedBody, err := domain.ValidateCommentBody(req.BodyText)
	if err != nil {
		h.writeBadRequest(w, r, err.Error())
		return
	}

	// Canonical logical intent bytes strictly excluding session credentials and headers
	intentBytes, err := CanonicalCreateIntent(req.OperationID, postID, req.ParentID, cleanedBody)
	if err != nil {
		h.writeBadRequest(w, r, "serialization error")
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
	if !h.writesAllowed() {
		h.writeError(w, r, domain.ErrForumDisabled)
		return
	}

	commentID := chi.URLParam(r, "comment_id")
	if !isUUID(commentID) {
		h.writeBadRequest(w, r, "invalid comment id")
		return
	}

	var req EditCommentRequest
	if err := readJSONBody(r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}

	principal, status, code, msg := h.authenticatePrincipal(r, req.SessionEnvelope)
	if status != 0 {
		w.Header().Set("Cache-Control", "no-store")
		httpapi.WriteError(w, r, status, code, msg, nil)
		return
	}

	if !isUUID(req.OperationID) {
		h.writeBadRequest(w, r, "invalid operation id")
		return
	}

	if req.ExpectedRevision < 1 {
		h.writeBadRequest(w, r, "expected_revision must be >= 1")
		return
	}

	cleanedBody, err := domain.ValidateCommentBody(req.BodyText)
	if err != nil {
		h.writeBadRequest(w, r, err.Error())
		return
	}

	intentBytes, err := CanonicalEditIntent(req.OperationID, commentID, req.ExpectedRevision, cleanedBody)
	if err != nil {
		h.writeBadRequest(w, r, "serialization error")
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
	if !h.writesAllowed() {
		h.writeError(w, r, domain.ErrForumDisabled)
		return
	}

	commentID := chi.URLParam(r, "comment_id")
	if !isUUID(commentID) {
		h.writeBadRequest(w, r, "invalid comment id")
		return
	}

	var req RemoveCommentRequest
	if err := readJSONBody(r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}

	principal, status, code, msg := h.authenticatePrincipal(r, req.SessionEnvelope)
	if status != 0 {
		w.Header().Set("Cache-Control", "no-store")
		httpapi.WriteError(w, r, status, code, msg, nil)
		return
	}

	if !isUUID(req.OperationID) {
		h.writeBadRequest(w, r, "invalid operation id")
		return
	}

	if req.ExpectedRevision < 1 {
		h.writeBadRequest(w, r, "expected_revision must be >= 1")
		return
	}

	intentBytes, err := CanonicalRemoveIntent(req.OperationID, commentID, req.ExpectedRevision)
	if err != nil {
		h.writeBadRequest(w, r, "serialization error")
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

// OwnerRead handles private owner projection reads of authored comments or topics.
func (h *DiscussionHandler) OwnerRead(w http.ResponseWriter, r *http.Request) {
	if !h.readsAllowed() {
		h.writeError(w, r, domain.ErrForumDisabled)
		return
	}

	var req OwnerReadRequest
	if err := readJSONBody(r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}

	principal, status, code, msg := h.authenticatePrincipal(r, req.SessionEnvelope)
	if status != 0 {
		w.Header().Set("Cache-Control", "no-store")
		httpapi.WriteError(w, r, status, code, msg, nil)
		return
	}

	hasComment := req.CommentID != nil && strings.TrimSpace(*req.CommentID) != ""
	hasTopic := req.TopicID != nil && strings.TrimSpace(*req.TopicID) != ""

	if !hasComment && !hasTopic {
		h.writeBadRequest(w, r, "comment_id or topic_id required")
		return
	}
	if hasComment && hasTopic {
		h.writeBadRequest(w, r, "specify either comment_id or topic_id, not both")
		return
	}

	if hasTopic {
		if h.Topics == nil {
			h.writeError(w, r, domain.ErrForumDisabled)
			return
		}
		topicID := strings.TrimSpace(*req.TopicID)
		if !isUUID(topicID) {
			h.writeBadRequest(w, r, "invalid topic id")
			return
		}

		topic, err := h.Topics.GetTopicByID(r.Context(), topicID)
		if err != nil {
			h.writeError(w, r, err)
			return
		}

		// B-BR-FOR-04 IDOR check: author must match authenticated session account.
		// Foreign owner returns 404 (indistinguishable from missing resource).
		if topic.AuthorAccountID != principal.AccountID {
			w.Header().Set("Cache-Control", "no-store")
			httpapi.WriteError(w, r, http.StatusNotFound, "forum.not-found", "topic not found", nil)
			return
		}

		receipt := mapPostToTopicReceiptDTO(topic, false)
		resp := OwnerTopicResponseDTO{Topic: receipt}
		bodyBytes, _ := json.Marshal(resp)
		w.Header().Set("Cache-Control", "no-store")
		httpapi.WriteJSON(w, r, http.StatusOK, "no-store", bodyBytes)
		return
	}

	if h.Comments == nil {
		h.writeError(w, r, domain.ErrForumDisabled)
		return
	}

	commentID := strings.TrimSpace(*req.CommentID)
	if !isUUID(commentID) {
		h.writeBadRequest(w, r, "invalid comment id")
		return
	}

	comment, err := h.Comments.GetCommentByID(r.Context(), commentID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	// B-BR-FOR-04 IDOR check: author must match authenticated session account.
	// Foreign owner returns 404 (indistinguishable from missing resource).
	if comment.AuthorAccountID != principal.AccountID {
		w.Header().Set("Cache-Control", "no-store")
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
	if !h.readsAllowed() {
		h.writeError(w, r, domain.ErrForumDisabled)
		return
	}

	postID := chi.URLParam(r, "post_id")
	if !isUUID(postID) {
		h.writeBadRequest(w, r, "invalid post id")
		return
	}

	limit := 20
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		if n, err := strconv.Atoi(rawLimit); err == nil && n >= 1 && n <= 50 {
			limit = n
		} else {
			h.writeBadRequest(w, r, "limit must be 1..50")
			return
		}
	}

	var cursorSeq *int64
	if rawCursor := strings.TrimSpace(r.URL.Query().Get("cursor")); rawCursor != "" {
		if n, err := strconv.ParseInt(rawCursor, 10, 64); err == nil && n > 0 {
			cursorSeq = &n
		} else {
			h.writeBadRequest(w, r, "invalid cursor")
			return
		}
	}

	query := application.ListPostCommentsQuery{
		PostID:         postID,
		Limit:          limit + 1,
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
			ReplyCount:  c.ReplyCount,
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
	w.Header().Set("Cache-Control", "no-store")
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", bodyBytes)
}

// ListReplies handles anonymous paginated reading of published visible replies.
func (h *DiscussionHandler) ListReplies(w http.ResponseWriter, r *http.Request) {
	if !h.readsAllowed() {
		h.writeError(w, r, domain.ErrForumDisabled)
		return
	}

	parentID := chi.URLParam(r, "comment_id")
	if !isUUID(parentID) {
		h.writeBadRequest(w, r, "invalid comment id")
		return
	}

	limit := 20
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		if n, err := strconv.Atoi(rawLimit); err == nil && n >= 1 && n <= 50 {
			limit = n
		} else {
			h.writeBadRequest(w, r, "limit must be 1..50")
			return
		}
	}

	var cursorSeq *int64
	if rawCursor := strings.TrimSpace(r.URL.Query().Get("cursor")); rawCursor != "" {
		if n, err := strconv.ParseInt(rawCursor, 10, 64); err == nil && n > 0 {
			cursorSeq = &n
		} else {
			h.writeBadRequest(w, r, "invalid cursor")
			return
		}
	}

	query := application.ListCommentRepliesQuery{
		ParentID:       parentID,
		Limit:          limit + 1,
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
	w.Header().Set("Cache-Control", "no-store")
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", bodyBytes)
}

// ReportPost handles reporting a published post for moderation review.
func (h *DiscussionHandler) ReportPost(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !h.writesAllowed() {
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "forum.service-unavailable", "forum writes are temporarily disabled", nil)
		return
	}
	postID := strings.TrimSpace(chi.URLParam(r, "post_id"))
	if !isUUID(postID) {
		h.writeBadRequest(w, r, "invalid post id")
		return
	}

	var req ReportRequest
	if err := readJSONBody(r, &req); err != nil {
		if errors.Is(err, httpapi.ErrBodyTooLarge) {
			h.writeBadRequest(w, r, "request body exceeds 16KiB limit")
			return
		}
		h.writeBadRequest(w, r, "invalid request body")
		return
	}

	principal, status, code, msg := h.authenticatePrincipal(r, req.SessionEnvelope)
	if status != 0 {
		httpapi.WriteError(w, r, status, code, msg, nil)
		return
	}

	cleanReason := strings.TrimSpace(req.Reason)
	if cleanReason == "" || len([]rune(cleanReason)) > 200 || !domain.ValidReportReason(cleanReason) {
		h.writeBadRequest(w, r, "valid report reason required (SPAM, HARASSMENT, DEFAMATION, OFF_TOPIC, ILLEGAL, OTHER)")
		return
	}
	if len([]rune(req.Detail)) > 500 {
		h.writeBadRequest(w, r, "report detail exceeds 500 characters")
		return
	}

	guard := h.makeTxGuard(principal)
	receipt, err := h.Reports.Report(r.Context(), application.ReportCommand{
		ReporterAccountID: principal.AccountID,
		TargetType:        "FORUM_POST",
		TargetID:          postID,
		Reason:            cleanReason,
		Detail:            req.Detail,
		Guard:             guard,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	resp := ReportResponseDTO{
		Receipt: ReportReceiptDTO{
			ReportID:   receipt.ID,
			CaseID:     receipt.ModerationCaseID,
			TargetType: receipt.TargetType,
			TargetID:   receipt.TargetID,
			Reason:     receipt.Reason,
			Status:     receipt.Status,
			CreatedAt:  receipt.CreatedAt.UTC().Format(time.RFC3339),
			Replayed:   receipt.Replayed,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// ReportComment handles reporting a comment for moderation review.
func (h *DiscussionHandler) ReportComment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !h.writesAllowed() {
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "forum.service-unavailable", "forum writes are temporarily disabled", nil)
		return
	}
	commentID := strings.TrimSpace(chi.URLParam(r, "comment_id"))
	if !isUUID(commentID) {
		h.writeBadRequest(w, r, "invalid comment id")
		return
	}

	var req ReportRequest
	if err := readJSONBody(r, &req); err != nil {
		if errors.Is(err, httpapi.ErrBodyTooLarge) {
			h.writeBadRequest(w, r, "request body exceeds 16KiB limit")
			return
		}
		h.writeBadRequest(w, r, "invalid request body")
		return
	}

	principal, status, code, msg := h.authenticatePrincipal(r, req.SessionEnvelope)
	if status != 0 {
		httpapi.WriteError(w, r, status, code, msg, nil)
		return
	}

	cleanReason := strings.TrimSpace(req.Reason)
	if cleanReason == "" || len([]rune(cleanReason)) > 200 || !domain.ValidReportReason(cleanReason) {
		h.writeBadRequest(w, r, "valid report reason required (SPAM, HARASSMENT, DEFAMATION, OFF_TOPIC, ILLEGAL, OTHER)")
		return
	}
	if len([]rune(req.Detail)) > 500 {
		h.writeBadRequest(w, r, "report detail exceeds 500 characters")
		return
	}

	guard := h.makeTxGuard(principal)
	receipt, err := h.Reports.Report(r.Context(), application.ReportCommand{
		ReporterAccountID: principal.AccountID,
		TargetType:        "FORUM_COMMENT",
		TargetID:          commentID,
		Reason:            cleanReason,
		Detail:            req.Detail,
		Guard:             guard,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	resp := ReportResponseDTO{
		Receipt: ReportReceiptDTO{
			ReportID:   receipt.ID,
			CaseID:     receipt.ModerationCaseID,
			TargetType: receipt.TargetType,
			TargetID:   receipt.TargetID,
			Reason:     receipt.Reason,
			Status:     receipt.Status,
			CreatedAt:  receipt.CreatedAt.UTC().Format(time.RFC3339),
			Replayed:   receipt.Replayed,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// CreateTopic handles signed owner submission of discussion topics.
func (h *DiscussionHandler) CreateTopic(w http.ResponseWriter, r *http.Request) {
	if !h.writesAllowed() {
		h.writeError(w, r, domain.ErrForumDisabled)
		return
	}
	if h.Topics == nil {
		h.writeError(w, r, domain.ErrForumDisabled)
		return
	}

	var req CreateTopicRequest
	if err := readJSONBody(r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}

	principal, status, code, msg := h.authenticatePrincipal(r, req.SessionEnvelope)
	if status != 0 {
		w.Header().Set("Cache-Control", "no-store")
		httpapi.WriteError(w, r, status, code, msg, nil)
		return
	}

	if !isUUID(req.OperationID) {
		h.writeBadRequest(w, r, "invalid operation id")
		return
	}

	cat, err := domain.ParseCategory(req.Category)
	if err != nil {
		h.writeBadRequest(w, r, err.Error())
		return
	}

	cleanedTitle, err := domain.ValidateTopicTitle(req.Title)
	if err != nil {
		h.writeBadRequest(w, r, err.Error())
		return
	}

	cleanedBody, err := domain.ValidateTopicBody(req.BodyText)
	if err != nil {
		h.writeBadRequest(w, r, err.Error())
		return
	}

	var ufStr string
	if req.RegionUF != nil {
		ufStr = *req.RegionUF
	}
	if err := domain.ValidateTopic(cat, cleanedTitle, cleanedBody, ufStr, req.RegionMunicipalityIBGE, req.FuelProduct, req.StationID); err != nil {
		h.writeBadRequest(w, r, err.Error())
		return
	}

	var publicAlias *string
	if req.PublicAlias != nil {
		trimmedAlias := strings.TrimSpace(*req.PublicAlias)
		if trimmedAlias != "" {
			if len([]rune(trimmedAlias)) > 64 {
				h.writeBadRequest(w, r, "public alias too long")
				return
			}
			publicAlias = &trimmedAlias
		}
	}

	var region *domain.Region
	if ufStr != "" {
		region = &domain.Region{
			UF:               ufStr,
			MunicipalityIBGE: req.RegionMunicipalityIBGE,
		}
	}

	intentBytes, err := CanonicalCreateTopicIntent(
		req.OperationID,
		string(cat),
		cleanedTitle,
		cleanedBody,
		publicAlias,
		req.RegionUF,
		req.RegionMunicipalityIBGE,
		req.FuelProduct,
		req.StationID,
	)
	if err != nil {
		h.writeBadRequest(w, r, "serialization error")
		return
	}

	guard := h.makeTxGuard(principal)

	runFn := func(ctx context.Context) (TopicReceiptDTO, int, error) {
		cmd := application.CreateTopicCommand{
			AuthorAccountID: principal.AccountID,
			Category:        cat,
			Title:           cleanedTitle,
			BodyText:        cleanedBody,
			PublicAlias:     publicAlias,
			Region:          region,
			FuelProduct:     req.FuelProduct,
			StationID:       req.StationID,
			OperationID:     req.OperationID,
			Guard:           guard,
		}
		res, err := h.Topics.CreateTopic(ctx, cmd)
		if err != nil {
			return TopicReceiptDTO{}, 0, err
		}
		statusCode := http.StatusCreated
		if res.Replayed {
			statusCode = http.StatusOK
		}
		return mapPostToTopicReceiptDTO(res.Topic, res.Replayed), statusCode, nil
	}

	if h.Runner != nil {
		key := identitydomain.IdempotencyKey{
			ContributorID: principal.ContributorID,
			Method:        r.Method,
			Route:         r.URL.Path,
			Key:           req.OperationID,
		}
		outcome, err := h.Runner.Do(r.Context(), key, intentBytes, func(ctx context.Context, _ pgx.Tx) (int, []byte, error) {
			receipt, statusCode, err := runFn(ctx)
			if err != nil {
				return 0, nil, err
			}
			bodyBytes, err := json.Marshal(receipt)
			if err != nil {
				return 0, nil, err
			}
			return statusCode, bodyBytes, nil
		})
		if err != nil {
			h.writeError(w, r, err)
			return
		}

		w.Header().Set("Cache-Control", "no-store")
		var receipt TopicReceiptDTO
		if err := json.Unmarshal(outcome.Response, &receipt); err == nil {
			receipt.Replayed = receipt.Replayed || outcome.Replayed
			bodyBytes, _ := json.Marshal(receipt)
			status := outcome.StatusCode
			if outcome.Replayed && status == http.StatusCreated {
				status = http.StatusOK
			}
			httpapi.WriteJSON(w, r, status, "no-store", bodyBytes)
			return
		}
		httpapi.WriteJSON(w, r, outcome.StatusCode, "no-store", outcome.Response)
		return
	}

	receipt, statusCode, err := runFn(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	respBytes, _ := json.Marshal(receipt)
	w.Header().Set("Cache-Control", "no-store")
	httpapi.WriteJSON(w, r, statusCode, "no-store", respBytes)
}

// EditTopic handles signed owner CAS editing of an existing authored topic.
func (h *DiscussionHandler) EditTopic(w http.ResponseWriter, r *http.Request) {
	if !h.writesAllowed() {
		h.writeError(w, r, domain.ErrForumDisabled)
		return
	}
	if h.Topics == nil {
		h.writeError(w, r, domain.ErrForumDisabled)
		return
	}

	topicID := chi.URLParam(r, "topic_id")
	if !isUUID(topicID) {
		h.writeBadRequest(w, r, "invalid topic id")
		return
	}

	var req EditTopicRequest
	if err := readJSONBody(r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}

	principal, status, code, msg := h.authenticatePrincipal(r, req.SessionEnvelope)
	if status != 0 {
		w.Header().Set("Cache-Control", "no-store")
		httpapi.WriteError(w, r, status, code, msg, nil)
		return
	}

	if !isUUID(req.OperationID) {
		h.writeBadRequest(w, r, "invalid operation id")
		return
	}

	if req.ExpectedRevision < 1 {
		h.writeBadRequest(w, r, "expected_revision must be >= 1")
		return
	}

	cat, err := domain.ParseCategory(req.Category)
	if err != nil {
		h.writeBadRequest(w, r, err.Error())
		return
	}

	cleanedTitle, err := domain.ValidateTopicTitle(req.Title)
	if err != nil {
		h.writeBadRequest(w, r, err.Error())
		return
	}

	cleanedBody, err := domain.ValidateTopicBody(req.BodyText)
	if err != nil {
		h.writeBadRequest(w, r, err.Error())
		return
	}

	var ufStr string
	if req.RegionUF != nil {
		ufStr = *req.RegionUF
	}
	if err := domain.ValidateTopic(cat, cleanedTitle, cleanedBody, ufStr, req.RegionMunicipalityIBGE, req.FuelProduct, req.StationID); err != nil {
		h.writeBadRequest(w, r, err.Error())
		return
	}

	var region *domain.Region
	if ufStr != "" {
		region = &domain.Region{
			UF:               ufStr,
			MunicipalityIBGE: req.RegionMunicipalityIBGE,
		}
	}

	intentBytes, err := CanonicalEditTopicIntent(
		req.OperationID,
		topicID,
		req.ExpectedRevision,
		string(cat),
		cleanedTitle,
		cleanedBody,
		req.RegionUF,
		req.RegionMunicipalityIBGE,
		req.FuelProduct,
		req.StationID,
	)
	if err != nil {
		h.writeBadRequest(w, r, "serialization error")
		return
	}

	guard := h.makeTxGuard(principal)

	runFn := func(ctx context.Context) (TopicReceiptDTO, error) {
		cmd := application.EditTopicCommand{
			TopicID:          topicID,
			AuthorAccountID:  principal.AccountID,
			ExpectedRevision: req.ExpectedRevision,
			Category:         cat,
			Title:            cleanedTitle,
			BodyText:         cleanedBody,
			Region:           region,
			FuelProduct:      req.FuelProduct,
			StationID:        req.StationID,
			Guard:            guard,
		}
		updated, err := h.Topics.EditTopic(ctx, cmd)
		if err != nil {
			return TopicReceiptDTO{}, err
		}
		return mapPostToTopicReceiptDTO(updated, false), nil
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
		var receipt TopicReceiptDTO
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

// RemoveTopic handles signed owner tombstoning of an authored topic.
func (h *DiscussionHandler) RemoveTopic(w http.ResponseWriter, r *http.Request) {
	if !h.writesAllowed() {
		h.writeError(w, r, domain.ErrForumDisabled)
		return
	}
	if h.Topics == nil {
		h.writeError(w, r, domain.ErrForumDisabled)
		return
	}

	topicID := chi.URLParam(r, "topic_id")
	if !isUUID(topicID) {
		h.writeBadRequest(w, r, "invalid topic id")
		return
	}

	var req RemoveTopicRequest
	if err := readJSONBody(r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}

	principal, status, code, msg := h.authenticatePrincipal(r, req.SessionEnvelope)
	if status != 0 {
		w.Header().Set("Cache-Control", "no-store")
		httpapi.WriteError(w, r, status, code, msg, nil)
		return
	}

	if !isUUID(req.OperationID) {
		h.writeBadRequest(w, r, "invalid operation id")
		return
	}

	if req.ExpectedRevision < 1 {
		h.writeBadRequest(w, r, "expected_revision must be >= 1")
		return
	}

	intentBytes, err := CanonicalRemoveTopicIntent(req.OperationID, topicID, req.ExpectedRevision)
	if err != nil {
		h.writeBadRequest(w, r, "serialization error")
		return
	}

	guard := h.makeTxGuard(principal)

	runFn := func(ctx context.Context) (TopicReceiptDTO, error) {
		cmd := application.RemoveTopicCommand{
			TopicID:          topicID,
			AuthorAccountID:  principal.AccountID,
			ExpectedRevision: req.ExpectedRevision,
			Guard:            guard,
		}
		removed, err := h.Topics.RemoveTopic(ctx, cmd)
		if err != nil {
			return TopicReceiptDTO{}, err
		}
		return mapPostToTopicReceiptDTO(removed, false), nil
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
		var receipt TopicReceiptDTO
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
