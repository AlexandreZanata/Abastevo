package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"
)

type Service interface {
	app.ReadStore
	app.DeviceStore
	app.BatchStore
	app.PullStore
	app.ImportStore
	app.SnapshotStore
	app.BackupStore
	app.SnapshotStorageStore
	app.RestoreStore
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
	case errors.Is(err, domain.ErrInvalid):
		status, code = 400, "invalid"
	case errors.Is(err, domain.ErrUnauthorized):
		status, code = 401, "auth-required"
	case errors.Is(err, domain.ErrDenied):
		status, code = 403, "capability-or-consent-required"
	case errors.Is(err, domain.ErrNotFound):
		status, code = 404, "not-found"
	case errors.Is(err, domain.ErrConflict):
		status, code = 409, "conflict"
	case errors.Is(err, domain.ErrResnapshot):
		status, code = 409, "resnapshot-required"
	case errors.Is(err, domain.ErrQuota):
		status, code = 429, "quota"
		w.Header().Set("Retry-After", "60")
	}
	httpapi.WriteError(w, r, status, "cloud."+code, "private cloud request unavailable or refused", nil)
}
func write(w http.ResponseWriter, r *http.Request, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		fail(w, r, domain.ErrUnavailable)
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
		return domain.ErrInvalid
	}
	raw, err := httpapi.ReadBody(r, 65536)
	if err != nil || httpapi.DecodeJSON(raw, v) != nil {
		return domain.ErrInvalid
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	r.ContentLength = int64(len(raw))
	return nil
}
func (h Handler) principal(r *http.Request, s session, signed bool) (app.Principal, error) {
	if !domain.ValidID(s.FamilyID) || len(s.AccessToken) < 1 || len(s.AccessToken) > 512 {
		return app.Principal{}, domain.ErrInvalid
	}
	if h.Service == nil || h.Sessions == nil {
		return app.Principal{}, domain.ErrUnavailable
	}
	owner, err := h.Sessions(r.Context(), s.FamilyID, s.AccessToken)
	if err != nil {
		return app.Principal{}, err
	}
	if !domain.ValidID(owner) {
		return app.Principal{}, domain.ErrUnauthorized
	}
	p := app.Principal{AccountID: owner, FamilyID: s.FamilyID}
	if signed {
		if h.SignedOwner == nil {
			return p, domain.ErrUnauthorized
		}
		p.ContributorID, p.KeyID, err = h.SignedOwner(r, owner)
		if err != nil {
			return p, err
		}
	}
	return p, nil
}
func (h Handler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/cloud/status", h.status)
	r.Post("/v1/cloud/inventory", h.inventory)
	r.Post("/v1/cloud/records", h.records)
	r.Post("/v1/cloud/export", h.records)
	r.Post("/v1/cloud/devices", h.registerDevice)
	r.Post("/v1/cloud/devices/list", h.devices)
	r.Post("/v1/cloud/devices/revoke", h.revokeDevice)
	r.Post("/v1/cloud/commands", h.commands)
	r.Post("/v1/cloud/erase", h.erase)
	r.Post("/v1/cloud/sync/pull", h.syncPull)
	r.Post("/v1/cloud/sync/push", h.syncPush)
	r.Post("/v1/cloud/consent", h.consent)
	r.Post("/v1/cloud/import/preview", h.importPreview)
	r.Post("/v1/cloud/sync/snapshot", h.syncSnapshot)
	r.Post("/v1/cloud/snapshots/manifest", h.backupManifest)
	r.Post("/v1/cloud/snapshots/list", h.snapshotsList)
	r.Post("/v1/cloud/restore/preview", h.restorePreview)
}
func (h Handler) status(w http.ResponseWriter, r *http.Request) {
	var dto session
	if err := read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	p, err := h.principal(r, dto, false)
	if err != nil {
		fail(w, r, err)
		return
	}
	got, err := h.Service.Status(r.Context(), p)
	if err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, got)
}
func (h Handler) inventory(w http.ResponseWriter, r *http.Request) {
	var dto session
	if err := read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	p, err := h.principal(r, dto, false)
	if err != nil {
		fail(w, r, err)
		return
	}
	if _, err = h.Service.Status(r.Context(), p); err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, struct {
		Version  int      `json:"version"`
		Scope    string   `json:"scope"`
		Datasets []string `json:"datasets"`
	}{1, "cloud", []string{"vehicles", "favorites", "preferences", "devices"}})
}

