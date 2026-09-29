//go:build integration

package adapters

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
)

func mustAction(t *testing.T, id, caseID, actor, action, reason string, at time.Time) domain.Action {
	t.Helper()
	a, _, err := domain.NewAction(domain.ActionParams{
		ID: id, CaseID: caseID, ActorID: actor,
		Action: action, Reason: reason, OccurredAt: at,
	})
	if err != nil {
		t.Fatalf("new action: %v", err)
	}
	return a
}

func captureEnqueue(jobs *[][3]string) func(context.Context, pgx.Tx, string, []byte, string) error {
	return func(_ context.Context, _ pgx.Tx, kind string, payload []byte, dedupe string) error {
		*jobs = append(*jobs, [3]string{kind, string(payload), dedupe})
		return nil
	}
}

func TestRecordActionMovesStatusAndEnqueues(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	base := time.Now().Truncate(time.Millisecond)
	c := mustCase(t, "c0000000-0000-4000-8000-000000000001",
		domain.TargetObservation, "b0000000-0000-4000-8000-000000000001",
		domain.PriorityP2, "report", base)
	if _, _, err := s.OpenCase(ctx, c); err != nil {
		t.Fatal(err)
	}
	var jobs [][3]string
	a := mustAction(t, "a0000000-0000-4000-8000-000000000001", c.ID,
		"op-7", domain.ActionReview, "triage", base.Add(time.Minute))
	id, err := s.RecordAction(ctx, a, domain.StatusInReview,
		"moderation-applied", []byte(`{"version":1}`), "moderation:"+c.ID+":"+a.ID,
		captureEnqueue(&jobs))
	if err != nil || id != a.ID {
		t.Fatalf("record = %q, %v", id, err)
	}
	got, err := s.Get(ctx, c.ID)
	if err != nil || got.Status != domain.StatusInReview {
		t.Errorf("case status = %+v, %v", got, err)
	}
	history, err := s.ListActions(ctx, c.ID)
	if err != nil || len(history) != 1 || history[0].ActorID != "op-7" {
		t.Errorf("audit = %+v, %v", history, err)
	}
	if len(jobs) != 1 || jobs[0][0] != "moderation-applied" {
		t.Errorf("jobs = %v", jobs)
	}
}

func TestRecordActionRollsBackOnEnqueueFailure(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	base := time.Now().Truncate(time.Millisecond)
	c := mustCase(t, "c0000000-0000-4000-8000-000000000001",
		domain.TargetObservation, "b0000000-0000-4000-8000-000000000001",
		domain.PriorityP2, "report", base)
	if _, _, err := s.OpenCase(ctx, c); err != nil {
		t.Fatal(err)
	}
	a := mustAction(t, "a0000000-0000-4000-8000-000000000001", c.ID,
		"op-7", domain.ActionReview, "triage", base.Add(time.Minute))
	failing := func(context.Context, pgx.Tx, string, []byte, string) error {
		return context.DeadlineExceeded
	}
	if _, err := s.RecordAction(ctx, a, domain.StatusInReview,
		"moderation-applied", []byte(`{}`), "dedupe", failing); err == nil {
		t.Fatal("enqueue failure accepted")
	}
	// Both the audit row and the status move rolled back together.
	got, err := s.Get(ctx, c.ID)
	if err != nil || got.Status != domain.StatusOpen {
		t.Errorf("case status after rollback = %+v, %v", got, err)
	}
	history, err := s.ListActions(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 0 {
		t.Errorf("orphaned audit rows after rollback: %+v", history)
	}
}

func TestRecordActionRefusesTerminalCase(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	base := time.Now().Truncate(time.Millisecond)
	c := mustCase(t, "c0000000-0000-4000-8000-000000000001",
		domain.TargetObservation, "b0000000-0000-4000-8000-000000000001",
		domain.PriorityP2, "report", base)
	if _, _, err := s.OpenCase(ctx, c); err != nil {
		t.Fatal(err)
	}
	var jobs [][3]string
	first := mustAction(t, "a0000000-0000-4000-8000-000000000001", c.ID,
		"op-7", domain.ActionResolve, "substantiated", base.Add(time.Minute))
	if _, err := s.RecordAction(ctx, first, domain.StatusResolved,
		"moderation-applied", []byte(`{}`), "d1", captureEnqueue(&jobs)); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	second := mustAction(t, "a0000000-0000-4000-8000-000000000002", c.ID,
		"op-7", domain.ActionDismiss, "late dismiss", base.Add(2*time.Minute))
	if _, err := s.RecordAction(ctx, second, domain.StatusRejected,
		"moderation-applied", []byte(`{}`), "d2", captureEnqueue(&jobs)); err == nil {
		t.Error("post-closure action accepted")
	}
	history, err := s.ListActions(ctx, c.ID)
	if err != nil || len(history) != 1 {
		t.Errorf("audit after refusal = %+v, %v", history, err)
	}
}
