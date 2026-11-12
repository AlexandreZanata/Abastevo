// Package http owns only the company account cookie plane and its three POSTs.
package http

import (
	"context"
	"encoding/json"
	"errors"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	CookieName   = "__Host-empresas-session"
	CSRFHeader   = "X-CSRF"
	MaxBodyBytes = 16 << 10
	PathPrefix   = "/v1/org-access/session/"
)

type Handler struct {
	Service      *app.Service
	origin, host string
}

func New(service *app.Service, origin string) (Handler, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery || u.Host != strings.ToLower(u.Host) {
		return Handler{}, domain.ErrInvalid
	}
	if service == nil {
		return Handler{}, domain.ErrUnavailable
	}
	return Handler{Service: service, origin: origin, host: u.Host}, nil
}

type endpoint struct {
	handler Handler
	rule    orgapi.CompanySessionRule
}

func (e endpoint) CompanySessionRule() orgapi.CompanySessionRule { return e.rule }
func (h Handler) RegisterRoutes(router chi.Router) {
	if h.Service == nil || !h.Service.Enabled {
		return
	}
	for _, action := range []string{"login", "read", "logout"} {
		rule := orgapi.CompanySessionRule{Action: action, AccountProof: action == "login", Cookie: action != "login", CSRF: action != "login", Fresh: action == "login", Success: 200}
		router.Method(http.MethodPost, PathPrefix+action, endpoint{handler: h, rule: rule})
	}
}
func secure(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Del("ETag")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
}
func (h Handler) originCheck(r *http.Request) error {
	if h.Service == nil || !h.Service.Enabled || !orgapi.IsOrganizationPath(r.URL.Path) {
		return domain.ErrDisabled
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		return domain.ErrInvalid
	}
	if r.Method != http.MethodPost || r.Host != h.host || len(r.Header.Values("Origin")) != 1 || r.Header.Get("Origin") != h.origin {
		return domain.ErrCSRF
	}
	if values := r.Header.Values("Sec-Fetch-Site"); len(values) > 0 && (len(values) != 1 || values[0] != "same-origin") {
		return domain.ErrCSRF
	}
	for _, name := range []string{"Authorization", "Signature", "Signature-Input", "Signature-Created", "Signature-Expires", "Signature-Keyid", "Signature-Nonce"} {
		if len(r.Header.Values(name)) > 0 {
			return domain.ErrUnauthorized
		}
	}
	return nil
}
func cookieValue(r *http.Request) (string, error) {
	cookies := r.CookiesNamed(CookieName)
	if len(cookies) != 1 || !domain.ValidSecret(cookies[0].Value, domain.TokenPrefix) {
		return "", domain.ErrUnauthorized
	}
	return cookies[0].Value, nil
}
func csrfValue(r *http.Request) (string, error) {
	values := r.Header.Values(CSRFHeader)
	if len(values) != 1 || !domain.ValidSecret(values[0], domain.CSRFPrefix) {
		return "", domain.ErrCSRF
	}
	return values[0], nil
}
func mapped(err error) error {
	switch {
	case errors.Is(err, domain.ErrDisabled):
		return orgapi.NotFound
	case errors.Is(err, domain.ErrUnauthorized):
		return orgapi.Unauthenticated
	case errors.Is(err, domain.ErrFresh):
		return orgapi.FreshAuthRequired
	case errors.Is(err, domain.ErrCSRF):
		return orgapi.Forbidden
	case errors.Is(err, domain.ErrInvalid):
		return orgapi.Malformed
	case errors.Is(err, domain.ErrRateLimited):
		return orgapi.RateLimited
	default:
		return orgapi.Unavailable
	}
}
func (e endpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	secure(w)
	h := e.handler
	refuse := func(err error) { orgapi.WriteError(w, r, mapped(err)) }
	if err := h.originCheck(r); err != nil {
		refuse(err)
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(r.Header.Values("Content-Type")) != 1 || r.URL.RawQuery != "" {
		refuse(domain.ErrInvalid)
		return
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(15 * time.Second))
	raw, err := httpapi.ReadBody(r, MaxBodyBytes)
	if err != nil {
		refuse(domain.ErrInvalid)
		return
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		refuse(domain.ErrInvalid)
		return
	}
	if e.rule.Action == "login" {
		var dto struct {
			Family string `json:"family_id"`
			Access string `json:"access_token"`
		}
		if len(fields) != 2 || fields["family_id"] == nil || fields["access_token"] == nil || httpapi.DecodeJSON(raw, &dto) != nil {
			refuse(domain.ErrInvalid)
			return
		}
		if string(fields["family_id"]) == "null" || string(fields["access_token"]) == "null" {
			refuse(domain.ErrInvalid)
			return
		}
		old := ""
		cookies := r.CookiesNamed(CookieName)
		if len(cookies) > 1 {
			refuse(domain.ErrUnauthorized)
			return
		}
		if len(cookies) == 1 {
			old = cookies[0].Value
		}
		result, err := h.Service.Login(r.Context(), dto.Family, dto.Access, old)
		if err != nil {
			refuse(err)
			return
		}
		setCookie(w, result.Token, result.Row.ExpiresAt, false)
		writeJSON(w, map[string]any{"account_id": result.Row.AccountID, "csrf_token": result.CSRF, "authenticated_at": result.Row.AuthenticatedAt, "expires_at": result.Row.ExpiresAt})
		return
	}
	if len(fields) != 0 || httpapi.DecodeJSON(raw, &struct{}{}) != nil {
		refuse(domain.ErrInvalid)
		return
	}
	token, err := cookieValue(r)
	if err != nil {
		refuse(err)
		return
	}
	csrf, err := csrfValue(r)
	if err != nil {
		refuse(err)
		return
	}
	if e.rule.Action == "logout" {
		if err := h.Service.Logout(r.Context(), token, csrf); err != nil {
			refuse(err)
			return
		}
		setCookie(w, "", time.Unix(1, 0), true)
		writeJSON(w, map[string]bool{"logged_out": true})
		return
	}
	row, err := h.Service.Authenticate(r.Context(), token, csrf, false)
	if err != nil {
		refuse(err)
		return
	}
	writeJSON(w, map[string]any{"account_id": row.AccountID, "authenticated_at": row.AuthenticatedAt, "fresh_until": row.AuthenticatedAt.Add(domain.FreshWindow), "expires_at": row.ExpiresAt})
}
func setCookie(w http.ResponseWriter, value string, expires time.Time, clear bool) {
	cookie := &http.Cookie{Name: CookieName, Value: value, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, Expires: expires.UTC()}
	if clear {
		cookie.MaxAge = -1
	}
	http.SetCookie(w, cookie)
}
func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
func (h Handler) request(r *http.Request, fresh, touch bool) (app.Row, error) {
	if err := h.originCheck(r); err != nil {
		return app.Row{}, mapped(err)
	}
	token, err := cookieValue(r)
	if err != nil {
		return app.Row{}, mapped(err)
	}
	csrf, err := csrfValue(r)
	if err != nil {
		return app.Row{}, mapped(err)
	}
	if !touch {
		row, err := h.Service.Peek(r.Context(), token, csrf)
		if err != nil {
			return app.Row{}, mapped(err)
		}
		return row, nil
	}
	row, err := h.Service.Authenticate(r.Context(), token, csrf, fresh)
	if err != nil {
		return app.Row{}, mapped(err)
	}
	return row, nil
}
func (h Handler) WebSession(r *http.Request) (string, error) {
	row, err := h.request(r, false, false)
	return row.AccountID, err
}
func (h Handler) WebSessionConfirm(r *http.Request, fresh bool) (string, error) {
	row, err := h.ConfirmSession(r, fresh)
	return row.AccountID, err
}

// ConfirmSession returns current server metadata for a transaction owner to
// recheck by ID. It never exposes the cookie/CSRF verifier or grants org access.
func (h Handler) ConfirmSession(r *http.Request, fresh bool) (app.Row, error) {
	return h.request(r, fresh, true)
}

type sessionKey struct{}

func SessionFromContext(ctx context.Context) (app.Row, bool) {
	row, ok := ctx.Value(sessionKey{}).(app.Row)
	return row, ok
}
func (h Handler) RequireSession(next http.Handler) http.Handler {
	return h.requireSession(next, false)
}

// RequireFreshSession is the account-scoped bootstrap entry. It confers no
// organization authority; the owning command must enforce its own admission.
func (h Handler) RequireFreshSession(next http.Handler) http.Handler {
	return h.requireSession(next, true)
}
func (h Handler) requireSession(next http.Handler, fresh bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secure(w)
		row, err := h.request(r, fresh, true)
		if err != nil {
			orgapi.WriteError(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey{}, row)))
	})
}
