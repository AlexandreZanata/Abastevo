// Package http exposes the FREE account contract through real request
// binding (P13-T02C, P13-T03C). Every account response is no-store; error
// envelopes carry stable machine codes and static messages, never
// addresses, codes, tokens or provider subjects. Code issuance answers
// 202 identically for unknown addresses, cooldown hits and quota
// exhaustion, so no oracle leaks registration. Provider link/unlink
// derive the caller account server-side from a live session; client
// account identifiers are never trusted.
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
	CreateKeyAccount(ctx context.Context, username string) (application.KeyAccount, error)
	LoginWithKey(ctx context.Context, rawKey string) (application.AuthResult, error)
	Refresh(ctx context.Context, familyID, refreshToken string) (application.Session, error)
	ValidateAccess(ctx context.Context, familyID, accessToken string) (string, error)
	RevokeAll(ctx context.Context, accountID string) error
	LinkProvider(ctx context.Context, accountID, provider, rawToken, audience, nonce string) (domain.ProviderLink, error)
	UnlinkProvider(ctx context.Context, accountID, provider string) error
	ListProviders(ctx context.Context, accountID string) ([]domain.ProviderLink, error)
	DeleteAccount(ctx context.Context, accountID string) error
	BindContributor(ctx context.Context, familyID, accessToken, contributorID, proof string) (domain.ContributorBinding, error)
	UnbindContributor(ctx context.Context, familyID, accessToken, contributorID string) error
	ListBindings(ctx context.Context, familyID, accessToken string) ([]domain.ContributorBinding, error)
}

// Handler serves the account routes with an injected service.
// Audience is the frozen server-side OIDC audience ("anpfuel-backend");
// client audience flags are untrusted and never read.
type Handler struct {
	Service  Service
	Audience string
}

// RegisterRoutes mounts the additive account paths.
func (h Handler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/accounts/email/codes", h.requestCode)
	r.Post("/v1/accounts/email/consume", h.consumeCode)
	r.Post("/v1/accounts/keys", h.createKeyAccount)
	r.Post("/v1/accounts/keys/login", h.loginWithKey)
	r.Post("/v1/accounts/sessions/refresh", h.refresh)
	r.Post("/v1/accounts/sessions/revoke", h.revoke)
	r.Post("/v1/accounts/providers/link", h.linkProvider)
	r.Post("/v1/accounts/providers/unlink", h.unlinkProvider)
	r.Post("/v1/accounts/providers/list", h.listProviders)
	r.Post("/v1/accounts/deletion", h.deleteAccount)
	r.Post("/v1/accounts/bindings/bind", h.bindContributor)
	r.Post("/v1/accounts/bindings/unbind", h.unbindContributor)
	r.Post("/v1/accounts/bindings/list", h.listBindings)
}

