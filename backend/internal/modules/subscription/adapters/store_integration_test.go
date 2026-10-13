//go:build integration

package adapters

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	subjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/adapters/jobs"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"
)

func freshDB(t *testing.T) *pgxpool.Pool { return freshFS(t, migrations.Files) }
func freshFS(t *testing.T, files fs.FS) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("explicit disposable PostGIS DSN required")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Hostname() != "127.0.0.1" {
		t.Fatal("literal loopback test DSN required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("disposable database unavailable")
	}
	name := fmt.Sprintf("premium_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	admin.Close(ctx)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, dsn)
		if err != nil {
			t.Error("cleanup connection failed")
			return
		}
		defer admin.Close(ctx)
		if _, err = admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	u.Path = "/" + name
	if _, err = migrate.Apply(ctx, u.String(), files); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
func TestSubscriptionSchemaExists(t *testing.T) {
	pool := freshDB(t)
	var exists bool
	if err := pool.QueryRow(context.Background(), "SELECT to_regclass('subscription_purchases') IS NOT NULL").Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("owned subscription persistence missing")
	}
}

func newid(t *testing.T) string {
	t.Helper()
	id, err := jobs.NewUUIDv4()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
func seedOwner(t *testing.T, pool *pgxpool.Pool) app.Principal {
	t.Helper()
	p := app.Principal{AccountID: newid(t), FamilyID: newid(t), ContributorID: newid(t), KeyID: newid(t)}
	ctx := context.Background()
	for _, v := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO accounts(id,alias,status) VALUES($1,$2,'active')", []any{p.AccountID, "synthetic-" + p.AccountID}},
		{"INSERT INTO account_session_families(id,account_id,refresh_salt,refresh_hash,access_salt,access_hash,access_expires,issued_at) VALUES($1,$2,'salt','hash','salt','hash',clock_timestamp()+interval '1 hour',clock_timestamp())", []any{p.FamilyID, p.AccountID}},
		{"INSERT INTO identity_contributors(id) VALUES($1)", []any{p.ContributorID}},
		{"INSERT INTO identity_keys(id,contributor_id,algorithm,public_jwk,fingerprint) VALUES($1,$2,'ecdsa-p256-sha512','{}',$3)", []any{p.KeyID, p.ContributorID, "synthetic-" + p.KeyID}},
		{"INSERT INTO account_contributor_bindings(account_id,contributor_id,key_fingerprint) VALUES($1,$2,'synthetic')", []any{p.AccountID, p.ContributorID}},
	} {
		if _, err := pool.Exec(ctx, v.sql, v.args...); err != nil {
			t.Fatal(err)
		}
	}
	return p
}
func attempt(t *testing.T, p app.Principal, token string) app.Attempt {
	t.Helper()
	c, _ := NewTokenCodec(bytes.Repeat([]byte{9}, 32))
	cipher, err := c.Seal(token, p.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte("verify\x00" + token))
	return app.Attempt{Principal: p, OperationID: newid(t), Digest: domain.TokenDigest("test", "com.anpfuel", token), PayloadHash: hex.EncodeToString(hash[:]), Environment: "test", PackageName: "com.anpfuel", CipherToken: cipher, Kind: "verify"}
}
func verified() app.VerifiedPurchase {
	return app.VerifiedPurchase{Snapshot: domain.Snapshot{State: domain.Active, VerifiedAt: time.Now().UTC(), ValidUntil: time.Now().UTC().Add(time.Hour), ProductID: "synthetic.premium", BasePlanID: "synthetic.monthly"}}
}
func TestPurchaseOwnershipReplayAndEncryptedIntent(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	p := seedOwner(t, pool)
	foreign := seedOwner(t, pool)
	ctx := context.Background()
	a := attempt(t, p, "synthetic-token")
	started, err := store.Begin(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Commit(ctx, started, verified()); err != nil {
		t.Fatal(err)
	}
	replay, err := store.Begin(ctx, a)
	if err != nil || !replay.Replay || replay.Result != "accepted" {
		t.Fatal("exact replay failed", err)
	}
	changed := a
	changed.PayloadHash = strings.Repeat("f", 64)
	if _, err = store.Begin(ctx, changed); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("divergent replay admitted", err)
	}
	stolen := attempt(t, foreign, "synthetic-token")
	if _, err = store.Begin(ctx, stolen); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("cross-owner purchase admitted", err)
	}
	v, err := store.Status(ctx, foreign)
	if err != nil || v != nil {
		t.Fatal("foreign read disclosed purchase")
	}
	wrong := p
	wrong.FamilyID = foreign.FamilyID
	if _, err = store.Status(ctx, wrong); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("foreign session admitted")
	}
	var count int
	var leaks bool
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM job_queue WHERE kind='subscription-acknowledge'").Scan(&count); err != nil || count != 1 {
		t.Fatal("ack intent not atomic", err, count)
	}
	if err = pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM subscription_purchases WHERE position(convert_to('synthetic-token','UTF8') in token_cipher)>0)").Scan(&leaks); err != nil || leaks {
		t.Fatal("plaintext purchase")
	}
}
func TestParallelAdmissionAndStaleGeneration(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	p := seedOwner(t, pool)
	ctx := context.Background()
	const n = 12
	out := make([]app.Attempt, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			out[i], errs[i] = store.Begin(ctx, attempt(t, p, "same-synthetic-token"))
		}(i)
	}
	wg.Wait()
	wins := 0
	var winner app.Attempt
	for i, err := range errs {
		if err == nil {
			wins++
			winner = out[i]
		} else if !errors.Is(err, domain.ErrBusy) {
			t.Fatal("unexpected admission error", err)
		}
	}
	if wins != 1 {
		t.Fatalf("want one generation owner, got %d", wins)
	}
	if _, err := pool.Exec(ctx, "UPDATE subscription_verification_slots SET lease_until=clock_timestamp()-interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	newAttempt := attempt(t, p, "same-synthetic-token")
	newAttempt, err := store.Begin(ctx, newAttempt)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Commit(ctx, winner, verified()); !errors.Is(err, domain.ErrStale) {
		t.Fatal("stale generation committed", err)
	}
	if err = store.Commit(ctx, newAttempt, verified()); err != nil {
		t.Fatal(err)
	}
}
func TestQuotaAndRevokedOwnerSessionKey(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	p := seedOwner(t, pool)
	ctx := context.Background()
	a, err := store.Begin(ctx, attempt(t, p, "first-token"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Begin(ctx, attempt(t, p, "second-token")); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Begin(ctx, attempt(t, p, "third-token")); !errors.Is(err, domain.ErrQuota) {
		t.Fatal("pending quota exceeded", err)
	}
	if _, err = pool.Exec(ctx, "UPDATE identity_keys SET revoked_at=clock_timestamp() WHERE id=$1", p.KeyID); err != nil {
		t.Fatal(err)
	}
	if err = store.Commit(ctx, a, verified()); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("revoked key committed", err)
	}
	if _, err = pool.Exec(ctx, "UPDATE identity_keys SET revoked_at=NULL WHERE id=$1", p.KeyID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "UPDATE account_session_families SET revoked_at=clock_timestamp() WHERE id=$1", p.FamilyID); err != nil {
		t.Fatal(err)
	}
	if err = store.Commit(ctx, a, verified()); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("revoked session committed", err)
	}
	if _, err = pool.Exec(ctx, "UPDATE account_session_families SET revoked_at=NULL WHERE id=$1", p.FamilyID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "UPDATE accounts SET status='deleted' WHERE id=$1", p.AccountID); err != nil {
		t.Fatal(err)
	}
	if err = store.Commit(ctx, a, verified()); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("deleted owner committed", err)
	}
	var copies int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM subscription_operations WHERE account_id=$1 AND token_cipher IS NOT NULL", p.AccountID).Scan(&copies); err != nil || copies != 0 {
		t.Fatal("owner erasure retained token", err, copies)
	}
}
func TestAtomicRollbackAndDeletionPurgesPurchasedToken(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	p := seedOwner(t, pool)
	ctx := context.Background()
	a, err := store.Begin(ctx, attempt(t, p, "rollback-token"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `CREATE FUNCTION fail_ack() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'synthetic failure'; END$$; CREATE TRIGGER fail_ack BEFORE INSERT ON job_queue FOR EACH ROW EXECUTE FUNCTION fail_ack()`); err != nil {
		t.Fatal(err)
	}
	if err = store.Commit(ctx, a, verified()); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("fault not propagated", err)
	}
	var count int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM subscription_purchases").Scan(&count); err != nil || count != 0 {
		t.Fatal("benefit escaped rollback")
	}
	if _, err = pool.Exec(ctx, "DROP TRIGGER fail_ack ON job_queue"); err != nil {
		t.Fatal(err)
	}
	if err = store.Commit(ctx, a, verified()); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "UPDATE accounts SET status='deleted' WHERE id=$1", p.AccountID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM subscription_purchases WHERE token_cipher IS NOT NULL OR NOT retired OR state<>'REVOKED'").Scan(&count); err != nil || count != 0 {
		t.Fatal("deleted token/rights retained")
	}
}
func TestSubscriptionUpgradeReplayAndIndexedStatus(t *testing.T) {
	old := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") && e.Name() < "000075" {
			raw, err := fs.ReadFile(migrations.Files, e.Name())
			if err != nil {
				t.Fatal(err)
			}
			old[e.Name()] = &fstest.MapFile{Data: raw}
		}
	}
	pool := freshFS(t, old)
	p := seedOwner(t, pool)
	dsn := pool.Config().ConnString()
	got, err := migrate.Apply(context.Background(), dsn, migrations.Files)
	if err != nil || len(got) != 2 || got[0] != "000075" || got[1] != "000076" {
		t.Fatal("upgrade", got, err)
	}
	if got, err = migrate.Apply(context.Background(), dsn, migrations.Files); err != nil || len(got) != 0 {
		t.Fatal("replay", got, err)
	}
	store := NewStore(pool)
	a, err := store.Begin(context.Background(), attempt(t, p, "own-token"))
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Commit(context.Background(), a, verified()); err != nil {
		t.Fatal(err)
	}
	other := seedOwner(t, pool)

	_, err = pool.Exec(context.Background(), `INSERT INTO subscription_purchases(token_digest,account_id,environment,package_name,product_id,base_plan_id,state,valid_until,verified_at) SELECT md5(i::text)||md5(i::text||'synthetic'),$1,'test','com.anpfuel','synthetic.premium','monthly','ACTIVE',clock_timestamp()+interval '1 hour',clock_timestamp() FROM generate_series(1,10000)i`, other.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(context.Background(), "ANALYZE subscription_purchases"); err != nil {
		t.Fatal(err)
	}

	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../../db/queries/subscription/subscription.sql"))
	if err != nil {
		t.Fatal(err)
	}
	query := strings.Split(strings.Split(string(raw), "-- name: ReadStatus :one\n")[1], ";")[0]
	var plan string
	if err = pool.QueryRow(context.Background(), "EXPLAIN (FORMAT JSON) "+query, p.AccountID).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan, "subscription_purchases_owner_idx") {
		t.Fatal("status plan scans unrelated history")
	}
}

