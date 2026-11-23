package application

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

const grantCode = "c0ffee.0.synthetic-native-code"

type exchangeCall struct{ provider, code, subject, aad string }

type fakeExchanger struct {
	mu    sync.Mutex
	calls []exchangeCall
	err   error
}

func (f *fakeExchanger) Exchange(_ context.Context, provider, code, subject, aad string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, exchangeCall{provider, code, subject, aad})
	if f.err != nil {
		return nil, f.err
	}
	return []byte("sealed|" + aad), nil
}

func (f *fakeExchanger) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

type fakeRevoker struct {
	mu   sync.Mutex
	aads []string
	err  error
}

func (f *fakeRevoker) Revoke(_ context.Context, provider string, sealed []byte, aad string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if provider != "apple" || string(sealed) != "sealed|"+aad {
		return errors.New("revoker: grant does not open under its binding")
	}
	f.aads = append(f.aads, aad)
	return f.err
}

type grantFixture struct {
	*linkFixture
	store     *MemStore
	exchanger *fakeExchanger
	failures  []error
}

func newGrantFixture(t *testing.T) *grantFixture {
	t.Helper()
	f := &grantFixture{linkFixture: nativeFixture(t), exchanger: &fakeExchanger{}}
	f.store = f.svc.Store.(*MemStore)
	f.svc.Grants = f.exchanger
	f.svc.OnGrantFailure = func(_ context.Context, provider string, err error) {
		if provider != "apple" {
			t.Errorf("failure hook provider = %q", provider)
		}
		f.failures = append(f.failures, err)
	}
	f.svc.RequestErasure = func(context.Context, string, []string) error { return nil }
	f.svc.RequestBillingExit = func(context.Context, string) error { return nil }
	return f
}

func (f *grantFixture) loginWithCode(t *testing.T, sub string, n int, code string) (AuthResult, error) {
	t.Helper()
	return f.svc.LoginWithProvider(context.Background(), ProviderLogin{
		Provider: "apple", IDToken: f.nativeToken(t, sub, "", rawNonce(n)), Nonce: rawNonce(n), AuthorizationCode: code,
	})
}

func (f *grantFixture) grants(accountID string) []domain.ProviderGrant {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()
	var out []domain.ProviderGrant
	for _, g := range f.store.grants[accountID] {
		out = append(out, g)
	}
	return out
}

func (f *grantFixture) queued() []domain.GrantRevocation {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()
	var out []domain.GrantRevocation
	for _, r := range f.store.revocations {
		out = append(out, r.rev)
	}
	return out
}

func TestProviderLoginCapturesSealedGrantBoundToItsID(t *testing.T) {
	f := newGrantFixture(t)
	got, err := f.loginWithCode(t, "grant-sub-1", 1, grantCode)
	if err != nil {
		t.Fatal(err)
	}
	grants := f.grants(got.Account.ID)
	if len(grants) != 1 || f.exchanger.count() != 1 {
		t.Fatalf("want one captured grant, got %d (calls %d)", len(grants), f.exchanger.count())
	}
	g, call := grants[0], f.exchanger.calls[0]
	if call.provider != "apple" || call.code != grantCode || call.subject != "grant-sub-1" || call.aad != domain.GrantAAD(g.ID) {
		t.Fatalf("exchange call = %+v for grant %+v", call, g)
	}
	if g.Subject != "grant-sub-1" || g.Provider != "apple" || string(g.Sealed) != "sealed|"+domain.GrantAAD(g.ID) {
		t.Fatalf("grant = %+v", g)
	}
	// A later sign-in replaces the grant: one live grant per subject.
	if _, err := f.loginWithCode(t, "grant-sub-1", 2, grantCode); err != nil {
		t.Fatal(err)
	}
	again := f.grants(got.Account.ID)
	if len(again) != 1 || again[0].ID == g.ID {
		t.Fatalf("re-login must replace the grant: %+v", again)
	}
}

