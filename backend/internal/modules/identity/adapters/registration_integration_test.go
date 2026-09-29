//go:build integration

package adapters

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
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

func freshRegistrar(t *testing.T) *Registrar {
	t.Helper()
	adminDSN := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("identity_test_%d", time.Now().UnixNano())
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
	applied, err := migrate.Apply(ctx, dsn, dbmigrations.Files)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	found := false
	for _, v := range applied {
		if v == "000004" {
			found = true
		}
	}
	if !found {
		t.Fatalf("identity migration missing from %v", applied)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return NewRegistrar(pool)
}

// testKey mints a P-256 key with crypto/rand (production-shaped entropy;
// vectors use fixed entropy only for byte stability).
func testKey(t *testing.T) (*ecdsa.PrivateKey, string, string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := priv.PublicKey.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding.EncodeToString
	return priv, enc(raw[1:33]), enc(raw[33:65])
}

func fingerprintOf(t *testing.T, x, y string) string {
	t.Helper()
	thumb := `{"crv":"P-256","kty":"EC","x":"` + x + `","y":"` + y + `"}`
	sum := sha256.Sum256([]byte(thumb))
	return fmt.Sprintf("fp:%x", sum)
}

// prove builds a registration request with a real signature over the
// server-rebuilt base shape. mutate alters one stored line afterwards to
// simulate tampering without the key.
func prove(t *testing.T, priv *ecdsa.PrivateKey, ch domain.Challenge, fp string, mutate func([]string) []string) domain.RegistrationRequest {
	t.Helper()
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
	raw := append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)
	stored := lines
	if mutate != nil {
		stored = mutate(append([]string{}, lines...))
	}
	xb, yb := jwkCoords(t, priv)
	return domain.RegistrationRequest{
		JWKX: xb, JWKY: yb,
		Challenge:  ch,
		BaseLines:  stored,
		Signature:  base64.RawURLEncoding.EncodeToString(raw),
		VerifiedAt: now,
	}
}

func jwkCoords(t *testing.T, priv *ecdsa.PrivateKey) (string, string) {
	t.Helper()
	raw, err := priv.PublicKey.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding.EncodeToString
	return enc(raw[1:33]), enc(raw[33:65])
}

func TestRegisterHappyPath(t *testing.T) {
	reg := freshRegistrar(t)
	ctx := context.Background()
	priv, x, y := testKey(t)
	fp := fingerprintOf(t, x, y)
	ch, err := reg.IssueChallenge(ctx, fp, domain.PurposeRegister)
	if err != nil {
		t.Fatalf("challenge: %v", err)
	}
	if ch.Nonce == "" || ch.ExpiresAt.IsZero() {
		t.Errorf("challenge = %+v", ch)
	}
	res, err := reg.Register(ctx, prove(t, priv, ch, fp, nil))
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if res.ContributorID == "" || res.KeyID == "" || res.Existed {
		t.Errorf("registration = %+v", res)
	}
	// Same key with a fresh challenge returns the existing identity.
	ch2, err := reg.IssueChallenge(ctx, fp, domain.PurposeRegister)
	if err != nil {
		t.Fatal(err)
	}
	again, err := reg.Register(ctx, prove(t, priv, ch2, fp, nil))
	if err != nil {
		t.Fatalf("duplicate register: %v", err)
	}
	if !again.Existed || again.ContributorID != res.ContributorID || again.KeyID != res.KeyID {
		t.Errorf("duplicate = %+v, want existing %+v", again, res)
	}
}

