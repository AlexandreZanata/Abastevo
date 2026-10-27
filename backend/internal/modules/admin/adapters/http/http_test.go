package http

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
	entdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/entitlement/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpserver"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/telemetry"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

const (
	testOrigin   = "https://admin.example.org"
	testCSRF     = "synthetic-csrf-token"
	testSalt     = "synthetic-csrf-salt"
	testSession  = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	testMutation = "11111111-1111-4111-8111-111111111111"
)

var testKey = domain.KeyPrefix + base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

type fakeStore struct {
	loginResult app.LoginResult
	loginErr    error
	session     app.Session
	authErr     error
	mutationErr error
	logouts     int
	revokes     int
}

func (f *fakeStore) Login(_ context.Context, in app.LoginInput) (app.LoginResult, error) {
	if f.loginErr != nil {
		return app.LoginResult{}, f.loginErr
	}
	if in.Verify == nil {
		return app.LoginResult{}, domain.ErrDenied
	}
	if _, ok := in.Verify(); !ok {
		return app.LoginResult{}, domain.ErrDenied
	}
	return f.loginResult, nil
}

func (f *fakeStore) Authenticate(context.Context, string, time.Time) (app.Session, error) {
	if f.authErr != nil {
		return app.Session{}, f.authErr
	}
	return f.session, nil
}

func (f *fakeStore) Logout(context.Context, app.Session, app.Mutation) (app.MutationResult, error) {
	f.logouts++
	return app.MutationResult{Status: 204}, f.mutationErr
}

func (f *fakeStore) RevokeAll(context.Context, app.Session, app.Mutation) (app.MutationResult, error) {
	f.revokes++
	return app.MutationResult{Status: 204}, f.mutationErr
}

func testHandler(store *fakeStore, configure func(*Handler)) Handler {
	hash := domain.HashKey(testKey)
	service := &app.Service{
		Store: store,
		Keys:  []domain.ConfiguredKey{{Hash: hash, Fingerprint: domain.Fingerprint(hash)}},
		Clock: func() time.Time { return time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC) },
	}
	handler := Handler{
		Service: service,
		Origin:  testOrigin,
		ClientIP: func(r *http.Request) (string, error) {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			return host, err
		},
	}
	if configure != nil {
		configure(&handler)
	}
	return handler
}

func sessionFixture() app.Session {
	return app.Session{
		ID: "22222222-2222-4222-8222-222222222222", KeyFingerprint: domain.Fingerprint(domain.HashKey(testKey)),
		CSRFSalt: testSalt, CSRFHash: domain.SaltedHash(testSalt, testCSRF),
		CreatedAt:  time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC),
		LastSeenAt: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC),
		ExpiresAt:  time.Date(2026, 10, 27, 20, 0, 0, 0, time.UTC),
	}
}

func successStore() *fakeStore {
	return &fakeStore{
		loginResult: app.LoginResult{
			Token: testSession, CSRFToken: testCSRF,
			Fingerprint: domain.Fingerprint(domain.HashKey(testKey)),
			ExpiresAt:   time.Date(2026, 10, 27, 20, 0, 0, 0, time.UTC),
		},
		session: sessionFixture(),
	}
}

func serve(t *testing.T, handler Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	router := chi.NewRouter()
	handler.RegisterRoutes(router)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func loginRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/session", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", testOrigin)
	req.RemoteAddr = "198.51.100.10:4321"
	return req
}

func sessionRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", testOrigin)
	req.Header.Set(csrfHeader, testCSRF)
	req.Header.Set(idempotencyHeader, testMutation)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: testSession})
	req.RemoteAddr = "198.51.100.10:4321"
	return req
}