func TestConcurrentDistinctAdmissionsStayBounded(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	p := seedOwner(t, pool)
	attempts := make([]app.Attempt, 12)
	for i := range attempts {
		attempts[i] = attempt(t, p, fmt.Sprintf("synthetic-quota-%d", i))
	}
	errs := make([]error, 12)
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Add(1)
		go func(i int) { defer wg.Done(); _, errs[i] = store.Begin(context.Background(), attempts[i]) }(i)
	}
	wg.Wait()
	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
		} else if !errors.Is(err, domain.ErrQuota) {
			t.Fatal(err)
		}
	}
	if wins != 2 {
		t.Fatalf("atomic pending cap want 2, got %d", wins)
	}
}

type verifyFunc func(context.Context, app.ProviderRequest) (app.ProviderPurchase, error)

func (f verifyFunc) Verify(ctx context.Context, r app.ProviderRequest) (app.ProviderPurchase, error) {
	return f(ctx, r)
}
func TestAdmissionRechecksRevocationAfterProviderHTTP(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	codec, _ := NewTokenCodec(bytes.Repeat([]byte{4}, 32))
	binding, _ := NewAccountBinding(bytes.Repeat([]byte{5}, 32), "test", "com.anpfuel")
	svc := &app.Service{Store: NewStore(pool), Tokens: codec, Environment: "test", PackageName: "com.anpfuel", AccountBinding: binding}
	called := false
	svc.Provider = verifyFunc(func(ctx context.Context, r app.ProviderRequest) (app.ProviderPurchase, error) {
		called = true
		if r.ExpectedAccount != binding(p.AccountID) {
			t.Error("unbound provider lookup")
		}
		// If admission held owner/session SQL locks across HTTP this would timeout.
		if _, err := pool.Exec(ctx, "UPDATE account_session_families SET revoked_at=clock_timestamp() WHERE id=$1", p.FamilyID); err != nil {
			t.Fatal("network work held SQL locks", err)
		}
		return app.ProviderPurchase{Snapshot: verified().Snapshot}, nil
	})
	_, err := svc.Admit(context.Background(), p, app.Command{OperationID: newid(t), Token: "synthetic-admission", Kind: "verify"})
	if !called || !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("late session revocation accepted", err)
	}
	var count int
	if err = pool.QueryRow(context.Background(), "SELECT count(*) FROM subscription_purchases").Scan(&count); err != nil || count != 0 {
		t.Fatal("revoked grant persisted")
	}
}
func TestAdmissionExactReplayAndDeniedProofRecovery(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	codec, _ := NewTokenCodec(bytes.Repeat([]byte{4}, 32))
	binding, _ := NewAccountBinding(bytes.Repeat([]byte{5}, 32), "test", "com.anpfuel")
	count := 0
	svc := &app.Service{Store: NewStore(pool), Tokens: codec, Environment: "test", PackageName: "com.anpfuel", AccountBinding: binding}
	svc.Provider = verifyFunc(func(context.Context, app.ProviderRequest) (app.ProviderPurchase, error) {
		count++
		return app.ProviderPurchase{Snapshot: verified().Snapshot}, nil
	})
	cmd := app.Command{OperationID: newid(t), Token: "synthetic-proof", Kind: "verify"}
	got, err := svc.Admit(context.Background(), p, cmd)
	if err != nil || len(got.Status.Capabilities) != 6 || got.Replayed {
		t.Fatal("verified admission", err)
	}
	got, err = svc.Admit(context.Background(), p, cmd)
	if err != nil || !got.Replayed || count != 1 {
		t.Fatal("provider replay not deduplicated", err)
	}
	svc.Provider = verifyFunc(func(context.Context, app.ProviderRequest) (app.ProviderPurchase, error) {
		return app.ProviderPurchase{}, domain.ErrDenied
	})
	denied := app.Command{OperationID: newid(t), Token: "synthetic-denied", Kind: "verify"}
	if _, err = svc.Admit(context.Background(), p, denied); !errors.Is(err, domain.ErrDenied) {
		t.Fatal(err)
	}
	var clear int
	if err = pool.QueryRow(context.Background(), "SELECT count(*) FROM subscription_operations WHERE status='denied' AND token_cipher IS NOT NULL").Scan(&clear); err != nil || clear != 0 {
		t.Fatal("denied proof retained secret")
	}
}