func TestProofFailureConsumesNothing(t *testing.T) {
	reg := freshRegistrar(t)
	ctx := context.Background()
	priv, x, y := testKey(t)
	fp := fingerprintOf(t, x, y)
	ch, err := reg.IssueChallenge(ctx, fp, domain.PurposeRegister)
	if err != nil {
		t.Fatal(err)
	}
	bad := prove(t, priv, ch, fp, func(lines []string) []string {
		lines[2] = `"@path": /v1/observations`
		return lines
	})
	if _, err := reg.Register(ctx, bad); !errors.Is(err, domain.ErrProofRequired) {
		t.Fatalf("tampered proof accepted: %v", err)
	}
	// The challenge survives: a corrected proof on the same challenge works.
	ok, err := reg.Register(ctx, prove(t, priv, ch, fp, nil))
	if err != nil || ok.Existed {
		t.Errorf("retry after failure = %+v, %v", ok, err)
	}
	// Replaying the spent challenge fails.
	if _, err := reg.Register(ctx, prove(t, priv, ch, fp, nil)); !errors.Is(err, domain.ErrChallengeSpent) {
		t.Errorf("replay accepted: %v", err)
	}
}

func count(t *testing.T, pool *pgxpool.Pool, table string) int64 {
	t.Helper()
	var n int64
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestBadKeyAndFingerprintMismatch(t *testing.T) {
	reg := freshRegistrar(t)
	ctx := context.Background()
	priv, x, y := testKey(t)
	fp := fingerprintOf(t, x, y)
	ch, err := reg.IssueChallenge(ctx, fp, domain.PurposeRegister)
	if err != nil {
		t.Fatal(err)
	}
	req := prove(t, priv, ch, fp, nil)
	req.JWKX = base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	if _, err := reg.Register(ctx, req); !errors.Is(err, domain.ErrProofRequired) {
		t.Errorf("degenerate key accepted: %v", err)
	}
	// A proof from another key cannot spend this fingerprint's challenge.
	privB, xb, yb := testKey(t)
	_ = privB
	_ = xb
	_ = yb
	otherFp := func() string {
		raw, err := privB.PublicKey.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		enc := base64.RawURLEncoding.EncodeToString
		sum := sha256.Sum256([]byte(`{"crv":"P-256","kty":"EC","x":"` + enc(raw[1:33]) + `","y":"` + enc(raw[33:65]) + `"}}`))
		return fmt.Sprintf("fp:%x", sum)
	}()
	_ = otherFp
	mismatched := prove(t, privB, ch, fp, nil)
	if _, err := reg.Register(ctx, mismatched); !errors.Is(err, domain.ErrProofRequired) {
		t.Errorf("fingerprint mismatch accepted: %v", err)
	}
	if n := count(t, reg.pool, "identity_contributors"); n != 0 {
		t.Errorf("failed registrations persisted %d contributors", n)
	}
	if n := count(t, reg.pool, "identity_keys"); n != 0 {
		t.Errorf("failed registrations persisted %d keys", n)
	}
}

func TestConcurrentSameKeyOneContributor(t *testing.T) {
	reg := freshRegistrar(t)
	ctx := context.Background()
	priv, x, y := testKey(t)
	fp := fingerprintOf(t, x, y)
	const workers = 8
	type outcome struct {
		res domain.Registration
		err error
	}
	outs := make([]outcome, workers)
	var wg sync.WaitGroup
	for i := range outs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ch, err := reg.IssueChallenge(ctx, fp, domain.PurposeRegister)
			if err != nil {
				outs[i] = outcome{err: err}
				return
			}
			res, err := reg.Register(ctx, prove(t, priv, ch, fp, nil))
			outs[i] = outcome{res: res, err: err}
		}(i)
	}
	wg.Wait()
	var contributor string
	for i, o := range outs {
		if o.err != nil {
			t.Fatalf("worker %d: %v", i, o.err)
		}
		if contributor == "" {
			contributor = o.res.ContributorID
		} else if o.res.ContributorID != contributor {
			t.Fatalf("forked identity: %+v", outs)
		}
	}
	pool := reg.pool
	var contributors, keys int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM identity_contributors").Scan(&contributors); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM identity_keys").Scan(&keys); err != nil {
		t.Fatal(err)
	}
	if contributors != 1 || keys != 1 {
		t.Errorf("contributors=%d keys=%d, want exactly one of each", contributors, keys)
	}
}
