package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters/oidc"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

const linkAudience = "anpfuel-backend"

type linkFixture struct {
	svc    *Service
	clock  *fakeClock
	google oidc.StubIssuer
	apple  oidc.StubIssuer
	nonces *oidc.MemNonces
}

func testLinkFixture(t *testing.T) *linkFixture {
	t.Helper()
	clock := &fakeClock{now: 1_800_000_000}
	mail := &stubMail{}
	codes := []string{"482916", "111111", "222222", "333333", "444444", "555555", "666666"}
	var ci int
	google, err := oidc.NewStubIssuer("https://accounts.google.com", "RS256")
	if err != nil {
		t.Fatal(err)
	}
	apple, err := oidc.NewStubIssuer("https://appleid.apple.com", "ES256")
	if err != nil {
		t.Fatal(err)
	}
	nonces := oidc.NewMemNonces()
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
	svc := &Service{
		Clock:    clock,
		Hasher:   domain.SHA256Hasher{},
		Mail:     mail,
		Store:    NewMemStore(),
		Verifier: verifier,
		CodeGen: func() (string, error) {
			c := codes[ci%len(codes)]
			ci++
			return c, nil
		},
		TokenGen: counter("tok"),
		AliasGen: counter("alias"),
		IDGen:    counter("id"),
	}
	return &linkFixture{svc: svc, clock: clock, google: google, apple: apple, nonces: nonces}
}

