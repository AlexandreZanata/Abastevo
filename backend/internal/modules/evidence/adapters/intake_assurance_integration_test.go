//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	evidence "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/evidence"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func recordHealthySweep(t *testing.T, s *Store) {
	t.Helper()
	epoch, err := s.BeginSweepObservation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FinishSweepObservation(context.Background(), epoch, true); err != nil {
		t.Fatal(err)
	}
}

func TestIntakeObservationFencesStaleUnknownFailedAndLateAcks(t *testing.T) {
	_, pool := freshStore(t)
	s := NewIntakeStore(pool, 4)
	ctx := context.Background()
	old, err := s.BeginSweepObservation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	latest, err := s.BeginSweepObservation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FinishSweepObservation(ctx, latest, false); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishSweepObservation(ctx, old, true); !errors.Is(err, domain.ErrBadTransition) {
		t.Fatal("old pass reopened intake", err)
	}
	if err := s.CheckIntake(ctx); !errors.Is(err, application.ErrEnforcementUnhealthy) {
		t.Fatal(err)
	}
	recordHealthySweep(t, s)
	if err := s.CheckIntake(ctx); err != nil {
		t.Fatal("verified recovery stayed closed", err)
	}
	running, err := s.BeginSweepObservation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CheckIntake(ctx); !errors.Is(err, application.ErrEnforcementUnhealthy) {
		t.Fatal("running pass retained old healthy authorization", err)
	}
	if err := s.FinishSweepObservation(ctx, running, true); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishSweepObservation(ctx, running, false); !errors.Is(err, domain.ErrBadTransition) {
		t.Fatal("duplicate acknowledgement changed latest outcome", err)
	}
	for _, delta := range []string{"-4 minutes", "1 minute"} {
		if _, err := pool.Exec(ctx, `UPDATE evidence_sweep_observation SET started_at=now()+$1::interval,finished_at=now()+$1::interval,last_success_at=now()+$1::interval`, delta); err != nil {
			t.Fatal(err)
		}
		if err := s.CheckIntake(ctx); !errors.Is(err, application.ErrEnforcementUnhealthy) {
			t.Fatal("stale/future observation accepted", delta, err)
		}
	}
	epoch, err := s.BeginSweepObservation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE evidence_sweep_observation SET started_at=now()-interval '2 minutes'`); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishSweepObservation(ctx, epoch, true); !errors.Is(err, domain.ErrBadTransition) {
		t.Fatal("late success became fresh heartbeat", err)
	}
	if err := s.FinishSweepObservation(ctx, epoch, false); err != nil {
		t.Fatal(err)
	}
	epoch, err = s.BeginSweepObservation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE FUNCTION fail_health_receipt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.healthy THEN RAISE EXCEPTION 'synthetic receipt fault'; END IF; RETURN NEW; END $$;
 CREATE TRIGGER fail_health_receipt BEFORE UPDATE ON evidence_sweep_observation FOR EACH ROW EXECUTE FUNCTION fail_health_receipt()`); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishSweepObservation(ctx, epoch, true); err == nil {
		t.Fatal("failed receipt acknowledged")
	}
	if err := s.CheckIntake(ctx); !errors.Is(err, application.ErrEnforcementUnhealthy) {
		t.Fatal("receipt rollback left intake open", err)
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER fail_health_receipt ON evidence_sweep_observation`); err != nil {
		t.Fatal(err)
	}
	recordHealthySweep(t, s)
	if _, err := pool.Exec(ctx, `DROP TABLE evidence_sweep_observation`); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckIntake(ctx); !errors.Is(err, application.ErrEnforcementUnhealthy) {
		t.Fatal("observation query outage became healthy", err)
	}
}

func TestIntakeNearDeadlineRefusesEveryNamespace(t *testing.T) {
	for _, namespace := range []string{"original", "final", "unconfirmed"} {
		t.Run(namespace, func(t *testing.T) {
			trusted, pool := freshStore(t)
			s := NewIntakeStore(pool, 4)
			ctx := context.Background()
			obj := readyFixture(t, trusted, newUUIDv4must(t), newUUIDv4must(t))
			if namespace != "original" {
				if _, err := pool.Exec(ctx, `UPDATE evidence_sessions SET quarantine_deleted_at=now()`); err != nil {
					t.Fatal(err)
				}
			}
			switch namespace {
			case "original":
				if _, err := pool.Exec(ctx, `UPDATE evidence_sessions SET created_at=now()-interval '22 hours'`); err != nil {
					t.Fatal(err)
				}
			case "final":
				if _, err := pool.Exec(ctx, `UPDATE evidence_objects SET received_at=now()-interval '22 hours'`); err != nil {
					t.Fatal(err)
				}
			case "unconfirmed":
				if _, err := pool.Exec(ctx, `INSERT INTO evidence_final_outputs(final_key,session_id,sanitized_sha256,state,deadline_at)
 SELECT final_key,session_id,sanitized_sha256,'PENDING',now()+interval '2 hours'-interval '1 second' FROM evidence_objects WHERE id=$1`, obj.ID); err != nil {
					t.Fatal(err)
				}
				if err := trusted.MarkFinalDeleted(ctx, obj.ID); err != nil {
					t.Fatal(err)
				}
			}
			recordHealthySweep(t, s)
			if err := s.CheckIntake(ctx); !errors.Is(err, application.ErrEnforcementUnhealthy) {
				t.Fatal("unsafe boundary did not stop intake", err)
			}
		})
	}
}

func TestIntakeCapacityRetainsUnconfirmedAndLegacyCopies(t *testing.T) {
	trusted, pool := freshStore(t)
	s := NewIntakeStore(pool, 1)
	ctx := context.Background()
	recordHealthySweep(t, s)
	sess := testSession(newUUIDv4must(t), newUUIDv4must(t))
	if _, _, err := s.ReserveSession(ctx, sess, openBudget()); err != nil {
		t.Fatal(err)
	}
	var queued [][]byte
	if err := trusted.CompleteSession(ctx, sess.ID, enqueueStub(&queued)); err != nil {
		t.Fatal(err)
	}
	obj := testObject(sess.ID)
	obj.FinalKey = "f/" + sess.ClaimedSHA256
	if _, err := trusted.BeginVerificationWrite(ctx, sess.ID, obj.FinalKey, obj.SanitizedSHA256, newUUIDv4must(t)); err != nil {
		t.Fatal(err)
	}
	if err := trusted.MarkQuarantineDeleted(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	other := testSession(newUUIDv4must(t), newUUIDv4must(t))
	other.ContributorRef = "different-capacity-owner"
	if _, _, err := s.ReserveSession(ctx, other, openBudget()); !errors.Is(err, application.ErrEnforcementUnhealthy) {
		t.Fatal("original marker refunded unresolved final", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE evidence_final_outputs SET lease_until=now()-interval '1 second'; UPDATE evidence_sessions SET status='EXPIRED'`); err != nil {
		t.Fatal(err)
	}
	if rep, err := trusted.ReconcileFinalOutputs(ctx, time.Now(), 32, func(context.Context, string) error { return nil }); err != nil || rep.Purged != 1 {
		t.Fatal(rep, err)
	}
	if _, _, err := s.ReserveSession(ctx, other, openBudget()); err != nil {
		t.Fatal("confirmed cleanup did not release capacity", err)
	}
	if err := trusted.MarkQuarantineDeleted(ctx, other.ID); err != nil {
		t.Fatal(err)
	}
	legacy := readyFixture(t, trusted, newUUIDv4must(t), newUUIDv4must(t))
	if _, err := pool.Exec(ctx, `UPDATE evidence_sessions SET quarantine_deleted_at=now()`); err != nil {
		t.Fatal(err)
	}
	newPhoto := testSession(newUUIDv4must(t), newUUIDv4must(t))
	if _, _, err := s.ReserveSession(ctx, newPhoto, openBudget()); !errors.Is(err, application.ErrEnforcementUnhealthy) {
		t.Fatal("legacy final escaped capacity", legacy.ID, err)
	}
}

