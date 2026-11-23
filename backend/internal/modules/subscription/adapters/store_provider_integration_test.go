//go:build integration

package adapters

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
)

func appStoreAttempt(t *testing.T, p app.Principal, original string) app.Attempt {
	t.Helper()
	c, _ := NewTokenCodec(bytes.Repeat([]byte{9}, 32))
	cipher, err := c.Seal(original, p.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte("verify\x00" + original))
	return app.Attempt{Principal: p, Provider: domain.ProviderAppStore, OperationID: newid(t),
		Digest:      domain.StoreTokenDigest(domain.ProviderAppStore, "test", "com.anpfuel.app", original),
		PayloadHash: hex.EncodeToString(hash[:]), Environment: "test", PackageName: "com.anpfuel.app", CipherToken: cipher, Kind: "verify"}
}

func appStoreVerified() app.VerifiedPurchase {
	return app.VerifiedPurchase{Snapshot: domain.Snapshot{Provider: domain.ProviderAppStore, State: domain.Active, VerifiedAt: time.Now().UTC(),
		ValidUntil: time.Now().UTC().Add(time.Hour), ProductID: "com.anpfuel.app.premium.monthly", BasePlanID: "com.anpfuel.app.premium.monthly", Acknowledged: true}}
}

func TestAppStorePurchaseIsStoredLabelledAndIsolatedFromPlay(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	ctx := context.Background()
	p := seedOwner(t, pool)
	a, err := store.Begin(ctx, appStoreAttempt(t, p, "2000000123456789"))
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Commit(ctx, a, appStoreVerified()); err != nil {
		t.Fatal(err)
	}
	var provider string
	var acknowledgementJobs int
	if err = pool.QueryRow(ctx, "SELECT provider FROM subscription_purchases WHERE token_digest=$1", a.Digest).Scan(&provider); err != nil || provider != domain.ProviderAppStore {
		t.Fatalf("provider %q %v", provider, err)
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM job_queue WHERE kind='subscription-acknowledge'").Scan(&acknowledgementJobs); err != nil || acknowledgementJobs != 0 {
		t.Fatalf("App Store purchases need no acknowledgement job: %d %v", acknowledgementJobs, err)
	}
	row, err := store.ForJob(ctx, a.Digest)
	if err != nil || row.Provider != domain.ProviderAppStore || row.PackageName != "com.anpfuel.app" {
		t.Fatalf("job row %+v %v", row, err)
	}
	snapshot, err := store.Status(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	status := domain.Evaluate(snapshot, time.Now(), true)
	if len(status.Tiers) != 1 || status.Tiers[0].Source != domain.ProviderAppStore || len(status.Capabilities) == 0 {
		t.Fatalf("status %+v", status)
	}
	rows, err := store.ExportRows(ctx, p, 10, time.Time{}, "")
	if err != nil || len(rows) != 1 || rows[0].Provider != domain.ProviderAppStore {
		t.Fatalf("export %+v %v", rows, err)
	}

	// A Play notification for the same app identity cannot touch it.
	wrong := app.Notification{Provider: domain.ProviderPlay, Environment: "test", PackageName: "com.anpfuel.app", MessageID: "cross-store", Digest: a.Digest, PayloadHash: strings.Repeat("a", 64)}
	if _, err = store.ReceiveNotification(ctx, wrong); !errors.Is(err, domain.ErrDenied) {
		t.Fatalf("cross-store notification: %v", err)
	}
	right := wrong
	right.Provider, right.MessageID = domain.ProviderAppStore, "app_store:002e14d5-51f5-4503-b5a8-c3a1af68eb20"
	if _, err = store.ReceiveNotification(ctx, right); err != nil {
		t.Fatal(err)
	}
	var refreshes int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM job_queue WHERE kind='subscription-refresh'").Scan(&refreshes); err != nil || refreshes != 1 {
		t.Fatalf("refresh jobs %d %v", refreshes, err)
	}

	// Reconciliation is scheduled per store.
	if _, err = pool.Exec(ctx, "UPDATE subscription_purchases SET next_check_at=clock_timestamp()-interval '1 minute', refresh_required=false"); err != nil {
		t.Fatal(err)
	}
	if n, err := store.ScheduleReconciliation(ctx, domain.ProviderPlay, "test", "com.anpfuel.app", 25); err != nil || n != 0 {
		t.Fatalf("Play reconciliation took an App Store row: %d %v", n, err)
	}
	if n, err := store.ScheduleReconciliation(ctx, domain.ProviderAppStore, "test", "com.anpfuel.app", 25); err != nil || n != 1 {
		t.Fatalf("App Store reconciliation: %d %v", n, err)
	}
	if _, err := store.ScheduleReconciliation(ctx, "web", "test", "com.anpfuel.app", 25); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("unknown store reconciliation: %v", err)
	}

	// The same purchase key under Play is a different purchase.
	play := attempt(t, p, "2000000123456789")
	play.PackageName = "com.anpfuel.app"
	play.Digest = domain.TokenDigest("test", "com.anpfuel.app", "2000000123456789")
	if play.Digest == a.Digest {
		t.Fatal("digests collide across stores")
	}
	if _, err = store.Begin(ctx, play); err != nil {
		t.Fatalf("independent Play purchase refused: %v", err)
	}
}

func TestAppStorePurchaseNeverTransfersBetweenAccounts(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	ctx := context.Background()
	owner, foreign := seedOwner(t, pool), seedOwner(t, pool)
	a, err := store.Begin(ctx, appStoreAttempt(t, owner, "2000000123456790"))
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Commit(ctx, a, appStoreVerified()); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Begin(ctx, appStoreAttempt(t, foreign, "2000000123456790")); !errors.Is(err, domain.ErrDenied) {
		t.Fatalf("restore moved a purchase to another account: %v", err)
	}
	// Concurrent restores by the owner admit at most one live attempt.
	var wg sync.WaitGroup
	results := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.Begin(ctx, appStoreAttempt(t, owner, "2000000123456790"))
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	admitted := 0
	for err := range results {
		if err == nil {
			admitted++
		} else if !errors.Is(err, domain.ErrBusy) && !errors.Is(err, domain.ErrQuota) {
			t.Fatalf("unexpected concurrent error %v", err)
		}
	}
	if admitted != 1 {
		t.Fatalf("concurrent restores admitted %d", admitted)
	}
}

func TestStoreProviderMigrationKeepsLegacyRowsAsPlay(t *testing.T) {
	pool := freshDB(t)
	ctx := context.Background()
	p := seedOwner(t, pool)
	digest := strings.Repeat("b", 64)
	if _, err := pool.Exec(ctx, `INSERT INTO subscription_purchases(token_digest,account_id,environment,package_name,product_id,base_plan_id,state,valid_until,verified_at)
VALUES($1,$2,'test','com.anpfuel','abastevo_premium','monthly','ACTIVE',clock_timestamp()+interval '1 hour',clock_timestamp())`, digest, p.AccountID); err != nil {
		t.Fatal(err)
	}
	var provider string
	if err := pool.QueryRow(ctx, "SELECT provider FROM subscription_purchases WHERE token_digest=$1", digest).Scan(&provider); err != nil || provider != domain.ProviderPlay {
		t.Fatalf("legacy insert provider %q %v", provider, err)
	}
	if _, err := pool.Exec(ctx, "UPDATE subscription_purchases SET provider='web' WHERE token_digest=$1", digest); err == nil {
		t.Fatal("unknown store accepted by the schema")
	}
}
