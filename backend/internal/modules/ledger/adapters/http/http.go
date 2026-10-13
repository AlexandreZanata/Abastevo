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

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
)

type Service interface {
	Apply(ctx context.Context, accountID string, batch application.Batch) (application.BatchResult, error)
	List(ctx context.Context, accountID, dataset, vehicleID, category, from, to, afterTime, afterID string, limit int) ([]application.Record, error)
	Delete(ctx context.Context, accountID, dataset, id string) (int64, error)
}

type Handler struct {
	Service      Service
	Sessions     func(context.Context, string, string) (string, error)
	SignedOwner  func(*http.Request, string) (string, string, error)
	CursorSecret []byte
	Origin       string
}

type session struct {
	FamilyID    string `json:"family_id"`
	AccessToken string `json:"access_token"`
}

func fail(w http.ResponseWriter, r *http.Request, err error) {
	status, code := 503, "unavailable"
	switch {
	case isInvalid(err):
		status, code = 400, "invalid"
	case isUnauthorized(err):
		status, code = 401, "auth-required"
	case isDenied(err):
		status, code = 403, "capability-required"
	case isNotFound(err):
		status, code = 404, "not-found"
	case isConflict(err):
		status, code = 409, "conflict"
	}
	httpapi.WriteError(w, r, status, "ledger."+code, "private ledger request unavailable or refused", nil)
}

func isInvalid(err error) bool {
	return err != nil && (contains(err, "invalid") || contains(err, "Invalid"))
}
func isUnauthorized(err error) bool { return err != nil && contains(err, "authentication") }
func isDenied(err error) bool       { return err != nil && contains(err, "capability") }
func isNotFound(err error) bool {
	return err != nil && contains(err, "unavailable") && contains(err, "record")
}
func isConflict(err error) bool { return err != nil && contains(err, "conflict") }

func contains(err error, sub string) bool {
	for err != nil {
		if msg := err.Error(); len(msg) >= len(sub) && (func() bool {
			for i := 0; i+len(sub) <= len(msg); i++ {
				if msg[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})() {
			return true
		}
		err = errors.Unwrap(err)
	}
	return false
}

func write(w http.ResponseWriter, r *http.Request, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		fail(w, r, err)
		return
	}
	w.Header().Del("ETag")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	_, _ = w.Write(raw)
}

func read(r *http.Request, v any) error {
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" {
		return errors.New("ledger: invalid input")
	}
	raw, err := httpapi.ReadBody(r, 65536)
	if err != nil || httpapi.DecodeJSON(raw, v) != nil {
		return errors.New("ledger: invalid input")
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	r.ContentLength = int64(len(raw))
	return nil
}

func (h Handler) principal(r *http.Request, s session, signed bool) (string, error) {
	if len(s.FamilyID) != 36 || len(s.AccessToken) < 1 || len(s.AccessToken) > 512 {
		return "", errors.New("ledger: invalid input")
	}
	if h.Service == nil || h.Sessions == nil {
		return "", errors.New("ledger: service unavailable")
	}
	owner, err := h.Sessions(r.Context(), s.FamilyID, s.AccessToken)
	if err != nil {
		return "", err
	}
	if len(owner) != 36 {
		return "", errors.New("ledger: authentication required")
	}
	if signed {
		if h.SignedOwner == nil {
			return "", errors.New("ledger: authentication required")
		}
		if _, _, err = h.SignedOwner(r, owner); err != nil {
			return "", err
		}
	}
	return owner, nil
}

func (h Handler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/ledger/commands", h.commands)
	r.Post("/v1/ledger/records", h.records)
	r.Post("/v1/ledger/delete", h.delete)
}

func (h Handler) commands(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		session
		application.Batch
	}
	if err := read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	owner, err := h.principal(r, dto.session, true)
	if err != nil {
		fail(w, r, err)
		return
	}
	got, err := h.Service.Apply(r.Context(), owner, dto.Batch)
	if err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, got)
}

func (h Handler) records(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		session
		Dataset  string `json:"dataset"`
		Vehicle  string `json:"vehicle_id,omitempty"`
		Category string `json:"category,omitempty"`
		From     string `json:"from,omitempty"`
		To       string `json:"to,omitempty"`
		Limit    *int   `json:"limit,omitempty"`
		Cursor   string `json:"cursor,omitempty"`
	}
	if err := read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	limit := 20
	if dto.Limit != nil {
		limit = *dto.Limit
	}
	if limit < 1 || limit > 100 || len(dto.Cursor) > 2048 {
		fail(w, r, errors.New("ledger: invalid input"))
		return
	}
	owner, err := h.principal(r, dto.session, false)
	if err != nil {
		fail(w, r, err)
		return
	}
	if len(h.CursorSecret) < 32 {
		fail(w, r, errors.New("ledger: service unavailable"))
		return
	}
	filter := httpapi.FilterHash("ledger-owner-page-v1", h.Origin, owner, dto.Dataset, dto.Vehicle, dto.Category, dto.From, dto.To)
	afterTime, afterID := "", ""
	if dto.Cursor != "" {
		last, err := httpapi.Open(h.CursorSecret, dto.Cursor, filter, time.Now())
		if err != nil {
			fail(w, r, errors.New("ledger: invalid input"))
			return
		}
		parts := splitCursor(last)
		afterTime, afterID = parts[0], parts[1]
	}
	rows, err := h.Service.List(r.Context(), owner, dto.Dataset, dto.Vehicle, dto.Category, dto.From, dto.To, afterTime, afterID, limit+1)
	if err != nil {
		fail(w, r, err)
		return
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		next = httpapi.Seal(h.CursorSecret, filter, last.Occurred+"|"+last.ID, time.Now())
	}
	if rows == nil {
		rows = []application.Record{}
	}
	write(w, r, struct {
		Items []application.Record `json:"items"`
		Next  string               `json:"next_cursor"`
	}{rows, next})
}

func splitCursor(last string) [2]string {
	for i := 0; i < len(last); i++ {
		if last[i] == '|' {
			return [2]string{last[:i], last[i+1:]}
		}
	}
	return [2]string{"", last}
}

func (h Handler) delete(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		session
		Dataset string `json:"dataset"`
		ID      string `json:"id"`
	}
	if err := read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	owner, err := h.principal(r, dto.session, true)
	if err != nil {
		fail(w, r, err)
		return
	}
	revision, err := h.Service.Delete(r.Context(), owner, dto.Dataset, dto.ID)
	if err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, struct {
		Revision int64 `json:"revision"`
	}{revision})
}