func (h Handler) audience() string {
	if h.Audience != "" {
		return h.Audience
	}
	return "anpfuel-backend"
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
	case "oidc-unknown-issuer":
		status, code, msg = http.StatusUnauthorized, "account.oidc-unknown-issuer", "provider proof rejected"
	case "oidc-wrong-audience":
		status, code, msg = http.StatusUnauthorized, "account.oidc-wrong-audience", "provider proof rejected"
	case "oidc-expired":
		status, code, msg = http.StatusUnauthorized, "account.oidc-expired", "provider proof expired"
	case "oidc-nonce-reused":
		status, code, msg = http.StatusUnauthorized, "account.oidc-nonce-reused", "provider proof already used"
	case "oidc-nonce-mismatch":
		status, code, msg = http.StatusUnauthorized, "account.oidc-nonce-mismatch", "provider proof rejected"
	case "oidc-unavailable":
		status, code, msg = http.StatusServiceUnavailable, "account.oidc-unavailable", "provider temporarily unavailable"
	case "link-cross-account-refused":
		status, code, msg = http.StatusForbidden, "account.link-cross-account-refused", "provider proof belongs to another account"
	case "link-email-only-refused":
		status, code, msg = http.StatusForbidden, "account.link-email-only-refused", "email match is not linking proof"
	case "link-last-method-refused":
		status, code, msg = http.StatusConflict, "account.link-last-method-refused", "last login method cannot be removed"
	case "link-provider-not-linked":
		status, code, msg = http.StatusNotFound, "account.link-provider-not-linked", "provider not linked"
	case "account-unknown":
		status, code, msg = http.StatusUnauthorized, "account.account-unknown", "valid account proof required"
	case "account-suspended":
		status, code, msg = http.StatusForbidden, "account.account-suspended", "account suspended"
	case "account-deleted":
		status, code, msg = http.StatusGone, "account.account-deleted", "account deleted"
	case "binding-cross-account-refused":
		status, code, msg = http.StatusForbidden, "account.binding-cross-account-refused", "contributor bound to another account"
	case "binding-not-found":
		status, code, msg = http.StatusNotFound, "account.binding-not-found", "contributor binding not found"
	case "binding-invalid":
		status, code, msg = http.StatusBadRequest, "account.binding-invalid", "malformed binding request"
	case "key-unavailable":
		status, code, msg = http.StatusServiceUnavailable, "account.key-unavailable", "key verifier unavailable"
	case "key-proof-denied":
		status, code, msg = http.StatusUnauthorized, "account.key-proof-denied", "key proof denied"
	case "username-taken":
		status, code, msg = http.StatusConflict, "account.username-taken", "username already taken"
	case "username-invalid":
		status, code, msg = http.StatusBadRequest, "account.username-invalid", "invalid username"
	case "key-invalid":
		status, code, msg = http.StatusUnauthorized, "account.key-invalid", "valid account key required"
	case "key-collision":
		status, code, msg = http.StatusServiceUnavailable, "account.unavailable", "account service unavailable"
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

// createKeyAccount mints an anonymous account for a username and returns
// the account key exactly once. The key never persists server-side beyond
// its salted verifier; the response is no-store like every account reply.
func (h Handler) createKeyAccount(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		Username string `json:"username"`
	}
	if err := read(r, &dto); err != nil || !domain.ValidUsername(dto.Username) {
		bad(w, r)
		return
	}
	got, err := h.Service.CreateKeyAccount(r.Context(), dto.Username)
	if err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, http.StatusCreated, map[string]any{
		"username":    got.Account.Alias,
		"account_key": got.Key,
	})
}

// loginWithKey opens a standard rotating session for the account key
// alone. Unknown, wrong and malformed keys share one 401 with no oracle.
func (h Handler) loginWithKey(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		AccountKey string `json:"account_key"`
	}
	if err := read(r, &dto); err != nil || dto.AccountKey == "" {
		bad(w, r)
		return
	}
	got, err := h.Service.LoginWithKey(r.Context(), dto.AccountKey)
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

type providerLinkJSON struct {
	Provider string `json:"provider"`
	Subject  string `json:"subject"`
	LinkedAt string `json:"linked_at"`
}

func providerLinkJSONOf(l domain.ProviderLink) providerLinkJSON {
	return providerLinkJSON{
		Provider: l.Provider,
		Subject:  l.Subject,
		LinkedAt: time.Unix(l.LinkedAt, 0).UTC().Format(time.RFC3339),
	}
}

func validProvider(provider string) bool {
	return provider == "google" || provider == "apple"
}

// linkProvider binds a verified provider subject to the session-owned
// account. The account derives server-side from the live session; client
// account identifiers are never trusted and the expected audience stays
// server-side.
func (h Handler) linkProvider(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		FamilyID    string `json:"family_id"`
		AccessToken string `json:"access_token"`
		Provider    string `json:"provider"`
		IDToken     string `json:"id_token"`
		Nonce       string `json:"nonce"`
	}
	if err := read(r, &dto); err != nil || dto.FamilyID == "" || dto.AccessToken == "" ||
		!validProvider(dto.Provider) || dto.IDToken == "" || dto.Nonce == "" {
		bad(w, r)
		return
	}
	accountID, err := h.Service.ValidateAccess(r.Context(), dto.FamilyID, dto.AccessToken)
	if err != nil {
		fail(w, r, err)
		return
	}
	link, err := h.Service.LinkProvider(r.Context(), accountID, dto.Provider, dto.IDToken, h.audience(), dto.Nonce)
	if err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, http.StatusOK, map[string]any{"provider_link": providerLinkJSONOf(link)})
}