// TestLogin_ADM05_SetsHardenedCookie is S3.
func TestLogin_ADM05_SetsHardenedCookie(t *testing.T) {
	store := successStore()
	w := serve(t, testHandler(store, nil), loginRequest(`{"key":"`+testKey+`"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var dto struct {
		CSRF      string    `json:"csrf_token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	raw := w.Result().Cookies()
	if len(raw) != 1 || raw[0].Name != cookieName || raw[0].Value != testSession {
		t.Fatalf("cookies=%+v", raw)
	}
	if !raw[0].HttpOnly || !raw[0].Secure || raw[0].SameSite != http.SameSiteStrictMode || raw[0].Path != "/" {
		t.Fatalf("cookie flags=%+v", raw[0])
	}
	if raw[0].Domain != "" {
		t.Fatal("__Host cookie must not carry Domain")
	}
	if err := json.Unmarshal(w.Body.Bytes(), &dto); err != nil || dto.CSRF != testCSRF || dto.ExpiresAt.IsZero() {
		t.Fatalf("body=%s err=%v", w.Body.String(), err)
	}
}

// TestLogin_ADM03_DenialsAreIndistinguishable is S4.
func TestLogin_ADM03_DenialsAreIndistinguishable(t *testing.T) {
	store := successStore()
	handler := testHandler(store, nil)
	longKey := strings.Repeat("k", 10<<10)
	var denied []string
	for _, body := range []string{
		`{"key":"abv_admin_wrong"}`,
		`{"key":""}`,
		`{"key":"` + longKey + `"}`,
	} {
		w := serve(t, handler, loginRequest(body))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		denied = append(denied, w.Body.String())
	}
	if denied[0] != denied[1] || denied[1] != denied[2] {
		t.Fatal("denial bodies differ")
	}
	badJSON := loginRequest(`{"key":"not-json"`)
	if w := serve(t, handler, badJSON); w.Code != http.StatusBadRequest {
		t.Fatalf("non-JSON status=%d", w.Code)
	}
	query := httptest.NewRequest(http.MethodPost, "/v1/admin/session?key="+testKey, strings.NewReader(`{"key":"value"}`))
	query.Header.Set("Content-Type", "application/json")
	query.Header.Set("Origin", testOrigin)
	query.RemoteAddr = "198.51.100.10:4321"
	if w := serve(t, handler, query); w.Code != http.StatusBadRequest {
		t.Fatalf("query key status=%d", w.Code)
	}
}

// TestLogin_ADM04_RateLimitedMapping is S5 at the HTTP boundary.
func TestLogin_ADM04_RateLimitedMapping(t *testing.T) {
	store := successStore()
	store.loginErr = &domain.RateLimitedError{RetryAfter: 90 * time.Second}
	w := serve(t, testHandler(store, nil), loginRequest(`{"key":"`+testKey+`"}`))
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "90" {
		t.Fatalf("status=%d retry=%q", w.Code, w.Header().Get("Retry-After"))
	}
}

// TestMutation_ADM06_CSRFAndOrigin is S12.
func TestMutation_ADM06_CSRFAndOrigin(t *testing.T) {
	handler := testHandler(successStore(), nil)
	body := `{"reason":"synthetic logout reason"}`
	cases := []struct {
		name   string
		mutate func(*http.Request)
	}{
		{"no-csrf", func(r *http.Request) { r.Header.Del(csrfHeader) }},
		{"wrong-csrf", func(r *http.Request) { r.Header.Set(csrfHeader, "wrong") }},
		{"wrong-origin", func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }},
		{"no-origin", func(r *http.Request) { r.Header.Del("Origin") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := sessionRequest(http.MethodDelete, "/v1/admin/session", body)
			tc.mutate(req)
			if w := serve(t, handler, req); w.Code != http.StatusForbidden {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
	if w := serve(t, handler, sessionRequest(http.MethodDelete, "/v1/admin/session", body)); w.Code != http.StatusNoContent {
		t.Fatalf("valid mutation status=%d body=%s", w.Code, w.Body.String())
	}
}

// TestAdminRoutes_ADM11_UserProofDoesNotAuthorize is S13.
func TestAdminRoutes_ADM11_UserProofDoesNotAuthorize(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/session", nil)
	req.Header.Set("Authorization", "Bearer account-access-token")
	req.Header.Set("Signature", "synthetic-signed-owner-proof")
	req.RemoteAddr = "198.51.100.10:4321"
	if w := serve(t, testHandler(successStore(), nil), req); w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", w.Code)
	}
}

// TestAdminRoutes_ADM11_AdminCookieDoesNotAuthorizeUserRoute is S14.
func TestAdminRoutes_ADM11_AdminCookieDoesNotAuthorizeUserRoute(t *testing.T) {
	router := chi.NewRouter()
	testHandler(successStore(), nil).RegisterRoutes(router)
	router.Get("/v1/premium/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-User-Session") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/premium/status", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: testSession})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatal("admin cookie authenticated a user route")
	}
}

// TestBody_ADM09_UnknownFieldAndLimit is S15.
func TestBody_ADM09_UnknownFieldAndLimit(t *testing.T) {
	handler := testHandler(successStore(), nil)
	unknown := sessionRequest(http.MethodDelete, "/v1/admin/session", `{"reason":"synthetic logout reason","extra":1}`)
	if w := serve(t, handler, unknown); w.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status=%d", w.Code)
	}
	large := sessionRequest(http.MethodDelete, "/v1/admin/session", `{"reason":"`+strings.Repeat("x", 17<<10)+`"}`)
	if w := serve(t, handler, large); w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body status=%d", w.Code)
	}
}

// TestAdminHeaders_ADM09_NoStoreNoIndex is S19.
func TestAdminHeaders_ADM09_NoStoreNoIndex(t *testing.T) {
	for _, tc := range []struct {
		req  *http.Request
		code int
	}{
		{loginRequest(`{"key":"` + testKey + `"}`), http.StatusOK},
		{loginRequest(`{"key":"wrong"}`), http.StatusUnauthorized},
	} {
		w := serve(t, testHandler(successStore(), nil), tc.req)
		if w.Code != tc.code {
			t.Fatalf("status=%d", w.Code)
		}
		if w.Header().Get("Cache-Control") != "no-store" ||
			w.Header().Get("X-Robots-Tag") != "noindex, nofollow" ||
			w.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatalf("headers=%v", w.Header())
		}
	}
}

