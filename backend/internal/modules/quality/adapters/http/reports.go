package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"io"
	"mime"
	"net/http"
	"strconv"
)

type ReportService interface {
	Execute(context.Context, adapters.ReportAccess, domain.ReportCommand) (domain.ReportReceipt, error)
	Get(context.Context, adapters.ReportAccess, string) (domain.Report, error)
	List(context.Context, adapters.ReportAccess, string) ([]domain.Report, error)
}

type ReportsHandler struct {
	Service     ReportService
	Sessions    func(context.Context, string, string) (string, error)
	SignedOwner func(*http.Request, string) (string, string, error)
	Guard       func(context.Context, pgx.Tx, string, string, string, string, string) error
	Enabled     func() bool
}
type reportSession struct {
	FamilyID    string `json:"family_id"`
	AccessToken string `json:"access_token"`
}

func (h ReportsHandler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/quality/reports/commands", h.command)
	r.Post("/v1/quality/reports/get", h.get)
	r.Post("/v1/quality/reports/list", h.list)
}

func reportFail(w http.ResponseWriter, r *http.Request, e error) {
	status, code := 503, "unavailable"
	var quota *domain.ReportQuotaError
	switch {
	case errors.As(e, &quota):
		status, code = 429, "quota"
		w.Header().Set("Retry-After", strconv.Itoa(quota.RetryAfterSeconds))
	case errors.Is(e, domain.ErrReportInvalid):
		status, code = 400, "invalid"
	case errors.Is(e, domain.ErrReportUnauthorized):
		status, code = 401, "auth-required"
	case errors.Is(e, domain.ErrReportNotFound):
		status, code = 404, "not-found"
	case errors.Is(e, domain.ErrReportConflict):
		status, code = 409, "conflict"
	}
	httpapi.WriteError(w, r, status, "quality.report-"+code, "quality report request unavailable or refused", nil)
}
func reportRead(r *http.Request, v any) error {
	typ, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if e != nil || typ != "application/json" {
		return domain.ErrReportInvalid
	}
	b, e := httpapi.ReadBody(r, 16<<10)
	if e != nil || httpapi.DecodeJSON(b, v) != nil {
		return domain.ErrReportInvalid
	}
	r.Body = io.NopCloser(bytes.NewReader(b))
	r.ContentLength = int64(len(b))
	return nil
}
func (h ReportsHandler) principal(r *http.Request, s reportSession) (adapters.ReportAccess, error) {
	var out adapters.ReportAccess
	if h.Sessions == nil || h.SignedOwner == nil || h.Guard == nil || h.Service == nil {
		return out, domain.ErrReportUnavailable
	}
	if len(s.FamilyID) != 36 || s.AccessToken == "" || len(s.AccessToken) > 512 {
		return out, domain.ErrReportUnauthorized
	}
	owner, e := h.Sessions(r.Context(), s.FamilyID, s.AccessToken)
	if e != nil {
		return out, e
	}
	contributor, key, e := h.SignedOwner(r, owner)
	if e != nil {
		return out, e
	}
	return adapters.ReportAccess{Owner: owner, Guard: func(ctx context.Context, tx pgx.Tx) error {
		return h.Guard(ctx, tx, owner, s.FamilyID, s.AccessToken, contributor, key)
	}}, nil
}
func reportWrite(w http.ResponseWriter, r *http.Request, v any) {
	b, e := json.Marshal(v)
	if e != nil {
		reportFail(w, r, e)
		return
	}
	httpapi.WriteJSON(w, r, 200, "no-store", b)
}
func (h ReportsHandler) command(w http.ResponseWriter, r *http.Request) {
	// The intake switch affects commands only; owner recovery reads remain live.
	var dto struct {
		reportSession
		domain.ReportCommand
	}
	if e := reportRead(r, &dto); e != nil {
		reportFail(w, r, e)
		return
	}
	if dto.Action != "WITHDRAW" && dto.Action != "APPEAL" && h.Enabled != nil && !h.Enabled() {
		reportFail(w, r, domain.ErrReportUnavailable)
		return
	}
	a, e := h.principal(r, dto.reportSession)
	if e != nil {
		reportFail(w, r, e)
		return
	}
	out, e := h.Service.Execute(r.Context(), a, dto.ReportCommand)
	if e != nil {
		reportFail(w, r, e)
		return
	}
	reportWrite(w, r, out)
}
func (h ReportsHandler) get(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		reportSession
		ReportID string `json:"report_id"`
	}
	if e := reportRead(r, &dto); e != nil {
		reportFail(w, r, e)
		return
	}
	a, e := h.principal(r, dto.reportSession)
	if e != nil {
		reportFail(w, r, e)
		return
	}
	out, e := h.Service.Get(r.Context(), a, dto.ReportID)
	if e != nil {
		reportFail(w, r, e)
		return
	}
	reportWrite(w, r, out)
}
func (h ReportsHandler) list(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		reportSession
		AfterID string `json:"after_id,omitempty"`
	}
	if e := reportRead(r, &dto); e != nil {
		reportFail(w, r, e)
		return
	}
	a, e := h.principal(r, dto.reportSession)
	if e != nil {
		reportFail(w, r, e)
		return
	}
	out, e := h.Service.List(r.Context(), a, dto.AfterID)
	if e != nil {
		reportFail(w, r, e)
		return
	}
	next := ""
	if len(out) > 50 {
		out = out[:50]
		next = out[49].ID
	}
	reportWrite(w, r, struct {
		Items []domain.Report `json:"items"`
		Next  string          `json:"next_after_id,omitempty"`
	}{out, next})
}
