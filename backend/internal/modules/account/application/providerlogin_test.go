package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

const nativeAudience = "com.anpfuel.app"

func nativeFixture(t *testing.T) *linkFixture {
	t.Helper()
	f := testLinkFixture(t)
	f.svc.NativeAudiences = map[string]string{"apple": nativeAudience}
	return f
}

// nativeToken mints an Apple identity token the way the iOS client
// requests it: aud is the bundle ID and the nonce claim is the SHA-256
// hex of the raw nonce the client later sends to the server.
func (f *linkFixture) nativeToken(t *testing.T, sub, email, rawNonce string) string {
	t.Helper()
	now := time.Unix(1_800_000_000, 0)
	tok, err := f.apple.TokenWith(sub, email, nativeAudience, domain.NativeNonceClaim(rawNonce), now, now.Add(10*time.Minute), domain.IssuerApple)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func rawNonce(n int) string { return fmt.Sprintf("raw-native-nonce-%06d", n) }

func (f *linkFixture) login(sub, token, nonce string) (AuthResult, error) {
	return f.svc.LoginWithProvider(context.Background(), ProviderLogin{Provider: "apple", IDToken: token, Nonce: nonce})
}

func TestProviderLoginNewSubjectCreatesAccountAndSession(t *testing.T) {
	f := nativeFixture(t)
	ctx := context.Background()
	got, err := f.login("apple-sub-1", f.nativeToken(t, "apple-sub-1", "relay@privaterelay.appleid.com", rawNonce(1)), rawNonce(1))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if !got.Created || got.Account.ID == "" || got.Account.Status != domain.StatusActive {
		t.Fatalf("want created active account, got %+v", got)
	}
	if got.Session.AccountID != got.Account.ID || got.Session.AccessToken == "" || got.Session.RefreshToken == "" {
		t.Fatalf("session not bound to the account: %+v", got.Session)
	}
	if owner, err := f.svc.ValidateAccess(ctx, got.Session.FamilyID, got.Session.AccessToken); err != nil || owner != got.Account.ID {
		t.Fatalf("issued session must validate: owner=%q err=%v", owner, err)
	}
	links, err := f.svc.ListProviders(ctx, got.Account.ID)
	if err != nil || len(links) != 1 || links[0].Provider != "apple" || links[0].Subject != "apple-sub-1" || links[0].Issuer != domain.IssuerApple {
		t.Fatalf("account must carry exactly the verified apple link: %+v err=%v", links, err)
	}
	// The only login method of a provider-created account cannot be
	// removed: unlinking would strand the account.
	if err := f.svc.UnlinkProvider(ctx, got.Account.ID, "apple"); !errors.Is(err, domain.ErrLastLoginMethod) {
		t.Fatalf("want last-method refusal, got %v", err)
	}
}

func TestProviderLoginExistingLinkReturnsSameAccount(t *testing.T) {
	f := nativeFixture(t)
	first, err := f.login("apple-sub-2", f.nativeToken(t, "apple-sub-2", "", rawNonce(1)), rawNonce(1))
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.login("apple-sub-2", f.nativeToken(t, "apple-sub-2", "", rawNonce(2)), rawNonce(2))
	if err != nil {
		t.Fatal(err)
	}
	if second.Created || second.Account.ID != first.Account.ID || second.Session.FamilyID == first.Session.FamilyID {
		t.Fatalf("existing link must sign in to the same account with a new family: first=%+v second=%+v", first, second)
	}
}

func TestProviderLoginUsesSessionLinkedSubject(t *testing.T) {
	f := nativeFixture(t)
	ctx := context.Background()
	accountID := f.signup(t, "native-linked@example.invalid", "482916")
	if _, err := f.svc.LinkProvider(ctx, accountID, "apple", f.appleToken(t, "apple-sub-3", "", "n-link-native"), linkAudience, "n-link-native"); err != nil {
		t.Fatalf("link: %v", err)
	}
	got, err := f.login("apple-sub-3", f.nativeToken(t, "apple-sub-3", "", rawNonce(3)), rawNonce(3))
	if err != nil {
		t.Fatal(err)
	}
	if got.Created || got.Account.ID != accountID {
		t.Fatalf("linked subject must sign in to the email account %q, got %+v", accountID, got)
	}
}

func TestProviderLoginNeverMergesOnEqualEmail(t *testing.T) {
	f := nativeFixture(t)
	emailAccount := f.signup(t, "same@example.invalid", "482916")
	got, err := f.login("apple-sub-4", f.nativeToken(t, "apple-sub-4", "same@example.invalid", rawNonce(4)), rawNonce(4))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Created || got.Account.ID == emailAccount {
		t.Fatalf("equal email must never select an account: %+v", got)
	}
}

func TestProviderLoginRefusesForeignProofs(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	cases := []struct {
		name  string
		token func(f *linkFixture, nonce string) (string, error)
		want  error
	}{
		{"bad issuer", func(f *linkFixture, nonce string) (string, error) {
			return f.apple.TokenWith("sub-x", "", nativeAudience, domain.NativeNonceClaim(nonce), now, now.Add(time.Hour), "https://appleid.example.invalid")
		}, domain.ErrOIDCUnknownIssuer},
		{"link audience", func(f *linkFixture, nonce string) (string, error) {
			return f.apple.TokenWith("sub-x", "", linkAudience, domain.NativeNonceClaim(nonce), now, now.Add(time.Hour), domain.IssuerApple)
		}, domain.ErrOIDCWrongAudience},
		{"other client audience", func(f *linkFixture, nonce string) (string, error) {
			return f.apple.TokenWith("sub-x", "", "com.example.other", domain.NativeNonceClaim(nonce), now, now.Add(time.Hour), domain.IssuerApple)
		}, domain.ErrOIDCWrongAudience},
		{"expired past skew", func(f *linkFixture, nonce string) (string, error) {
			return f.apple.TokenWith("sub-x", "", nativeAudience, domain.NativeNonceClaim(nonce), now.Add(-time.Hour), now.Add(-121*time.Second), domain.IssuerApple)
		}, domain.ErrOIDCExpired},
		{"raw nonce in claim", func(f *linkFixture, nonce string) (string, error) {
			return f.apple.TokenWith("sub-x", "", nativeAudience, nonce, now, now.Add(time.Hour), domain.IssuerApple)
		}, domain.ErrOIDCNonceMismatch},
		{"other nonce hash", func(f *linkFixture, nonce string) (string, error) {
			return f.apple.TokenWith("sub-x", "", nativeAudience, domain.NativeNonceClaim(nonce+"x"), now, now.Add(time.Hour), domain.IssuerApple)
		}, domain.ErrOIDCNonceMismatch},
		{"google key signs apple claims", func(f *linkFixture, nonce string) (string, error) {
			return f.google.TokenWith("sub-x", "", nativeAudience, domain.NativeNonceClaim(nonce), now, now.Add(time.Hour), domain.IssuerApple)
		}, domain.ErrOIDCUnknownIssuer},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := nativeFixture(t)
			nonce := rawNonce(100 + i)
			tok, err := tc.token(f, nonce)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.login("sub-x", tok, nonce); !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
			// Refusals never consume the nonce claim and never create
			// an account: the honest retry still works.
			if !f.nonces.Consume(domain.NativeNonceClaim(nonce)) {
				t.Fatal("refused proof consumed the nonce")
			}
			if _, found, _ := f.svc.Store.FindProviderOwner(context.Background(), "apple", "sub-x"); found {
				t.Fatal("refused proof created a provider link")
			}
		})
	}
}

