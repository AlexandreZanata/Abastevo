// Package http exposes the administrative session surface. It owns cookie,
// CSRF, Origin, security headers, body caps and error mapping; no raw
// credential is ever logged or echoed.
package http

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"mime"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
	entdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/entitlement/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
)

const (
	cookieName        = "__Host-abv_admin"
	csrfHeader        = "X-Admin-CSRF"
	idempotencyHeader = "Idempotency-Key"
	maxBodyBytes      = 16 << 10
)

type Handler struct {
	Service      *app.Service
	Origin       string
	AllowedCIDRs []netip.Prefix
	ClientIP     func(*http.Request) (string, error)
	CursorSecret []byte
}

func (h Handler) RegisterRoutes(r chi.Router) {
	if h.Service == nil || !h.Service.Enabled() {
		return
	}
	r.Group(func(admin chi.Router) {
		admin.Use(h.securityHeaders, h.fence)
		admin.Post("/v1/admin/session", h.login)
		admin.Group(func(auth chi.Router) {
			auth.Use(h.RequireSession)
			auth.Get("/v1/admin/session", h.whoami)
			auth.Delete("/v1/admin/session", h.logout)
			auth.Post("/v1/admin/sessions/revoke-all", h.revokeAll)
			auth.Get("/v1/admin/accounts", h.searchAccounts)
			auth.Get("/v1/admin/accounts/{id}", h.accountDetail)
			auth.Post("/v1/admin/accounts/{id}/grants", h.createGrant)
			auth.Get("/v1/admin/grants", h.listGrants)
			auth.Post("/v1/admin/grants/{id}/revoke", h.revokeGrant)
		})
	})
}

func (h Handler) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func (h Handler) fence(next http.Handler) http.Handler {
	if len(h.AllowedCIDRs) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := h.clientIP(r)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		addr, err := netip.ParseAddr(raw)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		addr = addr.Unmap()
		for _, prefix := range h.AllowedCIDRs {
			if prefix.Contains(addr) {
				next.ServeHTTP(w, r)
				return
			}
		}
		http.NotFound(w, r)
	})
}

type sessionKey struct{}

func (h Handler) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(cookieName)
		if err != nil || cookie.Value == "" {
			writeAdminError(w, r, http.StatusUnauthorized, "admin.denied")
			return
		}
		session, err := h.Service.Authenticate(r.Context(), cookie.Value)
		if err != nil {
			switch {
			case errors.Is(err, domain.ErrUnauthorized):
				writeAdminError(w, r, http.StatusUnauthorized, "admin.denied")
			case errors.Is(err, domain.ErrUnavailable):
				writeAdminError(w, r, http.StatusServiceUnavailable, "admin.unavailable")
			default:
				writeAdminError(w, r, http.StatusUnauthorized, "admin.denied")
			}
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey{}, session)))
	})
}

func SessionFromContext(ctx context.Context) (app.Session, bool) {
	session, ok := ctx.Value(sessionKey{}).(app.Session)
	return session, ok
}

func (h Handler) login(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Has("key") {
		writeAdminError(w, r, http.StatusBadRequest, "admin.invalid")
		return
	}
	if !h.validOrigin(r) {
		writeAdminError(w, r, http.StatusForbidden, "admin.csrf")
		return
	}
	var dto struct {
		Key string `json:"key"`
	}
	if err := readAdminJSON(r, &dto); err != nil {
		writeBodyError(w, r, err)
		return
	}
	rawIP, err := h.clientIP(r)
	if err != nil {
		writeAdminError(w, r, http.StatusBadRequest, "admin.invalid")
		return
	}
	result, err := h.Service.Login(r.Context(), dto.Key, hashIP(rawIP), r.Header.Get("X-Request-ID"))
	if err != nil {
		writeAdminServiceError(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: result.Token, Path: "/",
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
		Expires: result.ExpiresAt.UTC(),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"csrf_token": result.CSRFToken,
		"expires_at": result.ExpiresAt.UTC(),
	})
}

