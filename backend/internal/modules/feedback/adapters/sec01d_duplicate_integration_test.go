//go:build integration

package adapters

import (
	"context"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
)

// SEC01-D real-PostGIS duplicate intent (B-BR-F03/F04): an identical
// retry (same ID, same live account) replays converged without
// consuming another budget unit; foreign ID reuse fails closed.

func TestSEC01DDuplicateIDReplayOnRealSQL(t *testing.T) {
	svc, _, acc1, acc2 := commentSetup(t)
	ctx := context.Background()
	store := svc.Store.(interface {
		TryInsertComment(ctx context.Context, rec domain.StoredComment) error
		CountLiveCommentsByAccount(ctx context.Context, accountID string) (int, error)
	})
	rec := domain.StoredComment{
		ID: "aaaaaaaa-1111-4111-8111-000000009991", AccountID: acc1,
		StationID: testUUID(9001), Product: "GASOLINE_REGULAR",
		Text: "hello", Scalars: 5, Revision: 1,
		CreatedAt: 1_700_000_000, UpdatedAt: 1_700_000_000,
	}
	if err := store.TryInsertComment(ctx, rec); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := store.TryInsertComment(ctx, rec); err != nil {
		t.Fatalf("identical retry must replay converged, got %v", err)
	}
	if n, err := store.CountLiveCommentsByAccount(ctx, acc1); err != nil || n != 1 {
		t.Fatalf("duplicate live = %d (%v), want 1", n, err)
	}
	rebind := rec
	rebind.AccountID = acc2
	if err := store.TryInsertComment(ctx, rebind); err == nil {
		t.Fatal("foreign reuse of the same ID must fail")
	}
	if n, _ := store.CountLiveCommentsByAccount(ctx, acc2); n != 0 {
		t.Fatalf("foreign live = %d, want 0", n)
	}
}