// TestAdminFence_ADM08_OutsideCIDRIs404 is S20.
func TestAdminFence_ADM08_OutsideCIDRIs404(t *testing.T) {
	handler := testHandler(successStore(), func(h *Handler) {
		h.AllowedCIDRs = []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	})
	req := loginRequest(`{"key":"` + testKey + `"}`)
	req.RemoteAddr = "198.51.100.10:4321"
	if w := serve(t, handler, req); w.Code != http.StatusNotFound {
		t.Fatalf("outside status=%d", w.Code)
	}
	inside := loginRequest(`{"key":"` + testKey + `"}`)
	inside.RemoteAddr = "10.1.2.3:4321"
	if w := serve(t, handler, inside); w.Code != http.StatusOK {
		t.Fatalf("inside status=%d", w.Code)
	}
}

// TestLogs_ADM02_ADM10_NoSecrets is S18.
func TestLogs_ADM02_ADM10_NoSecrets(t *testing.T) {
	var logs bytes.Buffer
	logger, err := telemetry.New(&logs, "info", telemetry.RoleAPI)
	if err != nil {
		t.Fatal(err)
	}
	router := httpserver.NewRouter(logger)
	handler := testHandler(successStore(), nil)
	handler.RegisterRoutes(router)
	requests := []*http.Request{
		loginRequest(`{"key":"` + testKey + `"}`),
		loginRequest(`{"key":"wrong-key"}`),
		sessionRequest(http.MethodDelete, "/v1/admin/session", `{"reason":"synthetic logout reason"}`),
	}
	requests[2].Header.Del(csrfHeader)
	for _, req := range requests {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
	rendered := logs.String()
	for _, secret := range []string{testKey, testSession, testCSRF, "wrong-key"} {
		if strings.Contains(rendered, secret) {
			t.Fatalf("logs leaked %q: %s", secret, rendered)
		}
	}
	if rendered == "" {
		t.Fatal("access logger produced no evidence")
	}
}

// TestSession_ADM05_WhoAmI covers the authenticated read shape.
func TestSession_ADM05_WhoAmI(t *testing.T) {
	w := serve(t, testHandler(successStore(), nil), sessionRequest(http.MethodGet, "/v1/admin/session", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var dto map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &dto); err != nil {
		t.Fatal(err)
	}
	if dto["key_fingerprint"] != domain.Fingerprint(domain.HashKey(testKey)) || dto["expires_at"] == nil {
		t.Fatalf("body=%v", dto)
	}
}

func (f *fakeStore) Mutate(context.Context, app.Mutation, func(context.Context, pgx.Tx) (app.MutationResult, []app.AuditEvent, error)) (app.MutationResult, error) {
	return app.MutationResult{}, domain.ErrUnavailable
}

func (f *fakeStore) RunInTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	return fn(ctx, nil)
}

func (f *fakeStore) SearchAccounts(context.Context, string) ([]app.Account, error) {
	return nil, domain.ErrUnavailable
}

func (f *fakeStore) GetAccountTx(context.Context, pgx.Tx, string) (app.Account, error) {
	return app.Account{}, domain.ErrUnavailable
}

func (f *fakeStore) PaidSummaryTx(context.Context, pgx.Tx, string) (app.PaidSummary, error) {
	return app.PaidSummary{}, domain.ErrUnavailable
}

// TestGrant_ENT05_CapabilitiesFieldRejected covers the unknown-field rule:
// clients never send capability strings.
func TestGrant_ENT05_CapabilitiesFieldRejected(t *testing.T) {
	body := `{"tier":"PRO","reason":"synthetic grant reason","capabilities":["cloud.sync"]}`
	req := sessionRequest(http.MethodPost, "/v1/admin/accounts/33333333-3333-4333-8333-333333333333/grants", body)
	if w := serve(t, testHandler(successStore(), nil), req); w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

// TestSearchAccounts_BUCADM02_ShortQuery covers the 3-character minimum.
func TestSearchAccounts_BUCADM02_ShortQuery(t *testing.T) {
	req := sessionRequest(http.MethodGet, "/v1/admin/accounts?q=ab", "")
	if w := serve(t, testHandler(successStore(), nil), req); w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

// TestGrant_ENT02_ENT03_ErrorMapping covers the 409 codes used by the grant
// endpoints.
func TestGrant_ENT02_ENT03_ErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code int
		body string
	}{
		{entdomain.ErrIneligible, http.StatusConflict, "entitlement.account-ineligible"},
		{entdomain.ErrExists, http.StatusConflict, "entitlement.exists"},
		{entdomain.ErrAlreadyRevoked, http.StatusConflict, "entitlement.already-revoked"},
		{entdomain.ErrInvalid, http.StatusBadRequest, "entitlement.invalid"},
		{entdomain.ErrNotFound, http.StatusNotFound, "admin.not-found"},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/v1/admin/grants/x/revoke", nil)
		writeAdminServiceError(w, r, tc.err)
		if w.Code != tc.code {
			t.Fatalf("err=%v status=%d", tc.err, w.Code)
		}
		var payload struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil || payload.Error.Code != tc.body {
			t.Fatalf("err=%v body=%s", tc.err, w.Body.String())
		}
	}
}
