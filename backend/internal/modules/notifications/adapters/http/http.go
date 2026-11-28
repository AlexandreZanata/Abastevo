// Package http serves the account notification API (B-BR-NOTIF-03):
// POST /v1/notifications/list is a private session read and
// POST /v1/notifications/read a write signed by the device key bound to
// the account. Both are no-store and answer one generic authentication
// error for any session or proof failure.
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// Service is the use-case port the handler drives.
type Service interface {
	List(ctx context.Context, accountID string, before *domain.Position, limit int) (application.Page, error)
	MarkRead(ctx context.Context, accountID string, sel domain.Selection) (int64, error)
}

// Handler mounts the two owner routes.
type Handler struct {
	Service Service
	// Sessions resolves (family_id, access_token) to the live account.
	Sessions func(ctx context.Context, familyID, accessToken string) (string, error)
	// SignedOwner verifies the request proof and that its key is bound to
	// the account.
	SignedOwner func(r *http.Request, accountID string) (string, string, error)
	// CursorSecret seals the opaque `before` cursor (at least 32 bytes).
	CursorSecret []byte
	Origin       string
	Clock        func() time.Time
}

// Bodies stay small: a session plus at most 50 UUIDs or one cursor.
const maxBody = 16 << 10

// maxCursor bounds the opaque cursor accepted from clients.
const maxCursor = 512

var (
	errInvalid      = errors.New("notifications: invalid input")
	errUnauthorized = errors.New("notifications: authentication required")
	errUnavailable  = errors.New("notifications: service unavailable")
)

type session struct {
	FamilyID    string `json:"family_id"`
	AccessToken string `json:"access_token"`
}

func (h Handler) now() time.Time {
	if h.Clock != nil {
		return h.Clock()
	}
	return time.Now()
}

// RegisterRoutes mounts the API on r.
func (h Handler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/notifications/list", h.list)
	r.Post("/v1/notifications/read", h.read)
}

func fail(w http.ResponseWriter, r *http.Request, err error) {
	status, code := http.StatusServiceUnavailable, "notifications.unavailable"
	switch {
	case errors.Is(err, errUnauthorized):
		status, code = http.StatusUnauthorized, "notifications.auth-required"
	case errors.Is(err, errInvalid), errors.Is(err, domain.ErrInvalid):
		status, code = http.StatusBadRequest, "notifications.invalid"
	}
	httpapi.WriteError(w, r, status, code, "notification request unavailable or refused", nil)
}

func write(w http.ResponseWriter, r *http.Request, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		fail(w, r, errUnavailable)
		return
	}
	w.Header().Del("ETag")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

// decode reads one bounded strict JSON object and restores the body so
// the signed-request verifier sees the exact transmitted bytes.
func decode(r *http.Request, v any) error {
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" {
		return errInvalid
	}
	raw, err := httpapi.ReadBody(r, maxBody)
	if err != nil || httpapi.DecodeJSON(raw, v) != nil {
		return errInvalid
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	r.ContentLength = int64(len(raw))
	return nil
}

func validSession(s session) bool {
	return len(s.FamilyID) == 36 && len(s.AccessToken) >= 1 && len(s.AccessToken) <= 512
}

// principal resolves the account; every session or proof failure is the
// same generic authentication error.
func (h Handler) principal(r *http.Request, s session, signed bool) (string, error) {
	if h.Service == nil || h.Sessions == nil {
		return "", errUnavailable
	}
	owner, err := h.Sessions(r.Context(), s.FamilyID, s.AccessToken)
	if err != nil || !domain.IsUUID(owner) {
		return "", errUnauthorized
	}
	if signed {
		if h.SignedOwner == nil {
			return "", errUnauthorized
		}
		if _, _, err := h.SignedOwner(r, owner); err != nil {
			return "", errUnauthorized
		}
	}
	return owner, nil
}

func (h Handler) filter(owner string) string {
	return httpapi.FilterHash("notifications-page-v1", h.Origin, owner)
}

type listResponse struct {
	Items      []application.Item `json:"items"`
	NextBefore *string            `json:"next_before"`
	Unread     int64              `json:"unread"`
}

func (h Handler) list(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		session
		Limit  *int    `json:"limit,omitempty"`
		Before *string `json:"before,omitempty"`
	}
	if err := decode(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	limit, err := domain.Limit(dto.Limit)
	if err != nil || !validSession(dto.session) || (dto.Before != nil && (*dto.Before == "" || len(*dto.Before) > maxCursor)) {
		fail(w, r, errInvalid)
		return
	}
	owner, err := h.principal(r, dto.session, false)
	if err != nil {
		fail(w, r, err)
		return
	}
	if len(h.CursorSecret) < 32 {
		fail(w, r, errUnavailable)
		return
	}
	now := h.now()
	var before *domain.Position
	if dto.Before != nil {
		key, err := httpapi.Open(h.CursorSecret, *dto.Before, h.filter(owner), now)
		if err != nil {
			fail(w, r, errInvalid)
			return
		}
		position, err := domain.DecodePosition(key)
		if err != nil {
			fail(w, r, errInvalid)
			return
		}
		before = &position
	}
	page, err := h.Service.List(r.Context(), owner, before, limit)
	if err != nil {
		fail(w, r, err)
		return
	}
	out := listResponse{Items: page.Items, Unread: page.Unread}
	if out.Items == nil {
		out.Items = []application.Item{}
	}
	if page.Next != nil {
		next := httpapi.Seal(h.CursorSecret, h.filter(owner), domain.EncodePosition(*page.Next), now)
		out.NextBefore = &next
	}
	write(w, r, out)
}

func (h Handler) read(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		session
		OperationID string   `json:"operation_id"`
		IDs         []string `json:"notification_ids,omitempty"`
		All         *bool    `json:"all,omitempty"`
	}
	if err := decode(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	if !validSession(dto.session) || !domain.IsUUID(dto.OperationID) || (dto.All != nil && !*dto.All) {
		fail(w, r, errInvalid)
		return
	}
	sel, err := domain.NewSelection(dto.IDs, dto.All != nil)
	if err != nil {
		fail(w, r, errInvalid)
		return
	}
	owner, err := h.principal(r, dto.session, true)
	if err != nil {
		fail(w, r, err)
		return
	}
	marked, err := h.Service.MarkRead(r.Context(), owner, sel)
	if err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, struct {
		Marked int64 `json:"marked"`
	}{marked})
}
