//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters/oidc"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

const nativeAudience = "com.anpfuel.app"

func freshNativeService(t *testing.T) *linkService {
	t.Helper()
	f := freshLinkService(t)
	f.svc.NativeAudiences = map[string]string{"apple": nativeAudience}
	return f
}

func (f *linkService) nativeToken(t *testing.T, sub, email, raw string) string {
	t.Helper()
	now := time.Unix(1_800_000_000, 0)
	tok, err := f.apple.TokenWith(sub, email, nativeAudience, domain.NativeNonceClaim(raw), now, now.Add(10*time.Minute), domain.IssuerApple)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func (f *linkService) nativeLogin(token, raw string) (application.AuthResult, error) {
	return f.svc.LoginWithProvider(context.Background(), application.ProviderLogin{Provider: "apple", IDToken: token, Nonce: raw})
}

func (f *linkService) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", sql, err)
	}
	return n
}

func TestPGProviderLoginCreatesThenSignsInWithoutRetainingProof(t *testing.T) {
	f := freshNativeService(t)
	raw := "raw-pg-native-nonce-0001"
	token := f.nativeToken(t, "pg-apple-sub-1", "relay@privaterelay.appleid.com", raw)
	first, err := f.nativeLogin(token, raw)
	if err != nil {
		t.Fatalf("first login: %v", err)
	}
	if !first.Created || first.Account.Status != domain.StatusActive {
		t.Fatalf("want created active account, got %+v", first)
	}
	raw2 := "raw-pg-native-nonce-0002"
	second, err := f.nativeLogin(f.nativeToken(t, "pg-apple-sub-1", "", raw2), raw2)
	if err != nil {
		t.Fatalf("second login: %v", err)
	}
	if second.Created || second.Account.ID != first.Account.ID {
		t.Fatalf("existing link must return the same account: %+v vs %+v", second.Account, first.Account)
	}
	if n := f.count(t, `SELECT count(*) FROM accounts`); n != 1 {
		t.Fatalf("want one account row, got %d", n)
	}
	if n := f.count(t, `SELECT count(*) FROM account_provider_links WHERE provider='apple' AND subject=$1 AND issuer=$2 AND account_id=$3`,
		"pg-apple-sub-1", domain.IssuerApple, first.Account.ID); n != 1 {
		t.Fatalf("want one verified apple link, got %d", n)
	}
	// The ledger keeps only the hashed claim; neither the raw nonce nor
	// any token segment is persisted anywhere in the account tables.
	if n := f.count(t, `SELECT count(*) FROM account_oidc_nonces WHERE nonce=$1`, domain.NativeNonceClaim(raw)); n != 1 {
		t.Fatalf("hashed nonce claim must be consumed once, got %d", n)
	}
	signature := token[strings.LastIndex(token, ".")+1:]
	for _, secret := range []string{raw, raw2, signature} {
		n := f.count(t, `SELECT
 (SELECT count(*) FROM account_oidc_nonces x WHERE to_jsonb(x)::text LIKE '%' || $1 || '%') +
 (SELECT count(*) FROM account_provider_links x WHERE to_jsonb(x)::text LIKE '%' || $1 || '%') +
 (SELECT count(*) FROM accounts x WHERE to_jsonb(x)::text LIKE '%' || $1 || '%') +
 (SELECT count(*) FROM account_session_families x WHERE to_jsonb(x)::text LIKE '%' || $1 || '%')`, secret)
		if n != 0 {
			t.Fatalf("raw proof material persisted (%d rows)", n)
		}
	}
	// A replay across processes (fresh verifier, shared ledger) refuses.
	other := *f.svc
	other.Verifier = &oidc.Verifier{
		Clock:    func() time.Time { return time.Unix(1_800_000_000, 0) },
		Issuers:  map[string]string{"google": domain.IssuerGoogle, "apple": domain.IssuerApple},
		Keys:     oidc.NewMemKeys(map[string]oidc.StubIssuer{domain.IssuerApple: f.apple}),
		Nonces:   f.nonces,
		Skew:     120 * time.Second,
		CacheTTL: time.Hour,
	}
	if _, err := other.LoginWithProvider(context.Background(), application.ProviderLogin{Provider: "apple", IDToken: token, Nonce: raw}); !errors.Is(err, domain.ErrOIDCNonceReused) {
		t.Fatalf("cross-process replay must refuse as nonce-reused, got %v", err)
	}
	if n := f.count(t, `SELECT count(*) FROM account_session_families WHERE account_id=$1`, first.Account.ID); n != 2 {
		t.Fatalf("replay must not mint a family: %d", n)
	}
}

func TestPGProviderLoginConcurrentFirstSignInCreatesOneAccount(t *testing.T) {
	f := freshNativeService(t)
	// Below the per-account live-session budget (10) so every worker
	// must succeed; the budget itself is covered by SEC01-C tests.
	const workers = 8
	tokens := make([]string, workers)
	for i := range tokens {
		tokens[i] = f.nativeToken(t, "pg-apple-race", "", fmt.Sprintf("raw-pg-race-nonce-%04d", i))
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]application.AuthResult, workers)
	errs := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = f.nativeLogin(tokens[i], fmt.Sprintf("raw-pg-race-nonce-%04d", i))
		}(i)
	}
	close(start)
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
			t.Fatalf("workers resolved different accounts: %q vs %q", results[i].Account.ID, results[0].Account.ID)
		}
	}
	if created != 1 {
		t.Fatalf("want exactly one creator, got %d", created)
	}
	if n := f.count(t, `SELECT count(*) FROM accounts`); n != 1 {
		t.Fatalf("parallel first sign-in left %d accounts", n)
	}
	if n := f.count(t, `SELECT count(*) FROM account_provider_links WHERE subject='pg-apple-race'`); n != 1 {
		t.Fatalf("parallel first sign-in left %d links", n)
	}
	if n := f.count(t, `SELECT count(*) FROM account_session_families WHERE account_id=$1 AND revoked_at IS NULL`, results[0].Account.ID); n != workers {
		t.Fatalf("want %d live families, got %d", workers, n)
	}
}

