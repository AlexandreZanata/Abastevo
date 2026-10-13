//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/alerts/application"
	"github.com/jackc/pgx/v5"
)

func combinedPayload(t *testing.T, stations []string) json.RawMessage {
	t.Helper()
	predicates := []any{
		map[string]any{"kind": "threshold", "fuel": "ETHANOL", "unit": "L", "condition": "STANDARD", "source": "community", "threshold_milli": 6200},
		map[string]any{"kind": "change", "fuel": "ETHANOL", "unit": "L", "condition": "STANDARD", "source": "community", "drop_milli": 100, "baseline_hours": 24},
	}
	raw, _ := json.Marshal(map[string]any{
		"predicates": predicates, "operator": "AND", "target": map[string]any{"station_ids": stations},
		"schedule": map[string]any{"days": []int{0, 1, 2, 3, 4, 5, 6}, "window_start": "00:00", "window_end": "00:00", "timezone": "UTC", "quiet_start": "00:00", "quiet_end": "00:00", "cooldown_hours": 0},
	})
	return raw
}

func TestPipelineCombinedWindowsOutageRestartRotation(t *testing.T) {
	pool := freshDB(t)
	owner := newAccount(t, pool)
	stations := []string{newStation(t, pool), newStation(t, pool)}
	store := NewStore(pool)
	fake := &fakeProvider{}
	store.Sender = fake
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	ruleID := newid(t)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{{OperationID: newid(t), RuleID: ruleID, Payload: combinedPayload(t, stations)}}}); err != nil {
		t.Fatal(err)
	}
	for _, station := range stations {
		insertCommunityPrice(t, pool, station, "ETHANOL", "L", 6000, "STANDARD", now.Add(-48*time.Hour), true)
		insertCommunityPrice(t, pool, station, "ETHANOL", "L", 5800, "STANDARD", now.Add(-time.Hour), true)
	}
	evaluated, err := store.Evaluate(ctx, owner, ruleID, "", 100, now)
	if err != nil || evaluated.Candidates != 2 {
		t.Fatalf("combined %+v %v", evaluated, err)
	}
	if _, err = pool.Exec(ctx, `CREATE FUNCTION reject_pipeline_due() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic'; END $$; CREATE TRIGGER reject_pipeline_due BEFORE UPDATE ON alert_inbox FOR EACH ROW EXECUTE FUNCTION reject_pipeline_due()`); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Dispatch(ctx, now, 25); err == nil {
		t.Fatal("outage admitted")
	}
	if _, err = pool.Exec(ctx, "DROP TRIGGER reject_pipeline_due ON alert_inbox"); err != nil {
		t.Fatal(err)
	}
	restarted := NewStore(pool)
	restarted.Sender = fake
	dispatched, err := restarted.Dispatch(ctx, now, 25)
	if err != nil || dispatched.Dispatched != 2 {
		t.Fatalf("restart %+v %v", dispatched, err)
	}
	rotated := &fakeProvider{}
	restarted.Sender = rotated
	var inboxID string
	if err = pool.QueryRow(ctx, "SELECT inbox_id FROM alert_inbox WHERE account_id=$1 AND status='dispatched' LIMIT 1", owner).Scan(&inboxID); err != nil {
		t.Fatal(err)
	}
	if err = restarted.EnrollToken(ctx, owner, "rotated-token"); err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.SendInbox(ctx, owner, inboxID, "rotated-token"); err != nil {
		t.Fatalf("rotated send %v", err)
	}
	if fake.calls != 0 || rotated.calls != 1 {
		t.Fatalf("credential calls %d %d", fake.calls, rotated.calls)
	}
}