func TestIntakeCapacityBoundedPlanAndUpgrade(t *testing.T) {
	before, target := fstest.MapFS{}, fstest.MapFS{}
	names, err := fs.Glob(dbmigrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if name > "000073_evidence_intake_assurance.sql" {
			continue
		}
		raw, err := fs.ReadFile(dbmigrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		target[name] = &fstest.MapFile{Data: raw}
		if name < "000073_evidence_intake_assurance.sql" {
			before[name] = &fstest.MapFile{Data: raw}
		}
	}
	trusted, pool := freshStoreSchema(t, before)
	ctx := context.Background()
	sess := testSession(newUUIDv4must(t), newUUIDv4must(t))
	if _, _, err := trusted.ReserveSession(ctx, sess, openBudget()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := trusted.ClaimUpload(ctx, sess.ID, "upgrade-transfer", time.Now()); err != nil {
		t.Fatal(err)
	}
	if applied, err := migrate.Apply(ctx, pool.Config().ConnString(), target); err != nil || len(applied) != 1 || applied[0] != "000073" {
		t.Fatal(applied, err)
	}
	s := NewIntakeStore(pool, 4)
	if err := s.CheckIntake(ctx); !errors.Is(err, application.ErrEnforcementUnhealthy) {
		t.Fatal("upgrade fabricated healthy observation", err)
	}
	if err := trusted.FinishUpload(ctx, sess.ID, "upgrade-transfer", true, 9); err != nil {
		t.Fatal("upgrade lost transfer authority", err)
	}
	if applied, err := migrate.Apply(ctx, pool.Config().ConnString(), target); err != nil || len(applied) != 0 {
		t.Fatal(applied, err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO evidence_final_outputs(final_key,session_id,sanitized_sha256,state,deadline_at,purged_at)
 SELECT 'f/capacity-history-'||n,$1,'','PURGED',now()-interval '1 day',now() FROM generate_series(1,10000) n`, sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO evidence_objects(id,session_id,final_key,source_sha256,sanitized_sha256,width,height,dhash,created_at,final_deleted_at)
 SELECT md5('capacity-history-'||n)::uuid,$1,'f/capacity-history-'||n,'','',1,1,0,now()-interval '1 day',now() FROM generate_series(1,10000) n`, sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ANALYZE evidence_sessions;ANALYZE evidence_final_outputs;ANALYZE evidence_objects;`); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../../../db/queries/evidence/evidence.sql")
	if err != nil {
		t.Fatal(err)
	}
	query := strings.Split(strings.Split(string(raw), "-- name: CountPendingPhotoSessions :one\n")[1], ";\n")[0]
	query = strings.ReplaceAll(query, "@scan_limit", "$1")
	rows, err := pool.Query(ctx, "EXPLAIN (ANALYZE, BUFFERS) "+query, 5)
	if err != nil {
		t.Fatal(err)
	}
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line + "\n")
	}
	err = rows.Err()
	rows.Close()
	for _, index := range []string{"evidence_final_output_capacity_idx", "evidence_object_capacity_idx"} {
		if !strings.Contains(plan.String(), index) {
			t.Fatal(plan.String(), index)
		}
	}
	if err != nil || strings.Contains(plan.String(), "Seq Scan on evidence_objects") || strings.Contains(plan.String(), "Seq Scan on evidence_final_outputs") {
		t.Fatal(plan.String(), err)
	}
	if count, err := evidence.New(pool).CountPendingPhotoSessions(ctx, 5); err != nil || count != 1 {
		t.Fatal("terminal history affected live budget", count, err)
	}
}

func TestIntakeUnknownAndDisabledCannotReserveOrStartUpload(t *testing.T) {
	_, pool := freshStore(t)
	ctx := context.Background()
	s := NewIntakeStore(pool, 4)
	sess := testSession(newUUIDv4must(t), newUUIDv4must(t))
	if _, _, err := s.ReserveSession(ctx, sess, openBudget()); !errors.Is(err, application.ErrEnforcementUnhealthy) {
		t.Fatal("unknown cleanup allowed reservation", err)
	}
	recordHealthySweep(t, s)
	if _, _, err := NewIntakeStore(pool, 0).ReserveSession(ctx, sess, openBudget()); !errors.Is(err, application.ErrEnforcementUnhealthy) {
		t.Fatal("disabled budget admitted photo", err)
	}
	if _, _, err := s.ReserveSession(ctx, sess, openBudget()); err != nil {
		t.Fatal(err)
	}
	epoch, err := s.BeginSweepObservation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FinishSweepObservation(ctx, epoch, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ClaimUpload(ctx, sess.ID, "old-authorization", time.Now()); !errors.Is(err, application.ErrEnforcementUnhealthy) {
		t.Fatal("old URL introduced bytes after failure", err)
	}
	var attempts int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence_upload_transfers`).Scan(&attempts); err != nil || attempts != 0 {
		t.Fatal("refused ingress charged a lease", attempts, err)
	}
}

func TestIntakeGlobalCapacityAcrossOwnersIsAtomicAndReplaySafe(t *testing.T) {
	_, pool := freshStore(t)
	ctx := context.Background()
	s := NewIntakeStore(pool, 4)
	recordHealthySweep(t, s)
	var wg sync.WaitGroup
	winners := make(chan domain.Session, 40)
	for i := 0; i < 40; i++ {
		sess := testSession(newUUIDv4must(t), newUUIDv4must(t))
		sess.ContributorRef = fmt.Sprint("capacity-owner-", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, replay, err := s.ReserveSession(ctx, sess, openBudget())
			if err == nil && !replay {
				winners <- sess
			} else if !errors.Is(err, application.ErrEnforcementUnhealthy) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	close(winners)
	var admitted []domain.Session
	for sess := range winners {
		admitted = append(admitted, sess)
	}
	if len(admitted) != 4 {
		t.Fatal("global budget overshot", len(admitted))
	}
	for _, sess := range admitted {
		if got, replay, err := NewIntakeStore(pool, 4).ReserveSession(ctx, sess, openBudget()); err != nil || !replay || got.ID != sess.ID {
			t.Fatal(got, replay, err)
		}
	}
	if _, _, err := s.ClaimUpload(ctx, admitted[0].ID, "reserved-slot", time.Now()); err != nil {
		t.Fatal("full budget blocked already reserved bytes", err)
	}
}

func TestIntakeLockTimeoutIsRetryableAndDoesNotReserve(t *testing.T) {
	_, pool := freshStore(t)
	s := NewIntakeStore(pool, 4)
	ctx := context.Background()
	recordHealthySweep(t, s)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := evidence.New(tx).LockIntakeAdmission(ctx); err != nil {
		t.Fatal(err)
	}
	bounded, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	sess := testSession(newUUIDv4must(t), newUUIDv4must(t))
	if _, _, err := s.ReserveSession(bounded, sess, openBudget()); !errors.Is(err, application.ErrEnforcementUnhealthy) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("timeout became terminal or successful", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence_sessions`).Scan(&count); err != nil || count != 0 {
		t.Fatal("timeout left reservation", count, err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ReserveSession(ctx, sess, openBudget()); err != nil {
		t.Fatal("retry failed", err)
	}
}

func TestIntakeDuplicateFinalKeysCannotHideOtherSessions(t *testing.T) {
	trusted, pool := freshStore(t)
	ctx := context.Background()
	for i := 0; i < 6; i++ {
		sess := testSession(fmt.Sprintf("e0000000-0000-4000-8000-%012d", i+1), newUUIDv4must(t))
		sess.ContributorRef = fmt.Sprint("distinct-owner-", i)
		if _, _, err := trusted.ReserveSession(ctx, sess, openBudget()); err != nil {
			t.Fatal(err)
		}
		if err := trusted.MarkQuarantineDeleted(ctx, sess.ID); err != nil {
			t.Fatal(err)
		}
		keys := 1
		if i == 0 {
			keys = 35
		}
		for k := 0; k < keys; k++ {
			if _, err := pool.Exec(ctx, `INSERT INTO evidence_final_outputs(final_key,session_id,sanitized_sha256,state,deadline_at) VALUES($1,$2,'','PENDING',now()+interval '24 hours')`, fmt.Sprintf("f/distinct-%d-%d", i, k), sess.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	if count, err := evidence.New(pool).CountPendingPhotoSessions(ctx, 5); err != nil || count != 5 {
		t.Fatal("duplicate keys hid capacity", count, err)
	}
	s := NewIntakeStore(pool, 4)
	recordHealthySweep(t, s)
	if _, _, err := s.ReserveSession(ctx, testSession(newUUIDv4must(t), newUUIDv4must(t)), openBudget()); !errors.Is(err, application.ErrEnforcementUnhealthy) {
		t.Fatal("overfull inventory admitted more", err)
	}
}
