//go:build integration

package adapters

import (
	"context"
	"errors"
	"io/fs"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func TestIngressUpgradePreservesExistingSession(t *testing.T) {
	before := fstest.MapFS{}
	target := fstest.MapFS{}
	files, err := fs.Glob(dbmigrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if name > "000061_evidence_ingress.sql" {
			continue
		}
		raw, err := fs.ReadFile(dbmigrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		target[name] = &fstest.MapFile{Data: raw}
		if name < "000061_evidence_ingress.sql" {
			before[name] = &fstest.MapFile{Data: raw}
		}
	}
	store, pool := freshStoreSchema(t, before)
	ctx := context.Background()
	now := time.Now()
	s := sec03aSession("e0000000-0000-4000-8000-000000007003", "upgrade-ingress", 9, now)
	if _, _, err := store.ReserveSession(ctx, s, sec03aBudget(now)); err != nil {
		t.Fatal(err)
	}
	applied, err := migrate.Apply(ctx, pool.Config().ConnString(), target)
	if err != nil || len(applied) != 1 || applied[0] != "000061" {
		t.Fatalf("upgrade=%v err=%v", applied, err)
	}
	kept, err := store.Session(ctx, s.ID)
	if err != nil || kept.ClaimedSHA256 != s.ClaimedSHA256 || kept.Status != s.Status {
		t.Fatal("existing session altered", err)
	}
	if _, _, err := store.ClaimUpload(ctx, s.ID, "post-upgrade", now); err != nil {
		t.Fatal(err)
	}
	if applied, err := migrate.Apply(ctx, pool.Config().ConnString(), target); err != nil || len(applied) != 0 {
		t.Fatal("migration replay", applied, err)
	}
}

func TestIngressAttemptBudgetAndLeaseOnRealSQL(t *testing.T) {
	store, _ := freshStore(t)
	ctx := context.Background()
	now := time.Now()
	s := sec03aSession("e0000000-0000-4000-8000-000000007001", "ingress-budget", 9, now)
	if _, _, err := store.ReserveSession(ctx, s, sec03aBudget(now)); err != nil {
		t.Fatal(err)
	}
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := store.ClaimUpload(ctx, s.ID, "winner", now)
			if err == nil {
				wins.Add(1)
			} else if !errors.Is(err, domain.ErrQuotaDenied) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("concurrent claims=%d", wins.Load())
	}
	if err := store.FinishUpload(ctx, s.ID, "stale", true, 9); !errors.Is(err, domain.ErrBadTransition) {
		t.Fatal("stale lease completed", err)
	}
	if err := store.FinishUpload(ctx, s.ID, "winner", false, 0); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < 5; i++ {
		if _, _, err := store.ClaimUpload(ctx, s.ID, "retry", now); err != nil {
			t.Fatal(err)
		}
		if err := store.FinishUpload(ctx, s.ID, "retry", false, 0); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := store.ClaimUpload(ctx, s.ID, "over", now); !errors.Is(err, domain.ErrQuotaDenied) {
		t.Fatal("budget refunded", err)
	}
}

func TestIngressReceiptReplaysWithoutNewAttemptOnRealSQL(t *testing.T) {
	store, pool := freshStore(t)
	ctx := context.Background()
	now := time.Now()
	s := sec03aSession("e0000000-0000-4000-8000-000000007002", "ingress-replay", 9, now)
	if _, _, err := store.ReserveSession(ctx, s, sec03aBudget(now)); err != nil {
		t.Fatal(err)
	}
	if _, replay, err := store.ClaimUpload(ctx, s.ID, "first", now); err != nil || replay {
		t.Fatal(err)
	}
	if err := store.FinishUpload(ctx, s.ID, "first", true, 9); err != nil {
		t.Fatal(err)
	}
	if _, replay, err := store.ClaimUpload(ctx, s.ID, "second", now); err != nil || !replay {
		t.Fatalf("receipt lost: %v %v", replay, err)
	}
	var attempts int
	var size int64
	if err := pool.QueryRow(ctx, "SELECT attempts, actual_bytes FROM evidence_upload_transfers WHERE session_id=$1", s.ID).Scan(&attempts, &size); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || size != 9 {
		t.Fatalf("receipt accounting: %d %d", attempts, size)
	}
}
