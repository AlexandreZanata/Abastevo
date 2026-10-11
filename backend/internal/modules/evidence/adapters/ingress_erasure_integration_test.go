//go:build integration

package adapters

import (
	"context"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

func TestErasureCannotAcknowledgeAnAdmittedUploadOnSQL(t *testing.T) {
	store, pool := freshStore(t)
	ctx := context.Background()
	now := time.Now()
	session := testSession("e0000000-0000-4000-8000-000000000091", "late-upload")
	if _, _, err := store.ReserveSession(ctx, session, openBudget()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ClaimUpload(ctx, session.ID, "admitted-writer", now); err != nil {
		t.Fatal(err)
	}
	deleted := 0
	_, _, err := store.EraseContributor(ctx, session.ContributorRef, func(context.Context, string, string) error { deleted++; return nil })
	if err == nil || deleted != 0 {
		t.Fatalf("live upload erased prematurely: calls=%d error=%v", deleted, err)
	}
	var marked bool
	if err := pool.QueryRow(ctx, `SELECT quarantine_deleted_at IS NOT NULL FROM evidence_sessions WHERE id=$1`, session.ID).Scan(&marked); err != nil || marked {
		t.Fatal("live writer falsely acknowledged", marked, err)
	}
	if _, _, err := store.ClaimUpload(ctx, session.ID, "new-writer", time.Now()); err == nil {
		t.Fatal("erasure did not close new ingress")
	}
	// The admitted writer finishes after expiry; only then may cleanup ack.
	if err := store.FinishUpload(ctx, session.ID, "admitted-writer", true, 9); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.EraseContributor(ctx, session.ContributorRef, func(context.Context, string, string) error { deleted++; return nil }); err != nil || deleted != 1 {
		t.Fatal("late upload not recoverably deleted", deleted, err)
	}
	got, err := store.Session(ctx, session.ID)
	if err != nil || got.Status != domain.StateExpired {
		t.Fatal("session resurrected", got, err)
	}
}

func TestIngressAdmissionWaitsForConcurrentSessionErasureFence(t *testing.T) {
	store, pool := freshStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	now := time.Now()
	session := testSession("e0000000-0000-4000-8000-000000000092", "serialized-upload")
	if _, _, err := store.ReserveSession(ctx, session, openBudget()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ClaimUpload(ctx, session.ID, "first", now); err != nil {
		t.Fatal(err)
	}
	if err := store.FinishUpload(ctx, session.ID, "first", false, 0); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT id FROM evidence_sessions WHERE id=$1 FOR UPDATE`, session.ID); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		_, _, err := store.ClaimUpload(ctx, session.ID, "racing", time.Now())
		result <- err
	}()
	<-started
	select {
	case err := <-result:
		t.Fatal("ingress crossed the held erasure fence", err)
	case <-time.After(100 * time.Millisecond):
	}
	if _, err := tx.Exec(ctx, `UPDATE evidence_sessions SET status='EXPIRED' WHERE id=$1`, session.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("post-erasure claim succeeded")
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	var attempts int
	if err := pool.QueryRow(ctx, `SELECT attempts FROM evidence_upload_transfers WHERE session_id=$1`, session.ID).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatal("refused claim consumed transfer budget", attempts, err)
	}
}
