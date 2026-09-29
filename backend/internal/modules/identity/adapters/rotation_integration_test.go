//go:build integration

package adapters

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"fmt"
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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/auth"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func testDSNRotation(t *testing.T) string {
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

func freshRotationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	adminDSN := testDSNRotation(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("rotation_test_%d", time.Now().UnixNano())
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
	return pool
}

// enrollKey registers a fresh key through the real Registrar.
func enrollKey(t *testing.T, pool *pgxpool.Pool) (*ecdsa.PrivateKey, string, domain.Registration) {
	t.Helper()
	ctx := context.Background()
	reg := NewRegistrar(pool)
	priv, x, y := testKey(t)
	fp := fingerprintOf(t, x, y)
	ch, err := reg.IssueChallenge(ctx, fp, domain.PurposeRegister)
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
	res, err := reg.Register(ctx, domain.RegistrationRequest{
		JWKX: x, JWKY: y, Challenge: ch,
		BaseLines: lines, Signature: signLines(t, priv, lines),
		VerifiedAt: now,
	})
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	return priv, fp, res
}

func signLines(t *testing.T, priv *ecdsa.PrivateKey, lines []string) string {
	t.Helper()
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
	raw := append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)
	return base64.RawURLEncoding.EncodeToString(raw)
}

// rotationProof builds one signed SIGN side bound to a fresh challenge.
func rotationProof(t *testing.T, pool *pgxpool.Pool, priv *ecdsa.PrivateKey, fp string) domain.RotationProof {
	t.Helper()
	ctx := context.Background()
	reg := NewRegistrar(pool)
	ch, err := reg.IssueChallenge(ctx, fp, domain.PurposeSign)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	lines := []string{
		`"@method": POST`,
		`"@authority": test.invalid`,
		`"@path": /v1/contributors/me/keys/rotate`,
		`"@query": `,
		fmt.Sprintf(`"created": %d`, now.Add(-time.Minute).Unix()),
		fmt.Sprintf(`"expires": %d`, now.Add(4*time.Minute).Unix()),
		`"keyid": "` + fp + `"`,
		`"nonce": "` + ch.Nonce + `"`,
	}
	return domain.RotationProof{
		Challenge: ch, BaseLines: lines, Signature: signLines(t, priv, lines),
	}
}

func rotateKeys(t *testing.T, pool *pgxpool.Pool, oldPriv *ecdsa.PrivateKey, oldFp string, newPriv *ecdsa.PrivateKey) (domain.Rotation, error) {
	t.Helper()
	reg := NewRegistrar(pool)
	nx, ny := testKeyCoords(t, newPriv)
	newFp := fingerprintOf(t, nx, ny)
	old := rotationProof(t, pool, oldPriv, oldFp)
	new := rotationProof(t, pool, newPriv, newFp)
	return reg.Rotate(context.Background(), domain.RotationRequest{
		Old: old, NewJWKX: nx, NewJWKY: ny, New: new, VerifiedAt: time.Now(),
	})
}

func testKeyCoords(t *testing.T, priv *ecdsa.PrivateKey) (string, string) {
	t.Helper()
	raw, err := priv.PublicKey.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding.EncodeToString
	return enc(raw[1:33]), enc(raw[33:65])
}

