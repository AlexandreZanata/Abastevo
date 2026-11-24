//go:build integration

package adapters

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters/oidc"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
)

const (
	googleNativeAudience = "test-web.apps.example.invalid"
	googleAndroidParty   = "test-android.apps.example.invalid"
)

type pgGoogleFixture struct {
	*linkService
	verifier *oidc.Verifier
}

func freshGoogleService(t *testing.T) *pgGoogleFixture {
	t.Helper()
	f := orgtest.New(t)
	// CI mode uses the fixture login. Script mode independently uses
	// only the reviewed deployment grants, never the local grants below.
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), `
	GRANT SELECT, INSERT, UPDATE ON accounts TO `+role+`;
	GRANT SELECT, INSERT, DELETE ON account_addresses TO `+role+`;
	GRANT SELECT, INSERT, UPDATE, DELETE ON account_provider_links, account_email_codes, account_session_families TO `+role+`;
	GRANT SELECT, INSERT, UPDATE, DELETE ON account_key_admission_events TO `+role+`;
	GRANT SELECT, INSERT ON account_oidc_nonces, account_recovery_audit TO `+role+`;
	GRANT SELECT, INSERT, UPDATE ON account_key_credentials TO `+role); err != nil {
		t.Fatal(err)
	}
	pool := f.RuntimePool(t, 8)
	clock := &integClock{now: 1_800_000_000}
	google, err := oidc.NewStubIssuer(domain.IssuerGoogle, "RS256")
	if err != nil {
		t.Fatal(err)
	}
	nonces := NewPGNonces(pool)
	verifier := &oidc.Verifier{
		Clock:   func() time.Time { return time.Unix(clock.NowUnix(), 0) },
		Issuers: map[string]string{"google": domain.IssuerGoogle},
		Keys:    oidc.NewMemKeys(map[string]oidc.StubIssuer{domain.IssuerGoogle: google}), Nonces: nonces,
		Skew:                 domain.OIDCClockSkewSeconds * time.Second,
		GoogleNativeAudience: googleNativeAudience, GoogleAuthorizedParties: map[string]bool{googleAndroidParty: true},
	}
	var ids atomic.Int64
	svc := &application.Service{
		Clock: clock, Hasher: domain.SHA256Hasher{}, Store: NewPGStore(pool),
		Mail: &integMail{}, MailEnabled: true,
		NativeAudiences: map[string]string{"google": googleNativeAudience}, Verifier: verifier,
		IDGen:  func() (string, error) { return testUUID(int(ids.Add(1))), nil },
		KeyGen: domain.GenerateAccountKey, TokenGen: domain.GenerateToken, AliasGen: domain.GenerateAlias,
		CodeGen: func() (string, error) { return "482916", nil },
	}
	return &pgGoogleFixture{linkService: &linkService{svc: svc, clock: clock, pool: f.Admin, google: google}, verifier: verifier}
}

func (f *pgGoogleFixture) loginRequest(t *testing.T, sub, email, raw, issuer, audience string, expiry time.Time, claims oidc.GoogleClaims) application.ProviderLogin {
	t.Helper()
	token, err := f.google.TokenWithGoogleClaims(sub, email, audience, domain.NativeNonceClaim(raw), f.verifier.Clock(), expiry, issuer, claims)
	if err != nil {
		t.Fatal(err)
	}
	return application.ProviderLogin{Provider: "google", IDToken: token, Nonce: raw}
}

func (f *pgGoogleFixture) standardRequest(t *testing.T, sub, email, raw string) application.ProviderLogin {
	t.Helper()
	return f.loginRequest(t, sub, email, raw, domain.IssuerGoogle, googleNativeAudience, f.verifier.Clock().Add(time.Hour),
		oidc.GoogleClaims{EmailVerified: true, HostedDomain: "example.invalid", AuthorizedParty: googleAndroidParty})
}

