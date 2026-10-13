//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func freshDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("explicit disposable PostGIS DSN required")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Hostname() != "127.0.0.1" {
		t.Fatal("literal loopback test DSN required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("disposable database unavailable")
	}
	name := fmt.Sprintf("ledger_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	admin.Close(ctx)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, dsn)
		if err != nil {
			return
		}
		defer admin.Close(ctx)
		_, _ = admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize())
	})
	u.Path = "/" + name
	if _, err = migrate.Apply(ctx, u.String(), migrations.Files); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func newid(t *testing.T) string {
	t.Helper()
	id, err := jobs.NewUUIDv4()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func newAccount(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	accountID := newid(t)
	if _, err := pool.Exec(context.Background(), "INSERT INTO accounts(id,alias,status) VALUES($1,$2,'active')", accountID, "synthetic-"+accountID); err != nil {
		t.Fatal(err)
	}
	return accountID
}

func newVehicle(t *testing.T, pool *pgxpool.Pool, accountID string) string {
	t.Helper()
	vehicleID := newid(t)
	if _, err := pool.Exec(context.Background(), "INSERT INTO cloud_records(account_id,dataset,id,payload) VALUES($1,'vehicles',$2,'{}')", accountID, vehicleID); err != nil {
		t.Fatal(err)
	}
	return vehicleID
}

func refuelCommand(t *testing.T, id, vehicleID string, expected int64) application.Command {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"id": id, "vehicle_id": vehicleID, "occurred_at": "2026-03-15T10:30:00-03:00", "timezone": "America/Sao_Paulo", "fuel": "ETHANOL", "unit": "L", "quantity_milli": 20000, "total_centavos": 12400, "fill": "full"})
	return application.Command{OperationID: newid(t), Dataset: "refuel", ID: id, ExpectedRevision: expected, Payload: payload}
}

func expenseCommand(t *testing.T, id, vehicleID string, expected int64) application.Command {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"id": id, "vehicle_id": vehicleID, "occurred_at": "2026-03-15T10:30:00-03:00", "timezone": "America/Sao_Paulo", "amount_centavos": 5000, "category": "tolls"})
	return application.Command{OperationID: newid(t), Dataset: "expense", ID: id, ExpectedRevision: expected, Payload: payload}
}

func counts(t *testing.T, pool *pgxpool.Pool) (int, int, int) {
	t.Helper()
	var ledger, operations, cloud int
	if err := pool.QueryRow(context.Background(), "SELECT (SELECT COUNT(*) FROM ledger_records),(SELECT COUNT(*) FROM ledger_operations),(SELECT COUNT(*) FROM cloud_records)").Scan(&ledger, &operations, &cloud); err != nil {
		t.Fatal(err)
	}
	return ledger, operations, cloud
}

func TestLedgerForeignArchivedAndReplay(t *testing.T) {
	pool := freshDB(t)
	owner := newAccount(t, pool)
	foreign := newAccount(t, pool)
	vehicle := newVehicle(t, pool, owner)
	store := NewStore(pool)
	ctx := context.Background()
	foreignCmd := refuelCommand(t, newid(t), vehicle, 0)
	if _, err := store.Apply(ctx, foreign, application.Batch{Commands: []application.Command{foreignCmd}}); err == nil {
		t.Fatal("foreign vehicle admitted")
	}
	archivedVehicle := newid(t)
	if _, err := pool.Exec(ctx, "INSERT INTO cloud_records(account_id,dataset,id,payload) VALUES($1,'vehicles',$2,'{\"archived\":true}')", owner, archivedVehicle); err != nil {
		t.Fatal(err)
	}
	archivedCmd := refuelCommand(t, newid(t), archivedVehicle, 0)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{archivedCmd}}); err != nil {
		t.Fatalf("archived history refused %v", err)
	}
	id := newid(t)
	cmd := refuelCommand(t, id, vehicle, 0)
	first, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{cmd}})
	if err != nil || first.Results[0].Revision != 1 {
		t.Fatal("create", err)
	}
	again, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{cmd}})
	if err != nil || !again.Results[0].Replayed {
		t.Fatal("replay", err)
	}
	divergent := cmd
	divergent.Payload, _ = json.Marshal(map[string]any{"id": id, "vehicle_id": vehicle, "occurred_at": "2026-03-15T10:30:00-03:00", "timezone": "America/Sao_Paulo", "fuel": "ETHANOL", "unit": "L", "quantity_milli": 21000, "total_centavos": 12400, "fill": "full"})
	if _, err = store.Apply(ctx, owner, application.Batch{Commands: []application.Command{divergent}}); err == nil {
		t.Fatal("divergent reuse admitted")
	}
}