// signedGET builds a live signed request for the auth Verifier.
func signedGET(t *testing.T, pool *pgxpool.Pool, priv *ecdsa.PrivateKey, fp string) *http.Request {
	t.Helper()
	ctx := context.Background()
	reg := NewRegistrar(pool)
	ch, err := reg.IssueChallenge(ctx, fp, domain.PurposeSign)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	lines := []string{
		`"@method": GET`,
		`"@authority": api.example.invalid`,
		`"@path": /v1/contributors/me`,
		`"@query": `,
		fmt.Sprintf(`"created": %d`, now.Add(-time.Minute).Unix()),
		fmt.Sprintf(`"expires": %d`, now.Add(4*time.Minute).Unix()),
		`"keyid": "` + fp + `"`,
		`"nonce": "` + ch.Nonce + `"`,
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/contributors/me", nil)
	req.Header.Set(auth.HeaderSignature, signLines(t, priv, lines))
	req.Header.Set(auth.HeaderCreated, fmt.Sprint(now.Add(-time.Minute).Unix()))
	req.Header.Set(auth.HeaderExpires, fmt.Sprint(now.Add(4*time.Minute).Unix()))
	req.Header.Set(auth.HeaderKeyID, fp)
	req.Header.Set(auth.HeaderNonce, ch.Nonce)
	return req
}

func TestRotateHappyPath(t *testing.T) {
	pool := freshRotationPool(t)
	ctx := context.Background()
	oldPriv, oldFp, enrolled := enrollKey(t, pool)
	newPriv, _, _ := testKey(t)
	nx, ny := testKeyCoords(t, newPriv)
	newFp := fingerprintOf(t, nx, ny)
	res, err := rotateKeys(t, pool, oldPriv, oldFp, newPriv)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if res.ContributorID != enrolled.ContributorID || res.Existed {
		t.Errorf("rotation = %+v, enrolled = %+v", res, enrolled)
	}
	if res.OldKeyID != enrolled.KeyID || res.NewKeyID == "" || res.NewKeyID == res.OldKeyID {
		t.Errorf("key linkage = %+v", res)
	}
	v := &auth.Verifier{Pool: pool, Authority: "api.example.invalid"}
	if _, err := v.Verify(ctx, signedGET(t, pool, oldPriv, oldFp)); err == nil {
		t.Error("revoked old key still acts")
	}
	if _, err := v.Verify(ctx, signedGET(t, pool, newPriv, newFp)); err != nil {
		t.Errorf("new key denied: %v", err)
	}
}

func TestOldProofFailurePreservesChallenges(t *testing.T) {
	pool := freshRotationPool(t)
	oldPriv, oldFp, _ := enrollKey(t, pool)
	newPriv, _, _ := testKey(t)
	nx, ny := testKeyCoords(t, newPriv)
	newFp := fingerprintOf(t, nx, ny)
	reg := NewRegistrar(pool)
	old := rotationProof(t, pool, oldPriv, oldFp)
	badLines := append([]string{}, old.BaseLines...)
	badLines[2] = `"/v1/other"`
	badOld := domain.RotationProof{Challenge: old.Challenge, BaseLines: badLines, Signature: old.Signature}
	new := rotationProof(t, pool, newPriv, newFp)
	if _, err := reg.Rotate(context.Background(), domain.RotationRequest{
		Old: badOld, NewJWKX: nx, NewJWKY: ny, New: new, VerifiedAt: time.Now(),
	}); err == nil {
		t.Fatal("tampered old proof accepted")
	}
	// Both challenges survive: a corrected rotation with the same pair works.
	res, err := rotateKeys(t, pool, oldPriv, oldFp, newPriv)
	if err != nil {
		t.Errorf("retry after failure = %v", err)
	} else if res.Existed {
		t.Errorf("retry = %+v", res)
	}
}

func TestNewProofFailureAborts(t *testing.T) {
	pool := freshRotationPool(t)
	oldPriv, oldFp, _ := enrollKey(t, pool)
	newPriv, _, _ := testKey(t)
	nx, ny := testKeyCoords(t, newPriv)
	newFp := fingerprintOf(t, nx, ny)
	reg := NewRegistrar(pool)
	old := rotationProof(t, pool, oldPriv, oldFp)
	goodNew := rotationProof(t, pool, newPriv, newFp)
	badLines := append([]string{}, goodNew.BaseLines...)
	badLines[7] = `"nonce": "forged.nonce"`
	badNew := domain.RotationProof{Challenge: goodNew.Challenge, BaseLines: badLines, Signature: goodNew.Signature}
	if _, err := reg.Rotate(context.Background(), domain.RotationRequest{
		Old: old, NewJWKX: nx, NewJWKY: ny, New: badNew, VerifiedAt: time.Now(),
	}); err == nil {
		t.Fatal("tampered new proof accepted")
	}
	// Old key untouched and still usable.
	v := &auth.Verifier{Pool: pool, Authority: "api.example.invalid"}
	if _, err := v.Verify(context.Background(), signedGET(t, pool, oldPriv, oldFp)); err != nil {
		t.Errorf("old key damaged by failed rotation: %v", err)
	}
}

func TestConcurrentRotationsPreserveContributor(t *testing.T) {
	pool := freshRotationPool(t)
	oldPriv, oldFp, enrolled := enrollKey(t, pool)
	const workers = 4
	type outcome struct {
		res domain.Rotation
		err error
	}
	outs := make([]outcome, workers)
	var wg sync.WaitGroup
	for i := range outs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			np, _, _ := testKey(t)
			res, err := rotateKeys(t, pool, oldPriv, oldFp, np)
			outs[i] = outcome{res: res, err: err}
		}(i)
	}
	wg.Wait()
	// Exactly one rotation wins; concurrent losers observe the revoked old
	// key and fail instead of forking identity or stacking keys.
	won := 0
	for i, o := range outs {
		if o.err != nil {
			continue
		}
		won++
		if o.res.ContributorID != enrolled.ContributorID {
			t.Fatalf("worker %d forked identity: %+v", i, o.res)
		}
	}
	if won != 1 {
		t.Fatalf("winners = %d, want exactly one", won)
	}
	var keys int64
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM identity_keys").Scan(&keys); err != nil {
		t.Fatal(err)
	}
	if keys != 2 {
		t.Errorf("keys = %d, want old plus exactly one new", keys)
	}
	var revoked int64
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM identity_keys WHERE revoked_at IS NOT NULL").Scan(&revoked); err != nil {
		t.Fatal(err)
	}
	if revoked != 1 {
		t.Errorf("revoked = %d, want exactly the old key", revoked)
	}
}