func (f *linkFixture) signup(t *testing.T, address, code string) string {
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

func (f *linkFixture) googleToken(t *testing.T, sub, email, nonce string) string {
	t.Helper()
	now := time.Unix(1_800_000_000, 0)
	tok, err := f.google.Token(sub, email, linkAudience, nonce, now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func (f *linkFixture) appleToken(t *testing.T, sub, email, nonce string) string {
	t.Helper()
	now := time.Unix(1_800_000_000, 0)
	tok, err := f.apple.Token(sub, email, linkAudience, nonce, now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestLinkBothProvidersHappy(t *testing.T) {
	f := testLinkFixture(t)
	ctx := context.Background()
	accountID := f.signup(t, "link-a@example.invalid", "482916")

	got, err := f.svc.LinkProvider(ctx, accountID, "google", f.googleToken(t, "user-123", "user@example.com", "n-link-1"), linkAudience, "n-link-1")
	if err != nil {
		t.Fatalf("link google: %v", err)
	}
	if got.Provider != "google" || got.Subject != "user-123" || got.Issuer != "https://accounts.google.com" {
		t.Errorf("google binding wrong: %+v", got)
	}
	got, err = f.svc.LinkProvider(ctx, accountID, "apple", f.appleToken(t, "opaque-apple-001", "relay@privaterelay.appleid.com", "n-link-2"), linkAudience, "n-link-2")
	if err != nil {
		t.Fatalf("link apple: %v", err)
	}
	if got.Subject != "opaque-apple-001" {
		t.Errorf("apple must bind the opaque subject, got %+v", got)
	}
	links, err := f.svc.ListProviders(ctx, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 {
		t.Errorf("want 2 provider links, got %d", len(links))
	}
	owner, found, err := f.svc.Store.FindProviderOwner(ctx, "google", "user-123")
	if err != nil || !found || owner != accountID {
		t.Errorf("google owner must resolve to %q, got %q found=%v err=%v", accountID, owner, found, err)
	}
}

func TestLinkCrossAccountRefused(t *testing.T) {
	f := testLinkFixture(t)
	ctx := context.Background()
	accountA := f.signup(t, "cross-a@example.invalid", "482916")
	accountB := f.signup(t, "cross-b@example.invalid", "111111")

	if _, err := f.svc.LinkProvider(ctx, accountA, "google", f.googleToken(t, "shared-sub-1", "", "n-cross-1"), linkAudience, "n-cross-1"); err != nil {
		t.Fatalf("link A: %v", err)
	}
	// Same subject, fresh nonce: proof belongs to A, presenting inside B must refuse.
	_, err := f.svc.LinkProvider(ctx, accountB, "google", f.googleToken(t, "shared-sub-1", "", "n-cross-2"), linkAudience, "n-cross-2")
	if !errors.Is(err, domain.ErrLinkCrossAccount) {
		t.Errorf("cross-account link must be refused, got %v", err)
	}
	if got := domain.VerdictCode(err); got != "link-cross-account-refused" {
		t.Errorf("verdict = %q, want link-cross-account-refused", got)
	}
	links, _ := f.svc.ListProviders(ctx, accountB)
	if len(links) != 0 {
		t.Errorf("refused link must persist nothing, got %+v", links)
	}
}

func TestNonceReplayRefused(t *testing.T) {
	f := testLinkFixture(t)
	ctx := context.Background()
	accountA := f.signup(t, "replay-a@example.invalid", "482916")
	accountB := f.signup(t, "replay-b@example.invalid", "111111")

	tok := f.googleToken(t, "replay-sub-1", "", "n-replay-1")
	if _, err := f.svc.LinkProvider(ctx, accountA, "google", tok, linkAudience, "n-replay-1"); err != nil {
		t.Fatalf("first link: %v", err)
	}
	// Same token bytes replayed for B: the nonce is already consumed.
	if _, err := f.svc.LinkProvider(ctx, accountB, "google", tok, linkAudience, "n-replay-1"); !errors.Is(err, domain.ErrOIDCNonceReused) {
		t.Errorf("replayed nonce must be nonce-reused, got %v", err)
	}
	// Token bound to another nonce fails closed as mismatch.
	tok2 := f.googleToken(t, "replay-sub-1", "", "n-other")
	if _, err := f.svc.LinkProvider(ctx, accountB, "google", tok2, linkAudience, "n-wrong"); !errors.Is(err, domain.ErrOIDCNonceMismatch) {
		t.Errorf("nonce mismatch must fail closed, got %v", err)
	}
}

func TestUnlinkLastMethodRefused(t *testing.T) {
	f := testLinkFixture(t)
	ctx := context.Background()

	// Email-backed account: linking then unlinking one provider is allowed (email remains).
	accountID := f.signup(t, "unlink-a@example.invalid", "482916")
	if _, err := f.svc.LinkProvider(ctx, accountID, "google", f.googleToken(t, "unlink-sub-1", "", "n-unlink-1"), linkAudience, "n-unlink-1"); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.UnlinkProvider(ctx, accountID, "google"); err != nil {
		t.Errorf("unlink with email remaining must succeed, got %v", err)
	}
	if _, err := f.svc.LinkProvider(ctx, accountID, "google", f.googleToken(t, "unlink-sub-1", "", "n-unlink-2"), linkAudience, "n-unlink-2"); err != nil {
		t.Fatal(err)
	}
	// Unknown binding refuses as not-linked, not as last-method.
	if err := f.svc.UnlinkProvider(ctx, accountID, "apple"); !errors.Is(err, domain.ErrProviderNotLinked) {
		t.Errorf("unknown binding must be not-linked, got %v", err)
	}

	// Provider-only account (no email row): removing its only method refuses.
	bare := domain.Account{ID: "bare-1", Alias: "alias-bare", Status: domain.StatusActive, CreatedAt: 1_800_000_000}
	store := f.svc.Store.(*MemStore)
	store.mu.Lock()
	store.accountsByID[bare.ID] = bare
	store.mu.Unlock()
	if err := f.svc.Store.LinkProvider(ctx, domain.ProviderLink{
		AccountID: bare.ID, Provider: "google", Issuer: "https://accounts.google.com",
		Subject: "bare-sub-1", LinkedAt: 1_800_000_000,
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.UnlinkProvider(ctx, bare.ID, "google"); !errors.Is(err, domain.ErrLastLoginMethod) {
		t.Errorf("last method unlink must refuse, got %v", err)
	}
	if got := domain.VerdictCode(domain.ErrLastLoginMethod); got != "link-last-method-refused" {
		t.Errorf("verdict = %q, want link-last-method-refused", got)
	}
}

func TestEmailOnlyNeverMerges(t *testing.T) {
	f := testLinkFixture(t)
	ctx := context.Background()
	accountA := f.signup(t, "same-mail-a@example.invalid", "482916")
	accountB := f.signup(t, "same-mail-b@example.invalid", "111111")

	// Same display email string on two different verified subjects: two distinct links, no merge.
	if _, err := f.svc.LinkProvider(ctx, accountA, "google", f.googleToken(t, "mail-sub-a", "same@example.com", "n-mail-1"), linkAudience, "n-mail-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.LinkProvider(ctx, accountB, "google", f.googleToken(t, "mail-sub-b", "same@example.com", "n-mail-2"), linkAudience, "n-mail-2"); err != nil {
		t.Fatal(err)
	}
	ownerA, _, _ := f.svc.Store.FindProviderOwner(ctx, "google", "mail-sub-a")
	ownerB, _, _ := f.svc.Store.FindProviderOwner(ctx, "google", "mail-sub-b")
	if ownerA != accountA || ownerB != accountB {
		t.Errorf("equal emails must not merge: owners %q/%q want %q/%q", ownerA, ownerB, accountA, accountB)
	}
	// Empty token never links by email.
	if _, err := f.svc.LinkProvider(ctx, accountA, "google", "", linkAudience, "n-mail-3"); err == nil {
		t.Error("empty token must never link")
	}
}

func TestRotationRefusedWithDBParityNonces(t *testing.T) {
	f := testLinkFixture(t)
	ctx := context.Background()
	accountID := f.signup(t, "rot-a@example.invalid", "482916")

	rotated, err := oidc.NewStubIssuer("https://accounts.google.com", "RS256")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	tok, err := rotated.Token("user-123", "", linkAudience, "n-rot-1", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.LinkProvider(ctx, accountID, "google", tok, linkAudience, "n-rot-1"); !errors.Is(err, domain.ErrOIDCUnknownIssuer) {
		t.Errorf("rotated-away key must fail unknown-issuer, got %v", err)
	}
}

func TestMemNonceLedgerSingleUse(t *testing.T) {
	f := testLinkFixture(t)
	ctx := context.Background()
	ok, err := f.svc.Store.TryConsumeNonce(ctx, "n-ledger-1", 1_800_000_000)
	if err != nil || !ok {
		t.Fatalf("first nonce consume must admit, ok=%v err=%v", ok, err)
	}
	ok, err = f.svc.Store.TryConsumeNonce(ctx, "n-ledger-1", 1_800_000_000)
	if err != nil || ok {
		t.Errorf("second nonce consume must refuse, ok=%v err=%v", ok, err)
	}
}