type pageRequest struct {
	session
	Dataset string `json:"dataset,omitempty"`
	Limit   *int   `json:"limit,omitempty"`
	Cursor  string `json:"cursor,omitempty"`
}

func (h Handler) page(r *http.Request, dto pageRequest, dataset string) (app.Principal, int, string, string, error) {
	limit := 20
	if dto.Limit != nil {
		limit = *dto.Limit
	}
	if limit < 1 || limit > 100 || len(dto.Cursor) > 2048 {
		return app.Principal{}, 0, "", "", domain.ErrInvalid
	}
	p, err := h.principal(r, dto.session, false)
	if err != nil {
		return p, 0, "", "", err
	}
	if len(h.CursorSecret) < 32 {
		return p, 0, "", "", domain.ErrUnavailable
	}
	state, err := h.Service.Status(r.Context(), p)
	if err != nil {
		return p, 0, "", "", err
	}
	filter := httpapi.FilterHash("cloud-owner-page-v1", h.Origin, p.AccountID, dataset, strconv.FormatInt(state.Epoch, 10))
	after := ""
	if dto.Cursor != "" {
		after, err = httpapi.Open(h.CursorSecret, dto.Cursor, filter, time.Now())
		if err != nil || !domain.ValidID(after) {
			return p, 0, "", "", domain.ErrInvalid
		}
	}
	return p, limit, after, filter, nil
}
func (h Handler) records(w http.ResponseWriter, r *http.Request) {
	var dto pageRequest
	if err := read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	p, limit, after, filter, err := h.page(r, dto, dto.Dataset)
	if err != nil {
		fail(w, r, err)
		return
	}
	rows, err := h.Service.List(r.Context(), p, dto.Dataset, limit+1, after)
	if err != nil {
		fail(w, r, err)
		return
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		next = httpapi.Seal(h.CursorSecret, filter, rows[len(rows)-1].ID, time.Now())
	}
	if rows == nil {
		rows = []app.Record{}
	}
	write(w, r, struct {
		Items []app.Record `json:"items"`
		Next  string       `json:"next_cursor"`
	}{rows, next})
}
func (h Handler) devices(w http.ResponseWriter, r *http.Request) {
	var dto pageRequest
	if err := read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	if dto.Dataset != "" {
		fail(w, r, domain.ErrInvalid)
		return
	}
	p, limit, after, filter, err := h.page(r, dto, "devices")
	if err != nil {
		fail(w, r, err)
		return
	}
	rows, err := h.Service.Devices(r.Context(), p, limit+1, after)
	if err != nil {
		fail(w, r, err)
		return
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		next = httpapi.Seal(h.CursorSecret, filter, rows[len(rows)-1].ID, time.Now())
	}
	if rows == nil {
		rows = []app.Device{}
	}
	write(w, r, struct {
		Items []app.Device `json:"items"`
		Next  string       `json:"next_cursor"`
	}{rows, next})
}
func (h Handler) registerDevice(w http.ResponseWriter, r *http.Request) {
	var dto session
	if err := read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	p, err := h.principal(r, dto, true)
	if err != nil {
		fail(w, r, err)
		return
	}
	got, err := h.Service.RegisterDevice(r.Context(), p)
	if err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, got)
}
func (h Handler) revokeDevice(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		session
		DeviceID string `json:"device_id"`
	}
	if err := read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	p, err := h.principal(r, dto.session, true)
	if err != nil {
		fail(w, r, err)
		return
	}
	if err = h.Service.RevokeDevice(r.Context(), p, dto.DeviceID); err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, map[string]string{"status": "revoked"})
}
func (h Handler) commands(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		session
		DeviceID string `json:"device_id"`
		app.Batch
	}
	if err := read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	p, err := h.principal(r, dto.session, true)
	if err != nil {
		fail(w, r, err)
		return
	}
	p.DeviceID = dto.DeviceID
	got, err := h.Service.Apply(r.Context(), p, dto.Batch)
	if err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, got)
}
func (h Handler) erase(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		session
		Dataset string `json:"dataset"`
		ID      string `json:"id,omitempty"`
	}
	if err := read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	p, err := h.principal(r, dto.session, true)
	if err != nil {
		fail(w, r, err)
		return
	}
	epoch, err := h.Service.Erase(r.Context(), p, dto.Dataset, dto.ID)
	if err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, struct {
		Status string `json:"status"`
		Epoch  int64  `json:"epoch"`
	}{"erased", epoch})
}