func TestLedgerSimultaneousBackdatedRollbackMaxAndPlans(t *testing.T) {
	pool := freshDB(t)
	owner := newAccount(t, pool)
	vehicle := newVehicle(t, pool, owner)
	store := NewStore(pool)
	ctx := context.Background()
	id := newid(t)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{refuelCommand(t, id, vehicle, 0)}}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := id
			_ = name
			cmd := refuelCommand(t, id, vehicle, 1)
			cmd.OperationID = newid(t)
			_, e := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{cmd}})
			errs <- e
			_ = i
		}(i)
	}
	wg.Wait()
	close(errs)
	successes := 0
	for e := range errs {
		if e == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("simultaneous edit %d", successes)
	}
	backdated, _ := json.Marshal(map[string]any{"id": newid(t), "vehicle_id": vehicle, "occurred_at": "2020-01-05T08:00:00-03:00", "timezone": "America/Sao_Paulo", "fuel": "ETHANOL", "unit": "L", "quantity_milli": 10000, "total_centavos": 6000, "fill": "partial"})
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{{OperationID: newid(t), Dataset: "refuel", ID: newid(t), Payload: backdated}}}); err != nil {
		t.Fatal("backdated refused", err)
	}
	good := expenseCommand(t, newid(t), vehicle, 0)
	bad := refuelCommand(t, id, vehicle, 99)
	beforeLedger, beforeOps, _ := counts(t, pool)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{good, bad}}); err == nil {
		t.Fatal("partial batch admitted")
	}
	afterLedger, afterOps, _ := counts(t, pool)
	if afterLedger != beforeLedger || afterOps != beforeOps {
		t.Fatal("rollback left effects")
	}
	maxRefuel, _ := json.Marshal(map[string]any{"id": newid(t), "vehicle_id": vehicle, "occurred_at": "2026-03-15T10:30:00-03:00", "timezone": "America/Sao_Paulo", "fuel": "ETHANOL", "unit": "L", "quantity_milli": 1000000000, "total_centavos": 1000000000000, "fill": "unknown"})
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{{OperationID: newid(t), Dataset: "refuel", ID: newid(t), Payload: maxRefuel}}}); err != nil {
		t.Fatal("max values refused", err)
	}
	foreignOwner := newAccount(t, pool)
	if _, err := pool.Exec(ctx, `INSERT INTO ledger_records(account_id,dataset,id,vehicle_id,occurred_at,payload) SELECT $1,'refuel',md5(g::text)::uuid,$2,clock_timestamp(),'{}' FROM generate_series(1,10000)g`, foreignOwner, vehicle); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "ANALYZE ledger_records"); err != nil {
		t.Fatal(err)
	}
	plan, err := pool.Query(ctx, "EXPLAIN SELECT dataset,id,vehicle_id,occurred_at,revision,voided,deleted,payload FROM ledger_records WHERE account_id=$1 AND occurred_at>$2 ORDER BY occurred_at LIMIT 100", owner, "2020-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	text := ""
	for plan.Next() {
		var line string
		if err = plan.Scan(&line); err != nil {
			t.Fatal(err)
		}
		text += line
	}
	plan.Close()
	if len(text) == 0 {
		t.Fatal("empty plan")
	}
	var communityBefore, communityAfter int
	if err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM community_observations").Scan(&communityBefore); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Apply(ctx, owner, application.Batch{Commands: []application.Command{expenseCommand(t, newid(t), vehicle, 0)}}); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM community_observations").Scan(&communityAfter); err != nil {
		t.Fatal(err)
	}
	if communityAfter != communityBefore {
		t.Fatal("financial record reached public tables")
	}
	_ = errors.Is
}
