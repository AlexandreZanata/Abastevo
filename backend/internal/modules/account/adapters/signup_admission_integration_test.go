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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func TestSignupDeletionCannotRefundAdmissionOnSQL(t *testing.T) {
	svc, clock, _ := freshService(t)
	ctx := context.Background()
	svc.RequestErasure = func(context.Context, string, []string) error { return nil }
	var admitted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 115; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			acc, err := svc.CreateKeyAccount(ctx, fmt.Sprintf("cycleuser%d", i))
			if errors.Is(err, domain.ErrAggregateExhausted) {
				return
			}
			if err != nil {
				t.Error(err)
				return
			}
			admitted.Add(1)
			if _, err := svc.DeleteAccount(ctx, acc.Account.ID); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if admitted.Load() != domain.MaxKeyAccountsPerHour {
		t.Fatalf("concurrent create/delete admissions=%d", admitted.Load())
	}
	store := svc.Store.(*PGStore)
	restarted := NewPGStore(store.pool)
	if n, err := restarted.CountKeyAccountsSince(ctx, clock.NowUnix()-3600); err != nil || n != domain.MaxKeyAccountsPerHour {
		t.Fatalf("durable budget=%d %v", n, err)
	}
	clock.advance(3601 * time.Second)
	if _, err := svc.CreateKeyAccount(ctx, "afterexpiry"); err != nil {
		t.Fatal(err)
	}
	var retained int
	if err := store.pool.QueryRow(ctx, "SELECT count(*) FROM account_key_admission_events").Scan(&retained); err != nil || retained != 1 {
		t.Fatalf("bounded event retention=%d %v", retained, err)
	}
}

func TestSignupAdmissionUpgradeIncludesErasedAuditOnSQL(t *testing.T) {
	before, target := fstest.MapFS{}, fstest.MapFS{}
	files, err := fs.Glob(dbmigrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if name > "000062_account_admission_events.sql" {
			continue
		}
		raw, err := fs.ReadFile(dbmigrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		target[name] = &fstest.MapFile{Data: raw}
		if name < "000062_account_admission_events.sql" {
			before[name] = &fstest.MapFile{Data: raw}
		}
	}
	svc, _, _ := freshServiceSchema(t, before)
	store := svc.Store.(*PGStore)
	ctx := context.Background()
	for i, status := range []string{domain.StatusActive, domain.StatusDeleted} {
		if _, err := store.pool.Exec(ctx, "INSERT INTO accounts(id,alias,status,created_at) VALUES($1,$2,$3,now())", testUUID(700+i), fmt.Sprintf("migrationuser%d", i), status); err != nil {
			t.Fatal(err)
		}
	}
	applied, err := migrate.Apply(ctx, store.pool.Config().ConnString(), target)
	if err != nil || len(applied) != 1 || applied[0] != "000062" {
		t.Fatalf("upgrade=%v %v", applied, err)
	}
	if n, err := store.CountKeyAccountsSince(ctx, time.Now().Unix()-3600); err != nil || n != 2 {
		t.Fatalf("erased audit lost budget: %d %v", n, err)
	}
	if applied, err := migrate.Apply(ctx, store.pool.Config().ConnString(), target); err != nil || len(applied) != 0 {
		t.Fatal("replay", applied, err)
	}
}
