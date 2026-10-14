//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	modadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/adapters"
	modapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/application"
	moddomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trust/domain"
	jobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func TestModerationBlockCommitsAuditTrustCaseAndJobAtomically(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	modStore := modadapters.NewStore(pool)
	c, _, err := moddomain.NewCase(moddomain.CaseParams{ID: "c0000000-0000-4000-8000-000000000021",
		TargetType: moddomain.TargetContributor, TargetID: "synthetic-blocked-contributor", Priority: moddomain.PriorityP1,
		Reason: "synthetic reviewed abuse", OpenedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := modStore.OpenCase(ctx, c); err != nil {
		t.Fatal(err)
	}
	var sequence atomic.Int64
	newID := func() (string, error) { return fmt.Sprintf("a0000000-0000-4000-8000-%012d", sequence.Add(1)), nil }
	block := func(ctx context.Context, tx pgx.Tx, ref, caseID, reason string, at time.Time) error {
		id, err := newID()
		if err != nil {
			return err
		}
		d, _, err := domain.NewDecision(domain.DecisionParams{ID: id, ContributorRef: ref, Tier: domain.TierBlocked,
			Reason: reason, CaseRefs: []string{caseID}, OccurredAt: at})
		if err != nil {
			return err
		}
		_, err = s.AppendDecisionTx(ctx, tx, d)
		return err
	}
	enqueue := func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
		_, err := jobs.Enqueue(ctx, tx, kind, payload, dedupe, 5, time.Time{})
		return err
	}
	ports := modapp.ActPorts{Clock: func() time.Time { return now }, NewID: newID, Store: modStore, BlockContributor: block, EnqueueJob: enqueue}
	intent := modapp.ActDTO{CaseID: c.ID, Action: moddomain.ActionBlock, Reason: "synthetic confirmed abuse"}
	caller := modapp.Caller{OperatorID: "synthetic-operator"}
	assertState := func(blocked bool, count int) {
		t.Helper()
		tier, err := s.Tier(ctx, c.TargetID)
		wantTier := domain.TierNew
		wantStatus := moddomain.StatusOpen
		if blocked {
			wantTier, wantStatus = domain.TierBlocked, moddomain.StatusResolved
		}
		if err != nil || tier != wantTier {
			t.Fatalf("tier=%s error=%v, want %s", tier, err, wantTier)
		}
		current, err := modStore.Get(ctx, c.ID)
		if err != nil || current.Status != wantStatus {
			t.Fatalf("case=%+v error=%v", current, err)
		}
		for _, table := range []string{"trust_decisions", "moderation_actions", "job_queue"} {
			var actual int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&actual); err != nil || actual != count {
				t.Fatalf("%s count=%d error=%v, want %d", table, actual, err, count)
			}
		}
		var currentVersion int64
		if err := pool.QueryRow(ctx, `SELECT COALESCE(MAX(version),0) FROM trust_current WHERE contributor_ref=$1`, c.TargetID).Scan(&currentVersion); err != nil || currentVersion != int64(count) {
			t.Fatalf("failed verdict changed trust version: version=%d error=%v, want %d", currentVersion, err, count)
		}
	}
	failure := errors.New("synthetic trust failure")
	ports.BlockContributor = func(ctx context.Context, tx pgx.Tx, ref, caseID, reason string, at time.Time) error {
		if err := block(ctx, tx, ref, caseID, reason, at); err != nil {
			return err
		}
		return failure
	}
	if _, err := modapp.Act(ctx, ports, caller, intent); !errors.Is(err, failure) {
		t.Fatalf("trust failure=%v", err)
	}
	assertState(false, 0)
	ports.BlockContributor = block
	ports.EnqueueJob = func(context.Context, pgx.Tx, string, []byte, string) error { return failure }
	if _, err := modapp.Act(ctx, ports, caller, intent); !errors.Is(err, failure) {
		t.Fatalf("job failure=%v", err)
	}
	assertState(false, 0)
	ports.EnqueueJob = enqueue
	const operators = 6
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, operators)
	for i := 0; i < operators; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; _, err := modapp.Act(ctx, ports, caller, intent); results <- err }()
	}
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, moddomain.ErrTerminalCase) {
			t.Fatalf("concurrent block=%v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent accepted blocks=%d, want 1", successes)
	}
	assertState(true, 1)
	history, err := s.History(ctx, c.TargetID)
	if err != nil || len(history) != 1 || len(history[0].CaseRefs) != 1 || history[0].CaseRefs[0] != c.ID || history[0].Reason != intent.Reason {
		t.Fatalf("trust audit=%+v error=%v", history, err)
	}
	if _, err := modapp.Act(ctx, ports, caller, intent); !errors.Is(err, moddomain.ErrTerminalCase) {
		t.Fatalf("closed block=%v", err)
	}
	assertState(true, 1)
}

func TestTrustDecisionTxRequiresCallerTransactionAndRollsBack(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	d := testDecision("d0000000-0000-4000-8000-000000000041", domain.TierBlocked, time.Now())
	if _, err := s.AppendDecisionTx(ctx, nil, d); err == nil {
		t.Fatal("nil transaction accepted")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := s.AppendDecisionTx(ctx, tx, d); err != nil {
		t.Fatal(err)
	}
	if tier, err := s.Tier(ctx, d.ContributorRef); err != nil || tier != domain.TierNew {
		t.Fatalf("uncommitted tier published: %s %v", tier, err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if tier, err := s.Tier(ctx, d.ContributorRef); err != nil || tier != domain.TierNew {
		t.Fatalf("rolled back tier survived: %s %v", tier, err)
	}
	if history, err := s.History(ctx, d.ContributorRef); err != nil || len(history) != 0 {
		t.Fatalf("rolled back audit survived: %+v %v", history, err)
	}
}
