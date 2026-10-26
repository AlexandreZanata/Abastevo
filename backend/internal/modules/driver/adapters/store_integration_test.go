//go:build integration

package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/driver"
	cloud "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"io/fs"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func freshDriverDB(t *testing.T, files fs.FS) (*pgxpool.Pool, string) {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	u, e := url.Parse(dsn)
	if e != nil || u.Hostname() != "127.0.0.1" || dsn == "" {
		t.Fatal("explicit disposable loopback PostGIS DSN required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	admin, e := pgx.Connect(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	name := fmt.Sprintf("driver_test_%d", time.Now().UnixNano())
	if _, e = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	admin.Close(ctx)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		a, e := pgx.Connect(ctx, dsn)
		if e != nil {
			t.Error(e)
			return
		}
		defer a.Close(ctx)
		if _, e = a.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); e != nil {
			t.Error(e)
		}
	})
	u.Path = "/" + name
	if _, e = migrate.Apply(ctx, u.String(), files); e != nil {
		t.Fatal(e)
	}
	pool, e := pgxpool.New(ctx, u.String())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	return pool, u.String()
}
func TestDriverSchemaMigration(t *testing.T) {
	p, _ := freshDriverDB(t, migrations.Files)
	var exists bool
	if e := p.QueryRow(context.Background(), "SELECT to_regclass('driver_earnings') IS NOT NULL").Scan(&exists); e != nil {
		t.Fatal(e)
	}
	if !exists {
		t.Fatal("DRV-07 private earnings schema missing after actual migration")
	}
}