func TestPGProviderLoginRacingSessionLinkKeepsOneOwner(t *testing.T) {
	f := freshNativeService(t)
	ctx := context.Background()
	for i := 0; i < 6; i++ {
		subject := fmt.Sprintf("pg-apple-link-race-%d", i)
		emailAccount := f.signup(t, fmt.Sprintf("race-%d@example.invalid", i), []string{"482916", "111111", "222222", "333333", "444444", "555555", "666666"}[i%7])
		linkNonce := fmt.Sprintf("n-pg-link-race-%d", i)
		linkToken := f.appleToken(t, subject, "", linkNonce)
		raw := fmt.Sprintf("raw-pg-link-race-nonce-%d", i)
		loginToken := f.nativeToken(t, subject, "", raw)
		var wg sync.WaitGroup
		start := make(chan struct{})
		var linkErr, loginErr error
		var login application.AuthResult
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, linkErr = f.svc.LinkProvider(ctx, emailAccount, "apple", linkToken, linkAudience, linkNonce)
		}()
		go func() {
			defer wg.Done()
			<-start
			login, loginErr = f.nativeLogin(loginToken, raw)
		}()
		close(start)
		wg.Wait()
		if loginErr != nil {
			t.Fatalf("round %d: login must resolve an owner: %v", i, loginErr)
		}
		owner, found, err := f.svc.Store.FindProviderOwner(ctx, "apple", subject)
		if err != nil || !found {
			t.Fatalf("round %d: subject lost its owner: %v", i, err)
		}
		switch {
		case linkErr == nil:
			if owner != emailAccount || login.Account.ID != emailAccount || login.Created {
				t.Fatalf("round %d: link won but login diverged: owner=%s login=%+v", i, owner, login)
			}
		case errors.Is(linkErr, domain.ErrLinkCrossAccount):
			if owner != login.Account.ID || !login.Created {
				t.Fatalf("round %d: login won but owner diverged: owner=%s login=%+v", i, owner, login)
			}
		default:
			t.Fatalf("round %d: unexpected link outcome %v", i, linkErr)
		}
		if n := f.count(t, `SELECT count(*) FROM account_provider_links WHERE subject=$1`, subject); n != 1 {
			t.Fatalf("round %d: %d links for one subject", i, n)
		}
	}
}

func TestPGProviderLoginSuspendedAndDeletedAccounts(t *testing.T) {
	f := freshNativeService(t)
	ctx := context.Background()
	first, err := f.nativeLogin(f.nativeToken(t, "pg-apple-status", "", "raw-pg-status-nonce-01"), "raw-pg-status-nonce-01")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.SuspendAccount(ctx, first.Account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.nativeLogin(f.nativeToken(t, "pg-apple-status", "", "raw-pg-status-nonce-02"), "raw-pg-status-nonce-02"); !errors.Is(err, domain.ErrAccountSuspended) {
		t.Fatalf("suspended: want account-suspended, got %v", err)
	}
	if n := f.count(t, `SELECT count(*) FROM account_session_families WHERE account_id=$1 AND revoked_at IS NULL`, first.Account.ID); n != 0 {
		t.Fatalf("suspended account holds %d live families", n)
	}
	if err := f.svc.ReactivateAccount(ctx, first.Account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.nativeLogin(f.nativeToken(t, "pg-apple-status", "", "raw-pg-status-nonce-03"), "raw-pg-status-nonce-03"); err != nil {
		t.Fatalf("reactivated account must sign in: %v", err)
	}
	if _, err := f.svc.DeleteAccount(ctx, first.Account.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	again, err := f.nativeLogin(f.nativeToken(t, "pg-apple-status", "", "raw-pg-status-nonce-04"), "raw-pg-status-nonce-04")
	if err != nil {
		t.Fatalf("login after deletion: %v", err)
	}
	if !again.Created || again.Account.ID == first.Account.ID {
		t.Fatalf("deletion must not resurrect the account: %+v", again)
	}
	if n := f.count(t, `SELECT count(*) FROM accounts WHERE id=$1 AND status='deleted'`, first.Account.ID); n != 1 {
		t.Fatal("deleted account row must stay deleted")
	}
	// A link that still points at a deleted row (a deletion racing the
	// lookup) refuses with the deleted verdict.
	if _, err := f.pool.Exec(ctx, `UPDATE accounts SET status='deleted' WHERE id=$1`, again.Account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.nativeLogin(f.nativeToken(t, "pg-apple-status", "", "raw-pg-status-nonce-05"), "raw-pg-status-nonce-05"); !errors.Is(err, domain.ErrAccountDeleted) {
		t.Fatalf("deleted owner: want account-deleted, got %v", err)
	}
}