type ackProvider struct {
	acked bool
	calls int
}

func (p *ackProvider) Verify(context.Context, app.ProviderRequest) (app.ProviderPurchase, error) {
	v := verified().Snapshot
	v.Acknowledged = p.acked
	return app.ProviderPurchase{Snapshot: v}, nil
}
func (p *ackProvider) Acknowledge(context.Context, app.ProviderRequest, string) error {
	p.acked = true
	p.calls++
	return domain.ErrUnavailable
}
func TestAcknowledgementLostResponseRecoversDurableJob(t *testing.T) {
	pool := freshDB(t)
	owner := seedOwner(t, pool)
	codec, _ := NewTokenCodec(bytes.Repeat([]byte{4}, 32))
	binding, _ := NewAccountBinding(bytes.Repeat([]byte{5}, 32), "test", "com.anpfuel")
	provider := &ackProvider{}
	svc := &app.Service{Store: NewStore(pool), Tokens: codec, Provider: provider, Environment: "test", PackageName: "com.anpfuel", AccountBinding: binding}
	_, err := svc.Admit(context.Background(), owner, app.Command{OperationID: newid(t), Token: "synthetic-ack", Kind: "verify"})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher := jobs.Dispatcher{Queue: jobs.NewQueue(pool), WorkerID: "premium-test", Handlers: map[string]jobs.Handler{"subscription-acknowledge": subjobs.Handler{Name: "subscription-acknowledge", Run: svc.Acknowledge}}}
	if found, err := dispatcher.RunOnce(context.Background()); err != nil || !found {
		t.Fatal("first lost reply", err)
	}
	if _, err = pool.Exec(context.Background(), "UPDATE job_queue SET not_before=clock_timestamp() WHERE kind='subscription-acknowledge'"); err != nil {
		t.Fatal(err)
	}
	if found, err := dispatcher.RunOnce(context.Background()); err != nil || !found {
		t.Fatal("restart recovery", err)
	}
	var acknowledged bool
	var status string
	if err = pool.QueryRow(context.Background(), "SELECT acknowledged FROM subscription_purchases").Scan(&acknowledged); err != nil || !acknowledged || provider.calls != 1 {
		t.Fatal("ack replay duplicated or lost")
	}
	if err = pool.QueryRow(context.Background(), "SELECT status FROM job_queue WHERE kind='subscription-acknowledge'").Scan(&status); err != nil || status != "done" {
		t.Fatal("durable acknowledgement incomplete")
	}
}
func TestProviderReceiptDeduplicationAndRefreshFence(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	p := seedOwner(t, pool)
	ctx := context.Background()
	a := attempt(t, p, "notification-token")
	a, err := store.Begin(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	v := verified()
	v.Snapshot.Acknowledged = true
	if err = store.Commit(ctx, a, v); err != nil {
		t.Fatal(err)
	}
	n := app.Notification{Environment: "test", PackageName: "com.anpfuel", MessageID: "synthetic-message", Digest: a.Digest, PayloadHash: strings.Repeat("a", 64)}
	var wg sync.WaitGroup
	errs := make([]error, 12)
	for i := range 12 {
		wg.Add(1)
		go func(i int) { defer wg.Done(); _, errs[i] = store.ReceiveNotification(ctx, n) }(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM subscription_provider_notifications").Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate receipt")
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM job_queue WHERE kind='subscription-refresh'").Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate refresh job")
	}
	snapshot, err := store.Status(ctx, p)
	if err != nil || len(domain.Evaluate(snapshot, time.Now(), true).Capabilities) != 0 {
		t.Fatal("notification did not fence stale rights")
	}
	n.PayloadHash = strings.Repeat("b", 64)
	if _, err = store.ReceiveNotification(ctx, n); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("changed-payload message replay accepted", err)
	}
}