func TestPipelineTimezoneMassExpiryBasicAndCleanup(t *testing.T) {
	pool := freshDB(t)
	owner := newAccount(t, pool)
	store := NewStore(pool)
	fake := &fakeProvider{}
	store.Sender = fake
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	stations := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		stations = append(stations, newStation(t, pool))
	}
	ruleID := newid(t)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{{OperationID: newid(t), RuleID: ruleID, Payload: combinedPayload(t, stations)}}}); err != nil {
		t.Fatal(err)
	}
	for _, station := range stations {
		insertCommunityPrice(t, pool, station, "ETHANOL", "L", 6000, "STANDARD", now.Add(-48*time.Hour), true)
		insertCommunityPrice(t, pool, station, "ETHANOL", "L", 5800, "STANDARD", now.Add(-time.Hour), true)
	}
	evaluated, err := store.Evaluate(ctx, owner, ruleID, "", 100, now)
	if err != nil || evaluated.Candidates != 5 {
		t.Fatalf("mass fanout %+v %v", evaluated, err)
	}
	tzRule := newid(t)
	tzPayload, _ := json.Marshal(map[string]any{
		"predicates": []any{map[string]any{"kind": "threshold", "fuel": "ETHANOL", "unit": "L", "condition": "STANDARD", "source": "community", "threshold_milli": 6200}},
		"operator":   "AND", "target": map[string]any{"station_ids": stations[:1]},
		"schedule": map[string]any{"days": []int{0, 1, 2, 3, 4, 5, 6}, "window_start": "00:00", "window_end": "00:00", "timezone": "America/Sao_Paulo", "quiet_start": "00:00", "quiet_end": "00:00", "cooldown_hours": 0},
	})
	if _, err = store.Apply(ctx, owner, application.Batch{Commands: []application.Command{{OperationID: newid(t), RuleID: tzRule, Payload: tzPayload}}}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Evaluate(ctx, owner, tzRule, "", 100, now); err != nil {
		t.Fatal(err)
	}
	edited, _ := json.Marshal(map[string]any{
		"predicates": []any{map[string]any{"kind": "threshold", "fuel": "ETHANOL", "unit": "L", "condition": "STANDARD", "source": "community", "threshold_milli": 6200}},
		"operator":   "AND", "target": map[string]any{"station_ids": stations[:1]},
		"schedule": map[string]any{"days": []int{0, 1, 2, 3, 4, 5, 6}, "window_start": "00:00", "window_end": "00:00", "timezone": "UTC", "quiet_start": "00:00", "quiet_end": "00:00", "cooldown_hours": 0},
	})
	if _, err = store.Apply(ctx, owner, application.Batch{Commands: []application.Command{{OperationID: newid(t), RuleID: tzRule, ExpectedRevision: 1, Payload: edited}}}); err != nil {
		t.Fatal(err)
	}
	store.Paid = func(context.Context, pgx.Tx, string) error { return errors.New("alerts: capability required") }
	paused, err := store.Dispatch(ctx, now, 25)
	if err != nil {
		t.Fatal(err)
	}
	_ = paused
	store.Paid = nil
	var evaluatedCount, dispatchedCount int
	if err = pool.QueryRow(ctx, "SELECT (SELECT COUNT(*) FROM alert_inbox WHERE status='evaluated'),(SELECT COUNT(*) FROM alert_inbox WHERE status='dispatched')").Scan(&evaluatedCount, &dispatchedCount); err != nil {
		t.Fatal(err)
	}
	if evaluatedCount == 0 {
		t.Fatal("expiry drained evaluated backlog")
	}
	if _, err = pool.Exec(ctx, "INSERT INTO cloud_records(account_id,dataset,id,payload) VALUES($1,'vehicles',$2,'{\"price_drop_alert_enabled\":true}')", owner, newid(t)); err != nil {
		t.Fatal(err)
	}
	if err = store.EraseAll(ctx, owner); err != nil {
		t.Fatal(err)
	}
	var basic int
	if err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM cloud_records WHERE account_id=$1 AND dataset='vehicles'", owner).Scan(&basic); err != nil {
		t.Fatalf("basic query %v", err)
	}
	if basic != 1 {
		t.Fatalf("basic alert disturbed count=%d", basic)
	}
	var flag string
	if err = pool.QueryRow(ctx, "SELECT payload->>'price_drop_alert_enabled' FROM cloud_records WHERE account_id=$1 AND dataset='vehicles'", owner).Scan(&flag); err != nil || flag != "true" {
		t.Fatalf("basic flag disturbed %q %v", flag, err)
	}
}
