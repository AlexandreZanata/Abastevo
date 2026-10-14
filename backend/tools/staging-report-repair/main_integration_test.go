//go:build integration

package main

import (
	"context"
	"fmt"
	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"
)

func TestLegacyReportRepairDryRunAtomicAndConcurrentReplay(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("disposable database required")
	}
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("report_repair_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize())
	u, _ := url.Parse(dsn)
	u.Path = "/" + name
	if _, err := migrate.Apply(ctx, u.String(), migrations.Files); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	station := "d6c74c23-63db-4c24-a2e5-408cb23bad26"
	obs := "d6c74c23-63db-4c24-a2e5-408cb23bad27"
	for _, q := range []string{`INSERT INTO directory_stations(id,display_name) VALUES('` + station + `','Synthetic repair')`, `INSERT INTO community_observations(id,contributor_ref,client_submission_id,station_id,fuel_product,unit,amount_milli_brl,condition_kind,qualifier_key,policy_version) VALUES('` + obs + `','opaque-author','repair','` + station + `','ETHANOL','L',4500,'STANDARD','STANDARD','community-pricing-v1')`, `INSERT INTO community_disputes(id,target_observation_id,contributor_ref,client_submission_id,reason,detail,status,policy_version) VALUES(gen_random_uuid(),'` + obs + `','opaque-reporter','repair-report','WRONG_PRODUCT','synthetic detail','OPEN','community-pricing-v1')`} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if err := repair(ctx, pool, false); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM moderation_cases`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("dry-run mutated %d %v", n, err)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := repair(ctx, pool, true); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM moderation_cases WHERE target_type='OBSERVATION' AND target_id=$1 AND reason='WRONG_PRODUCT' AND status='OPEN'`, obs).Scan(&n); err != nil || n != 1 {
		t.Fatalf("repair forked/lost case %d %v", n, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE community_disputes SET detail=repeat('é',501),target_observation_id=$1`, obs); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM moderation_cases`); err != nil {
		t.Fatal(err)
	}
	if err := repair(ctx, pool, true); err == nil {
		t.Fatal("invalid historical detail accepted")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM moderation_cases`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("invalid repair partially committed %d %v", n, err)
	}
}