func TestPendingReplacementKeepsOldPaidAccess(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	owner := seedOwner(t, pool)
	ctx := context.Background()
	old, err := store.Begin(ctx, attempt(t, owner, "old-paid-token"))
	if err != nil {
		t.Fatal(err)
	}
	v := verified()
	v.Snapshot.Acknowledged = true
	if err = store.Commit(ctx, old, v); err != nil {
		t.Fatal(err)
	}
	next, err := store.Begin(ctx, attempt(t, owner, "pending-new-token"))
	if err != nil {
		t.Fatal(err)
	}
	pending := verified()
	pending.Snapshot.State = domain.Pending
	pending.Snapshot.ValidUntil = time.Time{}
	pending.LinkedDigest = old.Digest
	if err = store.Commit(ctx, next, pending); err != nil {
		t.Fatal(err)
	}
	status, err := store.Status(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(domain.Evaluate(status, time.Now(), true).Capabilities) != 6 {
		t.Fatal("pending replacement prematurely retired paid predecessor")
	}
}

func TestReplacementReconciliationAndCanonicalRevocation(t *testing.T) {
	pool := freshDB(t)
	owner := seedOwner(t, pool)
	store := NewStore(pool)
	codec, _ := NewTokenCodec(bytes.Repeat([]byte{4}, 32))
	binding, _ := NewAccountBinding(bytes.Repeat([]byte{5}, 32), "test", "com.anpfuel")
	svc := &app.Service{Store: store, Tokens: codec, Environment: "test", PackageName: "com.anpfuel", AccountBinding: binding}
	pending := true
	gone := false
	svc.Provider = verifyFunc(func(_ context.Context, r app.ProviderRequest) (app.ProviderPurchase, error) {
		if gone {
			return app.ProviderPurchase{}, domain.ErrNotFound
		}
		v := verified().Snapshot
		v.Acknowledged = true
		linked := ""
		if r.Token == "new-chain-token" {
			linked = "old-chain-token"
			if pending {
				v.State = domain.Pending
				v.ValidUntil = time.Time{}
			}
		}
		return app.ProviderPurchase{Snapshot: v, LinkedToken: linked}, nil
	})
	ctx := context.Background()
	if _, err := svc.Admit(ctx, owner, app.Command{OperationID: newid(t), Token: "old-chain-token", Kind: "verify"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Admit(ctx, owner, app.Command{OperationID: newid(t), Token: "new-chain-token", Kind: "verify"}); err != nil {
		t.Fatal(err)
	}
	before, err := svc.Status(ctx, owner)
	if err != nil || len(before.Capabilities) != 6 {
		t.Fatal("pending replacement removed rights", err)
	}
	pending = false
	digest := domain.TokenDigest("test", "com.anpfuel", "new-chain-token")
	notice := app.Notification{Environment: "test", PackageName: "com.anpfuel", MessageID: "chain-active", Digest: digest, PayloadHash: strings.Repeat("c", 64)}
	if _, err = store.ReceiveNotification(ctx, notice); err != nil {
		t.Fatal(err)
	}
	dispatcher := jobs.Dispatcher{Queue: jobs.NewQueue(pool), WorkerID: "refresh-test", Handlers: map[string]jobs.Handler{"subscription-refresh": subjobs.Handler{Name: "subscription-refresh", RunWithID: svc.Refresh}}}
	if found, err := dispatcher.RunOnce(ctx); err != nil || !found {
		t.Fatal("canonical refresh did not execute", err)
	}
	if _, err = svc.Admit(ctx, owner, app.Command{OperationID: newid(t), Token: "old-chain-token", Kind: "restore"}); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("retired predecessor restored", err)
	}
	late, err := store.Begin(ctx, attempt(t, owner, "new-chain-token"))
	if err != nil {
		t.Fatal(err)
	}
	gone = true
	notice.MessageID = "chain-revoked"
	notice.PayloadHash = strings.Repeat("d", 64)
	if _, err = store.ReceiveNotification(ctx, notice); err != nil {
		t.Fatal(err)
	}
	if err = svc.Refresh(ctx, digest, newid(t)); err != nil {
		t.Fatal(err)
	}
	if err = store.Commit(ctx, late, verified()); !errors.Is(err, domain.ErrStale) {
		t.Fatal("late renewal superseded revocation", err)
	}
	gone = false
	if err = svc.Refresh(ctx, digest, newid(t)); err != nil {
		t.Fatal(err)
	}
	after, err := svc.Status(ctx, owner)
	if err != nil || after.State != domain.Revoked || len(after.Capabilities) != 0 || !after.RetainedDataAccess {
		t.Fatal("revoked chain reactivated", err)
	}
	var copies int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM subscription_purchases WHERE token_digest=$1 AND token_cipher IS NOT NULL", digest).Scan(&copies); err != nil || copies != 0 {
		t.Fatal("revoked token bytes retained")
	}
}
func TestProviderRefreshOutageCannotRefreshRights(t *testing.T) {
	pool := freshDB(t)
	owner := seedOwner(t, pool)
	store := NewStore(pool)
	codec, _ := NewTokenCodec(bytes.Repeat([]byte{4}, 32))
	binding, _ := NewAccountBinding(bytes.Repeat([]byte{5}, 32), "test", "com.anpfuel")
	svc := &app.Service{Store: store, Tokens: codec, Environment: "test", PackageName: "com.anpfuel", AccountBinding: binding, Provider: verifyFunc(func(context.Context, app.ProviderRequest) (app.ProviderPurchase, error) {
		v := verified().Snapshot
		v.Acknowledged = true
		return app.ProviderPurchase{Snapshot: v}, nil
	})}
	ctx := context.Background()
	if _, err := svc.Admit(ctx, owner, app.Command{OperationID: newid(t), Token: "outage-token", Kind: "verify"}); err != nil {
		t.Fatal(err)
	}
	digest := domain.TokenDigest("test", "com.anpfuel", "outage-token")
	if _, err := store.ReceiveNotification(ctx, app.Notification{Environment: "test", PackageName: "com.anpfuel", MessageID: "outage", Digest: digest, PayloadHash: strings.Repeat("e", 64)}); err != nil {
		t.Fatal(err)
	}
	svc.Provider = verifyFunc(func(context.Context, app.ProviderRequest) (app.ProviderPurchase, error) {
		return app.ProviderPurchase{}, domain.ErrUnavailable
	})
	if err := svc.Refresh(ctx, digest, newid(t)); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("provider outage hidden")
	}
	got, err := svc.Status(ctx, owner)
	if err != nil || len(got.Capabilities) != 0 {
		t.Fatal("outage granted fresh rights")
	}
}