func (h Handler) whoami(w http.ResponseWriter, r *http.Request) {
	session, ok := SessionFromContext(r.Context())
	if !ok {
		writeAdminError(w, r, http.StatusUnauthorized, "admin.denied")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"key_fingerprint": session.KeyFingerprint,
		"expires_at":      session.ExpiresAt.UTC(),
	})
}

func (h Handler) logout(w http.ResponseWriter, r *http.Request) {
	session, ok := SessionFromContext(r.Context())
	if !ok {
		writeAdminError(w, r, http.StatusUnauthorized, "admin.denied")
		return
	}
	h.mutate(w, r, session, "admin.logout", "admin_session", session.ID, func(m app.Mutation) (app.MutationResult, error) {
		return h.Service.Logout(r.Context(), session, m)
	})
}

func (h Handler) revokeAll(w http.ResponseWriter, r *http.Request) {
	session, ok := SessionFromContext(r.Context())
	if !ok {
		writeAdminError(w, r, http.StatusUnauthorized, "admin.denied")
		return
	}
	h.mutate(w, r, session, "admin.sessions.revoke-all", "admin_session", "", func(m app.Mutation) (app.MutationResult, error) {
		return h.Service.RevokeAll(r.Context(), session, m)
	})
}

// mutate is the single path for reason-only administrative mutations
// (ADM-06/ADM-10/ADM-12). Grant mutations reuse prepareMutation/finishMutation
// with their own validated body.
func (h Handler) mutate(w http.ResponseWriter, r *http.Request, session app.Session, action, targetType, targetID string, run func(app.Mutation) (app.MutationResult, error)) {
	var dto struct {
		Reason string `json:"reason"`
	}
	m, ok := h.PrepareMutation(w, r, session, action, targetType, targetID, &dto)
	if !ok {
		return
	}
	reason := strings.TrimSpace(dto.Reason)
	if !domain.ValidReason(reason) {
		writeAdminError(w, r, http.StatusBadRequest, "admin.invalid")
		return
	}
	m.Reason = reason
	result, err := run(m)
	h.FinishMutation(w, r, result, err, true)
}

// prepareMutation validates Origin, CSRF, the UUID idempotency key and the
// 16 KiB body before decoding the caller's DTO. It returns the mutation
// context with the body hash already bound to action and target.
func (h Handler) PrepareMutation(w http.ResponseWriter, r *http.Request, session app.Session, action, targetType, targetID string, dst any) (app.Mutation, bool) {
	if !h.validOrigin(r) {
		writeAdminError(w, r, http.StatusForbidden, "admin.csrf")
		return app.Mutation{}, false
	}
	presented := r.Header.Get(csrfHeader)
	if subtle.ConstantTimeCompare([]byte(domain.SaltedHash(session.CSRFSalt, presented)), []byte(session.CSRFHash)) != 1 {
		writeAdminError(w, r, http.StatusForbidden, "admin.csrf")
		return app.Mutation{}, false
	}
	key := r.Header.Get(idempotencyHeader)
	if !domain.ValidUUID(key) {
		writeAdminError(w, r, http.StatusBadRequest, "admin.invalid")
		return app.Mutation{}, false
	}
	raw, err := readAdminJSONRaw(r, dst)
	if err != nil {
		writeBodyError(w, r, err)
		return app.Mutation{}, false
	}
	rawIP, err := h.clientIP(r)
	if err != nil {
		writeAdminError(w, r, http.StatusBadRequest, "admin.invalid")
		return app.Mutation{}, false
	}
	return app.Mutation{
		SessionID: session.ID, Action: action, TargetType: targetType, TargetID: targetID,
		Key: key, RequestHash: requestHash(action, targetType, targetID, raw),
		RequestID: r.Header.Get("X-Request-ID"), ClientIPHash: hashIP(rawIP),
	}, true
}

