//go:build integration

package jobs

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

func TestSafetyClaimPlanSkipsUnrelatedQueueHistory(t *testing.T) {
	_, pool := freshQueue(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO job_queue(id,kind,payload,status,created_at)
 SELECT md5('queue-history-'||n)::uuid,'general-import','{"version":1}',CASE WHEN n<=10000 THEN 'done' ELSE 'queued' END,now()-interval '1 day' FROM generate_series(1,20000) n;
 ANALYZE job_queue;`); err != nil {
		t.Fatal(err)
	}
	enqueueRaw(t, pool, "evidence-sweep", `{"version":1}`, "safety-index")
	raw, err := os.ReadFile("../../../db/queries/platform/jobs.sql")
	if err != nil {
		t.Fatal(err)
	}
	query := strings.Split(strings.Split(string(raw), "-- name: ClaimJob :one\n")[1], ";\n")[0]
	for from, to := range map[string]string{"@lease_ttl": "$1", "@worker_id": "$2", "@kind": "$3", "@excluded_kinds": "$4"} {
		query = strings.ReplaceAll(query, from, to)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := tx.Query(ctx, "EXPLAIN (ANALYZE, BUFFERS) "+query, "1 minute", "safety-plan", "evidence-sweep", []string{})
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
	if err != nil || !strings.Contains(plan.String(), "job_queue_live_kind_created_idx") || strings.Contains(plan.String(), "Seq Scan on job_queue") {
		t.Fatal(plan.String(), err)
	}
}

func TestSafetyClaimIndexOnlyUpgradePreservesQueueLeases(t *testing.T) {
	before, target := fstest.MapFS{}, fstest.MapFS{}
	names, err := fs.Glob(dbmigrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if name > "000072_safety_job_claim_index.sql" {
			continue
		}
		raw, err := fs.ReadFile(dbmigrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		target[name] = &fstest.MapFile{Data: raw}
		if name < "000072_safety_job_claim_index.sql" {
			before[name] = &fstest.MapFile{Data: raw}
		}
	}
	q, pool := freshQueueSchema(t, before)
	ctx := context.Background()
	enqueueRaw(t, pool, "evidence-sweep", `{"version":1}`, "live-upgrade")
	enqueueRaw(t, pool, "general-import", `{"version":1}`, "queued-upgrade")
	job, err := q.Claim(ctx, "evidence-sweep", "upgrade-owner", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if applied, err := migrate.Apply(ctx, pool.Config().ConnString(), target); err != nil || len(applied) != 1 || applied[0] != "000072" {
		t.Fatal(applied, err)
	}
	if err := q.Complete(ctx, job.ID, job.LeaseToken); err != nil {
		t.Fatal("upgrade invalidated live lease", err)
	}
	if returned, err := Enqueue(ctx, pool, "evidence-sweep", []byte(`{"version":1}`), "live-upgrade", 1, time.Time{}); err != nil || returned != job.ID {
		t.Fatal("upgrade forked dedupe", returned, err)
	}
	if foreign, err := q.claim(ctx, "", "general", time.Minute, []string{"evidence-sweep"}); err != nil || foreign.Kind != "general-import" {
		t.Fatal(foreign, err)
	}
	if applied, err := migrate.Apply(ctx, pool.Config().ConnString(), target); err != nil || len(applied) != 0 {
		t.Fatal(applied, err)
	}
}