func TestProviderLoginWithoutCaptureIgnoresCode(t *testing.T) {
	f := newGrantFixture(t)
	f.svc.Grants = nil
	got, err := f.loginWithCode(t, "grant-sub-2", 3, grantCode)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.grants(got.Account.ID)) != 0 || f.exchanger.count() != 0 || len(f.failures) != 0 {
		t.Fatal("unconfigured capture must record nothing")
	}
	// Deleting it queues nothing to revoke.
	if _, err := f.svc.DeleteAccount(context.Background(), got.Account.ID); err != nil {
		t.Fatal(err)
	}
	if len(f.queued()) != 0 {
		t.Fatal("deletion queued a revocation without a grant")
	}
}

func TestProviderLoginGrantFailureNeverBlocksSignIn(t *testing.T) {
	f := newGrantFixture(t)
	f.exchanger.err = errors.New("apple: authorization code exchange refused")
	got, err := f.loginWithCode(t, "grant-sub-3", 4, grantCode)
	if err != nil || got.Session.AccessToken == "" {
		t.Fatalf("sign-in must succeed without a grant: %v", err)
	}
	if len(f.grants(got.Account.ID)) != 0 || len(f.failures) != 1 {
		t.Fatalf("failure must be observed once and record nothing: %v", f.failures)
	}
	if strings.Contains(f.failures[0].Error(), grantCode) {
		t.Fatal("failure leaked the authorization code")
	}
}

func TestProviderLoginMalformedCodeRefusedBeforeVerification(t *testing.T) {
	f := newGrantFixture(t)
	for i, code := range []string{"code with space", strings.Repeat("c", domain.MaxAuthorizationCodeBytes+1), "tab\tcode"} {
		if _, err := f.loginWithCode(t, "grant-sub-4", 10+i, code); !errors.Is(err, domain.ErrProviderProofMalformed) {
			t.Fatalf("code %q: want malformed, got %v", code, err)
		}
		if !f.nonces.Consume(domain.NativeNonceClaim(rawNonce(10 + i))) {
			t.Fatal("malformed code consumed the nonce")
		}
	}
	if f.exchanger.count() != 0 {
		t.Fatal("malformed code reached the provider")
	}
}

