//go:build integration

package auth

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	parent "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("integration database unreachable at %s: %v (start it: docker compose -f infra/compose.dev.yml up -d db)", dsn, err)
	}
	defer conn.Close(ctx)
	return dsn
}

func freshVerifier(t *testing.T) (*Verifier, *pgxpool.Pool) {
	t.Helper()
	adminDSN := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("auth_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, adminDSN)
		if err != nil {
			t.Errorf("admin connect for drop: %v", err)
			return
		}
		defer admin.Close(ctx)
		if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Errorf("drop database: %v", err)
		}
	})
	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u.Path = "/" + name
	dsn := u.String()
	if _, err := migrate.Apply(ctx, dsn, dbmigrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return &Verifier{Pool: pool, Authority: "api.example.invalid"}, pool
}

// signer holds a registered key for proof building.
type signer struct {
	priv        *ecdsa.PrivateKey
	x, y, fp    string
	contributor string
}

// enroll registers the key through the real Registrar and returns a signer.
func enroll(t *testing.T, pool *pgxpool.Pool) *signer {
	t.Helper()
	ctx := context.Background()
	reg := parent.NewRegistrar(pool)
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
	ch, err := reg.IssueChallenge(ctx, fp, domain.PurposeRegister)
	if err != nil {
		t.Fatalf("challenge: %v", err)
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
	r, s, err := ecdsa.Sign(rand.Reader, priv, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)
	res, err := reg.Register(ctx, domain.RegistrationRequest{
		JWKX: x, JWKY: y, Challenge: ch,
		BaseLines: lines, Signature: base64.RawURLEncoding.EncodeToString(sig),
		VerifiedAt: now,
	})
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	return &signer{priv: priv, x: x, y: y, fp: fp, contributor: res.ContributorID}
}

// signedRequest builds a live request with a valid proof for a SIGN
// challenge. mutate alters one stored header afterwards to simulate
// tampering without the key. at overrides the proof time (zero = now).
func signedRequest(t *testing.T, pool *pgxpool.Pool, s *signer, method, target, body string, mutate func(http.Header), at time.Time) *http.Request {
	t.Helper()
	ctx := context.Background()
	reg := parent.NewRegistrar(pool)
	ch, err := reg.IssueChallenge(ctx, s.fp, domain.PurposeSign)
	if err != nil {
		t.Fatalf("sign challenge: %v", err)
	}
	now := at
	if now.IsZero() {
		now = time.Now()
	}
	u, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	ct := ""
	lines := []string{
		`"@method": ` + method,
		`"@authority": api.example.invalid`,
		`"@path": ` + u.EscapedPath(),
		`"@query": ` + u.RawQuery,
	}
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
		`"keyid": "`+s.fp+`"`,
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
	r, ss, err := ecdsa.Sign(rand.Reader, s.priv, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := append(r.FillBytes(make([]byte, 32)), ss.FillBytes(make([]byte, 32))...)
	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if body != "" {
		req.Header.Set("Content-Type", ct)
	}
	req.Header.Set(HeaderSignature, base64.RawURLEncoding.EncodeToString(sig))
	req.Header.Set(HeaderCreated, fmt.Sprint(now.Add(-time.Minute).Unix()))
	req.Header.Set(HeaderExpires, fmt.Sprint(now.Add(4*time.Minute).Unix()))
	req.Header.Set(HeaderKeyID, s.fp)
	req.Header.Set(HeaderNonce, ch.Nonce)
	if mutate != nil {
		mutate(req.Header)
	}
	return req
}

func TestSignedReadHappyPath(t *testing.T) {
	v, pool := freshVerifier(t)
	ctx := context.Background()
	s := enroll(t, pool)
	req := signedRequest(t, pool, s, http.MethodGet, "/v1/contributors/me?deep=true", "", nil, time.Time{})
	id, err := v.Verify(ctx, req)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if id.ContributorID != s.contributor || id.Fingerprint != s.fp || id.KeyID == "" {
		t.Errorf("identity = %+v, want contributor %s", id, s.contributor)
	}
	// The identity derives server-side: no client ID was ever supplied.
}

func TestSignedWriteWithBody(t *testing.T) {
	v, pool := freshVerifier(t)
	ctx := context.Background()
	s := enroll(t, pool)
	req := signedRequest(t, pool, s, http.MethodPost, "/v1/observations", `{"a":1}`, nil, time.Time{})
	if _, err := v.Verify(ctx, req); err != nil {
		t.Fatalf("verify with body: %v", err)
	}
}

func TestTamperedComponentsRejected(t *testing.T) {
	v, pool := freshVerifier(t)
	ctx := context.Background()
	s := enroll(t, pool)
	// Tampered path: sign for /a, send to /b with the same proof headers.
	req := signedRequest(t, pool, s, http.MethodGet, "/v1/contributors/me", "", nil, time.Time{})
	req.URL.Path = "/v1/other"
	if _, err := v.Verify(ctx, req); err == nil {
		t.Error("tampered path accepted")
	}
	// Tampered query.
	req = signedRequest(t, pool, s, http.MethodGet, "/v1/contributors/me?deep=true", "", nil, time.Time{})
	req.URL.RawQuery = "deep=false"
	if _, err := v.Verify(ctx, req); err == nil {
		t.Error("tampered query accepted")
	}
	// Tampered body.
	req = signedRequest(t, pool, s, http.MethodPost, "/v1/observations", `{"a":1}`, nil, time.Time{})
	req.Body = io.NopCloser(bytes.NewBufferString(`{"a":2}`))
	if _, err := v.Verify(ctx, req); err == nil {
		t.Error("tampered body accepted")
	}
	// Wrong authority configuration.
	v.Authority = "other.example"
	req = signedRequest(t, pool, s, http.MethodGet, "/v1/contributors/me", "", nil, time.Time{})
	if _, err := v.Verify(ctx, req); err == nil {
		t.Error("wrong authority accepted")
	}
	v.Authority = "api.example.invalid"
	// Duplicated auth header.
	req = signedRequest(t, pool, s, http.MethodGet, "/v1/contributors/me", "", nil, time.Time{})
	req.Header.Add(HeaderNonce, "second")
	if _, err := v.Verify(ctx, req); err == nil {
		t.Error("duplicated header accepted")
	}
	// Missing header.
	req = signedRequest(t, pool, s, http.MethodGet, "/v1/contributors/me", "", nil, time.Time{})
	req.Header.Del(HeaderSignature)
	if _, err := v.Verify(ctx, req); err == nil {
		t.Error("missing header accepted")
	}
}

func TestReplayAcceptedOnce(t *testing.T) {
	v, pool := freshVerifier(t)
	ctx := context.Background()
	s := enroll(t, pool)
	req := signedRequest(t, pool, s, http.MethodGet, "/v1/contributors/me", "", nil, time.Time{})
	if _, err := v.Verify(ctx, req); err != nil {
		t.Fatalf("first: %v", err)
	}
	// Rebuild an identical second request (fresh body reader, same headers).
	req2 := signedRequest(t, pool, s, http.MethodGet, "/v1/contributors/me", "", nil, time.Time{})
	_ = req2
	// Replay the EXACT first request: same nonce must fail.
	if _, err := v.Verify(ctx, req); err == nil {
		t.Error("replay accepted")
	} else if !errors.Is(err, ErrAuthReplayed) {
		t.Errorf("replay error = %v, want replayed", err)
	}
}

func TestConcurrentSameNonceAcceptsOnce(t *testing.T) {
	v, pool := freshVerifier(t)
	ctx := context.Background()
	s := enroll(t, pool)
	// One signed request cloned across workers: same nonce everywhere.
	base := signedRequest(t, pool, s, http.MethodGet, "/v1/contributors/me", "", nil, time.Time{})
	const workers = 8
	results := make([]error, workers)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := base.Clone(ctx)
			_, err := v.Verify(ctx, r)
			results[i] = err
		}(i)
	}
	wg.Wait()
	accepted := 0
	for i, err := range results {
		if err == nil {
			accepted++
		} else if !errors.Is(err, ErrAuthReplayed) {
			t.Fatalf("worker %d: unexpected %v", i, err)
		}
	}
	if accepted != 1 {
		t.Errorf("accepted %d, want exactly one", accepted)
	}
}

