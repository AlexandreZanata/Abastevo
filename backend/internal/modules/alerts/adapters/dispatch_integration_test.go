//go:build integration

package adapters

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/alerts/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
)

func TestDispatchRestartExpiredChangedLapseDelete(t *testing.T) {
	pool := freshDB(t)
	owner := newAccount(t, pool)
	station := newStation(t, pool)
	store := NewStore(pool)
	ctx := context.Background()
	now := alertEvaluationTime()
	ruleID := newid(t)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{{OperationID: newid(t), RuleID: ruleID, Payload: rulePayload(t, station)}}}); err != nil {
		t.Fatal(err)
	}
	insertCommunityPrice(t, pool, station, "ETHANOL", "L", 6000, "STANDARD", now.Add(-time.Hour), true)
	first, err := store.Evaluate(ctx, owner, ruleID, "", 100, now)
	if err != nil || first.Candidates != 1 {
		t.Fatalf("evaluate %+v %v", first, err)
	}
	var wg sync.WaitGroup
	results := make(chan application.DispatchResult, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, e := store.Dispatch(ctx, now, 25)
			results <- res
			errs <- e
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	total := 0
	for res := range results {
		total += res.Dispatched
	}
	if total != 1 {
		t.Fatalf("restart duplicated dispatch %d", total)
	}
	expiredRule := newid(t)
	if _, err = store.Apply(ctx, owner, application.Batch{Commands: []application.Command{{OperationID: newid(t), RuleID: expiredRule, Payload: rulePayload(t, station)}}}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Evaluate(ctx, owner, expiredRule, "", 100, now); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "UPDATE community_observations SET received_at=$1", now.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	dropped, err := store.Dispatch(ctx, now, 25)
	if err != nil || dropped.Dropped == 0 {
		t.Fatalf("expired price dispatched %+v %v", dropped, err)
	}
	changedRule := newid(t)
	if _, err = store.Apply(ctx, owner, application.Batch{Commands: []application.Command{{OperationID: newid(t), RuleID: changedRule, Payload: rulePayload(t, station)}}}); err != nil {
		t.Fatal(err)
	}
	insertCommunityPrice(t, pool, station, "ETHANOL", "L", 5900, "STANDARD", now.Add(-30*time.Minute), true)
	if _, err = store.Evaluate(ctx, owner, changedRule, "", 100, now); err != nil {
		t.Fatal(err)
	}
	off := false
	if _, err = store.Apply(ctx, owner, application.Batch{Commands: []application.Command{{OperationID: newid(t), RuleID: changedRule, ExpectedRevision: 1, Enabled: &off}}}); err != nil {
		t.Fatal(err)
	}
	changed, err := store.Dispatch(ctx, now, 25)
	if err != nil || changed.Dropped == 0 {
		t.Fatalf("changed rule dispatched %+v %v", changed, err)
	}
	lapseRule := newid(t)
	if _, err = store.Apply(ctx, owner, application.Batch{Commands: []application.Command{{OperationID: newid(t), RuleID: lapseRule, Payload: rulePayload(t, station)}}}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Evaluate(ctx, owner, lapseRule, "", 100, now); err != nil {
		t.Fatal(err)
	}
	store.Paid = func(context.Context, pgx.Tx, string) error { return errors.New("alerts: capability required") }
	lapsed, err := store.Dispatch(ctx, now, 25)
	if err != nil || lapsed.Skipped == 0 {
		t.Fatalf("lapsed dispatch %+v %v", lapsed, err)
	}
	store.Paid = nil
	delRule := newid(t)
	if _, err = store.Apply(ctx, owner, application.Batch{Commands: []application.Command{{OperationID: newid(t), RuleID: delRule, Payload: rulePayload(t, station)}}}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Evaluate(ctx, owner, delRule, "", 100, now); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Apply(ctx, owner, application.Batch{Commands: []application.Command{{OperationID: newid(t), RuleID: delRule, Delete: true}}}); err != nil {
		t.Fatal(err)
	}
	deleted, err := store.Dispatch(ctx, now, 25)
	if err != nil || deleted.Dropped == 0 {
		t.Fatalf("deleted rule dispatched %+v %v", deleted, err)
	}
	var evaluated, queued, dispatched int
	if err = pool.QueryRow(ctx, "SELECT (SELECT COUNT(*) FROM alert_inbox WHERE status='evaluated'),(SELECT COUNT(*) FROM alert_inbox WHERE status='queued'),(SELECT COUNT(*) FROM alert_inbox WHERE status='dispatched')").Scan(&evaluated, &queued, &dispatched); err != nil {
		t.Fatal(err)
	}
	if queued != 0 || dispatched == 0 {
		t.Fatalf("states evaluated=%d queued=%d dispatched=%d", evaluated, queued, dispatched)
	}
}

func TestDispatchQuietBacklogStaleLease(t *testing.T) {
	pool := freshDB(t)
	owner := newAccount(t, pool)
	station := newStation(t, pool)
	store := NewStore(pool)
	ctx := context.Background()
	base, _ := time.Parse(time.RFC3339, "2026-03-09T12:00:00-03:00")
	ruleID := newid(t)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{{OperationID: newid(t), RuleID: ruleID, Payload: rulePayload(t, station)}}}); err != nil {
		t.Fatal(err)
	}
	insertCommunityPrice(t, pool, station, "ETHANOL", "L", 6000, "STANDARD", base.Add(-time.Hour), true)
	if _, err := store.Evaluate(ctx, owner, ruleID, "", 100, base); err != nil {
		t.Fatal(err)
	}
	quietAt, _ := time.Parse(time.RFC3339, "2026-03-09T23:00:00-03:00")
	skipped, err := store.Dispatch(ctx, quietAt, 25)
	if err != nil || skipped.Skipped == 0 || skipped.Dispatched != 0 {
		t.Fatalf("quiet backlog %+v %v", skipped, err)
	}
	afterAt, _ := time.Parse(time.RFC3339, "2026-03-10T08:00:00-03:00")
	_ = afterAt
	morningPriceAt := base.Add(20 * time.Hour)
	if _, err = pool.Exec(ctx, "UPDATE community_observations SET received_at=$1", morningPriceAt.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	woken, err := store.Dispatch(ctx, morningPriceAt, 25)
	if err != nil || woken.Dispatched != 1 {
		t.Fatalf("delayed backlog %+v %v", woken, err)
	}
	queue := jobs.NewQueue(pool)
	claimed, err := queue.Claim(ctx, "alerts-dispatch", "test-worker", time.Minute)
	if err != nil {
		t.Fatalf("no job to lease %v", err)
	}
	if _, err = pool.Exec(ctx, "UPDATE job_queue SET lease_expires_at=clock_timestamp()-interval '1 hour' WHERE id=$1", uid(claimed.ID)); err != nil {
		t.Fatal(err)
	}
	reclaimed, err := queue.Claim(ctx, "alerts-dispatch", "test-worker-2", time.Minute)
	if err != nil {
		t.Fatalf("stale lease unreclaimed %v", err)
	}
	if reclaimed.LeaseToken == claimed.LeaseToken {
		t.Fatal("stale lease token reused")
	}
}
