//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters/oidc"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

const linkAudience = "anpfuel-backend"

type linkService struct {
	svc    *application.Service
	clock  *integClock
	pool   *pgxpool.Pool
	google oidc.StubIssuer
	apple  oidc.StubIssuer
	nonces *PGNonces
}

func freshLinkService(t *testing.T) *linkService {
	t.Helper()
	adminDSN := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if adminDSN == "" {
		adminDSN = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("integration database unreachable: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("account_link_test_%d", time.Now().UnixNano())
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
	for _, want := range []string{"000020", "000021"} {
		found := false
		for _, v := range applied {
			if v == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("migration %s missing from %v", want, applied)
		}
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	clock := &integClock{now: 1_800_000_000}
	google, err := oidc.NewStubIssuer("https://accounts.google.com", "RS256")
	if err != nil {
		t.Fatal(err)
	}
	apple, err := oidc.NewStubIssuer("https://appleid.apple.com", "ES256")
	if err != nil {
		t.Fatal(err)
	}
	nonces := NewPGNonces(pool)
	verifier := &oidc.Verifier{
		Clock:    func() time.Time { return time.Unix(1_800_000_000, 0) },
		Issuers:  map[string]string{"google": "https://accounts.google.com", "apple": "https://appleid.apple.com"},
		Audience: linkAudience,
		Keys: oidc.NewMemKeys(map[string]oidc.StubIssuer{
			"https://accounts.google.com": google,
			"https://appleid.apple.com":   apple,
		}),
		Nonces:   nonces,
		Skew:     120 * time.Second,
		CacheTTL: time.Hour,
	}
	codes := []string{"482916", "111111", "222222", "333333", "444444", "555555", "666666"}
	var ci, gi, tn, an int
	svc := &application.Service{
		Clock:    clock,
		Hasher:   domain.SHA256Hasher{},
		Mail:     &integMail{},
		Store:    NewPGStore(pool),
		Verifier: verifier,
		CodeGen: func() (string, error) {
			c := codes[ci%len(codes)]
			ci++
			return c, nil
		},
		TokenGen: func() (string, error) {
			tn++
			return fmt.Sprintf("tok-%d", tn), nil
		},
		AliasGen: func() (string, error) {
			an++
			return fmt.Sprintf("alias-%d", an), nil
		},
		IDGen: func() (string, error) {
			gi++
			return testUUID(gi), nil
		},
	}
	return &linkService{svc: svc, clock: clock, pool: pool, google: google, apple: apple, nonces: nonces}
}

func (f *linkService) signup(t *testing.T, address, code string) string {
	t.Helper()
	ctx := context.Background()
	if err := f.svc.RequestCode(ctx, address); err != nil {
		t.Fatalf("RequestCode: %v", err)
	}
	auth, err := f.svc.ConsumeCode(ctx, address, code)
	if err != nil {
		t.Fatalf("ConsumeCode: %v", err)
	}
	return auth.Account.ID
}

func (f *linkService) googleToken(t *testing.T, sub, email, nonce string) string {
	t.Helper()
	now := time.Unix(1_800_000_000, 0)
	tok, err := f.google.Token(sub, email, linkAudience, nonce, now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func (f *linkService) appleToken(t *testing.T, sub, email, nonce string) string {
	t.Helper()
	now := time.Unix(1_800_000_000, 0)
	tok, err := f.apple.Token(sub, email, linkAudience, nonce, now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestPGLinkBothProvidersHappy(t *testing.T) {
	f := freshLinkService(t)
	ctx := context.Background()
	accountID := f.signup(t, "link-a@example.invalid", "482916")

	if _, err := f.svc.LinkProvider(ctx, accountID, "google", f.googleToken(t, "user-123", "user@example.com", "n-pg-1"), linkAudience, "n-pg-1"); err != nil {
		t.Fatalf("link google: %v", err)
	}
	if _, err := f.svc.LinkProvider(ctx, accountID, "apple", f.appleToken(t, "opaque-apple-001", "relay@privaterelay.appleid.com", "n-pg-2"), linkAudience, "n-pg-2"); err != nil {
		t.Fatalf("link apple: %v", err)
	}
	links, err := f.svc.ListProviders(ctx, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 {
		t.Errorf("want 2 links, got %d", len(links))
	}
}

func TestPGLinkCrossAccountRefused(t *testing.T) {
	f := freshLinkService(t)
	ctx := context.Background()
	accountA := f.signup(t, "cross-a@example.invalid", "482916")
	accountB := f.signup(t, "cross-b@example.invalid", "111111")

	if _, err := f.svc.LinkProvider(ctx, accountA, "google", f.googleToken(t, "shared-sub-1", "", "n-pg-cross-1"), linkAudience, "n-pg-cross-1"); err != nil {
		t.Fatalf("link A: %v", err)
	}
	_, err := f.svc.LinkProvider(ctx, accountB, "google", f.googleToken(t, "shared-sub-1", "", "n-pg-cross-2"), linkAudience, "n-pg-cross-2")
	if !errors.Is(err, domain.ErrLinkCrossAccount) {
		t.Errorf("cross-account must refuse, got %v", err)
	}
}

func TestPGUnlinkLastMethodRefused(t *testing.T) {
	f := freshLinkService(t)
	ctx := context.Background()

	// Email-backed account keeps email after unlinking its only provider.
	accountID := f.signup(t, "unlink-a@example.invalid", "482916")
	if _, err := f.svc.LinkProvider(ctx, accountID, "google", f.googleToken(t, "unlink-sub-1", "", "n-pg-unlink-1"), linkAudience, "n-pg-unlink-1"); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.UnlinkProvider(ctx, accountID, "google"); err != nil {
		t.Errorf("unlink with email remaining must succeed, got %v", err)
	}

	// Bare provider-only account: insert without any email address row.
	bareID := testUUID(9001)
	if _, err := f.pool.Exec(ctx, `INSERT INTO accounts (id, alias, status, created_at) VALUES ($1, 'bare-alias', 'active', now())`, bareID); err != nil {
		t.Fatalf("insert bare account: %v", err)
	}
	if _, err := f.svc.LinkProvider(ctx, bareID, "google", f.googleToken(t, "bare-sub-1", "", "n-pg-bare-1"), linkAudience, "n-pg-bare-1"); err != nil {
		t.Fatalf("link bare: %v", err)
	}
	if err := f.svc.UnlinkProvider(ctx, bareID, "google"); !errors.Is(err, domain.ErrLastLoginMethod) {
		t.Errorf("last method unlink must refuse, got %v", err)
	}
}

func TestPGNonceReplayAcrossProcesses(t *testing.T) {
	f := freshLinkService(t)
	ctx := context.Background()
	accountA := f.signup(t, "replay-a@example.invalid", "482916")
	accountB := f.signup(t, "replay-b@example.invalid", "111111")

	tok := f.googleToken(t, "replay-sub-1", "", "n-pg-replay-1")
	if _, err := f.svc.LinkProvider(ctx, accountA, "google", tok, linkAudience, "n-pg-replay-1"); err != nil {
		t.Fatalf("first link: %v", err)
	}
	// Second verifier sharing the same DB nonce ledger but fresh cache.
	second := &oidc.Verifier{
		Clock:    func() time.Time { return time.Unix(1_800_000_000, 0) },
		Issuers:  map[string]string{"google": "https://accounts.google.com", "apple": "https://appleid.apple.com"},
		Audience: linkAudience,
		Keys: oidc.NewMemKeys(map[string]oidc.StubIssuer{
			"https://accounts.google.com": f.google,
			"https://appleid.apple.com":   f.apple,
		}),
		Nonces:   f.nonces,
		Skew:     120 * time.Second,
		CacheTTL: time.Hour,
	}
	if _, err := second.Verify(ctx, "google", tok, linkAudience, "n-pg-replay-1"); !errors.Is(err, domain.ErrOIDCNonceReused) {
		t.Errorf("DB nonce replay across processes must be nonce-reused, got %v", err)
	}
	// Direct ledger check converges too.
	ok, err := f.svc.Store.TryConsumeNonce(ctx, "n-pg-replay-1", 1_800_000_000)
	if err != nil || ok {
		t.Errorf("consumed DB nonce must refuse, ok=%v err=%v", ok, err)
	}
	_ = accountB
}

func TestPGRotationAgainstDBNonces(t *testing.T) {
	f := freshLinkService(t)
	ctx := context.Background()
	accountID := f.signup(t, "rot-a@example.invalid", "482916")

	rotated, err := oidc.NewStubIssuer("https://accounts.google.com", "RS256")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	tok, err := rotated.Token("user-123", "", linkAudience, "n-pg-rot-1", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.LinkProvider(ctx, accountID, "google", tok, linkAudience, "n-pg-rot-1"); !errors.Is(err, domain.ErrOIDCUnknownIssuer) {
		t.Errorf("rotated key must fail unknown-issuer, got %v", err)
	}
}
