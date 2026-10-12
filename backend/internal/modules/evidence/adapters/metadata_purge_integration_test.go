//go:build integration

package adapters

import (
	"context"
	"io/fs"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func TestMetadataPurgeRetainsUnconfirmedCopies(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	obj := readyFixture(t, s, "e0000000-0000-4000-8000-000000000001", "ancient-copy")
	backdateObject(t, pool, obj.ID, time.Now().Add(-91*24*time.Hour))
	cutoff := time.Now().Add(-90 * 24 * time.Hour)
	assertRetained := func() {
		t.Helper()
		if n, err := s.PurgeObjectsBefore(ctx, cutoff); err != nil || n != 0 {
			t.Fatalf("unconfirmed copy lost its inventory: %d, %v", n, err)
		}
	}
	assertRetained()
	if err := s.MarkFinalDeleted(ctx, obj.ID); err != nil {
		t.Fatal(err)
	}
	assertRetained() // An old marker without output inventory is insufficient.
	if _, err := pool.Exec(ctx, `INSERT INTO evidence_final_outputs(final_key,session_id,sanitized_sha256,state,deadline_at)
 SELECT final_key,session_id,sanitized_sha256,'PURGING',created_at+interval '24 hours' FROM evidence_objects WHERE id=$1`, obj.ID); err == nil {
		t.Fatal("PURGING without cleanup authority unexpectedly accepted")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO evidence_final_outputs(final_key,session_id,sanitized_sha256,state,deadline_at)
 SELECT final_key,session_id,sanitized_sha256,'PENDING',created_at+interval '24 hours' FROM evidence_objects WHERE id=$1`, obj.ID); err != nil {
		t.Fatal(err)
	}
	assertRetained()
	if _, err := pool.Exec(ctx, `UPDATE evidence_final_outputs SET state='PURGED',purged_at=now() WHERE final_key=$1`, obj.FinalKey); err != nil {
		t.Fatal(err)
	}
	if n, err := NewStore(pool).PurgeObjectsBefore(ctx, cutoff); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	var receipts int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence_final_outputs WHERE final_key=$1`, obj.FinalKey).Scan(&receipts); err != nil || receipts != 1 {
		t.Fatal("hash purge removed cleanup authority", receipts, err)
	}
}

func TestMetadataPurgeBoundedRestartRollbackAndPlan(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	sess, _ := finalVerifyingFixture(t, s)
	if _, err := pool.Exec(ctx, `INSERT INTO evidence_objects(id,session_id,final_key,source_sha256,sanitized_sha256,width,height,dhash,created_at,final_deleted_at)
 SELECT md5('purged-'||n)::uuid,$1,'f/purged-'||n,'','',1,1,0,now()-interval '91 days',now() FROM generate_series(1,205) n;
 `, sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO evidence_final_outputs(final_key,session_id,sanitized_sha256,state,deadline_at,purged_at)
 SELECT final_key,session_id,'','PURGED',created_at+interval '24 hours',now() FROM evidence_objects;
 INSERT INTO evidence_objects(id,session_id,final_key,source_sha256,sanitized_sha256,width,height,dhash,created_at)
 SELECT md5('live-'||n)::uuid,(SELECT session_id FROM evidence_objects LIMIT 1),'f/live-'||n,'','',1,1,0,now()-interval '91 days' FROM generate_series(1,10000) n;
 ANALYZE evidence_objects; ANALYZE evidence_final_outputs;`); err != nil {
		t.Fatal(err)
	}
	cutoff := time.Now().Add(-90 * 24 * time.Hour)
	raw, err := os.ReadFile("../../../../db/queries/evidence/evidence.sql")
	if err != nil {
		t.Fatal(err)
	}
	query := strings.Split(strings.Split(string(raw), "-- name: PurgeObjectsBefore :execrows\n")[1], ";\n")[0]
	query = strings.ReplaceAll(query, "@cutoff", "$1")
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := tx.Query(ctx, "EXPLAIN (ANALYZE, BUFFERS) "+query, cutoff)
	if err != nil {
		_ = tx.Rollback(ctx)
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
	if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
		t.Fatal(rollbackErr)
	}
	if err != nil || !strings.Contains(plan.String(), "evidence_objects_confirmed_purge_idx") || strings.Contains(plan.String(), "Seq Scan on evidence_objects") {
		t.Fatal(plan.String(), err)
	}
	if _, err := pool.Exec(ctx, `CREATE FUNCTION fail_hash_purge() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic purge fault'; END $$;
 CREATE TRIGGER fail_hash_purge BEFORE DELETE ON evidence_objects FOR EACH ROW EXECUTE FUNCTION fail_hash_purge()`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PurgeObjectsBefore(ctx, cutoff); err == nil {
		t.Fatal("failed purge acknowledged")
	}
	var pending int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence_objects WHERE final_deleted_at IS NOT NULL`).Scan(&pending); err != nil || pending != 205 {
		t.Fatal("purge partially committed", pending, err)
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER fail_hash_purge ON evidence_objects`); err != nil {
		t.Fatal(err)
	}
	for _, want := range []int64{100, 100, 5, 0} {
		if n, err := NewStore(pool).PurgeObjectsBefore(ctx, cutoff); err != nil || n != want {
			t.Fatal(n, err, "want", want)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO evidence_objects(id,session_id,final_key,source_sha256,sanitized_sha256,width,height,dhash,created_at,final_deleted_at)
 SELECT md5('purged-'||n)::uuid,$1,'f/purged-'||n,'','',1,1,0,now()-interval '91 days',now() FROM generate_series(1,205) n`, sess.ID); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	counts := make(chan int64, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := NewStore(pool).PurgeObjectsBefore(ctx, cutoff)
			if err != nil || n > 100 {
				t.Error(n, err)
			}
			counts <- n
		}()
	}
	wg.Wait()
	close(counts)
	var total int64
	for n := range counts {
		total += n
	}
	if total != 205 {
		t.Fatal("concurrent purge lost or double counted rows", total)
	}
}

func TestMetadataPurgeIndexUpgradePreservesUnconfirmedCopy(t *testing.T) {
	before, target := fstest.MapFS{}, fstest.MapFS{}
	names, err := fs.Glob(dbmigrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if name > "000070_evidence_confirmed_hash_purge.sql" {
			continue
		}
		raw, err := fs.ReadFile(dbmigrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		target[name] = &fstest.MapFile{Data: raw}
		if name < "000070_evidence_confirmed_hash_purge.sql" {
			before[name] = &fstest.MapFile{Data: raw}
		}
	}
	s, pool := freshStoreSchema(t, before)
	ctx := context.Background()
	obj := readyFixture(t, s, "e0000000-0000-4000-8000-000000000001", "upgrade-copy")
	backdateObject(t, pool, obj.ID, time.Now().Add(-91*24*time.Hour))
	if applied, err := migrate.Apply(ctx, pool.Config().ConnString(), target); err != nil || len(applied) != 1 || applied[0] != "000070" {
		t.Fatal(applied, err)
	}
	if n, err := s.PurgeObjectsBefore(ctx, time.Now().Add(-90*24*time.Hour)); err != nil || n != 0 {
		t.Fatal("upgrade lost unconfirmed copy", n, err)
	}
	if applied, err := migrate.Apply(ctx, pool.Config().ConnString(), target); err != nil || len(applied) != 0 {
		t.Fatal(applied, err)
	}
}