func TestIdempotentReplayAfterSuccess(t *testing.T) {
	pool := freshRotationPool(t)
	oldPriv, oldFp, enrolled := enrollKey(t, pool)
	newPriv, _, _ := testKey(t)
	first, err := rotateKeys(t, pool, oldPriv, oldFp, newPriv)
	if err != nil {
		t.Fatalf("first rotation: %v", err)
	}
	// Same rotation again with fresh challenges: proofs verify, challenges
	// consume, and the existing binding returns without key changes.
	again, err := rotateKeys(t, pool, oldPriv, oldFp, newPriv)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !again.Existed || again.ContributorID != enrolled.ContributorID || again.NewKeyID != first.NewKeyID {
		t.Errorf("replay = %+v, first = %+v", again, first)
	}
	var keys int64
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM identity_keys").Scan(&keys); err != nil {
		t.Fatal(err)
	}
	if keys != 2 {
		t.Errorf("keys = %d, want no duplicates from replay", keys)
	}
}

func TestTakeoverRefused(t *testing.T) {
	pool := freshRotationPool(t)
	oldPriv, oldFp, _ := enrollKey(t, pool)
	victimPriv, _, _ := testKey(t)
	victimFp := func() string {
		x, y := testKeyCoords(t, victimPriv)
		return fingerprintOf(t, x, y)
	}()
	victimReg := NewRegistrar(pool)
	vch, err := victimReg.IssueChallenge(context.Background(), victimFp, domain.PurposeRegister)
	if err != nil {
		t.Fatal(err)
	}
	_ = vch
	// Register the victim key properly first.
	vx, vy := testKeyCoords(t, victimPriv)
	now := time.Now()
	vlines := []string{
		`"@method": POST`,
		`"@authority": test.invalid`,
		`"@path": /v1/contributors`,
		`"@query": `,
		fmt.Sprintf(`"created": %d`, now.Add(-time.Minute).Unix()),
		fmt.Sprintf(`"expires": %d`, now.Add(4*time.Minute).Unix()),
		`"keyid": "` + victimFp + `"`,
		`"nonce": "` + vch.Nonce + `"`,
	}
	_ = now
	if _, err := victimReg.Register(context.Background(), domain.RegistrationRequest{
		JWKX: vx, JWKY: vy, Challenge: vch,
		BaseLines: vlines, Signature: signLines(t, victimPriv, vlines),
		VerifiedAt: time.Now(),
	}); err != nil {
		t.Fatalf("victim enroll: %v", err)
	}
	// Attack: rotate the old identity into the victim's key.
	reg := NewRegistrar(pool)
	old := rotationProof(t, pool, oldPriv, oldFp)
	new := rotationProof(t, pool, victimPriv, victimFp)
	if _, err := reg.Rotate(context.Background(), domain.RotationRequest{
		Old: old, NewJWKX: vx, NewJWKY: vy, New: new, VerifiedAt: time.Now(),
	}); !errors.Is(err, domain.ErrKeyTakeover) {
		t.Errorf("takeover = %v, want takeover refusal", err)
	}
	// Attacker's old key stays live and unrevoked.
	var revoked bool
	if err := pool.QueryRow(context.Background(),
		"SELECT revoked_at IS NOT NULL FROM identity_keys WHERE fingerprint = $1", oldFp).Scan(&revoked); err != nil {
		t.Fatal(err)
	}
	if revoked {
		t.Error("failed takeover revoked the old key")
	}
}

