//go:build integration

package adapters

import (
	"context"
	"errors"
	"testing"
	"time"

	evidence "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/evidence"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

type dispatchPrivateStorage struct{ objects finalStorageFixture }

func (s dispatchPrivateStorage) DeleteQuarantine(ctx context.Context, key string) error {
	return s.objects.delete(ctx, key)
}
func (s dispatchPrivateStorage) DeleteFinal(ctx context.Context, key string) error {
	return s.objects.delete(ctx, key)
}

func TestSweepPolicyTimeCannotExpireActualUploadLease(t *testing.T) {
	s, pool := freshStore(t)
	objects := finalObjectsFixture(t)
	ctx := context.Background()
	sess := testSession(newUUIDv4must(t), newUUIDv4must(t))
	if _, _, err := s.ReserveSession(ctx, sess, openBudget()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ClaimUpload(ctx, sess.ID, "live-source", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := objects.put(ctx, sess.QuarantineKey, loadSmallJPEG(t)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := objects.delete(context.Background(), sess.QuarantineKey); err != nil {
			t.Error(err)
		}
	})
	deps := app.SweepDeps{Clock: func() time.Time { return sess.CreatedAt.Add(25 * time.Hour) }, Batch: 100, Store: s, Storage: dispatchPrivateStorage{objects}}
	rep, err := app.Sweep(ctx, deps)
	if err != nil || rep.QuarantinesDeleted != 0 {
		t.Fatal("policy clock bypassed live writer", rep, err)
	}
	var marked bool
	if err := pool.QueryRow(ctx, `SELECT quarantine_deleted_at IS NOT NULL FROM evidence_sessions WHERE id=$1`, sess.ID).Scan(&marked); err != nil || marked {
		t.Fatal(marked, err)
	}
	if present, err := objects.exists(ctx, sess.QuarantineKey); err != nil || !present {
		t.Fatal("live writer bytes deleted", present, err)
	}
	if err := s.FinishUpload(ctx, sess.ID, "live-source", true, 10); err != nil {
		t.Fatal(err)
	}
	rep, err = app.Sweep(ctx, deps)
	if err != nil || rep.QuarantinesDeleted != 1 {
		t.Fatal("completed writer not recovered", rep, err)
	}
}

func TestPrivateFinalDeletedAtPredeadlineBoundary(t *testing.T) {
	s, pool := freshStore(t)
	objects := finalObjectsFixture(t)
	ctx := context.Background()
	sess, _ := finalPhotoFixture(t, s, objects)
	out, err := app.Verify(ctx, finalPorts(t, s, sess, objects), sess)
	if err != nil {
		t.Fatal(err)
	}
	obj := finalObject(t, out)
	if _, err := s.RecordVerifiedGuarded(ctx, sess.ID, obj, out.Write); err != nil {
		t.Fatal(err)
	}
	policy := sess.CreatedAt.Add(22*time.Hour - time.Microsecond)
	deps := app.SweepDeps{Clock: func() time.Time { return policy }, Batch: 100, Store: s, Storage: dispatchPrivateStorage{objects}}
	rep, err := app.Sweep(ctx, deps)
	if err != nil || rep.FinalsDeleted != 0 {
		t.Fatal("early cleanup before margin boundary", rep, err)
	}
	if present, err := objects.exists(ctx, out.FinalKey); err != nil || !present {
		t.Fatal(present, err)
	}
	policy = sess.CreatedAt.Add(22 * time.Hour)
	rep, err = app.Sweep(ctx, deps)
	if err != nil || rep.FinalsDeleted != 1 || rep.Overdue.Unhealthy() {
		t.Fatal("predeadline cleanup failed", rep, err)
	}
	if present, err := objects.exists(ctx, out.FinalKey); err != nil || present {
		t.Fatal(present, err)
	}
	row, err := evidence.New(pool).GetFinalOutputForUpdate(ctx, out.FinalKey)
	if err != nil || row.State != "PURGED" || !row.DeadlineAt.Time.Equal(sess.CreatedAt.Add(24*time.Hour)) {
		t.Fatal("immutable deadline moved", row, err)
	}
}

func TestFinalWriterAndRepairRefuseCleanupMargin(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	sess, obj := finalVerifyingFixture(t, s)
	if _, err := pool.Exec(ctx, `UPDATE evidence_sessions SET created_at=now()-interval '22 hours' WHERE id=$1`, sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginVerificationWrite(ctx, sess.ID, obj.FinalKey, obj.SanitizedSHA256, newUUIDv4must(t)); !errors.Is(err, domain.ErrBadTransition) {
		t.Fatal("writer admitted into cleanup margin", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE evidence_sessions SET created_at=now() WHERE id=$1`, sess.ID); err != nil {
		t.Fatal(err)
	}
	authority, err := s.BeginVerificationWrite(ctx, sess.ID, obj.FinalKey, obj.SanitizedSHA256, newUUIDv4must(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE evidence_sessions SET created_at=now()-interval '22 hours' WHERE id=$1`, sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordVerifiedGuarded(ctx, sess.ID, obj, authority); !errors.Is(err, domain.ErrBadTransition) {
		t.Fatal("writer committed into cleanup margin", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE evidence_final_outputs SET deadline_at=now()+interval '2 hours'-interval '1 second',lease_until=now()-interval '1 second' WHERE final_key=$1`, obj.FinalKey); err != nil {
		t.Fatal(err)
	}
	rep, err := s.ReconcileFinalOutputs(ctx, time.Now(), 32, func(context.Context, string) error { return nil })
	if err != nil || rep.Purged != 1 {
		t.Fatal(rep, err)
	}
	row, err := evidence.New(pool).GetFinalOutputForUpdate(ctx, obj.FinalKey)
	if err != nil || row.State != "PURGED" {
		t.Fatal("repair reopened inside cleanup margin", row, err)
	}
}

func TestSourceLeaseMustFitBeforeCleanupBoundary(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	now := time.Now()
	sess := testSessionAt(newUUIDv4must(t), newUUIDv4must(t), now.Add(-22*time.Hour+time.Minute))
	if _, _, err := s.ReserveSession(ctx, sess, openBudget()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ClaimUpload(ctx, sess.ID, "fits-before-margin", now); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishUpload(ctx, sess.ID, "fits-before-margin", false, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE evidence_sessions SET created_at=$2 WHERE id=$1`, sess.ID, now.Add(-22*time.Hour+30*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ClaimUpload(ctx, sess.ID, "crosses-margin", now); !errors.Is(err, domain.ErrBadTransition) {
		t.Fatal("source lease crossed cleanup boundary", err)
	}
	var attempts int
	if err := pool.QueryRow(ctx, `SELECT attempts FROM evidence_upload_transfers WHERE session_id=$1`, sess.ID).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatal("refused lease consumed budget", attempts, err)
	}
	if rows, err := s.ExpireIdleSessions(ctx, now.Add(30*time.Second), 32); err != nil || len(rows) != 1 {
		t.Fatal("idle source not closed at 22h", rows, err)
	}
}
