// Package http exposes station/fuel feedback through real request
// binding (P14-T03). Writes derive the author server-side from a live
// account session; reads stay public and anonymous. Every response is
// no-store; error envelopes carry stable machine codes and static
// messages, never account identifiers, addresses or precise locations.
// Comment text travels verbatim (JSON-escaped by the encoder, never
// rendered as HTML anywhere in this path).
package http

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// Service is the feedback use-case port behind the handlers.
type Service interface {
	SubmitComment(ctx context.Context, accountID, stationID, product, text string) (domain.StoredComment, error)
	Reply(ctx context.Context, accountID, stationID, product, parentID, text string) (domain.StoredComment, error)
	EditComment(ctx context.Context, accountID, commentID, text string, expectedRevision int) (domain.StoredComment, error)
	DeleteComment(ctx context.Context, accountID, commentID string) error
	ViewComment(ctx context.Context, id string) (domain.CommentView, error)
	ListComments(ctx context.Context, stationID, product, cursor string, limit int) (application.CommentPage, error)
	ListReplies(ctx context.Context, parentID, cursor string, limit int) (application.CommentPage, error)
}

// Handler serves the feedback routes with an injected service and a
// session validator. The validator resolves a live account session to
// its account ID; the composition root maps session failures onto
// feedback-domain errors so this package never imports other modules.
type Handler struct {
	Service  Service
	Sessions func(ctx context.Context, familyID, accessToken string) (string, error)
}

// RegisterRoutes mounts the additive feedback paths.
func (h Handler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/feedback/comments", h.submitComment)
	r.Post("/v1/feedback/comments/{id}/replies", h.reply)
	r.Post("/v1/feedback/comments/{id}/edit", h.editComment)
	r.Post("/v1/feedback/comments/{id}/remove", h.deleteComment)
	r.Get("/v1/feedback/comments", h.listComments)
	r.Get("/v1/feedback/comments/{id}/replies", h.listReplies)
}

func read(r *http.Request, dst any) error {
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" {
		return errors.New("JSON required")
	}
	raw, err := httpapi.ReadBody(r, 8<<10)
	if err != nil {
		return err
	}
	return httpapi.DecodeJSON(raw, dst)
}

func fail(w http.ResponseWriter, r *http.Request, err error) {
	status, code, msg := http.StatusInternalServerError, "feedback.unavailable", "feedback service unavailable"
	switch domain.VerdictCode(err) {
	case "text-empty", "text-too-long", "text-invalid-encoding", "target-invalid", "parent-invalid":
		status, code, msg = http.StatusBadRequest, "feedback."+domain.VerdictCode(err), "invalid feedback input"
	case "rating-out-of-range":
		status, code, msg = http.StatusBadRequest, "feedback.rating-out-of-range", "invalid feedback input"
	case "author-forbidden":
		status, code, msg = http.StatusForbidden, "feedback.author-forbidden", "account cannot write feedback"
	case "session-invalid":
		status, code, msg = http.StatusUnauthorized, "feedback.session-invalid", "valid account session required"
	case "comment-not-found", "not-author":
		// Missing and foreign share one 404: no ownership oracle.
		status, code, msg = http.StatusNotFound, "feedback.comment-not-found", "comment not found"
	case "stale-revision":
		status, code, msg = http.StatusConflict, "feedback.stale-revision", "comment changed, reload and retry"
	case "ok":
		status, code, msg = http.StatusServiceUnavailable, "feedback.unavailable", "feedback service unavailable"
	}
	httpapi.WriteError(w, r, status, code, msg, nil)
}

func bad(w http.ResponseWriter, r *http.Request) {
	httpapi.WriteError(w, r, 400, "feedback.bad-body", "malformed feedback request", nil)
}

func write(w http.ResponseWriter, r *http.Request, status int, body any) {
	raw, err := json.Marshal(body)
	if err != nil {
		httpapi.WriteError(w, r, 500, "feedback.unavailable", "feedback service unavailable", nil)
		return
	}
	httpapi.WriteJSON(w, r, status, "no-store", raw)
}

