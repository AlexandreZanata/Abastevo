//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	companyadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters"
	companyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/application"
	companydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
)

func pgNamedSignUp(t *testing.T, svc *application.Service, address, code string) application.VerifiedKeyResult {
	t.Helper()
	pgRequest(t, svc, address)
	signed, err := svc.SignUp(t.Context(), "  Ana  Téste  ", address, code)
	if err != nil {
		t.Fatalf("sign up: %v", err)
	}
	if signed.Account.DisplayName != "Ana Téste" || signed.Session.AccountID != signed.Account.ID || len(signed.Key) != domain.KeyLength {
		t.Fatal("signup must issue a named account, a key and a matching session")
	}
	return signed
}

func TestPGRecoveryRevokesOldKeyFamiliesAndCompanyCookie(t *testing.T) {
	f := orgtest.New(t)
	ctx := t.Context()
	if _, err := f.Admin.Exec(ctx, "GRANT SELECT,INSERT,UPDATE,DELETE ON accounts,account_session_families,account_addresses,account_key_credentials,account_key_admission_events,account_email_codes,account_recovery_audit,account_provider_links,company_web_sessions TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	runtime := f.RuntimePool(t, 4)
	store := NewPGStore(runtime)
	clock := &integClock{now: 1_800_000_000}
	codes := []string{"482916", "111111"}
	ids := 0
	svc := &application.Service{
		Clock: clock, Store: store, Hasher: domain.SHA256Hasher{}, Mail: &integMail{}, MailEnabled: true,
		CodeGen: func() (string, error) {
			val := codes[0]
			codes = codes[1:]
			return val, nil
		},
		AliasGen: domain.GenerateAlias, KeyGen: domain.GenerateAccountKey, TokenGen: domain.GenerateToken,
		IDGen: func() (string, error) { ids++; return testUUID(ids), nil },
	}
	business := &companyapp.Service{
		Enabled: true, Store: &companyadapters.Store{Pool: runtime}, Hasher: domain.SHA256Hasher{},
		Prove: companyadapters.AccountProof(svc), Now: func() time.Time { return time.Unix(clock.NowUnix(), 0) },
		NewID: svc.IDGen, NewSecret: domain.GenerateToken,
	}
	const address = "recovery-owner@example.invalid"
	old := pgNamedSignUp(t, svc, address, "482916")
	login, err := svc.LoginWithKey(ctx, old.Key)
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := business.Login(ctx, login.Session.FamilyID, login.Session.AccessToken, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := business.Authenticate(ctx, cookie.Token, cookie.CSRF, false); err != nil {
		t.Fatal(err)
	}
	pgRequest(t, svc, address)
	got, err := svc.RecoverKey(ctx, address, "111111")
	if err != nil {
		t.Fatal(err)
	}
	if got.Account.ID != old.Account.ID || got.Key == old.Key {
		t.Fatal("recovery must retain the owner and replace the key")
	}
	if _, err := svc.LoginWithKey(ctx, old.Key); !errors.Is(err, domain.ErrKeyInvalid) {
		t.Fatalf("old key: %v", err)
	}
	for _, session := range []application.Session{old.Session, login.Session} {
		if _, err := svc.ValidateAccess(ctx, session.FamilyID, session.AccessToken); !errors.Is(err, domain.ErrSessionRevoked) {
			t.Fatalf("old access: %v", err)
		}
		if _, err := svc.Refresh(ctx, session.FamilyID, session.RefreshToken); !errors.Is(err, domain.ErrSessionRevoked) {
			t.Fatalf("old refresh: %v", err)
		}
	}
	if _, err := business.Authenticate(ctx, cookie.Token, cookie.CSRF, false); !errors.Is(err, companydomain.ErrUnauthorized) {
		t.Fatalf("derived company session: %v", err)
	}
	if _, err := svc.ValidateAccess(ctx, got.Session.FamilyID, got.Session.AccessToken); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LoginWithKey(ctx, got.Key); err != nil {
		t.Fatal(err)
	}

	var usernameHash string
	if err := f.Admin.QueryRow(ctx, "SELECT username_hash FROM account_key_credentials WHERE account_id=$1", got.Account.ID).Scan(&usernameHash); err != nil || usernameHash != domain.UsernameHash("account:"+got.Account.ID) {
		t.Fatalf("compatibility username hash: %v", err)
	}
	if count, _, err := store.PurgeRecoveryAudit(ctx, time.Unix(clock.NowUnix()+domain.RecoveryAuditTTLSeconds-1, 0), 1); err != nil || count != 0 {
		t.Fatalf("early purge: count=%d, error=%v", count, err)
	}
	if count, _, err := store.PurgeRecoveryAudit(ctx, time.Unix(clock.NowUnix()+domain.RecoveryAuditTTLSeconds, 0), 1); err != nil || count != 1 {
		t.Fatalf("due purge: count=%d, error=%v", count, err)
	}
}

func TestPGVerifiedSignupConsumesOnlyProvenCodesAndConverges(t *testing.T) {
	svc, clock, _ := freshService(t)
	ctx := t.Context()
	const address = "signed-up@example.invalid"
	pgRequest(t, svc, address)
	clock.advance(60 * time.Second)
	pgRequest(t, svc, address)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, code := range []string{"482916", "111111"} {
		wg.Go(func() {
			_, err := svc.SignUp(ctx, "Ana Silva", address, code)
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	ok, linked := 0, 0
	for err := range errs {
		switch err {
		case nil:
			ok++
		case domain.ErrAddressLinked:
			linked++
		default:
			t.Fatalf("parallel signup: %v", err)
		}
	}
	if ok != 1 || linked != 1 {
		t.Fatalf("signup race: created=%d linked=%d", ok, linked)
	}
	for _, code := range []string{"482916", "111111"} {
		if _, err := svc.SignUp(ctx, "Ana Silva", address, code); !errors.Is(err, domain.ErrCodeConsumed) {
			t.Fatalf("replay after success/conflict: %v", err)
		}
	}
	pool := svc.Store.(*PGStore).pool
	var counts [3]int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM accounts),
		(SELECT count(*) FROM account_key_credentials), (SELECT count(*) FROM account_session_families)`).Scan(&counts[0], &counts[1], &counts[2]); err != nil || counts != [3]int{1, 1, 1} {
		t.Fatalf("persisted account/key/family count: %v, %v", counts, err)
	}
}

func TestPGVerifiedSignupFailureLeavesProofReusable(t *testing.T) {
	svc, clock, _ := freshService(t)
	ctx := t.Context()
	pgRequest(t, svc, "rollback@example.invalid")
	store := svc.Store.(*PGStore)
	if _, err := store.pool.Exec(ctx, "INSERT INTO account_key_admission_events (admitted_at) SELECT $1 FROM generate_series(1, $2)", clock.NowUnix(), domain.MaxKeyAccountsPerHour); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SignUp(ctx, "Ana Silva", "rollback@example.invalid", "482916"); !errors.Is(err, domain.ErrAggregateExhausted) {
		t.Fatalf("aggregate ceiling: %v", err)
	}
	if _, err := store.pool.Exec(ctx, "DELETE FROM account_key_admission_events"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, "ALTER TABLE account_session_families ADD CONSTRAINT synthetic_failure CHECK (false)"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SignUp(ctx, "Ana Silva", "rollback@example.invalid", "482916"); err == nil {
		t.Fatal("session write failure must refuse signup")
	}
	var partial bool
	if err := store.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM accounts)
		OR EXISTS (SELECT 1 FROM account_addresses)
		OR EXISTS (SELECT 1 FROM account_key_credentials)
		OR EXISTS (SELECT 1 FROM account_email_codes WHERE consumed_at IS NOT NULL)`).Scan(&partial); err != nil || partial {
		t.Fatalf("failed signup left partial state: %v %v", partial, err)
	}
	if _, err := store.pool.Exec(ctx, "ALTER TABLE account_session_families DROP CONSTRAINT synthetic_failure"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SignUp(ctx, "Ana Silva", "rollback@example.invalid", "482916"); err != nil {
		t.Fatal(err)
	}
}

func TestPGCodeFailuresSuspensionAndDeletedRecovery(t *testing.T) {
	svc, clock, _ := freshService(t)
	ctx := t.Context()
	const address = "owner-status@example.invalid"
	owner := pgNamedSignUp(t, svc, address, "482916")
	pgRequest(t, svc, address)
	for i := 0; i < domain.CodeMaxAttempts; i++ {
		for _, unknown := range []string{address, "unknown@example.invalid"} {
			if _, err := svc.RecoverKey(ctx, unknown, "000000"); !errors.Is(err, domain.ErrCodeUnknown) {
				t.Fatalf("wrong code or absent address must share a verdict: %v", err)
			}
		}
	}
	if _, err := svc.SignUp(ctx, "Ana Silva", address, "111111"); !errors.Is(err, domain.ErrCodeAttemptsExhausted) {
		t.Fatalf("locked code: %v", err)
	}
	clock.advance(time.Duration(domain.CodeTTLSeconds+1) * time.Second)
	if _, err := svc.SignUp(ctx, "Ana Silva", address, "111111"); !errors.Is(err, domain.ErrCodeExpired) {
		t.Fatalf("expired code: %v", err)
	}
	pgRequest(t, svc, address)
	if err := svc.SuspendAccount(ctx, owner.Account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecoverKey(ctx, address, "000000"); !errors.Is(err, domain.ErrCodeUnknown) {
		t.Fatalf("unproven recovery disclosed suspension: %v", err)
	}
	if _, err := svc.RecoverKey(ctx, address, "222222"); !errors.Is(err, domain.ErrAccountSuspended) {
		t.Fatalf("suspended recovery: %v", err)
	}
	if err := svc.ReactivateAccount(ctx, owner.Account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecoverKey(ctx, address, "222222"); err != nil {
		t.Fatalf("suspension burned the proof: %v", err)
	}
	pgRequest(t, svc, address)
	if err := svc.SuspendAccount(ctx, owner.Account.ID); err != nil {
		t.Fatal(err)
	}
	for range domain.CodeMaxAttempts {
		if _, err := svc.RecoverKey(ctx, address, "000000"); !errors.Is(err, domain.ErrCodeUnknown) {
			t.Fatalf("unproven recovery disclosed suspension or attempts: %v", err)
		}
	}
	if _, err := svc.RecoverKey(ctx, address, "333333"); !errors.Is(err, domain.ErrCodeAttemptsExhausted) {
		t.Fatalf("suspended account did not enforce the attempt limit: %v", err)
	}
	if err := svc.ReactivateAccount(ctx, owner.Account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecoverKey(ctx, address, "333333"); !errors.Is(err, domain.ErrCodeAttemptsExhausted) {
		t.Fatalf("reactivation unlocked guesses against the code: %v", err)
	}
	pgRequest(t, svc, address)
	if _, err := svc.DeleteAccount(ctx, owner.Account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecoverKey(ctx, address, "444444"); !errors.Is(err, domain.ErrRecoveryNoAccount) {
		t.Fatalf("verified deleted address: %v", err)
	}
	pool := svc.Store.(*PGStore).pool
	var removed bool
	if err := pool.QueryRow(ctx, "SELECT display_name IS NULL FROM accounts WHERE id=$1", owner.Account.ID).Scan(&removed); err != nil || !removed {
		t.Fatalf("deleted public name remains: %v", err)
	}
	clock.advance(time.Hour)
	if replacement := pgNamedSignUp(t, svc, address, "555555"); replacement.Account.ID == owner.Account.ID {
		t.Fatal("re-signup after deletion reused the deleted owner")
	}
}

type holdVerifiedKeyLookup struct {
	application.Store
	lookedUp chan struct{}
	resume   chan struct{}
}

func (store *holdVerifiedKeyLookup) FindKeyCredentialByLookup(ctx context.Context, lookup string) (domain.KeyCredential, bool, error) {
	cred, found, err := store.Store.FindKeyCredentialByLookup(ctx, lookup)
	close(store.lookedUp)
	select {
	case <-store.resume:
		return cred, found, err
	case <-ctx.Done():
		return domain.KeyCredential{}, false, ctx.Err()
	}
}

func TestPGRecoveryRacesDoNotCreateAnOldKeySession(t *testing.T) {
	svc, clock, _ := freshService(t)
	ctx := t.Context()
	const address = "recover-race@example.invalid"
	original := pgNamedSignUp(t, svc, address, "482916")
	pgRequest(t, svc, address)
	clock.advance(60 * time.Second)
	pgRequest(t, svc, address)
	block := &holdVerifiedKeyLookup{Store: svc.Store, lookedUp: make(chan struct{}), resume: make(chan struct{})}
	loginSvc := *svc
	loginSvc.Store = block
	oldLogin := make(chan error, 1)
	go func() {
		_, err := loginSvc.LoginWithKey(ctx, original.Key)
		oldLogin <- err
	}()
	select {
	case <-block.lookedUp:
	case <-time.After(5 * time.Second):
		close(block.resume)
		t.Fatal("login failed to reach credential lookup")
	}

	type result struct {
		auth application.VerifiedKeyResult
		err  error
	}
	got := make(chan result, 2)
	for _, code := range []string{"111111", "222222"} {
		go func() {
			auth, err := svc.RecoverKey(ctx, address, code)
			got <- result{auth: auth, err: err}
		}()
	}
	a, b := <-got, <-got
	close(block.resume)
	if a.err != nil || b.err != nil {
		t.Fatalf("parallel recovery: %v, %v", a.err, b.err)
	}
	if err := <-oldLogin; !errors.Is(err, domain.ErrKeyInvalid) {
		t.Fatalf("stale credential lookup opened a session: %v", err)
	}
	live := 0
	for _, r := range []result{a, b} {
		_, sessErr := svc.ValidateAccess(ctx, r.auth.Session.FamilyID, r.auth.Session.AccessToken)
		_, keyErr := svc.LoginWithKey(ctx, r.auth.Key)
		if sessErr == nil && keyErr == nil {
			live++
		} else if !errors.Is(sessErr, domain.ErrSessionRevoked) || !errors.Is(keyErr, domain.ErrKeyInvalid) {
			t.Fatalf("key/session mismatch: %v, %v", keyErr, sessErr)
		}
	}
	if live != 1 {
		t.Fatalf("active key and recovery session pairs = %d", live)
	}
}

func TestPGRecoveryEmailRequiresFreshLoginAndReplacesHash(t *testing.T) {
	svc, clock, _ := freshService(t)
	ctx := t.Context()
	const firstAddress = "first-address@example.invalid"
	const secondAddress = "second-address@example.invalid"
	key, err := svc.CreateKeyAccount(ctx, "legacyname")
	if err != nil {
		t.Fatal(err)
	}
	old, err := svc.LoginWithKey(ctx, key.Key)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := svc.Profile(ctx, old.Session.FamilyID, old.Session.AccessToken)
	if err != nil || profile.Account.PublicName() != "legacyname" || profile.Account.DisplayName != "" || !profile.HasAccountKey || profile.RecoveryEmailHint != "" {
		t.Fatalf("legacy profile mismatch: %v", err)
	}
	if _, err := svc.UpdateName(ctx, old.Session.FamilyID, old.Session.AccessToken, "Áurea Teste"); err != nil {
		t.Fatal(err)
	}
	clock.advance((domain.FreshLoginSeconds + 1) * time.Second)
	refresh, err := svc.Refresh(ctx, old.Session.FamilyID, old.Session.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	pgRequest(t, svc, firstAddress)
	if _, err := svc.SetRecoveryEmail(ctx, refresh.FamilyID, refresh.AccessToken, firstAddress, "482916"); !errors.Is(err, domain.ErrReauthenticationRequired) {
		t.Fatalf("refreshed but stale session: %v", err)
	}
	fresh, err := svc.LoginWithKey(ctx, key.Key)
	if err != nil {
		t.Fatal(err)
	}
	profile, err = svc.SetRecoveryEmail(ctx, fresh.Session.FamilyID, fresh.Session.AccessToken, firstAddress, "482916")
	if err != nil || profile.RecoveryEmailHint != "f***@example.invalid" {
		t.Fatalf("attach original recovery address: %v", err)
	}
	other := pgNamedSignUp(t, svc, "other-address@example.invalid", "111111")
	pgRequest(t, svc, "other-address@example.invalid")
	if _, err := svc.SetRecoveryEmail(ctx, fresh.Session.FamilyID, fresh.Session.AccessToken, "other-address@example.invalid", "222222"); !errors.Is(err, domain.ErrAddressLinked) {
		t.Fatalf("cross-account attach: %v", err)
	}
	if _, err := svc.ValidateAccess(ctx, other.Session.FamilyID, other.Session.AccessToken); err != nil {
		t.Fatal(err)
	}
	pgRequest(t, svc, secondAddress)
	profile, err = svc.SetRecoveryEmail(ctx, fresh.Session.FamilyID, fresh.Session.AccessToken, secondAddress, "333333")
	if err != nil || profile.RecoveryEmailHint != "s***@example.invalid" || profile.Account.PublicName() != "Áurea Teste" {
		t.Fatalf("replace recovery address: %v", err)
	}
	pgRequest(t, svc, firstAddress)
	if _, err := svc.RecoverKey(ctx, firstAddress, "444444"); !errors.Is(err, domain.ErrRecoveryNoAccount) {
		t.Fatalf("old email still resolves to account: %v", err)
	}
	pool := svc.Store.(*PGStore).pool
	var actions []string
	if err := pool.QueryRow(ctx, "SELECT array_agg(action ORDER BY id) FROM account_recovery_audit WHERE account_id=$1", key.Account.ID).Scan(&actions); err != nil || fmt.Sprint(actions) != "[email_add email_change]" {
		t.Fatalf("recovery audit actions = %v, error=%v", actions, err)
	}
}

func TestPGRecoveryFailurePreservesCredentialFamiliesAndCode(t *testing.T) {
	svc, _, _ := freshService(t)
	ctx := t.Context()
	const address = "error-recovery@example.invalid"
	first := pgNamedSignUp(t, svc, address, "482916")
	pgRequest(t, svc, address)
	pool := svc.Store.(*PGStore).pool
	if _, err := pool.Exec(ctx, "ALTER TABLE account_recovery_audit ADD CONSTRAINT synthetic_failure CHECK (action <> 'recovery')"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecoverKey(ctx, address, "111111"); err == nil {
		t.Fatal("recovery reported success without writing the audit record")
	}
	if _, err := svc.ValidateAccess(ctx, first.Session.FamilyID, first.Session.AccessToken); err != nil {
		t.Fatalf("rolled-back recovery revoked the existing session: %v", err)
	}
	if _, err := svc.LoginWithKey(ctx, first.Key); err != nil {
		t.Fatalf("rolled-back recovery changed the existing key: %v", err)
	}
	if _, err := pool.Exec(ctx, "ALTER TABLE account_recovery_audit DROP CONSTRAINT synthetic_failure"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecoverKey(ctx, address, "111111"); err != nil {
		t.Fatalf("rolled-back recovery consumed the proof: %v", err)
	}
}

type holdNativeFamilyInsert struct {
	*PGStore
	ready  chan struct{}
	resume chan struct{}
}

func (s *holdNativeFamilyInsert) TryCreateFamily(ctx context.Context, fam domain.SessionFamily, now int64) error {
	close(s.ready)
	select {
	case <-s.resume:
		return s.PGStore.TryCreateFamily(ctx, fam, now)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestPGRecoveryAndFreshNativeSignInKeepConsistentFamilies(t *testing.T) {
	f := freshGoogleService(t)
	ctx := t.Context()
	const address = "race-native-recovery@example.invalid"
	const subject = "native-recovery-race-subject"
	first, err := f.svc.LoginWithProvider(ctx, f.standardRequest(t, subject, address, "pg-native-login-prior-session"))
	if err != nil {
		t.Fatal(err)
	}
	pgRequest(t, f.svc, address)
	block := &holdNativeFamilyInsert{PGStore: f.svc.Store.(*PGStore), ready: make(chan struct{}), resume: make(chan struct{})}
	loginSvc := *f.svc
	loginSvc.Store = block
	req := f.standardRequest(t, subject, address, "pg-native-login-paused-session")
	type result struct {
		out application.AuthResult
		err error
	}
	later := make(chan result, 1)
	go func() {
		out, err := loginSvc.LoginWithProvider(ctx, req)
		later <- result{out: out, err: err}
	}()
	select {
	case <-block.ready:
	case <-time.After(5 * time.Second):
		close(block.resume)
		t.Fatal("native sign-in did not reach family creation")
	}
	recovered, err := f.svc.RecoverKey(ctx, address, "482916")
	close(block.resume)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.ValidateAccess(ctx, first.Session.FamilyID, first.Session.AccessToken); !errors.Is(err, domain.ErrSessionRevoked) {
		t.Fatalf("prior Google session must be revoked: %v", err)
	}
	newLogin := <-later
	if newLogin.err != nil {
		t.Fatalf("fresh independent Google proof failed: %v", newLogin.err)
	}
	for _, sess := range []application.Session{recovered.Session, newLogin.out.Session} {
		if id, err := f.svc.ValidateAccess(ctx, sess.FamilyID, sess.AccessToken); err != nil || id != first.Account.ID {
			t.Fatalf("new family does not belong to the recovered owner: %v", err)
		}
	}
	if logged, err := f.svc.LoginWithKey(ctx, recovered.Key); err != nil || logged.Account.ID != first.Account.ID {
		t.Fatalf("recovery did not create a key for the Google-only account: %v", err)
	}
}

func TestPGExpiredEmailCodePurgeBoundsBatchAndKeepsCutoff(t *testing.T) {
	f := orgtest.New(t)
	ctx := t.Context()
	const cutoff = int64(1_800_000_000)
	if _, err := f.Admin.Exec(ctx, "GRANT SELECT, DELETE ON account_email_codes TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(ctx, `
		INSERT INTO account_email_codes(id, address_hash, salt, hash, issued_at)
		SELECT gen_random_uuid(), 'synthetic-lookup', 'synthetic-salt', 'synthetic-verifier',
			to_timestamp(($1::bigint + i)::double precision)
		FROM generate_series(-502, 1) AS i`, cutoff); err != nil {
		t.Fatal(err)
	}

	store := NewPGStore(f.Runtime)
	for _, pass := range []struct {
		count  int
		oldest int64
	}{
		{500, cutoff - 2},
		{2, cutoff},
		{0, cutoff},
	} {
		n, err := store.PurgeExpiredCodes(ctx, cutoff)
		if err != nil || n != pass.count {
			t.Fatalf("purged %d, want %d: %v", n, pass.count, err)
		}
		var oldest, newest int64
		if err := f.Admin.QueryRow(ctx, `
			SELECT extract(epoch FROM min(issued_at))::bigint, extract(epoch FROM max(issued_at))::bigint
			FROM account_email_codes`).Scan(&oldest, &newest); err != nil || oldest != pass.oldest || newest != cutoff+1 {
			t.Fatalf("retention order or cutoff changed: oldest=%d newest=%d error=%v", oldest, newest, err)
		}
	}
}