func newDriverID(t *testing.T) string {
	t.Helper()
	id, e := jobs.NewUUIDv4()
	if e != nil {
		t.Fatal(e)
	}
	return id
}
func sourceHash(text string) string {
	s := sha256.Sum256([]byte(text))
	return hex.EncodeToString(s[:])
}
func seedDriverOwner(t *testing.T, p *pgxpool.Pool) string {
	t.Helper()
	id := newDriverID(t)
	if _, e := p.Exec(context.Background(), "INSERT INTO accounts(id,alias,status) VALUES($1,$2,'active')", id, "synthetic-driver-"+id); e != nil {
		t.Fatal(e)
	}
	return id
}
func seedDriverVehicle(t *testing.T, p *pgxpool.Pool, owner string) string {
	t.Helper()
	id := newDriverID(t)
	if _, e := p.Exec(context.Background(), "INSERT INTO cloud_records(account_id,dataset,id,payload) VALUES($1,'vehicles',$2,'{}')", owner, id); e != nil {
		t.Fatal(e)
	}
	return id
}
func earning(t *testing.T, vehicle string) app.SaveEarning {
	t.Helper()
	id := newDriverID(t)
	n := int64(45000)
	return app.SaveEarning{OperationID: newDriverID(t), Record: domain.EarningRecord{ID: id, VehicleID: vehicle, Platform: "UBER", SourceHash: sourceHash(id), Coverage: "SUMMARY", Currency: "BRL", From: time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC), To: time.Date(2026, 10, 26, 13, 0, 0, 0, time.UTC), Timezone: "America/Cuiaba", Money: domain.Earnings{Basis: "INCLUSIVE_NET", Net: &n}}}
}
func driverCount(t *testing.T, p *pgxpool.Pool, table string) int {
	t.Helper()
	allowed := map[string]bool{"driver_earnings": true, "driver_earning_revisions": true, "driver_earning_adjustments": true, "driver_earning_commands": true, "driver_earning_tombstones": true, "driver_earning_owner_state": true, "driver_shifts": true, "driver_shift_gaps": true, "driver_shift_commands": true, "driver_distance_observations": true, "driver_distance_commands": true, "driver_cost_profiles": true, "driver_cost_profile_items": true, "driver_cost_commands": true, "driver_cost_owner_state": true, "driver_cost_tombstones": true, "driver_maintenance_schedules": true, "driver_maintenance_commands": true, "driver_maintenance_owner_state": true, "driver_maintenance_tombstones": true, "driver_reconciliation_decisions": true, "driver_reconciliation_records": true, "driver_reconciliation_commands": true}
	if !allowed[table] {
		t.Fatal("test table not allowlisted")
	}
	var n int
	if e := p.QueryRow(context.Background(), "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}
func mustSave(t *testing.T, s *Store, owner string, c app.SaveEarning) app.Receipt {
	t.Helper()
	r, e := s.Save(context.Background(), owner, c)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func TestDriverReplayRevisionRestartAndOwnerIsolation(t *testing.T) {
	ctx := context.Background()
	p, dsn := freshDriverDB(t, migrations.Files)
	a, b := seedDriverOwner(t, p), seedDriverOwner(t, p)
	carA, carB := seedDriverVehicle(t, p, a), seedDriverVehicle(t, p, b)
	s := New(p, cloud.RequireDriverOwnedVehicle)
	c := earning(t, carA)
	first := mustSave(t, s, a, c)
	if first.Revision != 1 || first.Replayed {
		t.Fatal(first)
	}
	if r := mustSave(t, s, a, c); !r.Replayed || r.ID != first.ID {
		t.Fatal(r)
	}
	bad := c
	bad.Record.Money.Net = new(int64)
	*bad.Record.Money.Net = 44000
	if _, e := s.Save(ctx, a, bad); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("operation body drift", e)
	}
	update := bad
	update.OperationID = newDriverID(t)
	update.ExpectedRevision = 1
	if r := mustSave(t, s, a, update); r.Revision != 2 {
		t.Fatal(r)
	}
	if _, e := s.Get(ctx, b, c.Record.ID); !errors.Is(e, app.ErrNotFound) {
		t.Fatal("foreign read", e)
	}
	foreign := earning(t, carA)
	if _, e := s.Save(ctx, b, foreign); !errors.Is(e, app.ErrDenied) {
		t.Fatal("foreign vehicle", e)
	}
	ownB := earning(t, carB)
	ownB.Record.SourceHash = c.Record.SourceHash
	mustSave(t, s, b, ownB)
	if _, e := New(p, nil).Save(ctx, a, c); !errors.Is(e, app.ErrDenied) {
		t.Fatal("missing port accepted", e)
	}
	revisions, e := q.New(p).DriverRevisions(ctx, q.DriverRevisionsParams{AccountID: uid(a), RecordID: uid(c.Record.ID)})
	if e != nil || len(revisions) != 2 {
		t.Fatal(revisions, e)
	}
	var original domain.EarningRecord
	if e = json.Unmarshal(revisions[0].Payload, &original); e != nil || *original.Money.Net != 45000 || original.Money.Gross != nil {
		t.Fatal("original meaning lost", e)
	}

	versionsPage, pageErr := s.ListRevisions(ctx, a, c.Record.ID, 0, 1)
	if pageErr != nil || len(versionsPage.Revisions) != 1 || versionsPage.NextRevision != 1 || *versionsPage.Revisions[0].Record.Money.Net != 45000 {
		t.Fatal("original revision export", pageErr)
	}
	nextPage, pageErr := s.ListRevisions(ctx, a, c.Record.ID, versionsPage.NextRevision, 1)
	if pageErr != nil || len(nextPage.Revisions) != 1 || nextPage.NextRevision != 0 || *nextPage.Revisions[0].Record.Money.Net != 44000 {
		t.Fatal("corrected revision export", pageErr)
	}
	if _, pageErr = s.ListRevisions(ctx, b, c.Record.ID, 0, 1); !errors.Is(pageErr, app.ErrNotFound) {
		t.Fatal("foreign revision export", pageErr)
	}
	p.Close()
	cold, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(cold.Close)
	restarted := New(cold, cloud.RequireDriverOwnedVehicle)
	got, e := restarted.Get(ctx, a, c.Record.ID)
	if e != nil || got.Revision != 2 || *got.Record.Money.Net != 44000 || got.Record.Money.Gross != nil {
		t.Fatal(got, e)
	}
	replayed := mustSave(t, restarted, a, c)
	if replayed.Revision != 1 || !replayed.Replayed {
		t.Fatal("original receipt changed", replayed)
	}
	after, e := restarted.Get(ctx, a, c.Record.ID)
	if e != nil || after.Revision != 2 {
		t.Fatal("replay rewrote newer value", e)
	}
	duplicate := update
	duplicate.OperationID = newDriverID(t)
	duplicate.ExpectedRevision = 0
	duplicate.Record.ID = newDriverID(t)
	r := mustSave(t, restarted, a, duplicate)
	if r.ID != c.Record.ID || r.Revision != 2 || !r.Replayed {
		t.Fatal("source convergence", r)
	}
}
func TestDriverDuplicateAndCoverageRaces(t *testing.T) {
	ctx := context.Background()
	p, _ := freshDriverDB(t, migrations.Files)
	owner := seedDriverOwner(t, p)
	car := seedDriverVehicle(t, p, owner)
	s := New(p, cloud.RequireDriverOwnedVehicle)
	original := earning(t, car)
	gate := make(chan struct{})
	results := make(chan app.Receipt, 12)
	errs := make(chan error, 12)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			c := original
			c.OperationID = newDriverID(t)
			c.Record.ID = newDriverID(t)
			r, e := s.Save(ctx, owner, c)
			results <- r
			errs <- e
		}()
	}
	close(gate)
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	ids := map[string]bool{}
	for r := range results {
		ids[r.ID] = true
		if r.Revision != 1 {
			t.Fatal(r)
		}
	}
	if len(ids) != 1 || driverCount(t, p, "driver_earnings") != 1 || driverCount(t, p, "driver_earning_commands") != 12 {
		t.Fatal("duplicate source credited more than once")
	}
	gate = make(chan struct{})
	conflicts := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			c := earning(t, car)
			c.Record.Platform = "99"
			_, e := s.Save(ctx, owner, c)
			conflicts <- e
		}()
	}
	close(gate)
	wg.Wait()
	close(conflicts)
	success, blocked := 0, 0
	for e := range conflicts {
		if e == nil {
			success++
		} else if errors.Is(e, domain.ErrConflict) {
			blocked++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || blocked != 1 || driverCount(t, p, "driver_earnings") != 2 {
		t.Fatal("overlap not serialized", success, blocked)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	c := earning(t, car)
	c.Record.Platform = "INDRIVE"
	if _, e := s.Save(cancelled, owner, c); !errors.Is(e, context.Canceled) {
		t.Fatal("cancelled mutation", e)
	}
	if driverCount(t, p, "driver_earnings") != 2 {
		t.Fatal("cancel mutated state")
	}
}
func adjustment(t *testing.T, target string) app.SaveAdjustment {
	t.Helper()
	id := newDriverID(t)
	return app.SaveAdjustment{OperationID: newDriverID(t), SourceHash: sourceHash(id), Adjustment: domain.Adjustment{ID: id, TargetID: target, Kind: "REVERSAL", TargetRevision: 1, Delta: -5000}}
}
func TestDriverAdjustmentsSurviveAndExposeObsoleteTargets(t *testing.T) {
	ctx := context.Background()
	p, _ := freshDriverDB(t, migrations.Files)
	owner, foreign := seedDriverOwner(t, p), seedDriverOwner(t, p)
	car := seedDriverVehicle(t, p, owner)
	s := New(p, cloud.RequireDriverOwnedVehicle)
	c := earning(t, car)
	mustSave(t, s, owner, c)
	a := adjustment(t, c.Record.ID)
	r, e := s.SaveAdjustment(ctx, owner, a)
	if e != nil || r.ID != a.Adjustment.ID {
		t.Fatal(r, e)
	}
	dup := a
	dup.OperationID = newDriverID(t)
	dup.Adjustment.ID = newDriverID(t)
	r, e = s.SaveAdjustment(ctx, owner, dup)
	if e != nil || r.ID != a.Adjustment.ID || !r.Replayed {
		t.Fatal("adjustment source duplicate", r, e)
	}
	if _, e = s.SaveAdjustment(ctx, foreign, a); !errors.Is(e, app.ErrNotFound) {
		t.Fatal("foreign target", e)
	}
	page, e := New(p, cloud.RequireDriverOwnedVehicle).ListAdjustments(ctx, owner, c.Record.ID, "", 1)
	if e != nil || len(page.Adjustments) != 1 || page.Adjustments[0].Adjustment.Delta != -5000 {
		t.Fatal(page, e)
	}
	got, e := s.Get(ctx, owner, c.Record.ID)
	if e != nil || got.AdjustmentCount != 1 || got.NeedsReconciliation {
		t.Fatal(got, e)
	}
	update := c
	update.OperationID = newDriverID(t)
	update.ExpectedRevision = 1
	n := int64(46000)
	update.Record.Money.Net = &n
	mustSave(t, s, owner, update)
	got, e = s.Get(ctx, owner, c.Record.ID)
	if e != nil || !got.NeedsReconciliation {
		t.Fatal("obsolete adjustment silently applied", got, e)
	}
	stale := adjustment(t, c.Record.ID)
	if _, e = s.SaveAdjustment(ctx, owner, stale); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("stale target accepted", e)
	}
	bad := a
	bad.OperationID = newDriverID(t)
	bad.Adjustment.Delta = 5000
	if _, e = s.SaveAdjustment(ctx, owner, bad); !errors.Is(e, domain.ErrInvalid) {
		t.Fatal("positive reversal", e)
	}
}
func TestDriverPartialWritesAndLiveDenialRollback(t *testing.T) {
	ctx := context.Background()
	p, _ := freshDriverDB(t, migrations.Files)
	owner := seedDriverOwner(t, p)
	car := seedDriverVehicle(t, p, owner)
	s := New(p, cloud.RequireDriverOwnedVehicle)
	c := earning(t, car)
	_, e := p.Exec(ctx, `CREATE FUNCTION driver_test_fail() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic receipt failure'; END $$;
 CREATE TRIGGER driver_test_fail BEFORE INSERT ON driver_earning_commands FOR EACH ROW EXECUTE FUNCTION driver_test_fail()`)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Save(ctx, owner, c); !errors.Is(e, app.ErrUnavailable) {
		t.Fatal("failure classification", e)
	}
	for _, table := range []string{"driver_earnings", "driver_earning_revisions", "driver_earning_commands", "driver_earning_owner_state"} {
		if driverCount(t, p, table) != 0 {
			t.Fatal("partial transaction survived", table)
		}
	}
	if _, e = p.Exec(ctx, "DROP TRIGGER driver_test_fail ON driver_earning_commands; DROP FUNCTION driver_test_fail()"); e != nil {
		t.Fatal(e)
	}
	var calls atomic.Int32
	denyAtCommit := func(ctx context.Context, tx pgx.Tx, a, v string) (bool, error) {
		if calls.Add(1) == 2 {
			return false, nil
		}
		return cloud.RequireDriverOwnedVehicle(ctx, tx, a, v)
	}
	if _, e = New(p, denyAtCommit).Save(ctx, owner, c); !errors.Is(e, app.ErrDenied) {
		t.Fatal("live denial", e)
	}
	if driverCount(t, p, "driver_earnings") != 0 {
		t.Fatal("denied write survived")
	}

	cancelCtx, cancel := context.WithCancel(ctx)
	calls.Store(0)
	cancelAtCommit := func(cctx context.Context, tx pgx.Tx, a, v string) (bool, error) {
		if calls.Add(1) == 2 {
			cancel()
		}
		return cloud.RequireDriverOwnedVehicle(cctx, tx, a, v)
	}
	if _, e = New(p, cancelAtCommit).Save(cancelCtx, owner, c); !errors.Is(e, context.Canceled) {
		t.Fatal("mid-transaction cancellation", e)
	}
	cancel()
	if driverCount(t, p, "driver_earnings") != 0 || driverCount(t, p, "driver_earning_commands") != 0 {
		t.Fatal("cancelled transaction survived")
	}
	mustSave(t, s, owner, c)
	calls.Store(0)
	if data, e := New(p, denyAtCommit).Get(ctx, owner, c.Record.ID); !errors.Is(e, app.ErrDenied) || data.Record.ID != "" {
		t.Fatal("post-read denial exposed payload", e)
	}
	calls.Store(0)
	if e := New(p, denyAtCommit).EraseRecord(ctx, owner, c.Record.ID, 1, 0); !errors.Is(e, app.ErrDenied) {
		t.Fatal(e)
	}
	if driverCount(t, p, "driver_earnings") != 1 {
		t.Fatal("denied erasure mutated")
	}
}
func TestDriverEraseEpochAndAccountLifecycle(t *testing.T) {
	ctx := context.Background()
	p, _ := freshDriverDB(t, migrations.Files)
	owner := seedDriverOwner(t, p)
	car := seedDriverVehicle(t, p, owner)
	s := New(p, cloud.RequireDriverOwnedVehicle)
	c := earning(t, car)
	mustSave(t, s, owner, c)
	if _, e := s.SaveAdjustment(ctx, owner, adjustment(t, c.Record.ID)); e != nil {
		t.Fatal(e)
	}
	if e := s.EraseRecord(ctx, owner, c.Record.ID, 2, 0); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("stale erasure", e)
	}
	for i := 0; i < 2; i++ {
		if e := s.EraseRecord(ctx, owner, c.Record.ID, 1, 0); e != nil {
			t.Fatal(e)
		}
	}
	for _, table := range []string{"driver_earnings", "driver_earning_revisions", "driver_earning_adjustments"} {
		if driverCount(t, p, table) != 0 {
			t.Fatal("private rows not erased", table)
		}
	}
	if _, e := s.Save(ctx, owner, c); !errors.Is(e, app.ErrErased) {
		t.Fatal("stale receipt resurrected", e)
	}
	dup := c
	dup.OperationID = newDriverID(t)
	dup.Record.ID = newDriverID(t)
	if _, e := s.Save(ctx, owner, dup); !errors.Is(e, app.ErrErased) {
		t.Fatal("source tombstone bypass", e)
	}
	if e := s.EraseOwner(ctx, owner, 0); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Save(ctx, owner, dup); !errors.Is(e, app.ErrErased) {
		t.Fatal("old epoch admitted", e)
	}
	fresh := earning(t, car)
	fresh.ExpectedEpoch = 1
	mustSave(t, s, owner, fresh)
	if _, e := p.Exec(ctx, "UPDATE accounts SET status='deleted' WHERE id=$1", owner); e != nil {
		t.Fatal(e)
	}
	for _, table := range []string{"driver_earnings", "driver_earning_revisions", "driver_earning_adjustments", "driver_earning_commands", "driver_earning_tombstones"} {
		if driverCount(t, p, table) != 0 {
			t.Fatal("account erasure incomplete", table)
		}
	}
	if _, e := s.Get(ctx, owner, fresh.Record.ID); !errors.Is(e, app.ErrDenied) {
		t.Fatal("deleted owner read", e)
	}
}
func TestDriverDatabaseConstraintsAndBoundedExport(t *testing.T) {
	ctx := context.Background()
	p, _ := freshDriverDB(t, migrations.Files)
	owner := seedDriverOwner(t, p)
	car := seedDriverVehicle(t, p, owner)
	s := New(p, cloud.RequireDriverOwnedVehicle)
	c := earning(t, car)
	mustSave(t, s, owner, c)
	for _, query := range []string{"UPDATE driver_earnings SET net_centavos=-1", "UPDATE driver_earnings SET gross_centavos=1", "UPDATE driver_earnings SET to_at=from_at", "UPDATE driver_earnings SET currency='USD'", "UPDATE driver_earnings SET source_hash='not-a-digest'"} {
		if _, e := p.Exec(ctx, query); e == nil {
			t.Fatal("invalid direct SQL accepted", query)
		}
	}
	for _, platform := range []string{"99", "OTHER"} {
		next := earning(t, car)
		next.Record.Platform = platform
		mustSave(t, s, owner, next)
	}
	seen := map[string]bool{}
	after := ""
	for {
		page, e := s.List(ctx, owner, after, 1)
		if e != nil {
			t.Fatal(e)
		}
		if len(page.Records) != 1 {
			t.Fatal("truncated export", page)
		}
		id := page.Records[0].Record.ID
		if seen[id] {
			t.Fatal("cursor repeated")
		}
		seen[id] = true
		if page.NextID == "" {
			break
		}
		after = page.NextID
	}
	if len(seen) != 3 {
		t.Fatal("export lost rows")
	}
	if _, e := s.List(ctx, owner, "", 1001); !errors.Is(e, domain.ErrInvalid) {
		t.Fatal("unbounded read", e)
	}
	var forbidden int
	if e := p.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_name LIKE 'driver_%' AND column_name IN ('latitude','longitude','passenger','token','password','notes','raw_trace')`).Scan(&forbidden); e != nil || forbidden != 0 {
		t.Fatal("private schema collected forbidden fields", e)
	}
}
func migrationCopy(t *testing.T, include133 bool) fstest.MapFS {
	t.Helper()
	entries, e := fs.ReadDir(migrations.Files, ".")
	if e != nil {
		t.Fatal(e)
	}
	out := fstest.MapFS{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") || (!include133 && (strings.HasPrefix(entry.Name(), "000133_") || strings.HasPrefix(entry.Name(), "000134_") || strings.HasPrefix(entry.Name(), "000135_") || strings.HasPrefix(entry.Name(), "000136_"))) {
			continue
		}
		b, e := fs.ReadFile(migrations.Files, entry.Name())
		if e != nil {
			t.Fatal(e)
		}
		out[entry.Name()] = &fstest.MapFile{Data: b, Mode: 0600}
	}
	return out
}
func TestDriverUpgradeFailureRecoveryAndConcurrentMigrators(t *testing.T) {
	ctx := context.Background()
	before := migrationCopy(t, false)
	p, dsn := freshDriverDB(t, before)
	owner := seedDriverOwner(t, p)
	car := seedDriverVehicle(t, p, owner)
	id := newDriverID(t)
	if _, e := p.Exec(ctx, `INSERT INTO ledger_records(account_id,dataset,id,vehicle_id,occurred_at,payload) VALUES($1,'refuel',$2,$3,clock_timestamp(),'{"synthetic_legacy":true}')`, owner, id, car); e != nil {
		t.Fatal(e)
	}
	broken := migrationCopy(t, true)
	name := "000133_driver_earnings.sql"
	broken[name] = &fstest.MapFile{Data: append(append([]byte{}, broken[name].Data...), []byte("\nSELECT 1/0;")...), Mode: 0600}
	if _, e := migrate.Apply(ctx, dsn, broken); e == nil || !strings.Contains(e.Error(), "division by zero") {
		t.Fatal("expected actual SQL failure after the DDL", e)
	}
	var absent bool
	if e := p.QueryRow(ctx, "SELECT to_regclass('driver_earnings') IS NULL").Scan(&absent); e != nil || !absent {
		t.Fatal("migration partially committed", e)
	}
	gate := make(chan struct{})
	done := make(chan []string, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { <-gate; versions, e := migrate.Apply(ctx, dsn, migrations.Files); done <- versions; errs <- e }()
	}
	close(gate)
	total := 0
	for i := 0; i < 2; i++ {
		total += len(<-done)
		if e := <-errs; e != nil {
			t.Fatal(e)
		}
	}
	if total != 4 {
		t.Fatal("migrations not applied exactly once", total)
	}
	var legacy bool
	if e := p.QueryRow(ctx, "SELECT (payload->>'synthetic_legacy')::boolean FROM ledger_records WHERE account_id=$1 AND id=$2", owner, id).Scan(&legacy); e != nil || !legacy {
		t.Fatal("upgrade rewrote legacy ledger", e)
	}
	versions, e := migrate.Apply(ctx, dsn, migrations.Files)
	if e != nil || len(versions) != 0 {
		t.Fatal("migration replay", versions, e)
	}
}

func TestDriverCapturesMoneyBeforeOwnershipIO(t *testing.T) {
	ctx := context.Background()
	p, _ := freshDriverDB(t, migrations.Files)
	owner := seedDriverOwner(t, p)
	car := seedDriverVehicle(t, p, owner)
	c := earning(t, car)
	var changed atomic.Bool
	mutateCaller := func(ctx context.Context, tx pgx.Tx, a, v string) (bool, error) {
		if changed.CompareAndSwap(false, true) {
			*c.Record.Money.Net = 99000
		}
		return cloud.RequireDriverOwnedVehicle(ctx, tx, a, v)
	}
	mustSave(t, New(p, mutateCaller), owner, c)
	got, e := New(p, cloud.RequireDriverOwnedVehicle).Get(ctx, owner, c.Record.ID)
	if e != nil || *got.Record.Money.Net != 45000 {
		t.Fatalf("caller changed captured money across ownership I/O: got=%v err=%v", got.Record.Money.Net, e)
	}
	hash, e := got.Record.BusinessHash()
	if e != nil {
		t.Fatal(e)
	}
	var storedHash string
	if e = p.QueryRow(ctx, "SELECT content_sha256 FROM driver_earnings WHERE account_id=$1 AND id=$2", owner, c.Record.ID).Scan(&storedHash); e != nil || storedHash != hash {
		t.Fatal("money/fingerprint diverged", e)
	}
}

func TestDriverPhysicalOwnerCascadeAndAdjustmentPaging(t *testing.T) {
	ctx := context.Background()
	p, _ := freshDriverDB(t, migrations.Files)
	owner := seedDriverOwner(t, p)
	car := seedDriverVehicle(t, p, owner)
	s := New(p, cloud.RequireDriverOwnedVehicle)
	c := earning(t, car)
	mustSave(t, s, owner, c)
	for i := 0; i < 3; i++ {
		a := adjustment(t, c.Record.ID)
		if _, e := s.SaveAdjustment(ctx, owner, a); e != nil {
			t.Fatal(e)
		}
	}
	seen := map[string]bool{}
	after := ""
	for {
		page, e := s.ListAdjustments(ctx, owner, c.Record.ID, after, 1)
		if e != nil {
			t.Fatal(e)
		}
		if len(page.Adjustments) != 1 {
			t.Fatal("adjustment page lost data")
		}
		id := page.Adjustments[0].Adjustment.ID
		if seen[id] {
			t.Fatal("repeated adjustment cursor")
		}
		seen[id] = true
		if page.NextID == "" {
			break
		}
		after = page.NextID
	}
	if len(seen) != 3 {
		t.Fatal(seen)
	}
	// Existing cloud rows have their own lifecycle; remove only this synthetic
	// vehicle before exercising physical account deletion in the disposable DB.
	if _, e := p.Exec(ctx, "DELETE FROM cloud_records WHERE account_id=$1", owner); e != nil {
		t.Fatal(e)
	}
	if _, e := p.Exec(ctx, "DELETE FROM accounts WHERE id=$1", owner); e != nil {
		t.Fatal(e)
	}
	for _, table := range []string{"driver_earnings", "driver_earning_revisions", "driver_earning_adjustments", "driver_earning_commands", "driver_earning_owner_state"} {
		if driverCount(t, p, table) != 0 {
			t.Fatal("physical owner cascade failed", table)
		}
	}
}

func TestDriverAdjacentCoverageAndCrossOwnerAdjustmentConstraint(t *testing.T) {
	ctx := context.Background()
	p, _ := freshDriverDB(t, migrations.Files)
	owner, foreign := seedDriverOwner(t, p), seedDriverOwner(t, p)
	car := seedDriverVehicle(t, p, owner)
	s := New(p, cloud.RequireDriverOwnedVehicle)
	first := earning(t, car)
	mustSave(t, s, owner, first)
	adjacent := earning(t, car)
	adjacent.Record.From = first.Record.To
	adjacent.Record.To = first.Record.To.Add(time.Hour)
	mustSave(t, s, owner, adjacent)
	summary := earning(t, car)
	summary.Record.To = adjacent.Record.To
	if _, e := s.Save(ctx, owner, summary); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("summary/detail coverage doubled", e)
	}
	detail := earning(t, car)
	detail.Record.Coverage = "DETAIL"
	detail.Record.From = first.Record.From.Add(30 * time.Minute)
	detail.Record.To = first.Record.From.Add(45 * time.Minute)
	if _, e := s.Save(ctx, owner, detail); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("overlapping detail admitted", e)
	}
	_, e := p.Exec(ctx, `INSERT INTO driver_earning_adjustments(account_id,id,source_hash,target_id,target_revision,kind,delta_centavos,content_sha256) VALUES($1,$2,$3,$4,1,'REVERSAL',-1,$3)`, foreign, newDriverID(t), sourceHash("synthetic-foreign-link"), first.Record.ID)
	if e == nil {
		t.Fatal("database accepted cross-owner target revision")
	}
	if driverCount(t, p, "driver_earnings") != 2 || driverCount(t, p, "driver_earning_adjustments") != 0 {
		t.Fatal("refused coverage/link mutated facts")
	}
}
