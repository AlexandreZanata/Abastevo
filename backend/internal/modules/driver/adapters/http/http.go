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

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
)

type EarningsPageStore interface {
	List(context.Context, string, string, int) (app.Page, error)
}

type ReportService interface {
	Build(context.Context, string, app.ReportRequest) (app.Report, error)
	Get(context.Context, string, app.ReportRequest) (app.Report, error)
}

type Handler struct {
	Earnings     EarningsPageStore
	Reports      ReportService
	Sessions     func(context.Context, string, string) (string, error)
	SignedOwner  func(*http.Request, string) (string, string, error)
	CursorSecret []byte
	Origin       string
	Enabled      func() bool
}

type session struct {
	FamilyID    string `json:"family_id"`
	AccessToken string `json:"access_token"`
}

func (h Handler) enabled() bool { return h.Enabled == nil || h.Enabled() }

func fail(w http.ResponseWriter, r *http.Request, err error) {
	status, code := 503, "unavailable"
	switch {
	case errors.Is(err, app.ErrSignedProof):
		status, code = 401, "auth-required"
	case errors.Is(err, app.ErrDenied):
		status, code = 403, "capability-required"
	case errors.Is(err, app.ErrStale):
		status, code = 409, "stale"
	case errors.Is(err, app.ErrReportTooLarge):
		status, code = 409, "report-too-large"
	case errors.Is(err, domain.ErrInvalid):
		status, code = 400, "invalid"
	case errors.Is(err, app.ErrNotFound):
		status, code = 404, "not-found"
	case errors.Is(err, domain.ErrConflict):
		status, code = 409, "conflict"
	}
	httpapi.WriteError(w, r, status, "driver."+code, "private driver request unavailable or refused", nil)
}

func (h Handler) read(r *http.Request, v any) error {
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

func (h Handler) principal(r *http.Request, s session) (string, error) {
	if len(s.FamilyID) != 36 || len(s.AccessToken) < 1 || len(s.AccessToken) > 512 {
		return "", domain.ErrInvalid
	}
	if h.Sessions == nil || h.SignedOwner == nil {
		return "", app.ErrSignedProof
	}
	owner, err := h.Sessions(r.Context(), s.FamilyID, s.AccessToken)
	if err != nil || len(owner) != 36 {
		return "", app.ErrSignedProof
	}
	if _, _, err = h.SignedOwner(r, owner); err != nil {
		return "", app.ErrSignedProof
	}
	return owner, nil
}

func (h Handler) write(w http.ResponseWriter, r *http.Request, v any) {
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

func (h Handler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/driver/earnings", h.earnings)
	r.Post("/v1/driver/reports/build", h.reportBuild)
	r.Post("/v1/driver/reports/get", h.reportGet)
}

type earningItem struct {
	ID                  string          `json:"id"`
	VehicleID           string          `json:"vehicle_id"`
	Platform            string          `json:"platform"`
	Coverage            string          `json:"coverage"`
	Currency            string          `json:"currency"`
	From                string          `json:"from"`
	To                  string          `json:"to"`
	Timezone            string          `json:"timezone"`
	Money               domain.Earnings `json:"money"`
	Revision            int64           `json:"revision"`
	AdjustmentCount     int64           `json:"adjustment_count"`
	NeedsReconciliation bool            `json:"needs_reconciliation"`
}

func (h Handler) earnings(w http.ResponseWriter, r *http.Request) {
	if !h.enabled() {
		httpapi.WriteError(w, r, 404, "driver.disabled", "private driver routes are disabled", nil)
		return
	}
	var dto struct {
		session
		Limit  *int   `json:"limit,omitempty"`
		Cursor string `json:"cursor,omitempty"`
	}
	if err := h.read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	limit := 20
	if dto.Limit != nil {
		limit = *dto.Limit
	}
	if limit < 1 || limit > 100 || len(dto.Cursor) > 2048 || h.Earnings == nil {
		fail(w, r, domain.ErrInvalid)
		return
	}
	owner, err := h.principal(r, dto.session)
	if err != nil {
		fail(w, r, err)
		return
	}
	if len(h.CursorSecret) < 32 {
		fail(w, r, app.ErrUnavailable)
		return
	}
	filter := httpapi.FilterHash("driver-earnings-page-v1", h.Origin, owner)
	after := ""
	if dto.Cursor != "" {
		last, err := httpapi.Open(h.CursorSecret, dto.Cursor, filter, time.Now())
		if err != nil || !domain.ValidID(last) {
			fail(w, r, domain.ErrInvalid)
			return
		}
		after = last
	}
	page, err := h.Earnings.List(r.Context(), owner, after, limit+1)
	if err != nil {
		fail(w, r, err)
		return
	}
	next := ""
	if len(page.Records) > limit {
		page.Records = page.Records[:limit]
		next = httpapi.Seal(h.CursorSecret, filter, page.Records[len(page.Records)-1].Record.ID, time.Now())
	}
	items := make([]earningItem, len(page.Records))
	for i, stored := range page.Records {
		record := stored.Record
		items[i] = earningItem{
			ID: record.ID, VehicleID: record.VehicleID, Platform: record.Platform, Coverage: record.Coverage, Currency: record.Currency,
			From: record.From.UTC().Format(time.RFC3339), To: record.To.UTC().Format(time.RFC3339), Timezone: record.Timezone,
			Money: record.Money, Revision: stored.Revision, AdjustmentCount: stored.AdjustmentCount, NeedsReconciliation: stored.NeedsReconciliation,
		}
	}
	h.write(w, r, struct {
		Items []earningItem `json:"items"`
		Next  string        `json:"next_cursor"`
	}{items, next})
}

func (h Handler) reportBuild(w http.ResponseWriter, r *http.Request) {
	if !h.enabled() {
		httpapi.WriteError(w, r, 404, "driver.disabled", "private driver routes are disabled", nil)
		return
	}
	var dto struct {
		session
		app.ReportRequest
	}
	if err := h.read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	owner, err := h.principal(r, dto.session)
	if err != nil {
		fail(w, r, err)
		return
	}
	if h.Reports == nil {
		fail(w, r, app.ErrUnavailable)
		return
	}
	report, err := h.Reports.Build(r.Context(), owner, dto.ReportRequest)
	if err != nil {
		fail(w, r, err)
		return
	}
	h.write(w, r, report)
}

func (h Handler) reportGet(w http.ResponseWriter, r *http.Request) {
	if !h.enabled() {
		httpapi.WriteError(w, r, 404, "driver.disabled", "private driver routes are disabled", nil)
		return
	}
	var dto struct {
		session
		app.ReportRequest
	}
	if err := h.read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	owner, err := h.principal(r, dto.session)
	if err != nil {
		fail(w, r, err)
		return
	}
	if h.Reports == nil {
		fail(w, r, app.ErrUnavailable)
		return
	}
	report, err := h.Reports.Get(r.Context(), owner, dto.ReportRequest)
	if err != nil {
		fail(w, r, err)
		return
	}
	h.write(w, r, report)
}