func TestFreeExpiredExportIsOwnedPagedAndDeletionDoesNotNeedProvider(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	ctx := context.Background()
	p := seedOwner(t, pool)
	foreign := seedOwner(t, pool)
	for i := 0; i < 3; i++ {
		a, err := store.Begin(ctx, attempt(t, p, fmt.Sprintf("synthetic-export-%d", i)))
		if err != nil {
			t.Fatal(err)
		}
		v := verified()
		v.Snapshot.State = domain.Expired
		v.Snapshot.ValidUntil = time.Now().Add(-time.Hour)
		if err = store.Commit(ctx, a, v); err != nil {
			t.Fatal(err)
		}
	}
	svc := &app.Service{Store: store} // No provider, token key or paid capability needed.
	rows, err := svc.Export(ctx, p, 2, time.Time{}, "")
	if err != nil || len(rows) != 2 {
		t.Fatal("free expired export refused", err)
	}
	second, err := svc.Export(ctx, p, 2, rows[1].VerifiedAt, rows[1].Digest)
	if err != nil || len(second) != 1 || second[0].Digest == rows[1].Digest || second[0].Digest == rows[0].Digest {
		t.Fatal("keyset pagination lost/repeated row", err)
	}
	raw, err := json.Marshal(rows)
	if err != nil || strings.Contains(string(raw), "synthetic-export-") || strings.Contains(string(raw), rows[0].Digest) || strings.Contains(string(raw), "cipher") {
		t.Fatal("token material leaked", err)
	}
	own, err := svc.Export(ctx, foreign, 100, time.Time{}, "")
	if err != nil || len(own) != 0 {
		t.Fatal("foreign export leak", err)
	}
	wrong := p
	wrong.FamilyID = foreign.FamilyID
	if _, err = svc.Export(ctx, wrong, 2, time.Time{}, ""); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("foreign session accepted", err)
	}
	status, err := svc.Status(ctx, p)
	if err != nil || status.PaymentsConfigured || len(status.Capabilities) != 0 || !status.RetainedDataAccess {
		t.Fatal("disabled/free exit state", err)
	}
	if _, err = pool.Exec(ctx, "UPDATE accounts SET status='deleted' WHERE id=$1", p.AccountID); err != nil {
		t.Fatal(err)
	}
	var bytesRemain bool
	if err = pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM subscription_purchases WHERE account_id=$1 AND token_cipher IS NOT NULL)", p.AccountID).Scan(&bytesRemain); err != nil || bytesRemain {
		t.Fatal("deletion required provider", err)
	}
	if _, err = svc.Export(ctx, p, 2, time.Time{}, ""); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("deleted owner export accepted", err)
	}
}

