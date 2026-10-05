//go:build integration

package adapters

import (
	"context"
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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/authority"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/verify"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func freshReviewDB(t *testing.T) (*pgxpool.Pool, ClaimStore, ReviewDecisions, string) {
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
	name := fmt.Sprintf("review_test_%d", time.Now().UnixNano())
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
	queries := stationprofile.New(pool)
	var stationID string
	if err := pool.QueryRow(ctx, "INSERT INTO directory_stations (id, display_name) VALUES (gen_random_uuid(), 'Posto Revista') RETURNING id::text").Scan(&stationID); err != nil {
		t.Fatalf("seed station: %v", err)
	}
	return pool, ClaimStore{Q: queries}, ReviewDecisions{Pool: pool}, stationID
}

func reviewTestPorts(store ClaimStore, decisions ReviewDecisions) application.ReviewPorts {
	return application.ReviewPorts{
		Claims:    store,
		Decisions: decisions,
		OperatorOf: func(context.Context, string) (string, bool, error) {
			return "04218406000104", true, nil
		},
		AccountLive: func(context.Context, string) (bool, error) { return true, nil },
		NewID:       newReviewID,
	}
}

var (
	reviewIDMu sync.Mutex
	reviewIDN  = 900
)

func newReviewID() (string, error) {
	reviewIDMu.Lock()
	defer reviewIDMu.Unlock()
	reviewIDN++
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", reviewIDN), nil
}

func openReviewClaimDB(t *testing.T, store ClaimStore, key string, stationID string) application.ClaimRow {
	t.Helper()
	ports := application.ClaimPorts{
		Store: store,
		Clock: func() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) },
		NewID: newReviewID,
		OperatorOf: func(context.Context, string) (string, string, bool, error) {
			return "04218406000104", "registry", true, nil
		},
	}
	claim, created, err := application.OpenClaim(context.Background(), ports, "11111111-1111-4111-8111-111111111111", stationID, "administrator", []string{"profile.edit"}, key)
	if err != nil || !created {
		t.Fatalf("open = %+v, %v, %v", claim, created, err)
	}
	row, err := store.Claim(context.Background(), claim.ID)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	return row
}

func goodReviewEvidence() application.ReviewEvidence {
	return application.ReviewEvidence{
		Signature: verify.Result{Outcome: verify.OutcomeValid, Signer: "CN=INT-TEST"},
		Authority: authority.AuthorityResult{Outcome: authority.OutcomeSufficient},
	}
}

func TestReviewIntegrationApproveDenyAndRaces(t *testing.T) {
	pool, store, decisions, stationID := freshReviewDB(t)
	ctx := context.Background()
	ports := reviewTestPorts(store, decisions)

	row := openReviewClaimDB(t, store, "review-int-1", stationID)
	if err := application.ReviewClaim(ctx, ports, row.ID, "op-1", true, "", goodReviewEvidence()); err != nil {
		t.Fatalf("approve: %v", err)
	}
	grant, found, err := decisions.ActiveGrant(ctx, "11111111-1111-4111-8111-111111111111", stationID)
	if err != nil || !found || grant.Status != "active" || grant.Role != "administrator" {
		t.Fatalf("grant = %+v, %v, %v", grant, found, err)
	}
	// Self-review is refused even with perfect evidence.
	if err := application.ReviewClaim(ctx, ports, row.ID, "11111111-1111-4111-8111-111111111111", true, "", goodReviewEvidence()); err == nil {
		t.Fatal("self-review must fail")
	}
	// Concurrent approvals of a fresh claim converge on one grant.
	row2 := openReviewClaimDB(t, store, "review-int-2", stationID)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = application.ReviewClaim(ctx, reviewTestPorts(store, decisions), row2.ID, "op-1", true, "", goodReviewEvidence())
		}(i)
	}
	wg.Wait()
	closed := 0
	for i, err := range errs {
		if err != nil {
			message := err.Error()
			if message != "stationprofile: claim already decided" &&
				message != "stationprofile: claim is no longer open" {
				t.Fatalf("race %d: %v", i, err)
			}
			closed++
		}
	}
	if closed != 1 {
		t.Fatalf("exactly one loser expected, errs = %v", errs)
	}
	var grants int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM representation_grants WHERE account_id = '11111111-1111-4111-8111-111111111111' AND station_id = $1 AND status = 'active'", stationID).Scan(&grants); err != nil {
		t.Fatalf("count grants: %v", err)
	}
	if grants != 1 {
		t.Fatalf("active grants = %d, want exactly 1", grants)
	}
	// Denial grants nothing and closes the claim.
	row3 := openReviewClaimDB(t, store, "review-int-3", stationID)
	if err := application.ReviewClaim(ctx, ports, row3.ID, "op-1", false, "not eligible", goodReviewEvidence()); err != nil {
		t.Fatalf("deny: %v", err)
	}
	stored, err := store.Claim(ctx, row3.ID)
	if err != nil || stored.State != "denied" {
		t.Fatalf("stored = %+v, err = %v", stored, err)
	}
}