func TestPGGoogleNativeBindsOnlyAuthoritativeVerifiedEmailAndRetainsCanonicalIssuer(t *testing.T) {
	f := freshGoogleService(t)
	ctx := t.Context()
	for _, tt := range []struct {
		subject, email, issuer, hd string
		verified                   any
		wantHint                   string
	}{
		{"workspace-native-sub", "alpha@example.invalid", "accounts.google.com", "example.invalid", "true", "a***@example.invalid"},
		{"foreign-email-sub", "beta@example.invalid", domain.IssuerGoogle, "other.example.invalid", true, ""},
		{"unverified-email-sub", "gamma@example.invalid", domain.IssuerGoogle, "example.invalid", false, ""},
	} {
		t.Run(tt.subject, func(t *testing.T) {
			req := f.loginRequest(t, tt.subject, tt.email, "pg-google-nonce-"+tt.subject, tt.issuer, googleNativeAudience,
				f.verifier.Clock().Add(time.Hour), oidc.GoogleClaims{EmailVerified: tt.verified, HostedDomain: tt.hd, AuthorizedParty: googleAndroidParty})
			first, err := f.svc.LoginWithProvider(ctx, req)
			if err != nil || !first.Created || first.Account.DisplayName != "" {
				t.Fatalf("expected unnamed Google account: creation=%t, error=%v", first.Created, err)
			}
			view, err := f.svc.Profile(ctx, first.Session.FamilyID, first.Session.AccessToken)
			if err != nil || view.RecoveryEmailHint != tt.wantHint || view.HasAccountKey {
				t.Fatalf("initial Google profile did not have expected recovery or key state: %v", err)
			}
			if n := f.count(t, `SELECT count(*) FROM account_provider_links WHERE account_id=$1 AND issuer=$2 AND subject=$3 AND email=''`, first.Account.ID, domain.IssuerGoogle, tt.subject); n != 1 {
				t.Fatalf("want one canonical link with no raw email, got %d", n)
			}
			if n := f.count(t, `SELECT count(*) FROM account_addresses WHERE account_id=$1 AND address_hash=$2 AND email_hint=$3`, first.Account.ID, domain.AddressHash(tt.email), tt.wantHint); (tt.wantHint != "" && n != 1) || (tt.wantHint == "" && n != 0) {
				t.Fatal("unexpected recovery binding")
			}
			if _, err := f.svc.UpdateName(ctx, first.Session.FamilyID, first.Session.AccessToken, "Mariana Exemplo"); err != nil {
				t.Fatal(err)
			}
			again, err := f.svc.LoginWithProvider(ctx, f.standardRequest(t, tt.subject, "unused@example.invalid", "pg-google-again-"+tt.subject))
			if err != nil || again.Created || again.Account.ID != first.Account.ID || again.Account.DisplayName != "Mariana Exemplo" {
				t.Fatalf("known subject must preserve owner and public name: %v", err)
			}
			if n := f.count(t, `SELECT count(*) FROM account_addresses WHERE address_hash=$1`, domain.AddressHash("unused@example.invalid")); n != 0 {
				t.Fatal("known-subject sign-in must not alter recovery email")
			}
		})
	}
}

