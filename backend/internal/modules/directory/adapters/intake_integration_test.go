//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func intakeTestDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("integration database unreachable: %v", err)
	}
	defer conn.Close(ctx)
	return dsn
}

func freshIntake(t *testing.T) (application.IntakeService, *IntakeStore) {
	t.Helper()
	adminDSN := intakeTestDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("intake_test_%d", time.Now().UnixNano())
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
	store := &IntakeStore{Q: directory.New(pool)}
	n := 0
	svc := application.IntakeService{
		Store: store,
		Clock: func() time.Time { return time.Now() },
		NewID: func() (string, error) {
			n++
			return fmt.Sprintf("00000000-0000-4000-8000-%012d", n), nil
		},
	}
	return svc, store
}

func intakeInput() application.ProposalInput {
	return application.ProposalInput{DisplayName: "Posto Novo", MunicipalityCode: "3550308", State: "SP", CNPJ: "04218406000104"}
}

func TestIntakeIntegrationIdempotencyQuotaAndIsolation(t *testing.T) {
	svc, _ := freshIntake(t)
	ctx := context.Background()
	accA, accB := "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"

	first, created, err := svc.Submit(ctx, accA, "key-1", intakeInput())
	if err != nil || !created {
		t.Fatalf("submit = %+v, %v, %v", first, created, err)
	}
	// Same key + same body replays the same record (no duplicate row).
	second, created, err := svc.Submit(ctx, accA, "key-1", intakeInput())
	if err != nil || created || second.ID != first.ID {
		t.Fatalf("replay = %+v, %v, %v", second, created, err)
	}
	// Parallel same-station proposals from two accounts stay two
	// private rows (convergence happens at review, never by sharing).
	other, created, err := svc.Submit(ctx, accB, "key-1", intakeInput())
	if err != nil || !created || other.ID == first.ID {
		t.Fatalf("parallel = %+v, %v, %v", other, created, err)
	}
	// Owner isolation at SQL level: foreign reads/cancels fail.
	if _, err := svc.Owned(ctx, accB, first.ID); err == nil {
		t.Fatal("foreign read must fail")
	}
	if err := svc.Cancel(ctx, accB, first.ID); err == nil {
		t.Fatal("foreign cancel must fail")
	}
	// Cancel closes; the record stays readable as cancelled.
	if err := svc.Cancel(ctx, accA, first.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	owned, err := svc.Owned(ctx, accA, first.ID)
	if err != nil || owned.State != application.SuggestionCancelled {
		t.Fatalf("owned = %+v, err = %v", owned, err)
	}
	// Proposal bytes round-trip with the validated CNPJ preserved.
	raw, err := json.Marshal(first.Proposal)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), "04218406000104") {
		t.Fatalf("proposal lost CNPJ: %s", raw)
	}
}

func TestIntakeIntegrationQuotaBoundary(t *testing.T) {
	svc, _ := freshIntake(t)
	ctx := context.Background()
	acc := "44444444-4444-4444-8444-444444444444"

	for i := 0; i < application.MaxSuggestionsPerDay; i++ {
		if _, _, err := svc.Submit(ctx, acc, "quota-key-"+string(rune('a'+i/26))+string(rune('a'+i%26)), intakeInput()); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}
	if _, _, err := svc.Submit(ctx, acc, "quota-key-over", intakeInput()); err != application.ErrSuggestionQuota {
		t.Fatalf("21st submit err = %v, want quota", err)
	}
}
