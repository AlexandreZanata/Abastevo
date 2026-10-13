package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"
)

type Service interface {
	Status(context.Context, app.Principal) (domain.Status, error)
	Admit(context.Context, app.Principal, app.Command) (app.Receipt, error)
	Export(context.Context, app.Principal, int, time.Time, string) ([]app.ExportRow, error)
}
type Handler struct {
	Service      Service
	Sessions     func(context.Context, string, string) (string, error)
	SignedOwner  func(*http.Request, string) (string, string, error)
	CursorSecret []byte
}
type sessionRequest struct {
	FamilyID    string `json:"family_id"`
	AccessToken string `json:"access_token"`
}
type purchaseRequest struct {
	FamilyID    string `json:"family_id"`
	AccessToken string `json:"access_token"`
	OperationID string `json:"operation_id"`
	Token       string `json:"purchase_token"`
}

func (h Handler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/premium/status", h.status)
	r.Post("/v1/premium/purchases/verify", func(w http.ResponseWriter, r *http.Request) { h.purchase(w, r, "verify") })
	r.Post("/v1/premium/purchases/restore", func(w http.ResponseWriter, r *http.Request) { h.purchase(w, r, "restore") })
	r.Post("/v1/premium/export", h.export)
}
func premiumError(w http.ResponseWriter, r *http.Request, err error) {
	status, code := 503, "unavailable"
	switch {
	case errors.Is(err, domain.ErrInvalid):
		status, code = 400, "invalid"
	case errors.Is(err, domain.ErrUnauthorized):
		status, code = 401, "auth-required"
	case errors.Is(err, domain.ErrDenied):
		status, code = 403, "proof-denied"
	case errors.Is(err, domain.ErrNotFound):
		status, code = 404, "not-found"
	case errors.Is(err, domain.ErrConflict):
		status, code = 409, "conflict"
	case errors.Is(err, domain.ErrStale), errors.Is(err, domain.ErrBusy):
		status, code = 409, "retry-verification"
	case errors.Is(err, domain.ErrQuota):
		status, code = 429, "quota"
		w.Header().Set("Retry-After", "60")
	}
	httpapi.WriteError(w, r, status, "subscription."+code, "subscription request unavailable or refused", nil)
}
func readPremium(r *http.Request, dst any) ([]byte, error) {
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" {
		return nil, domain.ErrInvalid
	}
	raw, err := httpapi.ReadBody(r, 8192)
	if err != nil || httpapi.DecodeJSON(raw, dst) != nil {
		return nil, domain.ErrInvalid
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	r.ContentLength = int64(len(raw))
	return raw, nil
}
func (h Handler) principal(ctx context.Context, family, access string) (app.Principal, error) {
	if !domain.ValidID(family) || len(access) == 0 || len(access) > 512 {
		return app.Principal{}, domain.ErrInvalid
	}
	if h.Sessions == nil || h.Service == nil {
		return app.Principal{}, domain.ErrUnavailable
	}
	account, err := h.Sessions(ctx, family, access)
	if err != nil {
		return app.Principal{}, err
	}
	if !domain.ValidID(account) {
		return app.Principal{}, domain.ErrUnauthorized
	}
	return app.Principal{AccountID: account, FamilyID: family}, nil
}
func writePremium(w http.ResponseWriter, r *http.Request, value any) {
	raw, err := json.Marshal(value)
	if err != nil {
		premiumError(w, r, domain.ErrUnavailable)
		return
	}
	w.Header().Del("ETag")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}
func (h Handler) status(w http.ResponseWriter, r *http.Request) {
	var dto sessionRequest
	if _, err := readPremium(r, &dto); err != nil {
		premiumError(w, r, err)
		return
	}
	p, err := h.principal(r.Context(), dto.FamilyID, dto.AccessToken)
	if err != nil {
		premiumError(w, r, err)
		return
	}
	status, err := h.Service.Status(r.Context(), p)
	if err != nil {
		premiumError(w, r, err)
		return
	}
	writePremium(w, r, struct {
		domain.Status
		ManagementURL string `json:"management_url"`
	}{status, "https://play.google.com/store/account/subscriptions"})
}
func (h Handler) purchase(w http.ResponseWriter, r *http.Request, kind string) {
	var dto purchaseRequest
	if _, err := readPremium(r, &dto); err != nil {
		premiumError(w, r, err)
		return
	}
	if !domain.ValidID(dto.OperationID) || !domain.ValidToken(dto.Token) {
		premiumError(w, r, domain.ErrInvalid)
		return
	}
	p, err := h.principal(r.Context(), dto.FamilyID, dto.AccessToken)
	if err != nil {
		premiumError(w, r, err)
		return
	}
	if h.SignedOwner == nil {
		premiumError(w, r, domain.ErrUnauthorized)
		return
	}
	p.ContributorID, p.KeyID, err = h.SignedOwner(r, p.AccountID)
	if err != nil {
		premiumError(w, r, err)
		return
	}
	receipt, err := h.Service.Admit(r.Context(), p, app.Command{OperationID: dto.OperationID, Token: dto.Token, Kind: kind})
	if err != nil {
		premiumError(w, r, err)
		return
	}
	writePremium(w, r, receipt)
}
func (h Handler) export(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		FamilyID    string `json:"family_id"`
		AccessToken string `json:"access_token"`
		Limit       *int   `json:"limit,omitempty"`
		Cursor      string `json:"cursor,omitempty"`
	}
	if _, err := readPremium(r, &dto); err != nil {
		premiumError(w, r, err)
		return
	}
	limit := 20
	if dto.Limit != nil {
		limit = *dto.Limit
	}
	if limit < 1 || limit > 100 || len(dto.Cursor) > 2048 {
		premiumError(w, r, domain.ErrInvalid)
		return
	}
	p, err := h.principal(r.Context(), dto.FamilyID, dto.AccessToken)
	if err != nil {
		premiumError(w, r, err)
		return
	}
	if len(h.CursorSecret) < 32 {
		premiumError(w, r, domain.ErrUnavailable)
		return
	}
	filter := httpapi.FilterHash("premium-export-v1", p.AccountID)
	var after time.Time
	var key string
	if dto.Cursor != "" {
		position, e := httpapi.Open(h.CursorSecret, dto.Cursor, filter, time.Now())
		if e != nil {
			premiumError(w, r, domain.ErrInvalid)
			return
		}
		stamp, digest, ok := strings.Cut(position, ",")
		if !ok {
			premiumError(w, r, domain.ErrInvalid)
			return
		}
		after, err = time.Parse(time.RFC3339Nano, stamp)
		if err != nil {
			premiumError(w, r, domain.ErrInvalid)
			return
		}
		key = digest
	}
	rows, err := h.Service.Export(r.Context(), p, limit+1, after, key)
	if err != nil {
		premiumError(w, r, err)
		return
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		next = httpapi.Seal(h.CursorSecret, filter, last.VerifiedAt.UTC().Format(time.RFC3339Nano)+","+last.Digest, time.Now())
	}
	if rows == nil {
		rows = []app.ExportRow{}
	}
	writePremium(w, r, struct {
		Items []app.ExportRow `json:"items"`
		Next  string          `json:"next_cursor"`
	}{rows, next})
}
