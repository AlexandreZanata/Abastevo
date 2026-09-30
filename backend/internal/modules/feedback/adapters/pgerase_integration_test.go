//go:build integration

package adapters

import (
	"context"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
)

func eraseSetup(t *testing.T) (*application.Service, string, string, string) {
	t.Helper()
	svc, pool := freshService(t)
	store := svc.Store.(*PGStore)
	svc.Comments = store
	svc.Votes = store
	station := testUUID(9001)
	acc1 := seedAccount(t, pool, 41, "active")
	acc2 := seedAccount(t, pool, 42, "active")
	_ = pool
	return svc, station, acc1, acc2
}

func TestPGExportEraseAndG14Exit(t *testing.T) {
	svc, pool := freshService(t)
	store := svc.Store.(*PGStore)
	svc.Comments = store
	svc.Votes = store
	ctx := context.Background()
	station := testUUID(9001)
	acc1 := seedAccount(t, pool, 51, "active")
	acc2 := seedAccount(t, pool, 52, "active")

	if _, err := svc.Rate(ctx, acc1, station, "GASOLINE_REGULAR", 5); err != nil {
		t.Fatalf("rate acc1: %v", err)
	}
	if _, err := svc.Rate(ctx, acc2, station, "GASOLINE_REGULAR", 3); err != nil {
		t.Fatalf("rate acc2: %v", err)
	}
	c1, err := svc.SubmitComment(ctx, acc1, station, "GASOLINE_REGULAR", "acc-1 thread")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := svc.SubmitComment(ctx, acc2, station, "GASOLINE_REGULAR", "acc-2 thread")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Vote(ctx, acc2, c1.ID, domain.VoteValid); err != nil {
		t.Fatalf("vote c1: %v", err)
	}
	if _, err := svc.Vote(ctx, acc1, c2.ID, domain.VoteValid); err != nil {
		t.Fatalf("vote c2: %v", err)
	}

	env, err := svc.ExportAccount(ctx, acc1)
	if err != nil {
		t.Fatalf("ExportAccount: %v", err)
	}
	if len(env.Footprint.Ratings) != 1 || len(env.Footprint.Comments) != 1 || len(env.Footprint.Votes) != 1 {
		t.Fatalf("export must hold 1/1/1 for acc1, got %+v", env.Footprint)
	}
	for _, c := range env.Footprint.Comments {
		if c.Text != "acc-1 thread" {
			t.Fatalf("export text must be verbatim, got %+v", c)
		}
	}

	report, err := svc.EraseAccount(ctx, acc1)
	if err != nil {
		t.Fatalf("EraseAccount: %v", err)
	}
	if report.Ratings != 1 || report.Comments != 1 || report.Votes != 1 {
		t.Fatalf("erase must tombstone 1/1/1, got %+v", report)
	}
	if report.RatingKeys != 1 || report.Tallies != 1 {
		t.Fatalf("erase must rebuild 1 key + 1 tally, got %+v", report)
	}

	stats, found, err := svc.Store.Stats(ctx, station, "GASOLINE_REGULAR")
	if err != nil || !found {
		t.Fatalf("stats must persist: %+v %v %v", stats, found, err)
	}
	if stats.Count != 1 || stats.Sum != 3 {
		t.Fatalf("stats must be 1/3 after erase, got %+v", stats)
	}
	rebuilt, err := svc.RebuildStats(ctx, station, "GASOLINE_REGULAR")
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt != stats {
		t.Fatalf("G14 exit: maintained stats must equal rebuild: %+v vs %+v", stats, rebuilt)
	}
	tally, err := svc.Tally(ctx, c2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tally.Valid != 0 || tally.Invalid != 0 {
		t.Fatalf("c2 tally must be 0/0 after vote erase, got %+v", tally)
	}
	retallied, err := svc.RebuildTally(ctx, c2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retallied != tally {
		t.Fatalf("G14 exit: maintained tally must equal rebuild: %+v vs %+v", tally, retallied)
	}
	if _, err := svc.ViewComment(ctx, c1.ID); err == nil {
		t.Fatalf("erased comment must vanish from views")
	}
	page, err := svc.ListComments(ctx, station, "GASOLINE_REGULAR", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.ID == c1.ID {
			t.Fatalf("erased comment leaked into public list")
		}
	}
	if _, err := pool.Exec(ctx, `SELECT 1 FROM feedback_comments WHERE id = $1 AND deleted_at IS NULL AND visibility = 'hidden'`, c1.ID); err != nil {
		t.Fatalf("leak audit query: %v", err)
	}

	second, err := svc.EraseAccount(ctx, acc1)
	if err != nil {
		t.Fatalf("replay erase: %v", err)
	}
	if second.Ratings != 0 || second.Comments != 0 || second.Votes != 0 {
		t.Fatalf("replay must converge zero, got %+v", second)
	}

	// Restore replay: resurrect one rating row (simulating a restored
	// backup that predates erasure), then re-erase must reapply.
	if _, err := pool.Exec(ctx, `UPDATE feedback_ratings SET deleted_at = NULL WHERE account_id = $1`, acc1); err != nil {
		t.Fatalf("simulate restore: %v", err)
	}
	replay, err := svc.EraseAccount(ctx, acc1)
	if err != nil {
		t.Fatalf("restore replay erase: %v", err)
	}
	if replay.Ratings != 1 {
		t.Fatalf("restore replay must re-tombstone the resurrected rating, got %+v", replay)
	}
	final, _, err := svc.Store.Stats(ctx, station, "GASOLINE_REGULAR")
	if err != nil {
		t.Fatal(err)
	}
	if final.Count != 1 || final.Sum != 3 {
		t.Fatalf("final stats must stay 1/3, got %+v", final)
	}
}

func TestPGConcurrentDoubleEraseConverges(t *testing.T) {
	svc, pool := freshService(t)
	store := svc.Store.(*PGStore)
	svc.Comments = store
	svc.Votes = store
	ctx := context.Background()
	station := testUUID(9001)
	acc := seedAccount(t, pool, 61, "active")

	if _, err := svc.Rate(ctx, acc, station, "GASOLINE_REGULAR", 4); err != nil {
		t.Fatal(err)
	}
	c, err := svc.SubmitComment(ctx, acc, station, "GASOLINE_REGULAR", "erase race")
	if err != nil {
		t.Fatal(err)
	}
	_ = c

	const racers = 8
	var wg sync.WaitGroup
	reports := make([]application.EraseReport, racers)
	errs := make([]error, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			reports[i], errs[i] = svc.EraseAccount(ctx, acc)
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("concurrent erase: %v", err)
		}
	}
	var total int64
	for _, r := range reports {
		total += r.Ratings + r.Comments + r.Votes
	}
	// One rating + one comment, no votes: exactly 2 tombstones across
	// all racers, never double-counted.
	if total != 2 {
		t.Fatalf("concurrent erases must tombstone exactly once each (total 2), got %d (%+v)", total, reports)
	}
}
