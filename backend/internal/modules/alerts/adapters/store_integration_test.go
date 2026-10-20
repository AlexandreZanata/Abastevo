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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/alerts/application"
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
	name := fmt.Sprintf("alerts_test_%d", time.Now().UnixNano())
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

func newStation(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	stationID := newid(t)
	if _, err := pool.Exec(context.Background(), "INSERT INTO directory_stations(id,display_name) VALUES($1,'Synthetic station')", stationID); err != nil {
		t.Fatal(err)
	}
	return stationID
}

// alertEvaluationTime is Tuesday 10:00 in Sao Paulo, inside rulePayload's
// allowed days/window. Integration evidence must not depend on CI wall time.
func alertEvaluationTime() time.Time {
	return time.Date(2026, time.October, 20, 13, 0, 0, 0, time.UTC)
}

func rulePayload(t *testing.T, station string) json.RawMessage {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{
		"predicates": []any{map[string]any{"kind": "threshold", "fuel": "ETHANOL", "unit": "L", "condition": "STANDARD", "source": "community", "threshold_milli": 6200}},
		"operator":   "AND",
		"target":     map[string]any{"station_ids": []string{station}},
		"schedule":   map[string]any{"days": []int{1, 2, 3}, "window_start": "08:00", "window_end": "20:00", "timezone": "America/Sao_Paulo", "quiet_start": "22:00", "quiet_end": "07:00", "cooldown_hours": 1},
	})
	return raw
}

func ruleCommand(t *testing.T, ruleID string, expected int64, station string) application.Command {
	t.Helper()
	return application.Command{OperationID: newid(t), RuleID: ruleID, ExpectedRevision: expected, Payload: rulePayload(t, station)}
}

func TestAlertWrongOwnerStationExpiryReplay(t *testing.T) {
	pool := freshDB(t)
	owner := newAccount(t, pool)
	foreign := newAccount(t, pool)
	station := newStation(t, pool)
	store := NewStore(pool)
	ctx := context.Background()
	foreignCmd := ruleCommand(t, newid(t), 0, station)
	if _, err := store.Apply(ctx, foreign, application.Batch{Commands: []application.Command{foreignCmd}}); err != nil {
		t.Fatal("foreign account create", err)
	}
	rows, err := store.ListRules(ctx, owner)
	if err != nil || len(rows) != 0 {
		t.Fatalf("cross-owner rules leaked %+v %v", rows, err)
	}
	badCmd := ruleCommand(t, newid(t), 0, newid(t))
	if _, err = store.Apply(ctx, owner, application.Batch{Commands: []application.Command{badCmd}}); err == nil {
		t.Fatal("unknown station admitted")
	}
	store.Paid = func(context.Context, pgx.Tx, string) error { return errors.New("alerts: capability required") }
	if _, err = store.Apply(ctx, owner, application.Batch{Commands: []application.Command{ruleCommand(t, newid(t), 0, station)}}); err == nil {
		t.Fatal("expired write admitted")
	}
	rows, err = store.ListRules(ctx, owner)
	if err != nil {
		t.Fatal("expired read refused", err)
	}
	store.Paid = nil
	if _, err = store.Apply(ctx, owner, application.Batch{Commands: []application.Command{ruleCommand(t, newid(t), 0, station)}}); err != nil {
		t.Fatal("regrant refused", err)
	}
	id := newid(t)
	cmd := ruleCommand(t, id, 0, station)
	if _, err = store.Apply(ctx, owner, application.Batch{Commands: []application.Command{cmd}}); err != nil {
		t.Fatal(err)
	}
	again, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{cmd}})
	if err != nil || !again.Results[0].Replayed {
		t.Fatal("replay", err)
	}
	divergent := cmd
	divergent.Payload = rulePayload(t, newStation(t, pool))
	if _, err = store.Apply(ctx, owner, application.Batch{Commands: []application.Command{divergent}}); err == nil {
		t.Fatal("divergent reuse admitted")
	}
}

func TestAlertConcurrentEnableDeleteCapsAndErase(t *testing.T) {
	pool := freshDB(t)
	owner := newAccount(t, pool)
	station := newStation(t, pool)
	store := NewStore(pool)
	ctx := context.Background()
	id := newid(t)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{ruleCommand(t, id, 0, station)}}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	disabled := false
	enableCmd := ruleCommand(t, id, 1, station)
	_ = enableCmd
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			off := false
			_, e := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{{OperationID: newid(t), RuleID: id, ExpectedRevision: 1, Enabled: &off}}})
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	wins := 0
	for e := range errs {
		if e == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("concurrent enable %d", wins)
	}
	_ = disabled
	delCmd := application.Command{OperationID: newid(t), RuleID: id, Delete: true}
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{delCmd}}); err != nil {
		t.Fatal("delete", err)
	}
	rows, err := store.ListRules(ctx, owner)
	if err != nil || len(rows) != 0 {
		t.Fatalf("tombstone listed %+v %v", rows, err)
	}
	for i := 0; i < 20; i++ {
		if _, err = store.Apply(ctx, owner, application.Batch{Commands: []application.Command{ruleCommand(t, newid(t), 0, station)}}); err != nil {
			t.Fatalf("cap fill %d %v", i, err)
		}
	}
	if _, err = store.Apply(ctx, owner, application.Batch{Commands: []application.Command{ruleCommand(t, newid(t), 0, station)}}); err == nil {
		t.Fatal("rule cap race admitted")
	}
	if err = store.EraseAll(ctx, owner); err != nil {
		t.Fatal(err)
	}
	rows, err = store.ListRules(ctx, owner)
	if err != nil || len(rows) != 0 {
		t.Fatal("erasure listed")
	}
	inbox, err := store.ListInbox(ctx, owner, 100)
	if err != nil || len(inbox) != 0 {
		t.Fatal("inbox erasure")
	}
}
