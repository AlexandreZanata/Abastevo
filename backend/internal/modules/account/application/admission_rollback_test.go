package application

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// SEC01-D rollback/duplicate/rotation (AUD-01, B-BR-A02/A05): failed
// delivery retains nothing and consumes no budget; cooldown
// duplicates converge; hourly rotation re-admits; exhausted global
// budget stays nil with zero new rows.

func TestMailFailureRollsBackBudget(t *testing.T) {
	svc, _, mail := testService()
	ctx := context.Background()
	mail.fail = true
	if err := svc.RequestCode(ctx, "rollback@example.invalid"); err == nil {
		t.Fatal("failed delivery must error")
	}
	store := svc.Store.(*MemStore)
	if n, _ := store.CountAllCodesSince(ctx, 0); n != 0 {
		t.Fatalf("failed send retained %d rows, want zero", n)
	}
	if got := mail.count(); got != 0 {
		t.Fatalf("failed send issued %d mails, want zero", got)
	}
	// Retry after the dependency recovers succeeds with one row/mail.
	mail.fail = false
	// Cooldown does not block the retry: nothing was admitted before.
	if err := svc.RequestCode(ctx, "rollback@example.invalid"); err != nil {
		t.Fatalf("recovered retry: %v", err)
	}
	if n, _ := store.CountAllCodesSince(ctx, 0); n != 1 {
		t.Fatalf("recovered rows = %d, want 1", n)
	}
	if got := mail.count(); got != 1 {
		t.Fatalf("recovered mail = %d, want 1", got)
	}
}

func TestCooldownDuplicateConvergesWithoutNewRow(t *testing.T) {
	svc, _, mail := testService()
	ctx := context.Background()
	if err := svc.RequestCode(ctx, "dup@example.invalid"); err != nil {
		t.Fatal(err)
	}
	// Immediate retry (same intent, within cooldown) answers nil with
	// no new row or mail.
	if err := svc.RequestCode(ctx, "dup@example.invalid"); err != nil {
		t.Fatalf("duplicate retry must look successful, got %v", err)
	}
	store := svc.Store.(*MemStore)
	if n, _ := store.CountCodesSince(ctx, domain.AddressHash("dup@example.invalid"), 0); n != 1 {
		t.Fatalf("duplicate rows = %d, want 1", n)
	}
	if got := mail.count(); got != 1 {
		t.Fatalf("duplicate mail = %d, want 1", got)
	}
}

func TestHourlyRotationReadmitsSubjects(t *testing.T) {
	svc, clock, mail := testService()
	ctx := context.Background()
	for i := 0; i < domain.MaxAggregateCodesPerHour; i++ {
		if err := svc.RequestCode(ctx, fmt.Sprintf("rot-%d@example.invalid", i)); err != nil {
			t.Fatal(err)
		}
	}
	// Exhausted global budget: new distinct subject answers nil with
	// zero new rows/mail (no oracle, bounded growth).
	before, _ := svc.Store.(*MemStore).CountAllCodesSince(ctx, 0)
	if err := svc.RequestCode(ctx, "rot-overflow@example.invalid"); err != nil {
		t.Fatalf("exhausted budget must answer nil, got %v", err)
	}
	after, _ := svc.Store.(*MemStore).CountAllCodesSince(ctx, 0)
	if after != before {
		t.Fatalf("exhausted budget grew rows %d -> %d", before, after)
	}
	if got := mail.count(); got != domain.MaxAggregateCodesPerHour {
		t.Fatalf("exhausted mail = %d, want %d", got, domain.MaxAggregateCodesPerHour)
	}
	// Rotating window: past the hour, a legitimate new subject succeeds.
	clock.advance((3600 + 1) * time.Second)
	if err := svc.RequestCode(ctx, "rot-fresh@example.invalid"); err != nil {
		t.Fatalf("rotated subject: %v", err)
	}
	if got := mail.count(); got != domain.MaxAggregateCodesPerHour+1 {
		t.Fatalf("rotated mail = %d, want %d", got, domain.MaxAggregateCodesPerHour+1)
	}
}

func TestKeyRotationAndDuplicateUsernameStable(t *testing.T) {
	svc := testKeyService()
	ctx := context.Background()
	if _, err := svc.CreateKeyAccount(ctx, "stableuser"); err != nil {
		t.Fatal(err)
	}
	// Duplicate intent (same username) refuses taken without touching
	// the aggregate budget: filling the aggregate afterwards still
	// admits exactly to the ceiling.
	if _, err := svc.CreateKeyAccount(ctx, " STABLEUSER "); !errors.Is(err, domain.ErrUsernameTaken) {
		t.Fatalf("duplicate username must refuse taken, got %v", err)
	}
	for i := 0; i < domain.MaxKeyAccountsPerHour-1; i++ {
		if _, err := svc.CreateKeyAccount(ctx, fmt.Sprintf("filluser%d", i)); err != nil {
			t.Fatalf("fill %d: %v", i, err)
		}
	}
	if _, err := svc.CreateKeyAccount(ctx, "onebeyond"); !errors.Is(err, domain.ErrAggregateExhausted) {
		t.Fatalf("beyond ceiling must refuse aggregate-exhausted, got %v", err)
	}
}