func TestExpiredRevokedBlockedUnknown(t *testing.T) {
	v, pool := freshVerifier(t)
	ctx := context.Background()
	s := enroll(t, pool)
	// Expired proof window: signed 10 minutes ago, verified now.
	stale := signedRequest(t, pool, s, http.MethodGet, "/v1/contributors/me", "", nil, time.Now().Add(-10*time.Minute))
	if _, err := v.Verify(ctx, stale); !errors.Is(err, ErrAuthExpired) {
		t.Errorf("stale proof acted: %v", err)
	}
	// Revoked key cannot act.
	if _, err := pool.Exec(ctx, "UPDATE identity_keys SET revoked_at = now() WHERE fingerprint = $1", s.fp); err != nil {
		t.Fatal(err)
	}
	req2 := signedRequest(t, pool, s, http.MethodGet, "/v1/contributors/me", "", nil, time.Time{})
	if _, err := v.Verify(ctx, req2); !errors.Is(err, ErrAuthDenied) {
		t.Errorf("revoked key acted: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE identity_keys SET revoked_at = NULL WHERE fingerprint = $1", s.fp); err != nil {
		t.Fatal(err)
	}
	// Blocked contributor cannot act.
	if _, err := pool.Exec(ctx, "UPDATE identity_contributors SET status = 'blocked' WHERE id = (SELECT contributor_id FROM identity_keys WHERE fingerprint = $1)", s.fp); err != nil {
		t.Fatal(err)
	}
	req3 := signedRequest(t, pool, s, http.MethodGet, "/v1/contributors/me", "", nil, time.Time{})
	if _, err := v.Verify(ctx, req3); !errors.Is(err, ErrAuthDenied) {
		t.Errorf("blocked contributor acted: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE identity_contributors SET status = 'active' WHERE id = (SELECT contributor_id FROM identity_keys WHERE fingerprint = $1)", s.fp); err != nil {
		t.Fatal(err)
	}
	// Unknown fingerprint cannot act.
	req4 := signedRequest(t, pool, s, http.MethodGet, "/v1/contributors/me", "", nil, time.Time{})
	req4.Header.Set(HeaderKeyID, "fp:"+stringsRepeat("d", 64))
	if _, err := v.Verify(ctx, req4); !errors.Is(err, ErrAuthDenied) {
		t.Errorf("unknown key acted: %v", err)
	}
}

func stringsRepeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
