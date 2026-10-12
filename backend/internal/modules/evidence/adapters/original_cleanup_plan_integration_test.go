//go:build integration

package adapters

import (
	"context"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func TestOriginalCleanupActualQueryUsesHotIndexAndExcludesLiveUpload(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	sess := testSession(newUUIDv4must(t), newUUIDv4must(t))
	if _, _, err := s.ReserveSession(ctx, sess, openBudget()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ClaimUpload(ctx, sess.ID, "still-writing", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE evidence_sessions SET status='EXPIRED',created_at=now()-interval '3 days' WHERE id=$1`, sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO evidence_sessions(id,contributor_ref,client_session_id,mime,declared_bytes,claimed_sha256,quarantine_key,status,created_at,expires_at,policy_version,quarantine_deleted_at)
 SELECT md5('source-'||n)::uuid,'synthetic-source','source-'||n,'image/jpeg',1,'','q/source-'||n,'EXPIRED',now()-interval '2 days',now()-interval '1 day','evidence-v1',CASE WHEN n<=10000 THEN now() ELSE NULL END
 FROM generate_series(1,10065) n;
 ANALYZE evidence_sessions; ANALYZE evidence_upload_transfers;`); err != nil {
		t.Fatal(err)
	}
	cands, err := s.QuarantineCandidates(ctx, time.Now().Add(-22*time.Hour), 32)
	if err != nil || len(cands) != 32 {
		t.Fatal(len(cands), err)
	}
	for i, c := range cands {
		if c.ID == sess.ID {
			t.Fatal("live writer consumed eligible batch")
		}
		if i > 0 && c.ID < cands[i-1].ID {
			t.Fatal("unstable tied source order")
		}
	}
	raw, err := os.ReadFile("../../../../db/queries/evidence/evidence.sql")
	if err != nil {
		t.Fatal(err)
	}
	query := strings.Split(strings.Split(string(raw), "-- name: QuarantineCandidates :many\n")[1], ";\n")[0]
	query = strings.ReplaceAll(strings.ReplaceAll(query, "@old_cutoff", "$1"), "@batch", "$2")
	rows, err := pool.Query(ctx, "EXPLAIN (ANALYZE, BUFFERS) "+query, time.Now().Add(-22*time.Hour), 32)
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
	if err != nil || !strings.Contains(plan.String(), "evidence_original_cleanup_idx") || strings.Contains(plan.String(), "Seq Scan on evidence_sessions") {
		t.Fatal(plan.String(), err)
	}
}

func TestOriginalCleanupIndexOnlyUpgradeRetainsLiveLease(t *testing.T) {
	before, target := fstest.MapFS{}, fstest.MapFS{}
	names, err := fs.Glob(dbmigrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if name > "000071_evidence_original_cleanup_index.sql" {
			continue
		}
		raw, err := fs.ReadFile(dbmigrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		target[name] = &fstest.MapFile{Data: raw}
		if name < "000071_evidence_original_cleanup_index.sql" {
			before[name] = &fstest.MapFile{Data: raw}
		}
	}
	s, pool := freshStoreSchema(t, before)
	ctx := context.Background()
	sess := testSession(newUUIDv4must(t), newUUIDv4must(t))
	if _, _, err := s.ReserveSession(ctx, sess, openBudget()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ClaimUpload(ctx, sess.ID, "upgrade-live", time.Now()); err != nil {
		t.Fatal(err)
	}
	if applied, err := migrate.Apply(ctx, pool.Config().ConnString(), target); err != nil || len(applied) != 1 || applied[0] != "000071" {
		t.Fatal(applied, err)
	}
	if rows, err := s.ExpireIdleSessions(ctx, sess.CreatedAt.Add(22*time.Hour), 1); err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	if allowed, err := s.CanDeleteQuarantine(ctx, sess.ID); err != nil || allowed {
		t.Fatal("upgrade lost writer fence", allowed, err)
	}
	if applied, err := migrate.Apply(ctx, pool.Config().ConnString(), target); err != nil || len(applied) != 0 {
		t.Fatal(applied, err)
	}
}
