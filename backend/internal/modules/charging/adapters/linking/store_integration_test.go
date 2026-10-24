//go:build integration

package linking

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("explicit disposable PostGIS DSN required")
	}
	admin, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("linking_%d", time.Now().UnixNano())
	if _, err := admin.Exec(context.Background(), "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
		_ = admin.Close(context.Background())
	})
	u, _ := url.Parse(dsn)
	u.Path = "/" + name
	return u.String()
}

func freshDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testDSN(t)
	if _, err := migrate.Apply(context.Background(), dsn, dbmigrations.Files); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

const (
	stationA = "11111111-1111-4111-8111-111111111111"
	stationB = "22222222-2222-4222-8222-222222222222"
	batchID  = "33333333-3333-4333-8333-333333333333"
	placeID  = "44444444-4444-4444-8444-444444444444"
)

func seed(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	for i, station := range []string{stationA, stationB} {
		if _, err := pool.Exec(ctx, `INSERT INTO directory_stations(id,display_name,municipality_code,state)
			VALUES($1,$2,'3550308','SP')`, station, fmt.Sprintf("Station %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO charging_batches(id,format,profile_id,source_revision,manifest_sha256,input_count,accepted_count)
		VALUES($1,'charging-prepared-v1','test-profile','rev-1',repeat('a',64),1,1)`, batchID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO charging_places(id,batch_id,entity_key,external_id,name,country,latitude,longitude,public_access,reported_points)
		VALUES($1,$2,repeat('b',64),'place-ext','Test place','BR','-23.5505','-46.6333','PUBLIC',1)`, placeID, batchID); err != nil {
		t.Fatal(err)
	}
}

func TestReviewedLinkSupersessionAndFailureRollback(t *testing.T) {
	pool := freshDB(t)
	seed(t, pool)
	ctx := context.Background()
	store := Store{Pool: pool}

	if _, found, err := store.Active(ctx, placeID); err != nil || found {
		t.Fatalf("unreviewed link appeared: found=%v err=%v", found, err)
	}
	first, err := store.Review(ctx, Link{PlaceID: placeID, StationID: stationA, State: Reviewed, Method: MethodManual, ReviewerRef: "reviewer-1"})
	if err != nil {
		t.Fatal(err)
	}
	active, found, err := store.Active(ctx, placeID)
	if err != nil || !found || active.StationID != stationA || active.State != Reviewed {
		t.Fatalf("active link %+v found=%v err=%v", active, found, err)
	}
	if _, err := store.Review(ctx, Link{PlaceID: placeID, StationID: stationB, State: Reviewed, Method: MethodManual, ReviewerRef: "reviewer-2"}); err != nil {
		t.Fatal(err)
	}
	active, found, err = store.Active(ctx, placeID)
	if err != nil || !found || active.StationID != stationB {
		t.Fatalf("superseding link %+v found=%v err=%v", active, found, err)
	}
	var superseded int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM charging_place_station_links WHERE place_id=$1 AND superseded_by IS NOT NULL`, placeID).Scan(&superseded); err != nil || superseded != 1 {
		t.Fatalf("history superseded=%d err=%v", superseded, err)
	}
	missing := "99999999-9999-4999-8999-999999999999"
	if _, err := store.Review(ctx, Link{PlaceID: placeID, StationID: missing, State: Reviewed, Method: MethodManual, ReviewerRef: "reviewer-3"}); err == nil {
		t.Fatal("missing station accepted")
	}
	active, found, err = store.Active(ctx, placeID)
	if err != nil || !found || active.StationID != stationB {
		t.Fatalf("rollback lost active link: %+v found=%v err=%v", active, found, err)
	}
	if active.ID == first {
		t.Fatal("active id did not change")
	}
	if _, err := store.Review(ctx, Link{PlaceID: placeID, StationID: stationA, State: Rejected, Method: MethodAddressName, ReviewerRef: "reviewer-4"}); err != nil {
		t.Fatal(err)
	}
	active, found, err = store.Active(ctx, placeID)
	if err != nil || !found || active.State != Rejected {
		t.Fatalf("rejected decision %+v found=%v err=%v", active, found, err)
	}
}

func TestConcurrentReviewsKeepOneActiveLink(t *testing.T) {
	pool := freshDB(t)
	seed(t, pool)
	store := Store{Pool: pool}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			station := stationA
			if index%2 == 1 {
				station = stationB
			}
			_, err := store.Review(context.Background(), Link{PlaceID: placeID, StationID: station, State: Reviewed, Method: MethodManual, ReviewerRef: fmt.Sprintf("reviewer-%d", index)})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var total, active int
	if err := pool.QueryRow(context.Background(), `SELECT count(*), count(*) FILTER (WHERE superseded_by IS NULL) FROM charging_place_station_links WHERE place_id=$1`, placeID).Scan(&total, &active); err != nil {
		t.Fatal(err)
	}
	if total != 8 || active != 1 {
		t.Fatalf("links total=%d active=%d", total, active)
	}
}

func TestUpgradeFromPreviousSchemaPreservesExistingData(t *testing.T) {
	dsn := testDSN(t)
	ctx := context.Background()
	previous := fstest.MapFS{}
	files, err := fs.Glob(dbmigrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if name == "000121_charging_identity_links.sql" {
			continue
		}
		data, err := fs.ReadFile(dbmigrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		previous[name] = &fstest.MapFile{Data: data}
	}
	if _, err := migrate.Apply(ctx, dsn, previous); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	seed(t, pool)
	applied, err := migrate.Apply(ctx, dsn, dbmigrations.Files)
	if err != nil || len(applied) != 1 || applied[0] != "000121" {
		t.Fatalf("upgrade %v %v", applied, err)
	}
	var name string
	if err := pool.QueryRow(ctx, `SELECT name FROM charging_places WHERE id=$1`, placeID).Scan(&name); err != nil || name != "Test place" {
		t.Fatalf("existing place changed %q %v", name, err)
	}
	again, err := migrate.Apply(ctx, dsn, dbmigrations.Files)
	if err != nil || len(again) != 0 {
		t.Fatalf("replay %v %v", again, err)
	}
}

func TestAccessPointsAreSeparateAndSupersededPerKind(t *testing.T) {
	pool := freshDB(t)
	seed(t, pool)
	ctx := context.Background()
	store := Store{Pool: pool}
	display, err := store.RecordAccessPoint(ctx, AccessPoint{StationID: stationA, Kind: Display, Longitude: -46.6333, Latitude: -23.5505, Quality: "reviewed", SourceReference: "manual", ReviewerRef: "reviewer-1"})
	if err != nil {
		t.Fatal(err)
	}
	entrance, err := store.RecordAccessPoint(ctx, AccessPoint{StationID: stationA, Kind: Entrance, Longitude: -46.6335, Latitude: -23.5507, Quality: "reviewed", SourceReference: "manual", ReviewerRef: "reviewer-1"})
	if err != nil {
		t.Fatal(err)
	}
	points, err := store.ActiveAccessPoints(ctx, stationA)
	if err != nil || len(points) != 2 {
		t.Fatalf("points %+v err=%v", points, err)
	}
	newEntrance, err := store.RecordAccessPoint(ctx, AccessPoint{StationID: stationA, Kind: Entrance, Longitude: -46.6336, Latitude: -23.5508, Quality: "osm-mapped", SourceReference: "osm-node-1", ReviewerRef: "reviewer-2"})
	if err != nil {
		t.Fatal(err)
	}
	points, err = store.ActiveAccessPoints(ctx, stationA)
	if err != nil || len(points) != 2 {
		t.Fatalf("points after supersede %+v err=%v", points, err)
	}
	if points[0].ID != newEntrance && points[1].ID != newEntrance {
		t.Fatal("entrance not superseded")
	}
	_ = display
	_ = entrance
	for _, invalid := range []AccessPoint{
		{StationID: stationA, Kind: "SIDEWALK", Longitude: 0, Latitude: 0, Quality: "reviewed", SourceReference: "x", ReviewerRef: "r"},
		{StationID: stationA, Kind: Display, Longitude: 0, Latitude: 0, Quality: "guessed", SourceReference: "x", ReviewerRef: "r"},
		{StationID: stationA, Kind: Display, Longitude: 181, Latitude: 0, Quality: "reviewed", SourceReference: "x", ReviewerRef: "r"},
	} {
		if _, err := store.RecordAccessPoint(ctx, invalid); err == nil {
			t.Fatalf("invalid access point accepted: %+v", invalid)
		}
	}
	if _, err := store.RecordAccessPoint(ctx, AccessPoint{StationID: "99999999-9999-4999-8999-999999999999", Kind: Display, Longitude: 0, Latitude: 0, Quality: "reviewed", SourceReference: "x", ReviewerRef: "r"}); err == nil {
		t.Fatal("missing station accepted")
	}
}
