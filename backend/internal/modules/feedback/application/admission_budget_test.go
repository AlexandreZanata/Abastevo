package application

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
)

// SEC01-C comment accounting (B-BR-F03/F04): live counts bound growth,
// deletes free budget, parallel floods cannot overshoot the ceiling.

func TestCommentAccountingAndDeleteFreesBudget(t *testing.T) {
	svc, store := testCommentService(allowAll)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := svc.SubmitComment(ctx, "acc-1", "station-1", "GASOLINE_REGULAR", fmt.Sprintf("note %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := store.CountLiveCommentsByAccount(ctx, "acc-1"); err != nil || n != 3 {
		t.Fatalf("live comments = %d (%v), want 3", n, err)
	}
	rows, err := store.ListCommentsByAccount(ctx, "acc-1")
	if err != nil || len(rows) != 3 {
		t.Fatalf("list by account = %d (%v), want 3", len(rows), err)
	}
	if err := svc.DeleteComment(ctx, "acc-1", rows[0].ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := store.CountLiveCommentsByAccount(ctx, "acc-1"); n != 2 {
		t.Fatalf("post-delete live = %d, want 2", n)
	}
}

func TestParallelCommentsCannotOvershoot(t *testing.T) {
	svc, store := testCommentService(allowAll)
	ctx := context.Background()
	const racers = 140
	var wg sync.WaitGroup
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = svc.SubmitComment(ctx, "acc-par", "station-1", "GASOLINE_REGULAR", fmt.Sprintf("par %d", i))
		}(i)
	}
	wg.Wait()
	n, err := store.CountLiveCommentsByAccount(ctx, "acc-par")
	if err != nil {
		t.Fatal(err)
	}
	if n != domain.MaxLiveCommentsPerAccount {
		t.Fatalf("parallel live comments = %d, want exactly %d", n, domain.MaxLiveCommentsPerAccount)
	}
}

// SEC01-D duplicate intent: an identical retry (same ID, same live
// account) replays converged without consuming another budget unit;
// a foreign rebind of the same ID fails closed.
func TestDuplicateCommentIDReplaysConverged(t *testing.T) {
	_, store := testCommentService(allowAll)
	ctx := context.Background()
	rec := domain.StoredComment{
		ID: "comment-dup-1", AccountID: "acc-1", StationID: "station-1",
		Product: "GASOLINE_REGULAR", Text: "hello", Scalars: 5,
		Revision: 1, CreatedAt: 1_700_000_000, UpdatedAt: 1_700_000_000,
	}
	if err := store.TryInsertComment(ctx, rec); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := store.TryInsertComment(ctx, rec); err != nil {
		t.Fatalf("identical retry must replay converged, got %v", err)
	}
	if n, _ := store.CountLiveCommentsByAccount(ctx, "acc-1"); n != 1 {
		t.Fatalf("duplicate live = %d, want 1", n)
	}
	rebind := rec
	rebind.AccountID = "acc-2"
	if err := store.TryInsertComment(ctx, rebind); err == nil {
		t.Fatal("foreign rebind of the same ID must fail")
	}
}
