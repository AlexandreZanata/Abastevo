//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

const grantsMigration = "000301_account_provider_grants.sql"

type pgExchanger struct {
	mu    sync.Mutex
	calls int
}

func (e *pgExchanger) Exchange(_ context.Context, provider, code, subject, aad string) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls++
	if provider != "apple" || code == "" || subject == "" {
		return nil, errors.New("exchanger: unexpected call")
	}
	return []byte("sealed|" + aad), nil
}

type pgRevoker struct {
	mu   sync.Mutex
	aads []string
	err  error
}

func (r *pgRevoker) Revoke(_ context.Context, provider string, sealed []byte, aad string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if provider != "apple" || string(sealed) != "sealed|"+aad {
		return errors.New("revoker: grant does not open under its binding")
	}
	r.aads = append(r.aads, aad)
	return r.err
}

func freshGrantService(t *testing.T) (*linkService, *pgExchanger) {
	t.Helper()
	f := freshNativeService(t)
	ex := &pgExchanger{}
	f.svc.Grants = ex
	f.svc.OnGrantFailure = func(_ context.Context, _ string, err error) { t.Errorf("unexpected capture failure: %v", err) }
	return f, ex
}

func (f *linkService) loginWithCode(t *testing.T, sub, raw, code string) application.AuthResult {
	t.Helper()
	got, err := f.svc.LoginWithProvider(context.Background(), application.ProviderLogin{
		Provider: "apple", IDToken: f.nativeToken(t, sub, "", raw), Nonce: raw, AuthorizationCode: code,
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	return got
}

func TestPGGrantCapturedQueuedOnDeletionAndRevoked(t *testing.T) {
	f, ex := freshGrantService(t)
	ctx := context.Background()
	const code = "c0ffee.0.synthetic-pg-code"
	got := f.loginWithCode(t, "pg-grant-sub-1", "raw-pg-grant-nonce-01", code)
	var grantID string
	var sealed []byte
	if err := f.pool.QueryRow(ctx, `SELECT id::text, sealed_token FROM account_provider_grants WHERE account_id=$1 AND provider='apple' AND subject='pg-grant-sub-1'`, got.Account.ID).Scan(&grantID, &sealed); err != nil {
		t.Fatalf("grant row: %v", err)
	}
	if string(sealed) != "sealed|"+domain.GrantAAD(grantID) || ex.calls != 1 {
		t.Fatalf("grant must be the adapter's sealed bytes bound to its id")
	}
	// Re-login replaces the grant; still exactly one live row.
	f.loginWithCode(t, "pg-grant-sub-1", "raw-pg-grant-nonce-02", code)
	if n := f.count(t, `SELECT count(*) FROM account_provider_grants WHERE account_id=$1`, got.Account.ID); n != 1 {
		t.Fatalf("want one live grant after re-login, got %d", n)
	}
	if n := f.count(t, `SELECT count(*) FROM account_provider_grants g WHERE to_jsonb(g)::text LIKE '%' || $1 || '%'`, code); n != 0 {
		t.Fatal("authorization code persisted")
	}
	if err := f.pool.QueryRow(ctx, `SELECT id::text FROM account_provider_grants WHERE account_id=$1`, got.Account.ID).Scan(&grantID); err != nil {
		t.Fatal(err)
	}

	if _, err := f.svc.DeleteAccount(ctx, got.Account.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n := f.count(t, `SELECT count(*) FROM account_provider_grants WHERE account_id=$1`, got.Account.ID); n != 0 {
		t.Fatal("deletion left a live grant")
	}
	if n := f.count(t, `SELECT count(*) FROM account_provider_revocations WHERE id=$1 AND provider='apple' AND attempts=0 AND lease_nonce IS NULL`, grantID); n != 1 {
		t.Fatal("deletion must queue the grant under its id")
	}
	// The queue carries no identity: its columns are fixed.
	if n := f.count(t, `SELECT count(*) FROM information_schema.columns WHERE table_name='account_provider_revocations' AND column_name IN ('account_id','subject','email')`); n != 0 {
		t.Fatal("revocation queue stores identity columns")
	}

	revoker := &pgRevoker{}
	clock := &revocationClock{now: time.Unix(1_800_000_000, 0)}
	r := application.GrantRevocations{Store: f.svc.Store.(*PGStore), Revoker: revoker, NewNonce: newLeaseNonce(), Now: clock.Now}
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(revoker.aads) != 1 || revoker.aads[0] != domain.GrantAAD(grantID) {
		t.Fatalf("revoked %v", revoker.aads)
	}
	if n := f.count(t, `SELECT count(*) FROM account_provider_revocations`); n != 0 {
		t.Fatal("revoked grant kept its sealed token")
	}
}

// revocationClock is a settable wall clock for the reconcile.
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

func newLeaseNonce() func() (string, error) {
	var mu sync.Mutex
	n := 0
	return func() (string, error) {
		mu.Lock()
		defer mu.Unlock()
		n++
		return fmt.Sprintf("bbbbbbbb-2222-4222-8222-%012d", n), nil
	}
}

func TestPGGrantSaveSerializesWithDeletion(t *testing.T) {
	f, _ := freshGrantService(t)
	ctx := context.Background()
	store := f.svc.Store.(*PGStore)
	for i := 0; i < 8; i++ {
		acc := f.loginWithCode(t, fmt.Sprintf("pg-grant-race-%d", i), fmt.Sprintf("raw-pg-grant-race-%04d", i), "")
		grantID := testUUID(7000 + i)
		grant := domain.ProviderGrant{
			ID: grantID, AccountID: acc.Account.ID, Provider: "apple", Subject: fmt.Sprintf("pg-grant-race-%d", i),
			Sealed: []byte("sealed|" + domain.GrantAAD(grantID)), CapturedAt: 1_800_000_000,
		}
		var wg sync.WaitGroup
		start := make(chan struct{})
		var saveErr, deleteErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			saveErr = store.SaveProviderGrant(ctx, grant)
		}()
		go func() {
			defer wg.Done()
			<-start
			_, deleteErr = f.svc.DeleteAccount(ctx, acc.Account.ID)
		}()
		close(start)
		wg.Wait()
		if saveErr != nil || deleteErr != nil {
			t.Fatalf("round %d: save=%v delete=%v", i, saveErr, deleteErr)
		}
		// Whatever the interleaving, the deleted account keeps no live
		// grant and the sealed token is queued exactly once.
		if n := f.count(t, `SELECT count(*) FROM account_provider_grants WHERE account_id=$1`, acc.Account.ID); n != 0 {
			t.Fatalf("round %d: live grant survived deletion", i)
		}
		if n := f.count(t, `SELECT count(*) FROM account_provider_revocations WHERE id=$1`, grantID); n != 1 {
			t.Fatalf("round %d: grant queued %d times", i, n)
		}
	}
}

func TestPGGrantRevocationLeaseBackoffAndWindow(t *testing.T) {
	f, _ := freshGrantService(t)
	ctx := context.Background()
	store := f.svc.Store.(*PGStore)
	const at = int64(1_800_000_000)
	grantID := testUUID(8001)
	if _, err := f.pool.Exec(ctx, `INSERT INTO account_provider_revocations (id, provider, sealed_token, requested_at, next_attempt_at) VALUES ($1,'apple',$2,to_timestamp($3),to_timestamp($3))`,
		grantID, []byte("sealed|"+domain.GrantAAD(grantID)), at); err != nil {
		t.Fatal(err)
	}
	// Two concurrent claims: exactly one lease.
	var wg sync.WaitGroup
	found := make([]bool, 2)
	claims := make([]domain.GrantRevocation, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var err error
			claims[i], found[i], err = store.ClaimGrantRevocation(ctx, testUUID(8100+i), at)
			if err != nil {
				t.Errorf("claim %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	if found[0] == found[1] {
		t.Fatalf("want exactly one lease, got %v", found)
	}
	winner := claims[0]
	if found[1] {
		winner = claims[1]
	}
	// A stale lease settles nothing.
	stale := winner
	stale.LeaseNonce = testUUID(8199)
	if err := store.FinishGrantRevocation(ctx, stale, application.GrantRevoked, at); err != nil {
		t.Fatal(err)
	}
	if n := f.count(t, `SELECT count(*) FROM account_provider_revocations WHERE id=$1`, grantID); n != 1 {
		t.Fatal("stale lease deleted the row")
	}
	if err := store.FinishGrantRevocation(ctx, winner, application.GrantRetry, at); err != nil {
		t.Fatal(err)
	}
	if n := f.count(t, `SELECT count(*) FROM account_provider_revocations WHERE id=$1 AND attempts=1 AND lease_nonce IS NULL AND next_attempt_at=to_timestamp($2)`, grantID, at+domain.GrantRevocationBackoff(1)); n != 1 {
		t.Fatal("retry must back off and release the lease")
	}
	if _, ok, err := store.ClaimGrantRevocation(ctx, testUUID(8200), at+1); err != nil || ok {
		t.Fatalf("row claimable before its backoff: %v %v", ok, err)
	}
	// Past the window a failing provider abandons the row.
	revoker := &pgRevoker{err: errors.New("apple: token revocation refused")}
	clock := &revocationClock{now: time.Unix(at+domain.GrantRevocationWindowSeconds+1, 0)}
	r := application.GrantRevocations{Store: store, Revoker: revoker, NewNonce: newLeaseNonce(), Now: clock.Now}
	if err := r.Reconcile(ctx); !errors.Is(err, application.ErrGrantRevocationIncomplete) {
		t.Fatalf("abandonment must surface: %v", err)
	}
	if n := f.count(t, `SELECT count(*) FROM account_provider_revocations`); n != 0 {
		t.Fatal("abandoned grant kept its sealed token")
	}
}

func TestPGGrantConstraintsRefuseForeignRows(t *testing.T) {
	f, _ := freshGrantService(t)
	ctx := context.Background()
	acc := f.loginWithCode(t, "pg-grant-constraints", "raw-pg-grant-constraint-1", "")
	bad := []struct {
		name string
		sql  string
		args []any
	}{
		{"google grant", `INSERT INTO account_provider_grants (id, account_id, provider, subject, sealed_token, captured_at) VALUES ($1,$2,'google','s',$3,now())`, []any{testUUID(9001), acc.Account.ID, make([]byte, 40)}},
		{"short sealed grant", `INSERT INTO account_provider_grants (id, account_id, provider, subject, sealed_token, captured_at) VALUES ($1,$2,'apple','s',$3,now())`, []any{testUUID(9002), acc.Account.ID, make([]byte, 29)}},
		{"empty subject", `INSERT INTO account_provider_grants (id, account_id, provider, subject, sealed_token, captured_at) VALUES ($1,$2,'apple','',$3,now())`, []any{testUUID(9003), acc.Account.ID, make([]byte, 40)}},
		{"unknown account", `INSERT INTO account_provider_grants (id, account_id, provider, subject, sealed_token, captured_at) VALUES ($1,$2,'apple','s',$3,now())`, []any{testUUID(9004), testUUID(9999), make([]byte, 40)}},
		{"half lease", `INSERT INTO account_provider_revocations (id, provider, sealed_token, requested_at, next_attempt_at, lease_nonce) VALUES ($1,'apple',$2,now(),now(),$3)`, []any{testUUID(9005), make([]byte, 40), testUUID(9006)}},
		{"oversized sealed revocation", `INSERT INTO account_provider_revocations (id, provider, sealed_token, requested_at, next_attempt_at) VALUES ($1,'apple',$2,now(),now())`, []any{testUUID(9007), make([]byte, 4097)}},
	}
	for _, tc := range bad {
		if _, err := f.pool.Exec(ctx, tc.sql, tc.args...); err == nil {
			t.Fatalf("%s accepted", tc.name)
		}
	}
	if err := f.svc.Store.SaveProviderGrant(ctx, domain.ProviderGrant{ID: testUUID(9010), AccountID: testUUID(9999), Provider: "apple", Subject: "s", Sealed: make([]byte, 40)}); !errors.Is(err, domain.ErrAccountNotFound) {
		t.Fatalf("grant for unknown account: %v", err)
	}
}

// TestGrantsMigrationAppliesOnlyOverExistingAccounts is the 000301
// recovery proof: over a populated pre-upgrade schema it applies alone,
// preserves accounts and links, leaves both new tables empty and
// writable, keeps deletion working for pre-existing accounts, and
// replays as a no-op.
func TestGrantsMigrationAppliesOnlyOverExistingAccounts(t *testing.T) {
	adminDSN := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if adminDSN == "" {
		adminDSN = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	previous, target := fstest.MapFS{}, fstest.MapFS{}
	files, err := fs.Glob(dbmigrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if name > grantsMigration {
			continue
		}
		data, err := fs.ReadFile(dbmigrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		target[name] = &fstest.MapFile{Data: data}
		if name < grantsMigration {
			previous[name] = &fstest.MapFile{Data: data}
		}
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("integration database unreachable: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("grants_up_%d", time.Now().UnixNano())
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
		if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Errorf("drop database: %v", err)
		}
	})
	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	dsn := u.String()
	if _, err := migrate.Apply(ctx, dsn, previous); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	const accountID = "dddddddd-4444-4444-8444-000000000001"
	if _, err := conn.Exec(ctx, `INSERT INTO accounts (id, alias, status) VALUES ($1,'grants-up-1','active')`, accountID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO account_provider_links (account_id, provider, issuer, subject) VALUES ($1,'apple','https://appleid.apple.com','grants-up-sub')`, accountID); err != nil {
		t.Fatal(err)
	}
	applied, err := migrate.Apply(ctx, dsn, target)
	if err != nil || len(applied) != 1 || applied[0] != "000301" {
		t.Fatalf("upgrade applied %v: %v", applied, err)
	}
	var kept, grants, queued int
	if err := conn.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM account_provider_links WHERE account_id=$1 AND subject='grants-up-sub'),
 (SELECT count(*) FROM account_provider_grants),
 (SELECT count(*) FROM account_provider_revocations)`, accountID).Scan(&kept, &grants, &queued); err != nil || kept != 1 || grants != 0 || queued != 0 {
		t.Fatalf("upgrade state links=%d grants=%d queued=%d err=%v", kept, grants, queued, err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store := NewPGStore(pool)
	grantID := "dddddddd-4444-4444-8444-000000000002"
	if err := store.SaveProviderGrant(ctx, domain.ProviderGrant{ID: grantID, AccountID: accountID, Provider: "apple", Subject: "grants-up-sub", Sealed: []byte("sealed|" + domain.GrantAAD(grantID)), CapturedAt: 1_800_000_000}); err != nil {
		t.Fatalf("grant write after upgrade: %v", err)
	}
	if _, err := store.DeleteAccount(ctx, accountID, 1_800_000_100); err != nil {
		t.Fatalf("deletion after upgrade: %v", err)
	}
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM account_provider_revocations WHERE id=$1`, grantID).Scan(&queued); err != nil || queued != 1 {
		t.Fatalf("pre-existing account deletion must queue its grant: %d %v", queued, err)
	}
	again, err := migrate.Apply(ctx, dsn, target)
	if err != nil || len(again) != 0 {
		t.Fatalf("replay applied %v: %v", again, err)
	}
}
