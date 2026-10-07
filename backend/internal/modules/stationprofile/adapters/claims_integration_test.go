//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	stationprofile "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/stationprofile"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func freshClaimDB(t *testing.T) (*pgxpool.Pool, ClaimStore, string) {
	t.Helper()
	adminDSN := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if adminDSN == "" {
		adminDSN = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("claim_test_%d", time.Now().UnixNano())
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
		if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Errorf("drop database: %v", err)
		}
	})
	parsed, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	parsed.Path = "/" + name
	dsn := parsed.String()
	if _, err := migrate.Apply(ctx, dsn, dbmigrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	var stationID string
	if err := pool.QueryRow(ctx, "INSERT INTO directory_stations (id, display_name) VALUES (gen_random_uuid(), 'Posto Claim') RETURNING id::text").Scan(&stationID); err != nil {
		t.Fatalf("seed station: %v", err)
	}
	return pool, ClaimStore{Q: stationprofile.New(pool)}, stationID
}

func claimLifecyclePorts(store ClaimStore) application.ClaimPorts {
	n := 0
	var idMu sync.Mutex
	return application.ClaimPorts{
		Store: store,
		Clock: func() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) },
		NewID: func() (string, error) {
			idMu.Lock()
			defer idMu.Unlock()
			n++
			return fmt.Sprintf("00000000-0000-4000-8000-%012d", n), nil
		},
		OperatorOf: func(context.Context, string) (string, string, bool, error) {
			return "04218406000104", "registry", true, nil
		},
	}
}

func TestClaimIntegrationLifecycleAndPrivacy(t *testing.T) {
	_, store, stationID := freshClaimDB(t)
	ctx := context.Background()
	ports := claimLifecyclePorts(store)

	first, created, err := application.OpenClaim(ctx, ports, "11111111-1111-4111-8111-111111111111", stationID, "administrator", []string{"profile.edit"}, "key-1")
	if err != nil || !created {
		t.Fatalf("open = %+v, %v, %v", first, created, err)
	}
	// Competing claim from another account stays a separate private
	// row (no leakage, no shared visibility).
	second, created, err := application.OpenClaim(ctx, ports, "22222222-2222-4222-8222-222222222222", stationID, "manager", []string{"profile.edit"}, "key-1")
	if err != nil || !created || second.ID == first.ID {
		t.Fatalf("competing = %+v, %v, %v", second, created, err)
	}
	if _, err := application.ClaimStatus(ctx, store, "22222222-2222-4222-8222-222222222222", first.ID); err == nil {
		t.Fatal("cross-account status must fail")
	}
	// Reissue versions the challenge and kills the prior binding.
	reissued, err := application.ReissueDeclaration(ctx, ports, "11111111-1111-4111-8111-111111111111", first.ID)
	if err != nil || reissued.Version != 2 || reissued.Declaration == first.Declaration {
		t.Fatalf("reissue = %+v, err = %v", reissued, err)
	}
	// Parallel identical opens converge on one claim. A loser can land
	// in the winner's publish window and get the documented retryable
	// transient; like a real client it replays the same request until
	// the declaration is visible, then convergence must be exact.
	var wg sync.WaitGroup
	type result struct {
		id      string
		created bool
		err     error
	}
	results := make([]result, 4)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var claim application.Claim
			var created bool
			var err error
			for attempt := 0; attempt < 50; attempt++ {
				claim, created, err = application.OpenClaim(ctx, ports, "11111111-1111-4111-8111-111111111111", stationID, "administrator", []string{"profile.edit"}, "key-race")
				if !errors.Is(err, application.ErrClaimBusy) {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			results[i] = result{id: claim.ID, created: created, err: err}
		}(i)
	}
	wg.Wait()
	ids := map[string]bool{}
	createdCount := 0
	for i, res := range results {
		if res.err != nil {
			t.Fatalf("race %d: %v", i, res.err)
		}
		ids[res.id] = true
		if res.created {
			createdCount++
		}
	}
	if len(ids) != 1 || createdCount != 1 {
		t.Fatalf("race diverged: %+v", results)
	}
	// Cancel closes; status stays owner-readable as cancelled.
	if err := application.CancelClaim(ctx, store, "11111111-1111-4111-8111-111111111111", first.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	closed, err := application.ClaimStatus(ctx, store, "11111111-1111-4111-8111-111111111111", first.ID)
	if err != nil || closed.State != "cancelled" {
		t.Fatalf("closed = %+v, err = %v", closed, err)
	}
}

func TestOwnerStatusAfterDeclarationConsumedIntegration(t *testing.T) {
	pool, store, stationID := freshClaimDB(t)
	ctx := context.Background()
	accountID := "11111111-1111-4111-8111-111111111111"
	claim, _, err := application.OpenClaim(ctx, claimLifecyclePorts(store), accountID, stationID, "manager", []string{"profile.edit"}, "status-consumed-live")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE claim_declarations SET state='consumed' WHERE claim_id=$1::uuid", claim.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE profile_claims SET state='approved' WHERE id=$1::uuid", claim.ID); err != nil {
		t.Fatal(err)
	}
	status, err := application.ClaimStatus(ctx, store, accountID, claim.ID)
	if err != nil || status.DeclarationState != "consumed" || status.State != "approved" {
		t.Fatalf("terminal owner status lost: %v", err)
	}
	if _, err := store.ActiveDeclaration(ctx, claim.ID); err == nil {
		t.Fatal("consumed proof regained active declaration")
	}
	if _, err := application.ClaimStatus(ctx, store, "foreign", claim.ID); err == nil {
		t.Fatal("foreign status leaked")
	}
}
