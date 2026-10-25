//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func TestAccountErasureRecoversWithoutSessionOnSQL(t *testing.T) {
	svc, clock, _ := freshService(t)
	ctx := context.Background()
	store := svc.Store.(*PGStore)
	svc.RequestErasure = func(context.Context, string, []string) error { return errors.New("queue unavailable") }
	svc.RequestBillingExit = func(context.Context, string) error { return nil }
	acc, err := svc.CreateKeyAccount(ctx, "recoveryowner")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := store.BindContributor(ctx, domain.ContributorBinding{AccountID: acc.Account.ID, ContributorID: fmt.Sprintf("contributor-%d", i), KeyFingerprint: fmt.Sprintf("fp-%d", i), BoundAt: clock.NowUnix()}, testUUID(800+i)); err != nil {
			t.Fatal(err)
		}
	}
	receipt, err := svc.DeleteAccount(ctx, acc.Account.ID)
	if err != nil || !receipt.ErasureRequested {
		t.Fatal(receipt, err)
	}
	if stored, found, err := store.GetAccount(ctx, acc.Account.ID); err != nil || !found || stored.Status != domain.StatusDeleted {
		t.Fatal("account revocation lost", stored, found, err)
	}

	// Process restart: no caller session or in-memory state is used.
	restarted := NewPGStore(store.pool)
	var claims atomic.Int32
	var wg sync.WaitGroup
	var winner application.ErasureClaim
	var mu sync.Mutex
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, ok, err := restarted.ClaimErasure(ctx, testUUID(900+i), clock.NowUnix())
			if err != nil {
				t.Error(err)
			}
			if ok {
				claims.Add(1)
				mu.Lock()
				winner = c
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if claims.Load() != 1 {
		t.Fatalf("concurrent claims=%d", claims.Load())
	}
	// Failure releases a delayed intent, not a false completion.
	if err := restarted.FinishErasure(ctx, winner, false, clock.NowUnix()); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := restarted.ClaimErasure(ctx, testUUID(950), clock.NowUnix()); err != nil || ok {
		t.Fatal("retry delay", ok, err)
	}
	clock.advance(time.Minute)
	next, ok, err := restarted.ClaimErasure(ctx, testUUID(951), clock.NowUnix())
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := restarted.FinishErasure(ctx, winner, true, clock.NowUnix()); err == nil {
		t.Fatal("stale nonce acknowledged")
	}
	if err := restarted.FinishErasure(ctx, next, true, clock.NowUnix()); err != nil {
		t.Fatal(err)
	}
	next, ok, err = restarted.ClaimErasure(ctx, testUUID(952), clock.NowUnix())
	if err != nil || !ok || next.ContributorID == winner.ContributorID {
		t.Fatal("remaining scope lost", next, ok, err)
	}
	// A crash leaves the lease reclaimable; expired acknowledgement refuses.
	clock.advance(121 * time.Second)
	replacement, ok, err := restarted.ClaimErasure(ctx, testUUID(953), clock.NowUnix())
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := restarted.FinishErasure(ctx, next, true, clock.NowUnix()); err == nil {
		t.Fatal("expired lease acknowledged")
	}
	if err := restarted.FinishErasure(ctx, replacement, true, clock.NowUnix()); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := restarted.ClaimErasure(ctx, testUUID(954), clock.NowUnix()); err != nil || ok {
		t.Fatal("completed account replayed", ok, err)
	}
	var done bool
	if err := store.pool.QueryRow(ctx, "SELECT completed_at IS NOT NULL FROM account_erasure_requests WHERE account_id=$1", acc.Account.ID).Scan(&done); err != nil || !done {
		t.Fatal(done, err)
	}
}

func TestAccountErasureBindingFreeRecoveryOnSQL(t *testing.T) {
	svc, clock, _ := freshService(t)
	ctx := context.Background()
	store := svc.Store.(*PGStore)
	svc.RequestErasure = func(context.Context, string, []string) error { return errors.New("dispatch down") }
	svc.RequestBillingExit = func(context.Context, string) error { return nil }
	acc, err := svc.CreateKeyAccount(ctx, "bindingfreeowner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeleteAccount(ctx, acc.Account.ID); err != nil {
		t.Fatal(err)
	}
	calls := 0
	fail := true
	n := 1000
	recovery := application.ErasureRecovery{Store: NewPGStore(store.pool), Now: func() time.Time { return time.Unix(clock.NowUnix(), 0) }, NewNonce: func() (string, error) { n++; return testUUID(n), nil }, EraseFeedback: func(context.Context, string) error {
		calls++
		if fail {
			return errors.New("feedback down")
		}
		return nil
	}, EraseContributor: func(context.Context, string, string) error { t.Fatal("no binding expected"); return nil }}
	if err := recovery.Reconcile(ctx); err == nil {
		t.Fatal("scope failure hidden")
	}
	fail = false
	clock.advance(time.Minute)
	if err := recovery.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if err := recovery.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("binding-free intent lost or replayed", calls)
	}
}

func TestAccountErasureUpgradeBackfillsBindingFreeOnSQL(t *testing.T) {
	before, target := fstest.MapFS{}, fstest.MapFS{}
	files, err := fs.Glob(dbmigrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if name > "000063_account_erasure_recovery.sql" {
			continue
		}
		raw, err := fs.ReadFile(dbmigrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		target[name] = &fstest.MapFile{Data: raw}
		if name < "000063_account_erasure_recovery.sql" {
			before[name] = &fstest.MapFile{Data: raw}
		}
	}
	svc, _, _ := freshServiceSchema(t, before)
	store := svc.Store.(*PGStore)
	ctx := context.Background()
	id := testUUID(1800)
	if _, err := store.pool.Exec(ctx, "INSERT INTO accounts(id,alias,status,created_at) VALUES($1,'erased-migration','deleted',now())", id); err != nil {
		t.Fatal(err)
	}
	applied, err := migrate.Apply(ctx, store.pool.Config().ConnString(), target)
	if err != nil || len(applied) != 1 || applied[0] != "000063" {
		t.Fatal(applied, err)
	}
	claim, ok, err := store.ClaimErasure(ctx, testUUID(1801), time.Now().Unix()+60)
	if err != nil || !ok || claim.AccountID != id || claim.ContributorID != "" {
		t.Fatal(claim, ok, err)
	}
	if applied, err := migrate.Apply(ctx, store.pool.Config().ConnString(), target); err != nil || len(applied) != 0 {
		t.Fatal(applied, err)
	}
}