func TestProviderLoginRefusedAccountSkipsCapture(t *testing.T) {
	f := newGrantFixture(t)
	first, err := f.loginWithCode(t, "grant-sub-5", 20, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.SuspendAccount(context.Background(), first.Account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.loginWithCode(t, "grant-sub-5", 21, grantCode); !errors.Is(err, domain.ErrAccountSuspended) {
		t.Fatalf("want suspended, got %v", err)
	}
	if f.exchanger.count() != 0 {
		t.Fatal("refused account redeemed a code")
	}
}

func TestDeletionQueuesGrantsAndLateGrantsGoStraightToQueue(t *testing.T) {
	f := newGrantFixture(t)
	ctx := context.Background()
	got, err := f.loginWithCode(t, "grant-sub-6", 30, grantCode)
	if err != nil {
		t.Fatal(err)
	}
	grant := f.grants(got.Account.ID)[0]
	if _, err := f.svc.DeleteAccount(ctx, got.Account.ID); err != nil {
		t.Fatal(err)
	}
	if len(f.grants(got.Account.ID)) != 0 {
		t.Fatal("deletion left a live grant")
	}
	queued := f.queued()
	if len(queued) != 1 || queued[0].ID != grant.ID || string(queued[0].Sealed) != string(grant.Sealed) {
		t.Fatalf("deletion must queue the sealed grant under its id: %+v", queued)
	}
	// A capture that lands after deletion (slow exchange racing the
	// deletion) is queued, never kept as a live grant.
	late := domain.ProviderGrant{ID: "late-grant", AccountID: got.Account.ID, Provider: "apple", Subject: "grant-sub-6", Sealed: []byte("sealed|" + domain.GrantAAD("late-grant")), CapturedAt: 5}
	if err := f.store.SaveProviderGrant(ctx, late); err != nil {
		t.Fatal(err)
	}
	if len(f.grants(got.Account.ID)) != 0 || len(f.queued()) != 2 {
		t.Fatalf("late grant must be queued: live=%d queued=%d", len(f.grants(got.Account.ID)), len(f.queued()))
	}
}

type revocationClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *revocationClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *revocationClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func queueOne(t *testing.T, store *MemStore, id string, at int64) {
	t.Helper()
	store.mu.Lock()
	defer store.mu.Unlock()
	store.queueRevocationLocked(domain.ProviderGrant{ID: id, Provider: "apple", Sealed: []byte("sealed|" + domain.GrantAAD(id))}, at)
}

func TestGrantRevocationReconcileRevokesAndDeletes(t *testing.T) {
	store := NewMemStore()
	clock := &revocationClock{now: time.Unix(1_800_000_000, 0)}
	queueOne(t, store, "grant-a", clock.now.Unix())
	queueOne(t, store, "grant-b", clock.now.Unix())
	revoker := &fakeRevoker{}
	r := GrantRevocations{Store: store, Revoker: revoker, NewNonce: counter("lease"), Now: clock.Now}
	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(revoker.aads) != 2 || len(store.revocations) != 0 {
		t.Fatalf("revoked %v, remaining %d", revoker.aads, len(store.revocations))
	}
	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatalf("empty queue must be a no-op: %v", err)
	}
}

func TestGrantRevocationReconcileBacksOffThenAbandonsAtWindow(t *testing.T) {
	store := NewMemStore()
	clock := &revocationClock{now: time.Unix(1_800_000_000, 0)}
	queueOne(t, store, "grant-c", clock.now.Unix())
	revoker := &fakeRevoker{err: errors.New("apple: identity service unavailable")}
	r := GrantRevocations{Store: store, Revoker: revoker, NewNonce: counter("lease"), Now: clock.Now}
	if err := r.Reconcile(context.Background()); !errors.Is(err, ErrGrantRevocationIncomplete) {
		t.Fatalf("failed attempt must surface: %v", err)
	}
	if store.revocations["grant-c"].rev.Attempts != 1 {
		t.Fatal("attempt not counted")
	}
	// Not due again until the backoff elapses.
	if err := r.Reconcile(context.Background()); err != nil || len(revoker.aads) != 1 {
		t.Fatalf("row retried before its backoff: %v (%d attempts)", err, len(revoker.aads))
	}
	clock.advance(time.Duration(domain.GrantRevocationBackoff(1)) * time.Second)
	revoker.err = nil
	if err := r.Reconcile(context.Background()); err != nil || len(store.revocations) != 0 {
		t.Fatalf("recovered provider must revoke: %v", err)
	}

	queueOne(t, store, "grant-d", clock.now.Unix())
	revoker.err = errors.New("apple: token revocation refused")
	clock.advance(time.Duration(domain.GrantRevocationWindowSeconds+1) * time.Second)
	err := r.Reconcile(context.Background())
	if !errors.Is(err, ErrGrantRevocationIncomplete) || !strings.Contains(err.Error(), "1 abandoned") {
		t.Fatalf("past the window the grant must be abandoned: %v", err)
	}
	if len(store.revocations) != 0 {
		t.Fatal("abandoned grant kept its sealed token")
	}
}

func TestGrantRevocationWithoutRevokerRetriesThenDrains(t *testing.T) {
	store := NewMemStore()
	clock := &revocationClock{now: time.Unix(1_800_000_000, 0)}
	queueOne(t, store, "grant-e", clock.now.Unix())
	r := GrantRevocations{Store: store, NewNonce: counter("lease"), Now: clock.Now}
	if err := r.Reconcile(context.Background()); !errors.Is(err, ErrGrantRevocationIncomplete) || len(store.revocations) != 1 {
		t.Fatalf("unconfigured revoker must retry, not drop: %v", err)
	}
	clock.advance(time.Duration(domain.GrantRevocationWindowSeconds+1) * time.Second)
	if err := r.Reconcile(context.Background()); !errors.Is(err, ErrGrantRevocationIncomplete) || len(store.revocations) != 0 {
		t.Fatalf("window must drain the sealed token: %v", err)
	}
	if err := (GrantRevocations{}).Reconcile(context.Background()); err == nil {
		t.Fatal("unwired reconcile must refuse")
	}
}
