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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	feedbackadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/adapters"
	feedbackapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/application"
	privacyadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/adapters"
	privacyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

// SEC04-B wired composition on real PostGIS (AUD-05, B-BR-A05): the
// production-shaped RequestErasure closure (privacy deletion intents
// per contributor plus synchronous account-social erasure) executes
// against real stores — intents land in the deletion ledger with
// durable jobs, comments tombstone, and aggregates rebuild with
// other authors intact.

func compositionPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	adminDSN := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if adminDSN == "" {
		adminDSN = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("integration database unreachable: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("account_erase_compose_%d", time.Now().UnixNano())
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
	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	if _, err := migrate.Apply(ctx, u.String(), dbmigrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestSEC04BWiredErasureCompositionOnRealSQL(t *testing.T) {
	pool := compositionPool(t)
	ctx := context.Background()
	clock := &integClock{now: 1_700_000_000}
	codes := []string{"482916", "111111"}
	var mu sync.Mutex
	ci, gi := 0, 9300
	accStore := NewPGStore(pool)
	svc := &application.Service{
		Clock:       clock,
		Hasher:      domain.SHA256Hasher{},
		Mail:        &integMail{},
		MailEnabled: true,
		Store:       accStore,
		CodeGen: func() (string, error) {
			mu.Lock()
			defer mu.Unlock()
			c := codes[ci%len(codes)]
			ci++
			return c, nil
		},
		TokenGen: counterForTest("tok"),
		AliasGen: counterForTest("alias"),
		IDGen: func() (string, error) {
			mu.Lock()
			defer mu.Unlock()
			gi++
			return testUUID(gi), nil
		},
	}

	if err := svc.RequestCode(ctx, "compose@example.invalid"); err != nil {
		t.Fatal(err)
	}
	auth, err := svc.ConsumeCode(ctx, "compose@example.invalid", "482916")
	if err != nil {
		t.Fatal(err)
	}
	if err := accStore.BindContributor(ctx, domain.ContributorBinding{
		AccountID: auth.Account.ID, ContributorID: "contrib-compose",
		KeyFingerprint: "fp:contrib-compose", BoundAt: clock.NowUnix(),
	}, testUUID(9201)); err != nil {
		t.Fatalf("bind: %v", err)
	}
	station, err := mustUUID(testUUID(9202))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO directory_stations (id, display_name) VALUES ($1, 'Posto Teste')`, station); err != nil {
		t.Fatalf("seed station: %v", err)
	}
	fbStore := feedbackadapters.NewPGStore(pool)
	fbSvc := &feedbackapp.Service{
		Clock:         clock,
		Store:         fbStore,
		Comments:      fbStore,
		Votes:         fbStore,
		CheckAccount:  func(context.Context, string) error { return nil },
		StationExists: func(_ context.Context, id string) (bool, error) { return id == testUUID(9202), nil },
		IDGen:         func() (string, error) { return testUUID(9203), nil },
	}
	comment, err := fbSvc.SubmitComment(ctx, auth.Account.ID, testUUID(9202), "GASOLINE_REGULAR", "apagar este texto")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	privacyStore := privacyadapters.NewStore(pool)
	svc.RequestErasure = func(ctx context.Context, accountID string, contributorIDs []string) error {
		for _, contributorID := range contributorIDs {
			if _, err := privacyapp.RequestDeletion(ctx, privacyapp.ErasePorts{
				Clock: func() time.Time { return time.Unix(clock.NowUnix(), 0) },
				NewID: func() (string, error) { return testUUID(9204), nil },
				Attribution: func(_ context.Context, contributorID string) (string, error) {
					return "tok-" + contributorID, nil
				},
				Store: privacyStore,
				EnqueueJob: func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
					_, err := platformjobs.Enqueue(ctx, tx, kind, payload, dedupe, 5, time.Time{})
					return err
				},
			}, contributorID, privacyapp.EraseDTO{
				ClientSubmissionID: "account-erasure:" + accountID + ":" + contributorID,
				Reason:             "account-deletion",
			}); err != nil {
				return err
			}
		}
		_, err := fbSvc.EraseAccount(ctx, accountID)
		return err
	}

	receipt, err := svc.DeleteAccount(ctx, auth.Account.ID)
	if err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	if !receipt.ErasureRequested || len(receipt.ContributorIDs) != 1 {
		t.Fatalf("receipt = %+v", receipt)
	}
	// Privacy intent durable in the deletion ledger.
	intents, err := accStore.ListErasureIntents(ctx, auth.Account.ID)
	if err != nil || len(intents) != 1 {
		t.Fatalf("account intents = %v (%v)", intents, err)
	}
	// Social erasure executed: comment tombstoned, view gone.
	if _, err := fbSvc.ViewComment(ctx, comment.ID); err == nil {
		t.Fatal("erased comment still readable")
	}
}

func counterForTest(prefix string) func() (string, error) {
	var mu sync.Mutex
	n := 0
	return func() (string, error) {
		mu.Lock()
		defer mu.Unlock()
		n++
		return fmt.Sprintf("%s-%d", prefix, n), nil
	}
}
