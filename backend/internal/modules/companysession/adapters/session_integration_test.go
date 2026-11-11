//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	accountstore "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	adminstore "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/adapters"
	adminhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/adapters/http"
	admin "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/application"
	admindomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters"
	web "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Time() time.Time         { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *clock) NowUnix() int64          { return c.Time().Unix() }
func (c *clock) advance(d time.Duration) { c.mu.Lock(); defer c.mu.Unlock(); c.now = c.now.Add(d) }

type fixture struct {
	key      string
	db       *orgtest.Fixture
	clock    *clock
	accounts *account.Service
	auth     account.AuthResult
	service  *app.Service
	store    *adapters.Store
}

func setup(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{db: orgtest.New(t), clock: &clock{now: time.Now().UTC().Truncate(time.Second)}}
	f.accounts = &account.Service{Clock: f.clock, Store: accountstore.NewPGStore(f.db.Admin), Hasher: accdomain.SHA256Hasher{},
		IDGen: jobs.NewUUIDv4, TokenGen: accdomain.GenerateToken, KeyGen: accdomain.GenerateAccountKey}
	key, err := f.accounts.CreateKeyAccount(t.Context(), "companymember")
	if err != nil {
		t.Fatal(err)
	}
	f.key = key.Key
	f.auth, err = f.accounts.LoginWithKey(t.Context(), key.Key)
	if err != nil {
		t.Fatal(err)
	}
	role := pgx.Identifier{f.db.RuntimeRole}.Sanitize()
	if _, err := f.db.Admin.Exec(t.Context(), "GRANT SELECT,UPDATE ON accounts,account_session_families TO "+role+
		"; GRANT SELECT,INSERT,UPDATE,DELETE ON company_web_sessions TO "+role); err != nil {
		t.Fatal(err)
	}
	f.store = &adapters.Store{Pool: f.db.Runtime}
	f.service = &app.Service{Enabled: true, Store: f.store, Hasher: accdomain.SHA256Hasher{}, Prove: adapters.AccountProof(f.accounts),
		Now: f.clock.Time, NewID: jobs.NewUUIDv4, NewSecret: accdomain.GenerateToken}
	return f
}
func (f *fixture) login(t *testing.T, old string) app.Result {
	t.Helper()
	result, err := f.service.Login(t.Context(), f.auth.Session.FamilyID, f.auth.Session.AccessToken, old)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func seen(t *testing.T, f *fixture, id string) time.Time {
	t.Helper()
	var result time.Time
	if err := f.db.Admin.QueryRow(t.Context(), "SELECT last_seen_at FROM company_web_sessions WHERE id=$1", id).Scan(&result); err != nil {
		t.Fatal(err)
	}
	return result
}
func TestCompanySessionsUseCurrentAccountAuthorityAndIndependentLifetime(t *testing.T) {
	f := setup(t)
	result := f.login(t, "")
	var persisted string
	if err := f.db.Admin.QueryRow(t.Context(), "SELECT row_to_json(s)::text FROM company_web_sessions s").Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{result.Token, result.CSRF, f.auth.Session.AccessToken, f.auth.Session.RefreshToken} {
		if strings.Contains(persisted, secret) {
			t.Fatal("plaintext credential persisted")
		}
	}
	f.clock.advance(domain.FreshWindow)
	before := seen(t, f, result.Row.ID)
	if _, err := f.service.Authenticate(t.Context(), result.Token, result.CSRF, true); !errors.Is(err, domain.ErrFresh) {
		t.Fatal("stale fresh authority accepted", err)
	}
	if !seen(t, f, result.Row.ID).Equal(before) {
		t.Fatal("failed fresh check extended idle")
	}
	if _, err := f.service.Authenticate(t.Context(), result.Token, domain.CSRFPrefix+strings.Repeat("a", 64), false); !errors.Is(err, domain.ErrCSRF) {
		t.Fatal(err)
	}
	if !seen(t, f, result.Row.ID).Equal(before) {
		t.Fatal("csrf failure extended idle")
	}
	if _, err := f.service.Authenticate(t.Context(), result.Token, result.CSRF, false); err != nil {
		t.Fatal("cookie incorrectly depends on access-token expiry", err)
	}
	refreshed, err := f.accounts.Refresh(t.Context(), f.auth.Session.FamilyID, f.auth.Session.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	f.auth.Session = refreshed
	if _, err := f.service.Authenticate(t.Context(), result.Token, result.CSRF, true); !errors.Is(err, domain.ErrFresh) {
		t.Fatal("account refresh silently refreshed cookie authentication", err)
	}
	rotated := f.login(t, result.Token)
	if rotated.Token == result.Token || rotated.CSRF == result.CSRF {
		t.Fatal("cookie fixation")
	}
	if _, err := f.service.Authenticate(t.Context(), result.Token, result.CSRF, false); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("old company cookie remained active", err)
	}
	if err := f.service.Logout(t.Context(), rotated.Token, rotated.CSRF); err != nil {
		t.Fatal(err)
	}
	if _, err := f.accounts.ValidateAccess(t.Context(), f.auth.Session.FamilyID, f.auth.Session.AccessToken); err != nil {
		t.Fatal("company logout revoked personal account proof", err)
	}
	active := f.login(t, "")
	if err := f.accounts.RevokeAll(t.Context(), f.auth.Account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Authenticate(t.Context(), active.Token, active.CSRF, false); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("account-wide revocation did not invalidate cookie", err)
	}
}

func TestRevokedFamiliesDoNotConsumeCompanyAdmission(t *testing.T) {
	f := setup(t)
	for range domain.MaxLiveSessions {
		f.login(t, "")
	}
	if err := f.accounts.RevokeAll(t.Context(), f.auth.Account.ID); err != nil {
		t.Fatal(err)
	}
	refreshed, err := f.accounts.LoginWithKey(t.Context(), f.key)
	if err != nil {
		t.Fatal(err)
	}
	f.auth = refreshed
	if _, err := f.service.Login(t.Context(), f.auth.Session.FamilyID, f.auth.Session.AccessToken, ""); err != nil {
		t.Fatal("revoked family blocked fresh login", err)
	}
}
func TestClockRollbackDoesNotMakeCleanupDeleteUnexpiredCredentials(t *testing.T) {
	f := setup(t)
	issued := f.login(t, "")
	f.clock.advance(-time.Second)
	n, oldest, err := f.store.PurgeExpired(t.Context(), f.clock.Time(), 500)
	if err != nil || n != 0 || !oldest.IsZero() {
		t.Fatal("clock rollback deleted unexpired credentials", n, err)
	}
	f.clock.advance(time.Second)
	if _, err := f.service.Authenticate(t.Context(), issued.Token, issued.CSRF, false); err != nil {
		t.Fatal("restored clock could not recover current session", err)
	}
}
func TestCompanyIdleAbsoluteExpiryRollbackAndCleanup(t *testing.T) {
	for _, kind := range []string{"idle", "absolute", "rollback", "suspended", "deleted"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			result := f.login(t, "")
			switch kind {
			case "idle":
				f.clock.advance(domain.IdleTimeout)
			case "absolute":
				f.clock.advance(domain.AbsoluteLifetime)
				if _, err := f.db.Admin.Exec(t.Context(), "UPDATE company_web_sessions SET last_seen_at=$1 WHERE id=$2", f.clock.Time().Add(-time.Minute), result.Row.ID); err != nil {
					t.Fatal(err)
				}
			case "rollback":
				f.clock.advance(-time.Second)
			default:
				if _, err := f.db.Admin.Exec(t.Context(), "UPDATE accounts SET status=$1 WHERE id=$2", kind, f.auth.Account.ID); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := f.service.Authenticate(t.Context(), result.Token, result.CSRF, false); !errors.Is(err, domain.ErrUnauthorized) {
				t.Fatal("invalid current authority accepted", err)
			}
		})
	}
	f := setup(t)
	f.login(t, "")
	f.login(t, "")
	f.clock.advance(domain.IdleTimeout)
	n, oldest, err := f.store.PurgeExpired(t.Context(), f.clock.Time(), 1)
	if err != nil || n != 1 || oldest.IsZero() {
		t.Fatal("bounded cleanup or remaining backlog failed", n, err)
	}
	n, oldest, err = f.store.PurgeExpired(t.Context(), f.clock.Time(), 500)
	if err != nil || n != 1 || oldest.IsZero() {
		t.Fatal("cleanup did not converge", n, err)
	}
	n, oldest, err = f.store.PurgeExpired(t.Context(), f.clock.Time(), 500)
	if err != nil || n != 0 || !oldest.IsZero() {
		t.Fatal("empty cleanup reported overdue work", n, err)
	}
}
func TestCompanyAdmissionProofRaceAndFailedRotationAreAtomic(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	start := make(chan struct{})
	results := make(chan error, domain.MaxLiveSessions+2)
	for range domain.MaxLiveSessions + 2 {
		go func() {
			<-start
			_, err := f.service.Login(ctx, f.auth.Session.FamilyID, f.auth.Session.AccessToken, "")
			results <- err
		}()
	}
	close(start)
	ok, limited := 0, 0
	for range domain.MaxLiveSessions + 2 {
		err := <-results
		switch {
		case err == nil:
			ok++
		case errors.Is(err, domain.ErrRateLimited):
			limited++
		default:
			t.Fatal(err)
		}
	}
	if ok != domain.MaxLiveSessions || limited != 2 {
		t.Fatal("concurrent admission overshot", ok, limited)
	}
	var count int
	if err := f.db.Admin.QueryRow(ctx, "SELECT count(*) FROM company_web_sessions").Scan(&count); err != nil || count != domain.MaxLiveSessions {
		t.Fatal(count, err)
	}
	f = setup(t)
	old := f.login(t, "")
	// The source account proof is valid during Prove, then rotates before insert.
	prove := f.service.Prove
	f.service.Prove = func(ctx context.Context, family, token string) (app.Proof, error) {
		proof, err := prove(ctx, family, token)
		if err != nil {
			return app.Proof{}, err
		}
		_, err = f.accounts.Refresh(ctx, f.auth.Session.FamilyID, f.auth.Session.RefreshToken)
		return proof, err
	}
	if _, err := f.service.Login(ctx, f.auth.Session.FamilyID, f.auth.Session.AccessToken, old.Token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("stale proof won refresh race", err)
	}
	if _, err := f.service.Authenticate(ctx, old.Token, old.CSRF, false); err != nil {
		t.Fatal("failed exchange revoked prior cookie", err)
	}
	f = setup(t)
	old = f.login(t, "")
	if _, err := f.db.Admin.Exec(ctx, `CREATE FUNCTION company_commit_failure() RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN RAISE EXCEPTION 'synthetic company commit failure'; END $$;
	CREATE CONSTRAINT TRIGGER company_commit_failure AFTER INSERT ON company_web_sessions
	DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION company_commit_failure()`); err != nil {
		t.Fatal(err)
	}
	if result, err := f.service.Login(ctx, f.auth.Session.FamilyID, f.auth.Session.AccessToken, old.Token); !errors.Is(err, domain.ErrUnavailable) || result.Token != "" {
		t.Fatal("failed commit issued credentials", err)
	}
	if _, err := f.service.Authenticate(ctx, old.Token, old.CSRF, false); err != nil {
		t.Fatal("failed rotation lost old cookie", err)
	}
	if _, err := f.db.Admin.Exec(ctx, "DROP TRIGGER company_commit_failure ON company_web_sessions"); err != nil {
		t.Fatal(err)
	}
	f.login(t, old.Token)
}
func post(t *testing.T, server *httptest.Server, path string, body any, cookies []*http.Cookie, csrf string, change func(*http.Request)) (*http.Response, []byte) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	r, err := http.NewRequest("POST", server.URL+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Origin", server.URL)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("If-None-Match", "*")
	if csrf != "" {
		r.Header.Set(web.CSRFHeader, csrf)
	}
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	if change != nil {
		change(r)
	}
	response, err := server.Client().Do(r)
	if err != nil {
		t.Fatal("TLS request failed")
	}
	raw, err = io.ReadAll(io.LimitReader(response.Body, 32<<10))
	response.Body.Close()
	if err != nil {
		t.Fatal("response read failed")
	}
	return response, raw
}
func TestCompanyAndAdminCookiePlanesThroughTheirHTTPEntries(t *testing.T) {
	f := setup(t)
	router := chi.NewRouter()
	server := httptest.NewTLSServer(router)
	defer server.Close()
	company, err := web.New(f.service, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	company.RegisterRoutes(router)
	rawAdminKey := admindomain.KeyPrefix + base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	hash := admindomain.HashKey(rawAdminKey)
	adminService := &admin.Service{Store: adminstore.NewStore(f.db.Admin), Keys: []admindomain.ConfiguredKey{{Hash: hash, Fingerprint: admindomain.Fingerprint(hash)}}, Clock: f.clock.Time, NewID: jobs.NewUUIDv4}
	adminHandler := adminhttp.Handler{Service: adminService, Origin: "https://admin.example.invalid", AllowedCIDRs: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}, ClientIP: func(*http.Request) (string, error) { return "127.0.0.1", nil }}
	adminHandler.RegisterRoutes(router)
	body := map[string]string{"family_id": f.auth.Session.FamilyID, "access_token": f.auth.Session.AccessToken}
	response, raw := post(t, server, web.PathPrefix+"login", body, nil, "", nil)
	if response.StatusCode != 200 || len(response.Cookies()) != 1 {
		t.Fatal("company HTTP login failed", response.StatusCode)
	}
	cookie := response.Cookies()[0]
	var result struct {
		AccountID string `json:"account_id"`
		CSRF      string `json:"csrf_token"`
	}
	if json.Unmarshal(raw, &result) != nil || result.AccountID != f.auth.Account.ID {
		t.Fatal("wrong company identity")
	}
	if cookie.Name != web.CookieName || cookie.Domain != "" || cookie.Path != "/" || !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || strings.Contains(string(raw), cookie.Value) {
		t.Fatal("unsafe company cookie")
	}
	adminResponse, adminRaw := post(t, server, "/v1/admin/session", map[string]string{"key": rawAdminKey}, nil, "", func(r *http.Request) { r.Header.Set("Origin", "https://admin.example.invalid") })
	if adminResponse.StatusCode != 200 || len(adminResponse.Cookies()) != 1 {
		t.Fatal("admin fixture login failed", adminResponse.StatusCode)
	}
	adminCookie := adminResponse.Cookies()[0]
	var adminResult struct {
		CSRF string `json:"csrf_token"`
	}
	if json.Unmarshal(adminRaw, &adminResult) != nil {
		t.Fatal("admin fixture decode")
	}
	response, _ = post(t, server, web.PathPrefix+"read", map[string]any{}, []*http.Cookie{adminCookie}, adminResult.CSRF, nil)
	if response.StatusCode != 401 {
		t.Fatal("admin cookie authorized company", response.StatusCode)
	}
	request, _ := http.NewRequest("GET", server.URL+"/v1/admin/session", nil)
	request.AddCookie(cookie)
	adminOnly, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	adminOnly.Body.Close()
	if adminOnly.StatusCode != 401 {
		t.Fatal("company cookie authorized admin", adminOnly.StatusCode)
	}
	renamed := *adminCookie
	renamed.Name = web.CookieName
	response, _ = post(t, server, web.PathPrefix+"read", map[string]any{}, []*http.Cookie{&renamed}, result.CSRF, nil)
	if response.StatusCode != 401 {
		t.Fatal("renamed admin cookie authorized company")
	}
	for _, change := range []func(*http.Request){
		func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") },
		func(r *http.Request) { r.Header.Del(web.CSRFHeader) },
		func(r *http.Request) { r.Header.Set(web.CSRFHeader, domain.CSRFPrefix+strings.Repeat("a", 64)) },
		func(r *http.Request) { r.Header.Add(web.CSRFHeader, result.CSRF) },
		func(r *http.Request) { r.AddCookie(cookie) },
	} {
		response, _ = post(t, server, web.PathPrefix+"read", map[string]any{}, []*http.Cookie{cookie}, result.CSRF, change)
		if response.StatusCode == 200 || response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("ETag") != "" {
			t.Fatal("unsafe company HTTP request succeeded", response.StatusCode)
		}
	}
	response, _ = post(t, server, web.PathPrefix+"read", map[string]any{}, []*http.Cookie{cookie, adminCookie}, result.CSRF, nil)
	if response.StatusCode != 200 || response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("ETag") != "" {
		t.Fatal("valid private session read failed", response.StatusCode)
	}
	response, _ = post(t, server, web.PathPrefix+"logout", map[string]any{}, []*http.Cookie{cookie}, result.CSRF, nil)
	if response.StatusCode != 200 || len(response.Cookies()) != 1 || response.Cookies()[0].Name != web.CookieName || response.Cookies()[0].MaxAge != -1 {
		t.Fatal("company logout did not clear only its cookie")
	}
	if _, err := adminService.Authenticate(t.Context(), adminCookie.Value); err != nil {
		t.Fatal("company logout affected admin", err)
	}
	if _, err := f.accounts.ValidateAccess(t.Context(), f.auth.Session.FamilyID, f.auth.Session.AccessToken); err != nil {
		t.Fatal("company logout affected personal account", err)
	}
	// Admin logout likewise leaves a separately issued company cookie alive.
	newCompany := f.login(t, "")
	if err := adminService.Store.(*adminstore.Store).Pool.Ping(t.Context()); err != nil {
		t.Fatal(err)
	}
	adminDelete, _ := http.NewRequest("DELETE", server.URL+"/v1/admin/session", strings.NewReader(`{"reason":"Synthetic logout isolation"}`))
	adminDelete.AddCookie(adminCookie)
	adminDelete.Header.Set("Origin", "https://admin.example.invalid")
	adminDelete.Header.Set("Content-Type", "application/json")
	adminDelete.Header.Set("X-Admin-CSRF", adminResult.CSRF)
	operation, _ := jobs.NewUUIDv4()
	adminDelete.Header.Set("Idempotency-Key", operation)
	adminOut, err := server.Client().Do(adminDelete)
	if err != nil {
		t.Fatal(err)
	}
	adminOut.Body.Close()
	if adminOut.StatusCode != 204 {
		t.Fatal("admin fixture logout failed", adminOut.StatusCode)
	}
	if _, err := f.service.Authenticate(t.Context(), newCompany.Token, newCompany.CSRF, false); err != nil {
		t.Fatal("admin logout affected company", err)
	}
}
func TestCompanyMiddlewareKeepsItsOwnContextAndRouteBoundary(t *testing.T) {
	f := setup(t)
	issued := f.login(t, "")
	router := chi.NewRouter()
	server := httptest.NewTLSServer(router)
	defer server.Close()
	company, err := web.New(f.service, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	var called atomic.Int32
	entry := company.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		row, ok := web.SessionFromContext(r.Context())
		if !ok || row.AccountID != f.auth.Account.ID {
			t.Error("missing independent company identity")
			w.WriteHeader(500)
			return
		}
		called.Add(1)
		w.WriteHeader(200)
	}))
	router.Method("POST", "/v1/org-access/probe", entry)
	router.Method("POST", "/v1/admin/company-probe", entry)
	cookie := &http.Cookie{Name: web.CookieName, Value: issued.Token}
	response, _ := post(t, server, "/v1/org-access/probe", map[string]any{}, []*http.Cookie{cookie}, issued.CSRF, nil)
	if response.StatusCode != 200 || called.Load() != 1 || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("company middleware refused valid session", response.StatusCode)
	}
	response, _ = post(t, server, "/v1/admin/company-probe", map[string]any{}, []*http.Cookie{cookie}, issued.CSRF, nil)
	if response.StatusCode != 404 || called.Load() != 1 {
		t.Fatal("company middleware accepted admin namespace", response.StatusCode)
	}
	response, _ = post(t, server, "/v1/org-access/probe", map[string]any{}, []*http.Cookie{cookie}, "", nil)
	if response.StatusCode != 403 || called.Load() != 1 {
		t.Fatal("company middleware omitted CSRF", response.StatusCode)
	}
	if _, ok := web.SessionFromContext(t.Context()); ok {
		t.Fatal("company identity leaked outside middleware")
	}
}
func TestCompanyGatewayRechecksPathScopeAndFreshnessWithoutIdleRenewal(t *testing.T) {
	f := setup(t)
	issued := f.login(t, "")
	origin := "https://company.example.invalid"
	company, err := web.New(f.service, origin)
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	calls := 0
	orgA, orgB := f.db.Organizations[0].ID, f.db.Organizations[1].ID
	gateway := orgapi.Gateway{Enabled: true, Origin: origin, CursorSecret: bytes.Repeat([]byte{1}, 32), WebSession: company.WebSession, WebSessionConfirm: company.WebSessionConfirm,
		Resolve: func(_ context.Context, actor orgapi.Actor, scope kernel.OrgScope, _ orgapi.Rule) (orgapi.Membership, error) {
			if scope.OrgID().String() != orgA {
				return orgapi.Membership{}, orgapi.NotFound
			}
			return orgapi.Membership{ID: orgA, AccountID: actor.AccountID, OrgID: scope.OrgID()}, nil
		}}
	if err := orgapi.Register(router, "/v1/orgs/{org_id}/synthetic/read", gateway, orgapi.Rule{Permission: "org.settings.read", Fresh: true}, func(context.Context, orgapi.Principal, orgapi.Session) (any, error) {
		calls++
		return map[string]bool{"ok": true}, nil
	}); err != nil {
		t.Fatal(err)
	}
	call := func(org string) int {
		r := httptest.NewRequest("POST", origin+"/v1/orgs/"+org+"/synthetic/read", strings.NewReader("{}"))
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set(web.CSRFHeader, issued.CSRF)
		r.AddCookie(&http.Cookie{Name: web.CookieName, Value: issued.Token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w.Code
	}
	if status := call(orgA); status != 200 || calls != 1 {
		t.Fatal("current company scope refused", status)
	}
	f.clock.advance(domain.FreshWindow)
	before := seen(t, f, issued.Row.ID)
	if status := call(orgB); status != 404 {
		t.Fatal("cookie inferred another organization", status)
	}
	if status := call(orgA); status != 401 || calls != 1 {
		t.Fatal("stale company authentication accepted", status)
	}
	if !seen(t, f, issued.Row.ID).Equal(before) {
		t.Fatal("foreign/fresh denial renewed idle")
	}
}