func TestProviderLoginReplayedNonceRefused(t *testing.T) {
	f := nativeFixture(t)
	tok := f.nativeToken(t, "apple-sub-5", "", rawNonce(5))
	first, err := f.login("apple-sub-5", tok, rawNonce(5))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.login("apple-sub-5", tok, rawNonce(5)); !errors.Is(err, domain.ErrOIDCNonceReused) {
		t.Fatalf("replay must refuse as nonce-reused, got %v", err)
	}
	n, err := f.svc.Store.CountLiveFamiliesByAccount(context.Background(), first.Account.ID, f.clock.NowUnix())
	if err != nil || n != 1 {
		t.Fatalf("replay must not mint a session: live=%d err=%v", n, err)
	}
}

func TestProviderLoginUnknownOrUnconfiguredProvider(t *testing.T) {
	f := nativeFixture(t)
	nonce := rawNonce(6)
	tok := f.nativeToken(t, "apple-sub-6", "", nonce)
	if _, err := f.svc.LoginWithProvider(context.Background(), ProviderLogin{Provider: "facebook", IDToken: tok, Nonce: nonce}); !errors.Is(err, domain.ErrOIDCUnknownIssuer) {
		t.Fatalf("unknown provider: %v", err)
	}
	if _, err := f.svc.LoginWithProvider(context.Background(), ProviderLogin{Provider: "google", IDToken: tok, Nonce: nonce}); !errors.Is(err, domain.ErrProviderLoginUnavailable) {
		t.Fatalf("google native sign-in must stay unavailable: %v", err)
	}
	for _, audiences := range []map[string]string{nil, {"apple": ""}, {"google": nativeAudience}} {
		f.svc.NativeAudiences = audiences
		if _, err := f.login("apple-sub-6", tok, nonce); !errors.Is(err, domain.ErrProviderLoginUnavailable) {
			t.Fatalf("audiences %v: want provider-unavailable, got %v", audiences, err)
		}
	}
	f.svc.NativeAudiences = map[string]string{"apple": nativeAudience}
	f.svc.Verifier = nil
	if _, err := f.login("apple-sub-6", tok, nonce); !errors.Is(err, domain.ErrProviderLoginUnavailable) {
		t.Fatalf("missing verifier must fail closed: %v", err)
	}
	if !f.nonces.Consume(domain.NativeNonceClaim(nonce)) {
		t.Fatal("unavailable sign-in consumed the nonce")
	}
}

