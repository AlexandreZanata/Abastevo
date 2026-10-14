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

	communityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/adapters"
	communityapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	communitydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
	jobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func TestOpenCaseTxDoesNotPublishBeforeCommitOrSurviveRollback(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	c := mustCase(t, "c0000000-0000-4000-8000-000000000081", domain.TargetObservation,
		"b0000000-0000-4000-8000-000000000081", domain.PriorityP2, "WRONG_PRODUCT", time.Now())
	if _, _, err := s.OpenCaseTx(ctx, nil, c); err == nil {
		t.Fatal("missing report transaction accepted")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, _, err := s.OpenCaseTx(ctx, tx, c); err != nil {
		t.Fatal(err)
	}
	if open, err := s.OpenReview(ctx, c.TargetType, c.TargetID); err != nil || open {
		t.Fatalf("uncommitted case published: %v %v", open, err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if open, err := s.OpenReview(ctx, c.TargetType, c.TargetID); err != nil || open {
		t.Fatalf("rolled back case survived: %v %v", open, err)
	}
}

func TestDisputeCaseAndJobCommitRollbackAndConvergeTogether(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	stationID := "b0000000-0000-4000-8000-000000000091"
	observationID := "b0000000-0000-4000-8000-000000000092"
	if _, err := pool.Exec(ctx, `INSERT INTO directory_stations(id,display_name) VALUES($1,'Synthetic report station')`, stationID); err != nil {
		t.Fatal(err)
	}
	store := communityadapters.NewStore(pool)
	obs, _, err := communitydomain.NewObservation(communitydomain.Params{
		ID: observationID, ContributorRef: "synthetic-author", ClientSubmissionID: "synthetic-anchor", StationID: stationID,
		Product: "ETHANOL", Unit: "L", AmountMilli: 4500, RawText: "4,500", ConditionKind: "STANDARD", ReceivedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Submit(ctx, obs, func(context.Context, pgx.Tx, string, []byte, string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	claim, err := communitydomain.ClaimValidation(obs, communitydomain.StateReceived, "synthetic-job", communitydomain.ActorWorker, now, 1)
	if err != nil {
		t.Fatal(err)
	}
	admit, err := communitydomain.Admit(obs, communitydomain.StateValidating, communitydomain.ActorWorker, now, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []communitydomain.Decision{claim, admit} {
		if err := store.RecordDecision(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	var sequence atomic.Int64
	newID := func() (string, error) { return fmt.Sprintf("c0000000-0000-4000-8000-%012d", sequence.Add(1)), nil }
	openCase := func(ctx context.Context, tx pgx.Tx, target, reason, detail string) error {
		id, err := newID()
		if err != nil {
			return err
		}
		c, _, err := domain.NewCase(domain.CaseParams{ID: id, TargetType: domain.TargetObservation, TargetID: target,
			Priority: domain.PriorityP2, Reason: reason, Detail: detail, OpenedAt: now})
		if err != nil {
			return err
		}
		_, _, err = s.OpenCaseTx(ctx, tx, c)
		return err
	}
	enqueue := func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
		_, err := jobs.Enqueue(ctx, tx, kind, payload, dedupe, 5, time.Time{})
		return err
	}
	ports := communityapp.VotePorts{Clock: func() time.Time { return now }, NewID: newID, Store: store,
		CheckQuota:   func(context.Context, string, string) (time.Duration, error) { return 0, nil },
		CheckAccount: func(context.Context, string) error { return nil }, OpenReportCase: openCase, EnqueueJob: enqueue}
	caller := communityapp.Caller{ContributorID: "synthetic-reporter", Fingerprint: "synthetic-fingerprint", Token: "synthetic-reporter"}
	intent := communityapp.DisputeDTO{TargetObservationID: observationID, ClientSubmissionID: "synthetic-report", Reason: communitydomain.ReasonWrongProduct, Detail: "synthetic wrong label"}
	assertCounts := func(wantReports, wantCases, wantJobs int) {
		t.Helper()
		for table, want := range map[string]int{"community_disputes": wantReports, "moderation_cases": wantCases, "job_queue": wantJobs} {
			var got int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("%s count=%d, want %d", table, got, want)
			}
		}
	}
	failure := errors.New("synthetic transaction failure")
	ports.OpenReportCase = func(ctx context.Context, tx pgx.Tx, target, reason, detail string) error {
		if err := openCase(ctx, tx, target, reason, detail); err != nil {
			return err
		}
		return failure
	}
	if _, err := communityapp.Dispute(ctx, ports, caller, intent); !errors.Is(err, failure) {
		t.Fatalf("case failure=%v", err)
	}
	assertCounts(0, 0, 0)
	ports.OpenReportCase = openCase
	ports.EnqueueJob = func(context.Context, pgx.Tx, string, []byte, string) error { return failure }
	if _, err := communityapp.Dispute(ctx, ports, caller, intent); !errors.Is(err, failure) {
		t.Fatalf("job failure=%v", err)
	}
	assertCounts(0, 0, 0)
	ports.EnqueueJob = enqueue
	first, err := communityapp.Dispute(ctx, ports, caller, intent)
	if err != nil || first.Replayed {
		t.Fatalf("first=%+v %v", first, err)
	}
	retry, err := communityapp.Dispute(ctx, ports, caller, intent)
	if err != nil || !retry.Replayed || retry.DisputeID != first.DisputeID {
		t.Fatalf("retry=%+v %v", retry, err)
	}
	assertCounts(1, 1, 1)
	const reporters = 6
	var wg sync.WaitGroup
	errorsByReporter := make(chan error, reporters)
	for i := 0; i < reporters; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			other := caller
			other.ContributorID, other.Token = fmt.Sprintf("synthetic-reporter-%d", index), fmt.Sprintf("synthetic-reporter-%d", index)
			_, err := communityapp.Dispute(ctx, ports, other, intent)
			errorsByReporter <- err
		}(i)
	}
	wg.Wait()
	close(errorsByReporter)
	for err := range errorsByReporter {
		if err != nil {
			t.Fatal(err)
		}
	}
	// Cases converge; each distinct report event has its own recompute intent.
	assertCounts(1+reporters, 1, 1+reporters)
	queue, err := s.ListOpen(ctx, -1, time.Time{}, "", 20)
	if err != nil || len(queue) != 1 || queue[0].TargetType != domain.TargetObservation || queue[0].TargetID != observationID || queue[0].Reason != intent.Reason || queue[0].Detail != intent.Detail {
		t.Fatalf("operator queue=%+v %v", queue, err)
	}
	tally, err := communityapp.TallyFor(ctx, communityapp.HelpfulnessPorts{Store: communityadapters.NewHelpfulnessStore(pool), Reviews: s}, communityapp.Caller{}, observationID, 2)
	if err != nil || !tally.UnderReview {
		t.Fatalf("price report not annotated: %+v %v", tally, err)
	}
	decisions, err := store.Decisions(ctx, observationID)
	if err != nil || len(decisions) != 2 || decisions[1].ToState != communitydomain.StateValidated {
		t.Fatalf("unreviewed reports changed price authority: %+v %v", decisions, err)
	}
}