func TestLostKeyHasNoRecovery(t *testing.T) {
	pool := freshRotationPool(t)
	_, oldFp, _ := enrollKey(t, pool)
	strangerPriv, _, _ := testKey(t)
	sx, sy := testKeyCoords(t, strangerPriv)
	strangerFp := fingerprintOf(t, sx, sy)
	reg := NewRegistrar(pool)
	// Stranger signs a well-formed old proof, but the fingerprint was never
	// registered: no identity to preserve, no rotation.
	ch, err := reg.IssueChallenge(context.Background(), strangerFp, domain.PurposeSign)
	if err != nil {
		t.Fatal(err)
	}
	_ = ch
	_ = oldFp
	now := time.Now()
	lines := []string{
		`"@method": POST`,
		`"@authority": test.invalid`,
		`"@path": /v1/contributors/me/keys/rotate`,
		`"@query": `,
		fmt.Sprintf(`"created": %d`, now.Add(-time.Minute).Unix()),
		fmt.Sprintf(`"expires": %d`, now.Add(4*time.Minute).Unix()),
		`"keyid": "` + strangerFp + `"`,
		`"nonce": "` + ch.Nonce + `"`,
	}
	newPriv, _, _ := testKey(t)
	nx, ny := testKeyCoords(t, newPriv)
	newFp := fingerprintOf(t, nx, ny)
	newCh, err := reg.IssueChallenge(context.Background(), newFp, domain.PurposeSign)
	if err != nil {
		t.Fatal(err)
	}
	_ = now
	newLines := append([]string{}, lines...)
	newLines[7] = `"nonce": "` + newCh.Nonce + `"`
	if _, err := reg.Rotate(context.Background(), domain.RotationRequest{
		Old:     domain.RotationProof{Challenge: ch, BaseLines: lines, Signature: signLines(t, strangerPriv, lines)},
		NewJWKX: nx, NewJWKY: ny,
		New:        domain.RotationProof{Challenge: newCh, BaseLines: newLines, Signature: signLines(t, newPriv, newLines)},
		VerifiedAt: time.Now(),
	}); err == nil {
		t.Error("unregistered key rotated")
	}
}
