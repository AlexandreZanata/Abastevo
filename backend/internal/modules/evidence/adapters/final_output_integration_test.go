//go:build integration

package adapters

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	evidence "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/evidence"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func finalVerifyingFixture(t *testing.T, s *Store) (domain.Session, ObjectData) {
	t.Helper()
	sess := testSession(newUUIDv4must(t), newUUIDv4must(t))
	if _, _, err := s.ReserveSession(context.Background(), sess, openBudget()); err != nil {
		t.Fatal(err)
	}
	var queued [][]byte
	if err := s.CompleteSession(context.Background(), sess.ID, enqueueStub(&queued)); err != nil {
		t.Fatal(err)
	}
	sess, err := s.Session(context.Background(), sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	obj := testObject(sess.ID)
	obj.FinalKey = "f/" + sess.ClaimedSHA256
	return sess, obj
}

func TestFinalOutputInventoryAndConcurrentAuthority(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	sess, obj := finalVerifyingFixture(t, s)
	var wins atomic.Int32
	var wg sync.WaitGroup
	tokens := make(chan string, 12)
	for i := 0; i < 12; i++ {
		token := newUUIDv4must(t)
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, err := s.BeginVerificationWrite(ctx, sess.ID, obj.FinalKey, obj.SanitizedSHA256, token)
			if err == nil {
				wins.Add(1)
				tokens <- a.Nonce
			} else if !errors.Is(err, domain.ErrBadTransition) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	close(tokens)
	if wins.Load() != 1 {
		t.Fatalf("writer authorities=%d", wins.Load())
	}
	token := <-tokens
	row, err := evidence.New(pool).GetFinalOutputForUpdate(ctx, obj.FinalKey)
	if err != nil || row.State != "WRITING" || row.Attempts != 1 || uuidString(row.LeaseNonce) != token || !row.DeadlineAt.Time.Equal(sess.CreatedAt.Add(application.CopyRetention)) {
		t.Fatal(row, err)
	}
	if _, err := s.RecordVerifiedGuarded(ctx, sess.ID, obj, application.FinalWrite{Nonce: newUUIDv4must(t)}); !errors.Is(err, domain.ErrBadTransition) {
		t.Fatal("foreign nonce committed", err)
	}
	// Simulate already-cleaned source; erasure must still find/expire this final writer.
	if err := s.MarkQuarantineDeleted(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	deletes := 0
	_, _, err = s.EraseContributor(ctx, sess.ContributorRef, func(context.Context, string, string) error { deletes++; return nil })
	if err == nil || deletes != 0 {
		t.Fatalf("live final write erased: deletes=%d err=%v", deletes, err)
	}
	live, err := s.Session(ctx, sess.ID)
	if err != nil || live.Status != domain.StateExpired {
		t.Fatal(live, err)
	}
	if _, err := s.RecordVerifiedGuarded(ctx, sess.ID, obj, application.FinalWrite{Nonce: token}); !errors.Is(err, domain.ErrBadTransition) {
		t.Fatal("expired writer recreated READY", err)
	}
	var objects int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence_objects`).Scan(&objects); err != nil || objects != 0 {
		t.Fatal(objects, err)
	}
}

func TestFinalOutputCommitFailureAndImmutableAnchor(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	sess, obj := finalVerifyingFixture(t, s)
	authority, err := s.BeginVerificationWrite(ctx, sess.ID, obj.FinalKey, obj.SanitizedSHA256, newUUIDv4must(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE FUNCTION fail_final_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic commit fault'; END $$; CREATE TRIGGER fail_final_insert BEFORE INSERT ON evidence_objects FOR EACH ROW EXECUTE FUNCTION fail_final_insert()`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordVerifiedGuarded(ctx, sess.ID, obj, authority); err == nil {
		t.Fatal("injected DB failure accepted")
	}
	row, err := evidence.New(pool).GetFinalOutputForUpdate(ctx, obj.FinalKey)
	if err != nil || row.State != "WRITING" || row.LeaseNonce.Valid == false {
		t.Fatal(row, err)
	}
	live, err := s.Session(ctx, sess.ID)
	if err != nil || live.Status != domain.StateVerifying {
		t.Fatal(live, err)
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER fail_final_insert ON evidence_objects`); err != nil {
		t.Fatal(err)
	}
	obj.ReceivedAt = time.Now().Add(24 * time.Hour)
	if _, err := s.RecordVerifiedGuarded(ctx, sess.ID, obj, authority); err != nil {
		t.Fatal(err)
	}
	var anchor time.Time
	if err := pool.QueryRow(ctx, `SELECT received_at FROM evidence_objects WHERE final_key=$1`, obj.FinalKey).Scan(&anchor); err != nil || !anchor.Equal(sess.CreatedAt) {
		t.Fatal("caller extended anchor", anchor, err)
	}
	other, dup := finalVerifyingFixture(t, s)
	dup.FinalKey = obj.FinalKey
	reuse, err := s.BeginVerificationWrite(ctx, other.ID, dup.FinalKey, dup.SanitizedSHA256, newUUIDv4must(t))
	if err != nil || !reuse.Reused || reuse.Nonce != "" {
		t.Fatal(reuse, err)
	}
	id, err := s.RecordVerifiedGuarded(ctx, other.ID, dup, reuse)
	if err != nil || id != obj.ID {
		t.Fatal(id, err)
	}
	if _, err := s.BeginVerificationWrite(ctx, other.ID, dup.FinalKey, strings.Repeat("c", 64), newUUIDv4must(t)); err == nil {
		t.Fatal("terminal session admitted")
	}
	row, err = evidence.New(pool).GetFinalOutputForUpdate(ctx, obj.FinalKey)
	if err != nil || row.Attempts != 1 || !row.DeadlineAt.Time.Equal(sess.CreatedAt.Add(application.CopyRetention)) {
		t.Fatal("reuse altered output", row, err)
	}
}

func TestFinalOutputLeaseExpiryRequiresCleanup(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	sess, obj := finalVerifyingFixture(t, s)
	authority, err := s.BeginVerificationWrite(ctx, sess.ID, obj.FinalKey, obj.SanitizedSHA256, newUUIDv4must(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE evidence_final_outputs SET lease_until=now()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordVerifiedGuarded(ctx, sess.ID, obj, authority); !errors.Is(err, domain.ErrBadTransition) {
		t.Fatal("stale output committed", err)
	}
	if _, err := NewStore(pool).BeginVerificationWrite(ctx, sess.ID, obj.FinalKey, obj.SanitizedSHA256, newUUIDv4must(t)); !errors.Is(err, domain.ErrBadTransition) {
		t.Fatal("unresolved PUT overwritten", err)
	}
}

func TestFinalOutputUpgradeInventoriesLegacyCopies(t *testing.T) {
	before := fstest.MapFS{}
	target := fstest.MapFS{}
	names, err := fs.Glob(dbmigrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		raw, err := fs.ReadFile(dbmigrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		target[name] = &fstest.MapFile{Data: raw}
		if name < "000068_evidence_final_output.sql" {
			before[name] = &fstest.MapFile{Data: raw}
		}
	}
	s, pool := freshStoreSchema(t, before)
	ctx := context.Background()
	ready := readyFixture(t, s, newUUIDv4must(t), "legacy-ready")
	orphan, _ := finalVerifyingFixture(t, s)
	applied, err := migrate.Apply(ctx, pool.Config().ConnString(), target)
	if err != nil || len(applied) != 1 || applied[0] != "000068" {
		t.Fatal(applied, err)
	}
	row, err := evidence.New(pool).GetFinalOutputForUpdate(ctx, ready.FinalKey)
	if err != nil || row.State != "READY" || row.SanitizedSha256 != ready.SanitizedSHA256 {
		t.Fatal(row, err)
	}
	row, err = evidence.New(pool).GetFinalOutputForUpdate(ctx, "f/"+orphan.ClaimedSHA256)
	if err != nil || row.State != "PENDING" || uuidString(row.SessionID) != orphan.ID {
		t.Fatal("legacy potential orphan lost", row, err)
	}
	if applied, err := migrate.Apply(ctx, pool.Config().ConnString(), target); err != nil || len(applied) != 0 {
		t.Fatal(applied, err)
	}
}

func TestFinalOutputDeadlineIdentityAndAttemptBudget(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	sess, obj := finalVerifyingFixture(t, s)
	for _, key := range []string{"f/" + strings.Repeat("c", 64), "q/invalid"} {
		if _, err := s.BeginVerificationWrite(ctx, sess.ID, key, obj.SanitizedSHA256, newUUIDv4must(t)); err == nil {
			t.Fatal("foreign destination admitted", key)
		}
	}
	for i := 0; i < 5; i++ {
		if _, err := s.BeginVerificationWrite(ctx, sess.ID, obj.FinalKey, obj.SanitizedSHA256, newUUIDv4must(t)); err != nil {
			t.Fatal(i, err)
		}
		// Isolate the admission counter. Real cleanup-to-PENDING recovery is B3.2b.
		if _, err := pool.Exec(ctx, `UPDATE evidence_final_outputs SET state='PENDING',lease_nonce=NULL,lease_until=NULL`); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.BeginVerificationWrite(ctx, sess.ID, obj.FinalKey, obj.SanitizedSHA256, newUUIDv4must(t)); !errors.Is(err, domain.ErrBadTransition) {
		t.Fatal("sixth write admitted", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE evidence_final_outputs SET attempts=0,deadline_at=now()+interval '30 seconds'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginVerificationWrite(ctx, sess.ID, obj.FinalKey, obj.SanitizedSHA256, newUUIDv4must(t)); !errors.Is(err, domain.ErrBadTransition) {
		t.Fatal("lease beyond deadline admitted", err)
	}
	row, err := evidence.New(pool).GetFinalOutputForUpdate(ctx, obj.FinalKey)
	if err != nil || row.Attempts != 0 {
		t.Fatal("refusal charged another attempt", row, err)
	}
}
