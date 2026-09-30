// Package http exposes the FREE account contract through real request
// binding (P13-T02C). Every account response is no-store; error envelopes
// carry stable machine codes and static messages, never addresses, codes
// or tokens. Code issuance answers 202 identically for unknown addresses,
// cooldown hits and quota exhaustion, so no oracle leaks registration.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// Service is the account use-case port behind the handlers.
type Service interface {
	RequestCode(ctx context.Context, address string) error
	ConsumeCode(ctx context.Context, address, code string) (application.AuthResult, error)
	Refresh(ctx context.Context, familyID, refreshToken string) (application.Session, error)
	ValidateAccess(ctx context.Context, familyID, accessToken string) (string, error)
	RevokeAll(ctx context.Context, accountID string) error
}

// Handler serves the account routes with an injected service.
type Handler struct {
	Service Service
}

// RegisterRoutes mounts the additive account paths.
func (h Handler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/accounts/email/codes", h.requestCode)
	r.Post("/v1/accounts/email/consume", h.consumeCode)
	r.Post("/v1/accounts/sessions/refresh", h.refresh)
	r.Post("/v1/accounts/sessions/revoke", h.revoke)
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
	status, code, msg := http.StatusUnauthorized, "account.code-unknown", "valid account proof required"
	switch domain.VerdictCode(err) {
	case "code-consumed":
		status, code, msg = http.StatusUnauthorized, "account.code-consumed", "code already used"
	case "code-expired":
		status, code, msg = http.StatusUnauthorized, "account.code-expired", "code expired"
	case "code-attempts-exhausted":
		status, code, msg = http.StatusUnauthorized, "account.code-attempts-exhausted", "code locked"
	case "session-reuse-revoked":
		status, code, msg = http.StatusUnauthorized, "account.session-reuse-revoked", "session revoked"
	case "session-revoked":
		status, code, msg = http.StatusUnauthorized, "account.session-revoked", "session revoked"
	case "session-expired":
		status, code, msg = http.StatusUnauthorized, "account.session-expired", "session expired"
	case "ok":
		status, code, msg = http.StatusServiceUnavailable, "account.unavailable", "account service unavailable"
	}
	httpapi.WriteError(w, r, status, code, msg, nil)
}

func bad(w http.ResponseWriter, r *http.Request) {
	httpapi.WriteError(w, r, 400, "account.bad-body", "malformed account request", nil)
}

func write(w http.ResponseWriter, r *http.Request, status int, body any) {
	raw, err := json.Marshal(body)
	if err != nil {
		httpapi.WriteError(w, r, 500, "account.unavailable", "account service unavailable", nil)
		return
	}
	httpapi.WriteJSON(w, r, status, "no-store", raw)
}

func validEmail(address string) bool {
	address = strings.TrimSpace(address)
	if len(address) == 0 || len(address) > 254 {
		return false
	}
	at := strings.IndexByte(address, '@')
	return at > 0 && at < len(address)-1 && !strings.Contains(address, " ")
}

func (h Handler) requestCode(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		Email string `json:"email"`
	}
	if err := read(r, &dto); err != nil || !validEmail(dto.Email) {
		bad(w, r)
		return
	}
	if err := h.Service.RequestCode(r.Context(), dto.Email); err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, http.StatusAccepted, map[string]string{"status": "sent"})
}

type accountJSON struct {
	AccountID string `json:"account_id"`
	Alias     string `json:"public_alias"`
	Status    string `json:"status"`
}

type sessionJSON struct {
	FamilyID        string `json:"family_id"`
	AccountID       string `json:"account_id"`
	AccessToken     string `json:"access_token"`
	RefreshToken    string `json:"refresh_token"`
	AccessExpires   string `json:"access_expires_at"`
	AbsoluteExpires string `json:"absolute_expires_at"`
}

func sessionJSONOf(s application.Session) sessionJSON {
	return sessionJSON{
		FamilyID:        s.FamilyID,
		AccountID:       s.AccountID,
		AccessToken:     s.AccessToken,
		RefreshToken:    s.RefreshToken,
		AccessExpires:   time.Unix(s.AccessExpires, 0).UTC().Format(time.RFC3339),
		AbsoluteExpires: time.Unix(s.AbsoluteExpires, 0).UTC().Format(time.RFC3339),
	}
}

func (h Handler) consumeCode(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := read(r, &dto); err != nil || !validEmail(dto.Email) || dto.Code == "" {
		bad(w, r)
		return
	}
	got, err := h.Service.ConsumeCode(r.Context(), dto.Email, dto.Code)
	if err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, http.StatusOK, map[string]any{
		"account": accountJSON{AccountID: got.Account.ID, Alias: got.Account.Alias, Status: got.Account.Status},
		"session": sessionJSONOf(got.Session),
		"created": got.Created,
	})
}

func (h Handler) refresh(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		FamilyID     string `json:"family_id"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := read(r, &dto); err != nil || dto.FamilyID == "" || dto.RefreshToken == "" {
		bad(w, r)
		return
	}
	sess, err := h.Service.Refresh(r.Context(), dto.FamilyID, dto.RefreshToken)
	if err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, http.StatusOK, map[string]any{"session": sessionJSONOf(sess)})
}

func (h Handler) revoke(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		FamilyID    string `json:"family_id"`
		AccessToken string `json:"access_token"`
	}
	if err := read(r, &dto); err != nil || dto.FamilyID == "" || dto.AccessToken == "" {
		bad(w, r)
		return
	}
	accountID, err := h.Service.ValidateAccess(r.Context(), dto.FamilyID, dto.AccessToken)
	if err != nil {
		fail(w, r, err)
		return
	}
	if err := h.Service.RevokeAll(r.Context(), accountID); err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, http.StatusOK, map[string]string{"status": "revoked"})
}
