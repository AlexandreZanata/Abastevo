//go:build integration

package http

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	identityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	identityauth "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/auth"
	identitydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	privacyadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/adapters"
	privacyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	privacydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

// CS12A: consent endpoints against the live stack — real verifier,
// real registrar attribution, real guarded stores on disposable
// PostGIS, real ECDSA proofs. Every endpoint proves the owner,
// guest, revoked-key, tampered-signature and replay paths through
// the same composition cmd/api mounts. Terms come from the same
// frozen version the server enforces; the digest is fixture-scoped.

type signedCaller struct {
	priv *ecdsa.PrivateKey
	fp   string
	id   string
}

func signedConsentFixture(t *testing.T) (chi.Router, *pgxpool.Pool) {
	return signedConsentFixtureFor(t, "api.example.invalid", testConsentTerms{})
}

func signedConsentFixtureFor(t *testing.T, authority string, terms privacyapp.TermsProvider) (chi.Router, *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, consentTestDSN(t))
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("privacy_consent_signed_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, consentTestDSN(t))
		if err != nil {
			t.Errorf("admin connect for drop: %v", err)
			return
		}
		defer admin.Close(ctx)
		if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Errorf("drop database: %v", err)
		}
	})
	u, err := url.Parse(consentTestDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	dsn := u.String()
	if _, err := migrate.Apply(ctx, dsn, dbmigrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	registrar := identityadapters.NewRegistrar(pool)
	verifier := &identityauth.Verifier{Pool: pool, Authority: authority}
	ports := privacyapp.ConsentPorts{
		Clock: time.Now,
		Attribution: func(ctx context.Context, contributorID string) (string, error) {
			return registrar.AttributionToken(ctx, contributorID)
		},
		Store: privacyadapters.NewConsentStore(pool),
		Terms: terms,
	}
	h := Handler{
		Authenticate: func(r *http.Request) (privacyapp.Caller, error) {
			id, err := verifier.Verify(r.Context(), r)
			if err != nil {
				return privacyapp.Caller{}, err
			}
			return privacyapp.Caller{ContributorID: id.ContributorID}, nil
		},
		GrantConsent: func(ctx context.Context, caller privacyapp.Caller, dto privacyapp.GrantDTO) (privacyapp.ConsentResult, error) {
			return privacyapp.GrantConsent(ctx, ports, caller, dto)
		},
		RevokeConsent: func(ctx context.Context, caller privacyapp.Caller, dto privacyapp.RevokeDTO) (privacyapp.ConsentResult, error) {
			return privacyapp.RevokeConsent(ctx, ports, caller, dto)
		},
		ReadConsent: func(ctx context.Context, caller privacyapp.Caller) (privacyapp.ConsentView, error) {
			return privacyapp.ReadConsent(ctx, ports, caller)
		},
	}
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r, pool
}

type testConsentTerms struct{}

func (testConsentTerms) CurrentVersion() string { return privacydomain.TrainingTermsV1 }
func (testConsentTerms) CurrentDigest() string  { return "digest-v1" }

func enrollSignedCaller(t *testing.T, pool *pgxpool.Pool) *signedCaller {
	t.Helper()
	ctx := context.Background()
	reg := identityadapters.NewRegistrar(pool)
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := priv.PublicKey.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding.EncodeToString
	x, y := enc(raw[1:33]), enc(raw[33:65])
	thumb := `{"crv":"P-256","kty":"EC","x":"` + x + `","y":"` + y + `"}`
	sum := sha256.Sum256([]byte(thumb))
	fp := fmt.Sprintf("fp:%x", sum)
	ch, err := reg.IssueChallenge(ctx, fp, identitydomain.PurposeRegister)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	lines := []string{
		`"@method": POST`,
		`"@authority": test.invalid`,
		`"@path": /v1/contributors`,
		`"@query": `,
		fmt.Sprintf(`"created": %d`, now.Add(-time.Minute).Unix()),
		fmt.Sprintf(`"expires": %d`, now.Add(4*time.Minute).Unix()),
		`"keyid": "` + fp + `"`,
		`"nonce": "` + ch.Nonce + `"`,
	}
	joined := ""
	for i, l := range lines {
		if i > 0 {
			joined += "\n"
		}
		joined += l
	}
	digest := sha512.Sum512([]byte(joined))
	rr, ss, err := ecdsa.Sign(rand.Reader, priv, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := append(rr.FillBytes(make([]byte, 32)), ss.FillBytes(make([]byte, 32))...)
	registration, err := reg.Register(ctx, identitydomain.RegistrationRequest{
		JWKX: x, JWKY: y, Challenge: ch,
		BaseLines: lines, Signature: base64.RawURLEncoding.EncodeToString(sig),
		VerifiedAt: now,
	})
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	return &signedCaller{priv: priv, fp: fp, id: registration.ContributorID}
}

func signedConsentRequest(t *testing.T, pool *pgxpool.Pool, c *signedCaller, method, target, body string) *http.Request {
	t.Helper()
	ctx := context.Background()
	reg := identityadapters.NewRegistrar(pool)
	ch, err := reg.IssueChallenge(ctx, c.fp, identitydomain.PurposeSign)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	u, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	lines := []string{
		`"@method": ` + method,
		`"@authority": api.example.invalid`,
		`"@path": ` + u.EscapedPath(),
		`"@query": ` + u.RawQuery,
	}
	ct := ""
	if body != "" {
		ct = "application/json"
		sum := sha512.Sum512([]byte(body))
		lines = append(lines,
			`"content-type": `+ct,
			`"content-digest": "sha-512=:`+base64.StdEncoding.EncodeToString(sum[:])+`:"`,
		)
	}
	lines = append(lines,
		fmt.Sprintf(`"created": %d`, now.Add(-time.Minute).Unix()),
		fmt.Sprintf(`"expires": %d`, now.Add(4*time.Minute).Unix()),
		`"keyid": "`+c.fp+`"`,
		`"nonce": "`+ch.Nonce+`"`,
	)
	joined := ""
	for i, l := range lines {
		if i > 0 {
			joined += "\n"
		}
		joined += l
	}
	digest := sha512.Sum512([]byte(joined))
	r, s, err := ecdsa.Sign(rand.Reader, c.priv, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)
	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if body != "" {
		req.Header.Set("Content-Type", ct)
	}
	req.Header.Set("Signature", base64.RawURLEncoding.EncodeToString(sig))
	req.Header.Set("Signature-Created", fmt.Sprint(now.Add(-time.Minute).Unix()))
	req.Header.Set("Signature-Expires", fmt.Sprint(now.Add(4*time.Minute).Unix()))
	req.Header.Set("Signature-Keyid", c.fp)
	req.Header.Set("Signature-Nonce", ch.Nonce)
	return req
}

func doSigned(t *testing.T, r chi.Router, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func grantBody(revision int) string {
	return fmt.Sprintf(`{"terms_version":"training-terms-v1","terms_digest":"digest-v1","revision":%d}`, revision)
}

func TestSignedGrantStatusRevokeFlow(t *testing.T) {
	r, pool := signedConsentFixture(t)
	c := enrollSignedCaller(t, pool)
	w := doSigned(t, r, signedConsentRequest(t, pool, c, http.MethodPost, "/v1/privacy/training-consent/grant", grantBody(1)))
	if w.Code != http.StatusOK {
		t.Fatalf("grant = %d: %s", w.Code, w.Body.String())
	}
	var receipt map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt["state"] != privacydomain.StateGranted || receipt["replayed"] != false {
		t.Fatalf("receipt = %v", receipt)
	}
	w = doSigned(t, r, signedConsentRequest(t, pool, c, http.MethodGet, "/v1/privacy/training-consent", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	var view map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view["granted"] != true || view["terms_digest"] != "digest-v1" {
		t.Fatalf("view = %v", view)
	}
	w = doSigned(t, r, signedConsentRequest(t, pool, c, http.MethodPost, "/v1/privacy/training-consent/revoke", `{"revision":2}`))
	if w.Code != http.StatusOK {
		t.Fatalf("revoke = %d: %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt["delete_owned_training_assets"] != true {
		t.Fatalf("revoke receipt = %v", receipt)
	}
	w = doSigned(t, r, signedConsentRequest(t, pool, c, http.MethodGet, "/v1/privacy/training-consent", ""))
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view["granted"] != false {
		t.Fatalf("post-revoke view = %v", view)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q", w.Header().Get("Cache-Control"))
	}
}

func TestSignedReplayConvergesLiveClock(t *testing.T) {
	r, pool := signedConsentFixture(t)
	c := enrollSignedCaller(t, pool)
	w := doSigned(t, r, signedConsentRequest(t, pool, c, http.MethodPost, "/v1/privacy/training-consent/grant", grantBody(1)))
	if w.Code != http.StatusOK {
		t.Fatalf("grant = %d: %s", w.Code, w.Body.String())
	}
	// Lost response under a live clock: same revision retries converge.
	w = doSigned(t, r, signedConsentRequest(t, pool, c, http.MethodPost, "/v1/privacy/training-consent/grant", grantBody(1)))
	if w.Code != http.StatusOK {
		t.Fatalf("replay = %d: %s", w.Code, w.Body.String())
	}
	var receipt map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt["replayed"] != true {
		t.Fatalf("replay receipt = %v", receipt)
	}
}

func TestSignedSecondOwnerIsolated(t *testing.T) {
	r, pool := signedConsentFixture(t)
	owner := enrollSignedCaller(t, pool)
	other := enrollSignedCaller(t, pool)
	w := doSigned(t, r, signedConsentRequest(t, pool, owner, http.MethodPost, "/v1/privacy/training-consent/grant", grantBody(1)))
	if w.Code != http.StatusOK {
		t.Fatalf("owner grant = %d", w.Code)
	}
	w = doSigned(t, r, signedConsentRequest(t, pool, other, http.MethodGet, "/v1/privacy/training-consent", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("other status = %d", w.Code)
	}
	var view map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view["granted"] != false {
		t.Fatalf("cross-owner read = %v", view)
	}
}

func TestGuestRefusesAllEndpoints(t *testing.T) {
	r, pool := signedConsentFixture(t)
	_ = pool
	for _, tc := range []struct{ method, target, body string }{
		{http.MethodPost, "/v1/privacy/training-consent/grant", grantBody(1)},
		{http.MethodPost, "/v1/privacy/training-consent/revoke", `{"revision":1}`},
		{http.MethodGet, "/v1/privacy/training-consent", ""},
	} {
		req := httptest.NewRequest(tc.method, tc.target, bytes.NewBufferString(tc.body))
		w := doSigned(t, r, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", tc.method, tc.target, w.Code)
		}
		var errBody map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &errBody); err != nil {
			t.Fatal(err)
		}
		if inner, _ := errBody["error"].(map[string]any); inner["code"] != "privacy.auth-required" {
			t.Errorf("%s %s code = %v", tc.method, tc.target, inner["code"])
		}
	}
}

func TestRevokedKeyRefuses(t *testing.T) {
	r, pool := signedConsentFixture(t)
	c := enrollSignedCaller(t, pool)
	reg := identityadapters.NewRegistrar(pool)
	if _, _, err := reg.DeleteContributor(context.Background(), c.id); err != nil {
		t.Fatalf("revoke keys: %v", err)
	}
	w := doSigned(t, r, signedConsentRequest(t, pool, c, http.MethodPost, "/v1/privacy/training-consent/grant", grantBody(1)))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("revoked key grant = %d: %s", w.Code, w.Body.String())
	}
	// Nothing persisted through the refused write.
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM privacy_training_consents`).Scan(&n); err != nil || n != 0 {
		t.Errorf("rows = %d, %v", n, err)
	}
}

func TestTamperedSignatureRefuses(t *testing.T) {
	r, pool := signedConsentFixture(t)
	c := enrollSignedCaller(t, pool)
	req := signedConsentRequest(t, pool, c, http.MethodPost, "/v1/privacy/training-consent/grant", grantBody(1))
	// Flip one body byte after signing: the content digest no longer matches.
	raw := []byte(grantBody(1))
	raw[len(raw)-2] ^= 0x01
	req.Body = io.NopCloser(bytes.NewReader(raw))
	req.ContentLength = int64(len(raw))
	w := doSigned(t, r, req)
	if w.Code == http.StatusOK {
		t.Errorf("tampered body accepted: %s", w.Body.String())
	}
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM privacy_training_consents`).Scan(&n); err != nil || n != 0 {
		t.Errorf("rows = %d, %v", n, err)
	}
}

func TestStaleTermsRefuseLive(t *testing.T) {
	r, pool := signedConsentFixture(t)
	c := enrollSignedCaller(t, pool)
	w := doSigned(t, r, signedConsentRequest(t, pool, c, http.MethodPost, "/v1/privacy/training-consent/grant",
		`{"terms_version":"training-terms-v0","terms_digest":"digest-v0","revision":1}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("old terms = %d: %s", w.Code, w.Body.String())
	}
	var errBody map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &errBody); err != nil {
		t.Fatal(err)
	}
	if inner, _ := errBody["error"].(map[string]any); inner["code"] != "privacy.terms-outdated" {
		t.Errorf("code = %v", inner["code"])
	}
}