// unlinkProvider removes one provider binding from the session-owned
// account, refusing the last login method.
func (h Handler) unlinkProvider(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		FamilyID    string `json:"family_id"`
		AccessToken string `json:"access_token"`
		Provider    string `json:"provider"`
	}
	if err := read(r, &dto); err != nil || dto.FamilyID == "" || dto.AccessToken == "" ||
		!validProvider(dto.Provider) {
		bad(w, r)
		return
	}
	accountID, err := h.Service.ValidateAccess(r.Context(), dto.FamilyID, dto.AccessToken)
	if err != nil {
		fail(w, r, err)
		return
	}
	if err := h.Service.UnlinkProvider(r.Context(), accountID, dto.Provider); err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, http.StatusOK, map[string]string{"status": "unlinked"})
}

// bindContributor links one device contributor to the session-owned
// account (P13-T04C). Both proofs travel in the JSON body: the live
// account session and the contributor key proof over the ceremony
// statement. The account derives server-side; caller-supplied account
// IDs are never trusted.
func (h Handler) bindContributor(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		FamilyID      string `json:"family_id"`
		AccessToken   string `json:"access_token"`
		ContributorID string `json:"contributor_id"`
		Proof         string `json:"proof"`
	}
	if err := read(r, &dto); err != nil || dto.FamilyID == "" || dto.AccessToken == "" ||
		strings.TrimSpace(dto.ContributorID) == "" || strings.TrimSpace(dto.Proof) == "" {
		bad(w, r)
		return
	}
	binding, err := h.Service.BindContributor(r.Context(), dto.FamilyID, dto.AccessToken, dto.ContributorID, dto.Proof)
	if err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, http.StatusOK, map[string]any{"binding": bindingJSONOf(binding)})
}

// unbindContributor removes one contributor binding from the
// session-owned account.
func (h Handler) unbindContributor(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		FamilyID      string `json:"family_id"`
		AccessToken   string `json:"access_token"`
		ContributorID string `json:"contributor_id"`
	}
	if err := read(r, &dto); err != nil || dto.FamilyID == "" || dto.AccessToken == "" ||
		strings.TrimSpace(dto.ContributorID) == "" {
		bad(w, r)
		return
	}
	if err := h.Service.UnbindContributor(r.Context(), dto.FamilyID, dto.AccessToken, dto.ContributorID); err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, http.StatusOK, map[string]string{"status": "unbound"})
}

// listBindings returns the contributor bindings of the session-owned
// account. Proofs and fingerprints in the response are public binding
// identifiers, never secrets.
func (h Handler) listBindings(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		FamilyID    string `json:"family_id"`
		AccessToken string `json:"access_token"`
	}
	if err := read(r, &dto); err != nil || dto.FamilyID == "" || dto.AccessToken == "" {
		bad(w, r)
		return
	}
	bindings, err := h.Service.ListBindings(r.Context(), dto.FamilyID, dto.AccessToken)
	if err != nil {
		fail(w, r, err)
		return
	}
	out := make([]bindingJSON, 0, len(bindings))
	for _, b := range bindings {
		out = append(out, bindingJSONOf(b))
	}
	write(w, r, http.StatusOK, map[string]any{"bindings": out})
}

type bindingJSON struct {
	ContributorID  string `json:"contributor_id"`
	KeyFingerprint string `json:"key_fingerprint"`
	BoundAt        string `json:"bound_at"`
}

func bindingJSONOf(b domain.ContributorBinding) bindingJSON {
	return bindingJSON{
		ContributorID:  b.ContributorID,
		KeyFingerprint: b.KeyFingerprint,
		BoundAt:        time.Unix(b.BoundAt, 0).UTC().Format(time.RFC3339),
	}
}

// deleteAccount erases the session-owned account (P13-T04A rights
// deletion): status flips to deleted, every session family is revoked
// and address plus provider bindings drop. The account row stays as an
// audit record; later signups mint new accounts.
func (h Handler) deleteAccount(w http.ResponseWriter, r *http.Request) {
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
	if err := h.Service.DeleteAccount(r.Context(), accountID); err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// listProviders returns the provider bindings of the session-owned
// account. Session proof travels in the JSON body (never the URL) and
// the response carries opaque subjects only.
func (h Handler) listProviders(w http.ResponseWriter, r *http.Request) {
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
	links, err := h.Service.ListProviders(r.Context(), accountID)
	if err != nil {
		fail(w, r, err)
		return
	}
	out := make([]providerLinkJSON, 0, len(links))
	for _, l := range links {
		out = append(out, providerLinkJSONOf(l))
	}
	write(w, r, http.StatusOK, map[string]any{"providers": out})
}