type commentJSON struct {
	ID        string  `json:"id"`
	Alias     string  `json:"public_alias"`
	StationID string  `json:"station_id"`
	Product   string  `json:"product"`
	ParentID  *string `json:"parent_id"`
	Depth     int     `json:"depth"`
	Text      string  `json:"text"`
	Revision  int     `json:"revision"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

func commentJSONOf(v domain.CommentView) commentJSON {
	var parent *string
	if v.ParentID != "" {
		parent = &v.ParentID
	}
	return commentJSON{
		ID:        v.ID,
		Alias:     v.Alias,
		StationID: v.StationID,
		Product:   v.Product,
		ParentID:  parent,
		Depth:     v.Depth,
		Text:      v.Text,
		Revision:  v.Revision,
		CreatedAt: time.Unix(v.CreatedAt, 0).UTC().Format(time.RFC3339),
		UpdatedAt: time.Unix(v.UpdatedAt, 0).UTC().Format(time.RFC3339),
	}
}

type sessionDTO struct {
	FamilyID    string `json:"family_id"`
	AccessToken string `json:"access_token"`
}

func (h Handler) sessionAccount(w http.ResponseWriter, r *http.Request, dto sessionDTO) (string, bool) {
	if dto.FamilyID == "" || dto.AccessToken == "" {
		bad(w, r)
		return "", false
	}
	accountID, err := h.Sessions(r.Context(), dto.FamilyID, dto.AccessToken)
	if err != nil {
		fail(w, r, err)
		return "", false
	}
	return accountID, true
}

func (h Handler) submitComment(w http.ResponseWriter, r *http.Request) {
	var raw struct {
		FamilyID  string `json:"family_id"`
		AccessTok string `json:"access_token"`
		StationID string `json:"station_id"`
		Product   string `json:"product"`
		Text      string `json:"text"`
	}
	if err := read(r, &raw); err != nil || raw.StationID == "" || raw.Product == "" {
		bad(w, r)
		return
	}
	accountID, ok := h.sessionAccount(w, r, sessionDTO{FamilyID: raw.FamilyID, AccessToken: raw.AccessTok})
	if !ok {
		return
	}
	rec, err := h.Service.SubmitComment(r.Context(), accountID, raw.StationID, raw.Product, raw.Text)
	if err != nil {
		fail(w, r, err)
		return
	}
	view, err := h.Service.ViewComment(r.Context(), rec.ID)
	if err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, http.StatusCreated, map[string]any{"comment": commentJSONOf(view)})
}

func (h Handler) reply(w http.ResponseWriter, r *http.Request) {
	var raw struct {
		FamilyID  string `json:"family_id"`
		AccessTok string `json:"access_token"`
		Text      string `json:"text"`
	}
	if err := read(r, &raw); err != nil {
		bad(w, r)
		return
	}
	parentID := chi.URLParam(r, "id")
	if parentID == "" {
		bad(w, r)
		return
	}
	accountID, ok := h.sessionAccount(w, r, sessionDTO{FamilyID: raw.FamilyID, AccessToken: raw.AccessTok})
	if !ok {
		return
	}
	parent, err := h.Service.ViewComment(r.Context(), parentID)
	if err != nil {
		fail(w, r, err)
		return
	}
	rec, err := h.Service.Reply(r.Context(), accountID, parent.StationID, parent.Product, parentID, raw.Text)
	if err != nil {
		fail(w, r, err)
		return
	}
	view, err := h.Service.ViewComment(r.Context(), rec.ID)
	if err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, http.StatusCreated, map[string]any{"comment": commentJSONOf(view)})
}

func (h Handler) editComment(w http.ResponseWriter, r *http.Request) {
	var raw struct {
		FamilyID         string `json:"family_id"`
		AccessTok        string `json:"access_token"`
		Text             string `json:"text"`
		ExpectedRevision int    `json:"expected_revision"`
	}
	if err := read(r, &raw); err != nil {
		bad(w, r)
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		bad(w, r)
		return
	}
	accountID, ok := h.sessionAccount(w, r, sessionDTO{FamilyID: raw.FamilyID, AccessToken: raw.AccessTok})
	if !ok {
		return
	}
	rec, err := h.Service.EditComment(r.Context(), accountID, id, raw.Text, raw.ExpectedRevision)
	if err != nil {
		fail(w, r, err)
		return
	}
	view, err := h.Service.ViewComment(r.Context(), rec.ID)
	if err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, http.StatusOK, map[string]any{"comment": commentJSONOf(view)})
}

func (h Handler) deleteComment(w http.ResponseWriter, r *http.Request) {
	var raw struct {
		FamilyID  string `json:"family_id"`
		AccessTok string `json:"access_token"`
	}
	if err := read(r, &raw); err != nil {
		bad(w, r)
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		bad(w, r)
		return
	}
	accountID, ok := h.sessionAccount(w, r, sessionDTO{FamilyID: raw.FamilyID, AccessToken: raw.AccessTok})
	if !ok {
		return
	}
	if err := h.Service.DeleteComment(r.Context(), accountID, id); err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

func pageQuery(r *http.Request) (cursor string, limit int, ok bool) {
	cursor = r.URL.Query().Get("cursor")
	limit = 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			return "", 0, false
		}
		limit = n
	}
	return cursor, limit, true
}

func writePage(w http.ResponseWriter, r *http.Request, page application.CommentPage) {
	items := make([]commentJSON, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, commentJSONOf(item))
	}
	var cursor *string
	if page.NextCursor != "" {
		cursor = &page.NextCursor
	}
	write(w, r, http.StatusOK, map[string]any{"comments": items, "next_cursor": cursor})
}

func (h Handler) listComments(w http.ResponseWriter, r *http.Request) {
	stationID := r.URL.Query().Get("station_id")
	product := r.URL.Query().Get("product")
	cursor, limit, ok := pageQuery(r)
	if !ok || stationID == "" || product == "" {
		bad(w, r)
		return
	}
	page, err := h.Service.ListComments(r.Context(), stationID, product, cursor, limit)
	if err != nil {
		fail(w, r, err)
		return
	}
	writePage(w, r, page)
}

func (h Handler) listReplies(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cursor, limit, ok := pageQuery(r)
	if !ok || id == "" {
		bad(w, r)
		return
	}
	page, err := h.Service.ListReplies(r.Context(), id, cursor, limit)
	if err != nil {
		fail(w, r, err)
		return
	}
	writePage(w, r, page)
}
