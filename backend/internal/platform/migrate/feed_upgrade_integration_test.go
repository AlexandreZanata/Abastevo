//go:build integration

package migrate

import (
	"context"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
)

func TestFeedIndexUpgradePreservesFactsAndOldReads(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	dsn := freshDB(t, testDSN(t))
	previous := fstest.MapFS{}
	files, err := fs.Glob(dbmigrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if name == "000041_community_feed_indexes.sql" {
			continue
		}
		data, err := fs.ReadFile(dbmigrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		previous[name] = &fstest.MapFile{Data: data}
	}
	if _, err := Apply(ctx, dsn, previous); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `INSERT INTO directory_stations(id,display_name,state,municipality_code) VALUES ('00000000-0000-0000-0000-000000000001','Synthetic migration station','MT','5107925');
 INSERT INTO community_current_prices(station_id,fuel_product,unit,condition_kind,qualifier_key,amount_milli_brl,availability,confidence,anchor_received_at,expires_at,algorithm_version,policy_config_version) VALUES ('00000000-0000-0000-0000-000000000001','ETHANOL','L','STANDARD','STANDARD',3981,'AVAILABLE','LOW',now(),now()+interval '24 hours','test','test')`); err != nil {
		t.Fatal(err)
	}
	applied, err := Apply(ctx, dsn, dbmigrations.Files)
	if err != nil || len(applied) != 1 || applied[0] != "000041" {
		t.Fatalf("upgrade %v %v", applied, err)
	}
	var amount int64
	if err := conn.QueryRow(ctx, `SELECT amount_milli_brl FROM community_current_prices WHERE station_id='00000000-0000-0000-0000-000000000001' AND fuel_product='ETHANOL'`).Scan(&amount); err != nil || amount != 3981 {
		t.Fatalf("old read changed %d %v", amount, err)
	}
	var indexes int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE schemaname='public' AND indexname IN ('community_feed_recent_idx','community_feed_cheapest_idx')`).Scan(&indexes); err != nil || indexes != 2 {
		t.Fatalf("indexes %d %v", indexes, err)
	}
	again, err := Apply(ctx, dsn, dbmigrations.Files)
	if err != nil || len(again) != 0 {
		t.Fatalf("replay %v %v", again, err)
	}
}
