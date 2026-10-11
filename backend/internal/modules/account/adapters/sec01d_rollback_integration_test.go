//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// SEC01-D real-PostGIS rollback/duplicate/rotation (B-BR-A02/A05):
// failed delivery retains nothing, cooldown duplicates converge,
// exhausted global budget stays nil with zero new rows, and the
// hourly window re-admits legitimate subjects.

type failingMail struct{ err error }

func (m *failingMail) SendCode(context.Context, string, string) error { return m.err }

func TestSEC01DMailRollbackOnRealSQL(t *testing.T) {
	svc, clock, _ := freshService(t)
	ctx := context.Background()
	svc.Mail = &failingMail{err: errors.New("delivery down")}
	if err := svc.RequestCode(ctx, "rollback@example.invalid"); err == nil {
		t.Fatal("failed delivery must error")
	}
	store := svc.Store.(*PGStore)
	if n, err := store.CountAllCodesSince(ctx, clock.NowUnix()-3600); err != nil || n != 0 {
		t.Fatalf("failed send retained %d rows (%v), want zero", n, err)
	}
}

func TestSEC01DDuplicateAndRotationOnRealSQL(t *testing.T) {
	svc, clock, mail := freshService(t)
	ctx := context.Background()
	if err := svc.RequestCode(ctx, "dup@example.invalid"); err != nil {
		t.Fatal(err)
	}
	if err := svc.RequestCode(ctx, "dup@example.invalid"); err != nil {
		t.Fatalf("cooldown duplicate must answer nil, got %v", err)
	}
	store := svc.Store.(*PGStore)
	hash := domain.AddressHash("dup@example.invalid")
	if n, err := store.CountCodesSince(ctx, hash, clock.NowUnix()-3600); err != nil || n != 1 {
		t.Fatalf("duplicate per-address rows = %d (%v), want 1", n, err)
	}
	// Fill the aggregate exactly, then prove a new distinct subject is
	// suppressed with zero new rows.
	for i := 1; i < domain.MaxAggregateCodesPerHour; i++ {
		clock.advance(61 * time.Second)
		if err := svc.RequestCode(ctx, fmt.Sprintf("fill-%d@example.invalid", i)); err != nil {
			t.Fatal(err)
		}
	}
	before, _ := store.CountAllCodesSince(ctx, clock.NowUnix()-3600)
	if err := svc.RequestCode(ctx, "overflow@example.invalid"); err != nil {
		t.Fatalf("exhausted budget must answer nil, got %v", err)
	}
	after, _ := store.CountAllCodesSince(ctx, clock.NowUnix()-3600)
	if after != before {
		t.Fatalf("exhausted budget grew rows %d -> %d", before, after)
	}
	// Rotation: past the window a legitimate subject succeeds.
	clock.advance(3601 * time.Second)
	if err := svc.RequestCode(ctx, "fresh@example.invalid"); err != nil {
		t.Fatalf("rotated subject: %v", err)
	}
	if got := mail.count(); got != domain.MaxAggregateCodesPerHour+1 {
		t.Fatalf("rotated mail = %d, want %d", got, domain.MaxAggregateCodesPerHour+1)
	}
}