func TestProviderLoginRefusesMalformedProofBeforeVerification(t *testing.T) {
	f := nativeFixture(t)
	nonce := rawNonce(7)
	tok := f.nativeToken(t, "apple-sub-7", "", nonce)
	cases := []ProviderLogin{
		{Provider: "apple", IDToken: tok + strings.Repeat("A", domain.MaxProviderTokenBytes), Nonce: nonce},
		{Provider: "apple", IDToken: "", Nonce: nonce},
		{Provider: "apple", IDToken: tok + " ", Nonce: nonce},
		{Provider: "apple", IDToken: tok, Nonce: "short"},
		{Provider: "apple", IDToken: tok, Nonce: strings.Repeat("n", domain.NativeNonceMaxLength+1)},
		{Provider: "apple", IDToken: tok, Nonce: "raw nonce with spaces"},
	}
	for i, req := range cases {
		if _, err := f.svc.LoginWithProvider(context.Background(), req); !errors.Is(err, domain.ErrProviderProofMalformed) {
			t.Fatalf("case %d: want malformed, got %v", i, err)
		}
	}
	if _, err := f.login("apple-sub-7", tok, nonce); err != nil {
		t.Fatalf("malformed attempts must not burn the honest proof: %v", err)
	}
}

func TestProviderLoginSuspendedAccountRefused(t *testing.T) {
	f := nativeFixture(t)
	ctx := context.Background()
	first, err := f.login("apple-sub-8", f.nativeToken(t, "apple-sub-8", "", rawNonce(8)), rawNonce(8))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.SuspendAccount(ctx, first.Account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.login("apple-sub-8", f.nativeToken(t, "apple-sub-8", "", rawNonce(9)), rawNonce(9)); !errors.Is(err, domain.ErrAccountSuspended) {
		t.Fatalf("suspended account must refuse, got %v", err)
	}
	if n, _ := f.svc.Store.CountLiveFamiliesByAccount(ctx, first.Account.ID, f.clock.NowUnix()); n != 0 {
		t.Fatalf("suspended account gained %d live families", n)
	}
}

func TestProviderLoginAfterDeletionMintsNewAccount(t *testing.T) {
	f := nativeFixture(t)
	ctx := context.Background()
	first, err := f.login("apple-sub-10", f.nativeToken(t, "apple-sub-10", "", rawNonce(10)), rawNonce(10))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Store.DeleteAccount(ctx, first.Account.ID, f.clock.NowUnix()); err != nil {
		t.Fatal(err)
	}
	again, err := f.login("apple-sub-10", f.nativeToken(t, "apple-sub-10", "", rawNonce(11)), rawNonce(11))
	if err != nil {
		t.Fatal(err)
	}
	if !again.Created || again.Account.ID == first.Account.ID {
		t.Fatalf("deletion drops the link; the subject must mint a new account without carryover: %+v", again)
	}
}

// deletedOwnerStore models a link that still resolves to a deleted
// account (for example a deletion racing the lookup).
type deletedOwnerStore struct{ *MemStore }

func (s deletedOwnerStore) ResolveProviderAccount(_ context.Context, link domain.ProviderLink, _ domain.Account) (domain.Account, bool, error) {
	return domain.Account{ID: "dead-" + link.Subject, Status: domain.StatusDeleted}, false, nil
}

func TestProviderLoginDeletedOwnerRefused(t *testing.T) {
	f := nativeFixture(t)
	f.svc.Store = deletedOwnerStore{NewMemStore()}
	if _, err := f.login("apple-sub-12", f.nativeToken(t, "apple-sub-12", "", rawNonce(12)), rawNonce(12)); !errors.Is(err, domain.ErrAccountDeleted) {
		t.Fatalf("want account-deleted, got %v", err)
	}
}

// suspendingStore suspends the resolved account right after resolution,
// modeling a moderator action interleaved with session issuance.
type suspendingStore struct{ *MemStore }

func (s suspendingStore) ResolveProviderAccount(ctx context.Context, link domain.ProviderLink, candidate domain.Account) (domain.Account, bool, error) {
	acc, created, err := s.MemStore.ResolveProviderAccount(ctx, link, candidate)
	if err == nil {
		err = s.MemStore.SuspendAccount(ctx, acc.ID, 1)
	}
	return acc, created, err
}

func TestProviderLoginInterleavedSuspensionRevokesFreshSession(t *testing.T) {
	f := nativeFixture(t)
	store := suspendingStore{NewMemStore()}
	f.svc.Store = store
	if _, err := f.login("apple-sub-13", f.nativeToken(t, "apple-sub-13", "", rawNonce(13)), rawNonce(13)); !errors.Is(err, domain.ErrAccountSuspended) {
		t.Fatalf("want account-suspended, got %v", err)
	}
	owner, found, _ := store.FindProviderOwner(context.Background(), "apple", "apple-sub-13")
	if !found {
		t.Fatal("resolution must have created the account")
	}
	if n, _ := store.CountLiveFamiliesByAccount(context.Background(), owner, f.clock.NowUnix()); n != 0 {
		t.Fatalf("interleaved suspension left %d live families", n)
	}
}

func TestProviderLoginConcurrentFirstSignInCreatesOneAccount(t *testing.T) {
	f := nativeFixture(t)
	const workers = 8
	tokens := make([]string, workers)
	for i := range tokens {
		tokens[i] = f.nativeToken(t, "apple-sub-race", "", rawNonce(200+i))
	}
	var wg sync.WaitGroup
	results := make([]AuthResult, workers)
	errs := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = f.login("apple-sub-race", tokens[i], rawNonce(200+i))
		}(i)
	}
	wg.Wait()
	created := 0
	for i := 0; i < workers; i++ {
		if errs[i] != nil {
			t.Fatalf("worker %d: %v", i, errs[i])
		}
		if results[i].Created {
			created++
		}
		if results[i].Account.ID != results[0].Account.ID {
			t.Fatalf("workers diverged: %q vs %q", results[i].Account.ID, results[0].Account.ID)
		}
	}
	if created != 1 {
		t.Fatalf("want exactly one creation, got %d", created)
	}
}