func TestPGGoogleNativeRejectsWrongProofAndReusesSharedNonceLedger(t *testing.T) {
	f := freshGoogleService(t)
	ctx := t.Context()
	now := f.verifier.Clock()
	for _, tt := range []struct {
		name, audience, party string
		expiry                time.Time
		alterNonce            bool
		want                  error
	}{
		{"link audience", linkAudience, googleAndroidParty, now.Add(time.Hour), false, domain.ErrOIDCWrongAudience},
		{"presenter mismatch", googleNativeAudience, "unapproved-party", now.Add(time.Hour), false, domain.ErrOIDCWrongAudience},
		{"expired token", googleNativeAudience, googleAndroidParty, now.Add(-time.Hour), false, domain.ErrOIDCExpired},
		{"raw nonce mismatch", googleNativeAudience, googleAndroidParty, now.Add(time.Hour), true, domain.ErrOIDCNonceMismatch},
	} {
		t.Run(tt.name, func(t *testing.T) {
			raw := "pg-native-test-proof-" + fmt.Sprint(len(tt.name))
			req := f.loginRequest(t, "provider-proof-test-subject", "alpha@example.invalid", raw, domain.IssuerGoogle, tt.audience, tt.expiry,
				oidc.GoogleClaims{EmailVerified: true, HostedDomain: "example.invalid", AuthorizedParty: tt.party})
			if tt.alterNonce {
				req.Nonce += "-mismatch"
			}
			if _, err := f.svc.LoginWithProvider(ctx, req); !errors.Is(err, tt.want) {
				t.Fatalf("want %v, got %v", tt.want, err)
			}
		})
	}
	if n := f.count(t, "SELECT count(*) FROM account_oidc_nonces"); n != 0 {
		t.Fatalf("bad Google proofs consumed %d nonces", n)
	}
	if n := f.count(t, "SELECT count(*) FROM accounts"); n != 0 {
		t.Fatalf("bad Google proofs created %d accounts", n)
	}
	req := f.standardRequest(t, "provider-proof-test-subject", "alpha@example.invalid", "pg-native-test-proof-valid")
	f.svc.NativeAudiences = nil
	if _, err := f.svc.LoginWithProvider(ctx, req); !errors.Is(err, domain.ErrProviderLoginUnavailable) {
		t.Fatalf("unset audience: %v", err)
	}
	f.svc.NativeAudiences = map[string]string{"google": googleNativeAudience}
	keys := f.verifier.Keys
	f.verifier.Keys = oidc.FailingKeys{}
	if _, err := f.svc.LoginWithProvider(ctx, req); !errors.Is(err, domain.ErrOIDCUnavailable) {
		t.Fatalf("JWKS outage: %v", err)
	}
	f.verifier.Keys = keys
	if _, err := f.svc.LoginWithProvider(ctx, req); err != nil {
		t.Fatalf("good proof refused following unsuccessful attempts: %v", err)
	}
	other := *f.verifier
	f.svc.Verifier = &other
	if _, err := f.svc.LoginWithProvider(ctx, req); !errors.Is(err, domain.ErrOIDCNonceReused) {
		t.Fatalf("shared nonce ledger failed: %v", err)
	}
	if n := f.count(t, "SELECT count(*) FROM account_session_families"); n != 1 {
		t.Fatalf("want one live issuance, got %d", n)
	}
}

func TestPGGoogleNativeConcurrentFirstSignInsConverge(t *testing.T) {
	f := freshGoogleService(t)
	ctx := t.Context()
	const workers = 8
	var results [workers]application.AuthResult
	var errs [workers]error
	start := make(chan struct{})
	var wait sync.WaitGroup
	for i := range workers {
		req := f.standardRequest(t, "pg-parallel-google-subject", "parallel@example.invalid", fmt.Sprintf("pg-google-shared-subject-race-%d", i))
		wait.Go(func() {
			<-start
			results[i], errs[i] = f.svc.LoginWithProvider(ctx, req)
		})
	}
	close(start)
	wait.Wait()
	created := 0
	for i, got := range results {
		if errs[i] != nil || got.Account.ID == "" || got.Account.ID != results[0].Account.ID {
			t.Fatalf("login %d failed convergence: %v", i, errs[i])
		}
		if got.Created {
			created++
		}
	}
	if created != 1 || f.count(t, "SELECT count(*) FROM accounts") != 1 ||
		f.count(t, "SELECT count(*) FROM account_addresses") != 1 || f.count(t, "SELECT count(*) FROM account_provider_links") != 1 {
		t.Fatal("parallel first sign-ins must leave exactly one owner, binding and link")
	}
}