func TestRestorePurchaseSameOwnerNeverTransfersOrRecoversSession(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	foreign := seedOwner(t, pool)
	codec, _ := NewTokenCodec(bytes.Repeat([]byte{4}, 32))
	binding, _ := NewAccountBinding(bytes.Repeat([]byte{5}, 32), "test", "com.anpfuel")
	lookups := 0
	svc := &app.Service{Store: NewStore(pool), Tokens: codec, Environment: "test", PackageName: "com.anpfuel", AccountBinding: binding}
	svc.Provider = verifyFunc(func(_ context.Context, r app.ProviderRequest) (app.ProviderPurchase, error) {
		lookups++
		if r.ExpectedAccount != binding(p.AccountID) {
			t.Fatal("restore not bound to established owner")
		}
		return app.ProviderPurchase{Snapshot: verified().Snapshot}, nil
	})
	ctx := context.Background()
	verify := app.Command{OperationID: newid(t), Token: "synthetic-restore-purchase", Kind: "verify"}
	if _, err := svc.Admit(ctx, p, verify); err != nil {
		t.Fatal(err)
	}
	restore := verify
	restore.OperationID = newid(t)
	restore.Kind = "restore"
	got, err := svc.Admit(ctx, p, restore)
	if err != nil || got.Replayed || len(got.Status.Capabilities) != 6 || lookups != 2 {
		t.Fatal("same owner restore failed", err)
	}
	got, err = svc.Admit(ctx, p, restore)
	if err != nil || !got.Replayed || lookups != 2 {
		t.Fatal("restore replay called provider", err)
	}
	changed := restore
	changed.Kind = "verify"
	if _, err = svc.Admit(ctx, p, changed); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("operation kind changed", err)
	}
	stolen := restore
	stolen.OperationID = newid(t)
	if _, err = svc.Admit(ctx, foreign, stolen); !errors.Is(err, domain.ErrDenied) || lookups != 2 {
		t.Fatal("purchase transferred across accounts", err)
	}
	if _, err = pool.Exec(ctx, "UPDATE account_session_families SET revoked_at=clock_timestamp() WHERE id=$1", p.FamilyID); err != nil {
		t.Fatal(err)
	}
	restore.OperationID = newid(t)
	if _, err = svc.Admit(ctx, p, restore); !errors.Is(err, domain.ErrUnauthorized) || lookups != 2 {
		t.Fatal("restore recovered revoked session", err)
	}
}