func (h Handler) FinishMutation(w http.ResponseWriter, r *http.Request, result app.MutationResult, err error, clear bool) {
	if err != nil {
		writeAdminServiceError(w, r, err)
		return
	}
	if clear {
		clearCookie(w)
	}
	if result.Status == http.StatusNoContent {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(result.Status)
	_, _ = w.Write(result.Body)
}

type accountView struct {
	AccountID string    `json:"account_id"`
	Alias     string    `json:"alias"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type paidView struct {
	State      string     `json:"state"`
	ValidUntil *time.Time `json:"valid_until"`
}

func (h Handler) searchAccounts(w http.ResponseWriter, r *http.Request) {
	items, err := h.Service.SearchAccounts(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		writeAdminServiceError(w, r, err)
		return
	}
	out := make([]accountView, 0, len(items))
	for _, item := range items {
		out = append(out, accountView{AccountID: item.ID, Alias: item.Alias, Status: item.Status, CreatedAt: item.CreatedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h Handler) accountDetail(w http.ResponseWriter, r *http.Request) {
	detail, err := h.Service.AccountDetail(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeAdminServiceError(w, r, err)
		return
	}
	grants := make([]app.GrantView, 0, len(detail.Grants))
	for _, grant := range detail.Grants {
		grants = append(grants, app.View(grant))
	}
	writeJSON(w, http.StatusOK, struct {
		AccountID string          `json:"account_id"`
		Alias     string          `json:"alias"`
		Status    string          `json:"status"`
		CreatedAt time.Time       `json:"created_at"`
		Grants    []app.GrantView `json:"grants"`
		Paid      paidView        `json:"paid"`
	}{detail.Account.ID, detail.Account.Alias, detail.Account.Status, detail.Account.CreatedAt, grants,
		paidView{State: detail.Paid.State, ValidUntil: detail.Paid.ValidUntil}})
}

type createGrantDTO struct {
	Tier    string     `json:"tier"`
	EndAt   *time.Time `json:"end_at"`
	Reason  string     `json:"reason"`
	Replace bool       `json:"replace"`
}

func (h Handler) createGrant(w http.ResponseWriter, r *http.Request) {
	session, ok := SessionFromContext(r.Context())
	if !ok {
		writeAdminError(w, r, http.StatusUnauthorized, "admin.denied")
		return
	}
	accountID := chi.URLParam(r, "id")
	var dto createGrantDTO
	m, ok := h.PrepareMutation(w, r, session, "admin.grant.create", "entitlement_grant", accountID, &dto)
	if !ok {
		return
	}
	reason := strings.TrimSpace(dto.Reason)
	if !domain.ValidReason(reason) {
		writeAdminError(w, r, http.StatusBadRequest, "admin.invalid")
		return
	}
	m.Reason = reason
	result, err := h.Service.CreateGrant(r.Context(), session, app.CreateGrantInput{
		AccountID: accountID, Tier: dto.Tier, EndAt: dto.EndAt, Reason: reason, Replace: dto.Replace,
		Key: m.Key, RequestHash: m.RequestHash, RequestID: m.RequestID, ClientIPHash: m.ClientIPHash,
	})
	if err != nil {
		writeAdminServiceError(w, r, err)
		return
	}
	body, err := json.Marshal(result)
	if err != nil {
		writeAdminError(w, r, http.StatusServiceUnavailable, "admin.unavailable")
		return
	}
	h.FinishMutation(w, r, app.MutationResult{Status: http.StatusCreated, Body: body, Replayed: result.Replayed}, nil, false)
}

type revokeGrantDTO struct {
	Reason string `json:"reason"`
}

func (h Handler) revokeGrant(w http.ResponseWriter, r *http.Request) {
	session, ok := SessionFromContext(r.Context())
	if !ok {
		writeAdminError(w, r, http.StatusUnauthorized, "admin.denied")
		return
	}
	grantID := chi.URLParam(r, "id")
	var dto revokeGrantDTO
	m, ok := h.PrepareMutation(w, r, session, "admin.grant.revoke", "entitlement_grant", grantID, &dto)
	if !ok {
		return
	}
	reason := strings.TrimSpace(dto.Reason)
	if !domain.ValidReason(reason) {
		writeAdminError(w, r, http.StatusBadRequest, "admin.invalid")
		return
	}
	m.Reason = reason
	view, replayed, err := h.Service.RevokeGrant(r.Context(), session, app.RevokeGrantInput{
		GrantID: grantID, Reason: reason, Key: m.Key, RequestHash: m.RequestHash,
		RequestID: m.RequestID, ClientIPHash: m.ClientIPHash,
	})
	if err != nil {
		writeAdminServiceError(w, r, err)
		return
	}
	body, err := json.Marshal(view)
	if err != nil {
		writeAdminError(w, r, http.StatusServiceUnavailable, "admin.unavailable")
		return
	}
	h.FinishMutation(w, r, app.MutationResult{Status: http.StatusOK, Body: body, Replayed: replayed}, nil, false)
}

func (h Handler) listGrants(w http.ResponseWriter, r *http.Request) {
	tierRaw := strings.TrimSpace(r.URL.Query().Get("tier"))
	stateRaw := strings.TrimSpace(r.URL.Query().Get("state"))
	if tierRaw != "" && !entdomain.ValidTier(entdomain.Tier(tierRaw)) {
		writeAdminError(w, r, http.StatusBadRequest, "admin.invalid")
		return
	}
	if stateRaw != "" && stateRaw != string(entdomain.StateActive) && stateRaw != string(entdomain.StateRevoked) {
		writeAdminError(w, r, http.StatusBadRequest, "admin.invalid")
		return
	}
	limit, err := httpapi.ParseLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeAdminError(w, r, http.StatusBadRequest, "admin.invalid")
		return
	}
	filter := entdomain.GrantFilter{Tier: entdomain.Tier(tierRaw), State: entdomain.State(stateRaw), Limit: limit + 1}
	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	if cursor != "" {
		if len(h.CursorSecret) < 32 {
			writeAdminError(w, r, http.StatusServiceUnavailable, "admin.unavailable")
			return
		}
		position, err := httpapi.Open(h.CursorSecret, cursor, httpapi.FilterHash("admin-grants-v1", tierRaw+"|"+stateRaw), time.Now())
		if err != nil {
			writeAdminError(w, r, http.StatusBadRequest, "admin.invalid")
			return
		}
		stamp, id, ok := strings.Cut(position, ",")
		if !ok || !domain.ValidUUID(id) {
			writeAdminError(w, r, http.StatusBadRequest, "admin.invalid")
			return
		}
		created, err := time.Parse(time.RFC3339Nano, stamp)
		if err != nil {
			writeAdminError(w, r, http.StatusBadRequest, "admin.invalid")
			return
		}
		filter.BeforeCreated, filter.BeforeID = created, id
	}
	grants, err := h.Service.ListGrants(r.Context(), filter)
	if err != nil {
		writeAdminServiceError(w, r, err)
		return
	}
	next := ""
	if len(grants) > limit {
		grants = grants[:limit]
		last := grants[len(grants)-1]
		next = httpapi.Seal(h.CursorSecret, httpapi.FilterHash("admin-grants-v1", tierRaw+"|"+stateRaw), last.CreatedAt.UTC().Format(time.RFC3339Nano)+","+last.ID, time.Now())
	}
	items := make([]app.GrantView, 0, len(grants))
	for _, grant := range grants {
		items = append(items, app.View(grant))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "next_cursor": next})
}

func (h Handler) validOrigin(r *http.Request) bool {
	return h.Origin != "" && r.Header.Get("Origin") == h.Origin
}

func (h Handler) clientIP(r *http.Request) (string, error) {
	if h.ClientIP == nil {
		return "", errors.New("admin: client resolver unavailable")
	}
	return h.ClientIP(r)
}

func readAdminJSON(r *http.Request, dst any) error {
	_, err := readAdminJSONRaw(r, dst)
	return err
}

func readAdminJSONRaw(r *http.Request, dst any) ([]byte, error) {
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" {
		return nil, domain.ErrInvalid
	}
	raw, err := httpapi.ReadBody(r, maxBodyBytes)
	if err != nil {
		return nil, err
	}
	if err := httpapi.DecodeJSON(raw, dst); err != nil {
		return nil, domain.ErrInvalid
	}
	return raw, nil
}

func writeBodyError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, httpapi.ErrBodyTooLarge) {
		writeAdminError(w, r, http.StatusRequestEntityTooLarge, "admin.too-large")
		return
	}
	writeAdminError(w, r, http.StatusBadRequest, "admin.invalid")
}

func writeAdminServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var limited *domain.RateLimitedError
	switch {
	case errors.As(err, &limited):
		w.Header().Set("Retry-After", strconv.FormatInt(int64(math.Ceil(limited.RetryAfter.Seconds())), 10))
		writeAdminError(w, r, http.StatusTooManyRequests, "admin.rate-limited")
	case errors.Is(err, domain.ErrDenied), errors.Is(err, domain.ErrUnauthorized):
		writeAdminError(w, r, http.StatusUnauthorized, "admin.denied")
	case errors.Is(err, domain.ErrCSRF):
		writeAdminError(w, r, http.StatusForbidden, "admin.csrf")
	case errors.Is(err, domain.ErrInvalid):
		writeAdminError(w, r, http.StatusBadRequest, "admin.invalid")
	case errors.Is(err, domain.ErrConflict):
		writeAdminError(w, r, http.StatusConflict, "admin.conflict")
	case errors.Is(err, entdomain.ErrIneligible):
		writeAdminError(w, r, http.StatusConflict, "entitlement.account-ineligible")
	case errors.Is(err, entdomain.ErrExists):
		writeAdminError(w, r, http.StatusConflict, "entitlement.exists")
	case errors.Is(err, entdomain.ErrAlreadyRevoked):
		writeAdminError(w, r, http.StatusConflict, "entitlement.already-revoked")
	case errors.Is(err, entdomain.ErrNotFound), errors.Is(err, domain.ErrNotFound):
		writeAdminError(w, r, http.StatusNotFound, "admin.not-found")
	case errors.Is(err, entdomain.ErrInvalid):
		writeAdminError(w, r, http.StatusBadRequest, "entitlement.invalid")
	case errors.Is(err, entdomain.ErrUnavailable):
		writeAdminError(w, r, http.StatusServiceUnavailable, "entitlement.unavailable")
	default:
		writeAdminError(w, r, http.StatusServiceUnavailable, "admin.unavailable")
	}
}

func writeAdminError(w http.ResponseWriter, r *http.Request, status int, code string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("Referrer-Policy", "no-referrer")
	httpapi.WriteError(w, r, status, code, "admin request refused", nil)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	raw, err := json.Marshal(value)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}

func clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: "", Path: "/", HttpOnly: true, Secure: true,
		SameSite: http.SameSiteStrictMode, MaxAge: -1, Expires: time.Unix(1, 0).UTC(),
	})
}

func hashIP(raw string) string {
	sum := sha256.Sum256([]byte("admin-client-ip\x00" + raw))
	return hex.EncodeToString(sum[:])
}

func requestHash(action, targetType, targetID string, body []byte) string {
	sum := sha256.Sum256([]byte(action + "\x00" + targetType + "\x00" + targetID + "\x00" + string(body)))
	return hex.EncodeToString(sum[:])
}

// AdminMiddleware exposes the same security headers, CIDR fence and session
// check to later admin surfaces (contributors, roles) mounted from other
// modules.
func (h Handler) AdminMiddleware() []func(http.Handler) http.Handler {
	return []func(http.Handler) http.Handler{h.securityHeaders, h.fence, h.RequireSession}
}