func TestPGGoogleNativeAndKeySignupShareAddressAdmission(t *testing.T) {
	f := freshGoogleService(t)
	ctx := t.Context()
	const occupied = "bound@example.invalid"
	if err := f.svc.RequestCode(ctx, occupied); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SignUp(ctx, "First Account", occupied, "482916"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.LoginWithProvider(ctx, f.standardRequest(t, "foreign-subject", occupied, "pg-google-fails-email-taken")); !errors.Is(err, domain.ErrProviderEmailInUse) {
		t.Fatalf("authoritative duplicate should safely refuse, got %v", err)
	}
	if f.count(t, "SELECT count(*) FROM accounts") != 1 || f.count(t, "SELECT count(*) FROM account_provider_links") != 0 || f.count(t, "SELECT count(*) FROM account_addresses") != 1 {
		t.Fatal("failed login left a partial Google account or link")
	}
	const racers = 4
	googleWins := 0
	for i := range racers {
		address := fmt.Sprintf("race-%d@example.invalid", i)
		if err := f.svc.RequestCode(ctx, address); err != nil {
			t.Fatal(err)
		}
		req := f.standardRequest(t, fmt.Sprintf("pg-google-race-%d", i), address, fmt.Sprintf("pg-google-address-ownership-race-%d", i))
		var wg sync.WaitGroup
		start := make(chan struct{})
		var signupErr, googleErr error
		wg.Go(func() { <-start; _, signupErr = f.svc.SignUp(ctx, "Race Candidate", address, "482916") })
		wg.Go(func() { <-start; _, googleErr = f.svc.LoginWithProvider(ctx, req) })
		close(start)
		wg.Wait()
		switch {
		case signupErr == nil && errors.Is(googleErr, domain.ErrProviderEmailInUse):
		case googleErr == nil && errors.Is(signupErr, domain.ErrAddressLinked):
			googleWins++
		default:
			t.Fatalf("one proof must win email binding, signup=%v google=%v", signupErr, googleErr)
		}
		if n := f.count(t, "SELECT count(*) FROM account_addresses WHERE address_hash=$1", domain.AddressHash(address)); n != 1 {
			t.Fatalf("found %d email owners", n)
		}
	}
	if f.count(t, "SELECT count(*) FROM accounts") != 1+racers || f.count(t, "SELECT count(*) FROM account_provider_links") != googleWins {
		t.Fatal("email race created an extra account or link")
	}
}

func TestPGGoogleNativeFreshLinkIsIdempotentAndCannotStealSubject(t *testing.T) {
	f := freshGoogleService(t)
	ctx := t.Context()
	first, err := f.svc.CreateKeyAccount(ctx, "firstowner")
	if err != nil {
		t.Fatal(err)
	}
	other, err := f.svc.CreateKeyAccount(ctx, "secondowner")
	if err != nil {
		t.Fatal(err)
	}
	firstLogin, err := f.svc.LoginWithKey(ctx, first.Key)
	if err != nil {
		t.Fatal(err)
	}
	otherLogin, err := f.svc.LoginWithKey(ctx, other.Key)
	if err != nil {
		t.Fatal(err)
	}
	link := func(s application.Session, nonce string) (domain.ProviderLink, error) {
		req := f.standardRequest(t, "pg-link-native-subject", "link-native@example.invalid", nonce)
		return f.svc.LinkGoogleNative(ctx, s.FamilyID, s.AccessToken, req)
	}
	firstLink, err := link(firstLogin.Session, "pg-google-fresh-first")
	if err != nil {
		t.Fatal(err)
	}
	f.clock.advance(600 * time.Second)
	if again, err := link(firstLogin.Session, "pg-google-fresh-second"); err != nil || again.LinkedAt != firstLink.LinkedAt {
		t.Fatalf("fresh link at inclusive boundary must be idempotent: %v", err)
	}
	if _, err := link(otherLogin.Session, "pg-google-fresh-other"); !errors.Is(err, domain.ErrLinkCrossAccount) {
		t.Fatalf("cross-account subject link: %v", err)
	}
	f.clock.advance(time.Second)
	if _, err := link(firstLogin.Session, "pg-google-fresh-third"); !errors.Is(err, domain.ErrReauthenticationRequired) {
		t.Fatalf("stale session linked a subject: %v", err)
	}
	if f.count(t, "SELECT count(*) FROM account_provider_links WHERE account_id=$1 AND email=''", first.Account.ID) != 1 ||
		f.count(t, "SELECT count(*) FROM account_addresses") != 0 || f.count(t, "SELECT count(*) FROM account_provider_links WHERE account_id=$1", other.Account.ID) != 0 {
		t.Fatal("native linking changed recovery email ownership or linked a foreign account")
	}
}
