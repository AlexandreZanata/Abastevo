//go:build integration

package adapters

import (
	"context"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/alerts/application"
	"github.com/jackc/pgx/v5/pgxpool"
)

func insertCommunityPrice(t *testing.T, pool *pgxpool.Pool, station, fuel, unit string, milli int64, condition string, received time.Time, validated bool) {
	t.Helper()
	ctx := context.Background()
	observation := newid(t)
	if _, err := pool.Exec(ctx, "INSERT INTO community_observations(id,contributor_ref,client_submission_id,station_id,fuel_product,unit,amount_milli_brl,condition_kind,qualifier_key,received_at,policy_version) VALUES($1,'synthetic','synthetic-"+observation+"',$2,$3,$4,$5,$6,'',$7,'v1')", observation, station, fuel, unit, milli, condition, received); err != nil {
		t.Fatal(err)
	}
	state := "REJECTED"
	if validated {
		state = "VALIDATED"
	}
	if _, err := pool.Exec(ctx, "INSERT INTO community_observation_decisions(id,observation_id,sequence,to_state,policy_version) VALUES($1,$2,1,$3,'v1')", newid(t), observation, state); err != nil {
		t.Fatal(err)
	}
}

func TestEvaluateThresholdChangeCoverageFanoutRollback(t *testing.T) {
	pool := freshDB(t)
	owner := newAccount(t, pool)
	station := newStation(t, pool)
	other := newStation(t, pool)
	store := NewStore(pool)
	ctx := context.Background()
	now := alertEvaluationTime()
	ruleID := newid(t)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{{OperationID: newid(t), RuleID: ruleID, Payload: rulePayload(t, station)}}}); err != nil {
		t.Fatal(err)
	}
	insertCommunityPrice(t, pool, station, "ETHANOL", "L", 6000, "STANDARD", now.Add(-time.Hour), true)
	insertCommunityPrice(t, pool, other, "ETHANOL", "L", 5000, "STANDARD", now.Add(-30*time.Minute), true)
	got, err := store.Evaluate(ctx, owner, ruleID, "", 100, now)
	if err != nil || got.Candidates != 1 {
		t.Fatalf("threshold %+v %v", got, err)
	}
	again, err := store.Evaluate(ctx, owner, ruleID, "", 100, now)
	if err != nil || again.Candidates != 0 {
		t.Fatalf("duplicate job %+v %v", again, err)
	}
	var inbox int
	if err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM alert_inbox WHERE account_id=$1", owner).Scan(&inbox); err != nil || inbox != 1 {
		t.Fatalf("inbox %d %v", inbox, err)
	}
	insertCommunityPrice(t, pool, station, "ETHANOL", "L", 7000, "STANDARD", now.Add(-48*time.Hour), true)
	insertCommunityPrice(t, pool, station, "ETHANOL", "L", 6100, "STANDARD", now.Add(-time.Hour), false)
	second, err := store.Evaluate(ctx, owner, ruleID, "", 100, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	_ = second
	var community int
	if err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM community_observations").Scan(&community); err != nil {
		t.Fatal(err)
	}
	_ = community
	if _, err = pool.Exec(ctx, `CREATE FUNCTION reject_inbox_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic'; END $$; CREATE TRIGGER reject_inbox_insert BEFORE INSERT ON alert_inbox FOR EACH ROW EXECUTE FUNCTION reject_inbox_insert()`); err != nil {
		t.Fatal(err)
	}
	freshRule := newid(t)
	if _, err = store.Apply(ctx, owner, application.Batch{Commands: []application.Command{{OperationID: newid(t), RuleID: freshRule, Payload: rulePayload(t, other)}}}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Evaluate(ctx, owner, freshRule, "", 100, now); err == nil {
		t.Fatal("rollback admitted")
	}
	if _, err = pool.Exec(ctx, "DROP TRIGGER reject_inbox_insert ON alert_inbox"); err != nil {
		t.Fatal(err)
	}
	after, err := store.Evaluate(ctx, owner, freshRule, "", 100, now)
	if err != nil || after.Candidates != 1 {
		t.Fatalf("retry %+v %v", after, err)
	}
	plan, err := pool.Query(ctx, "EXPLAIN SELECT o.id FROM community_observations o WHERE o.station_id=ANY($1::uuid[]) AND o.fuel_product=$2 AND o.unit=$3 AND o.received_at>$4 ORDER BY o.station_id,o.received_at DESC LIMIT 100", []string{station}, "ETHANOL", "L", now.Add(-24*time.Hour))
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
}

func TestAlertScheduleClockIndependentOfCIRunDay(t *testing.T) {
	for _, tc := range []struct {
		name string
		now  time.Time
		want int
	}{
		{"allowed-Tuesday", alertEvaluationTime(), 1},
		{"excluded-Sunday", alertEvaluationTime().AddDate(0, 0, -2), 0},
		{"outside-window", alertEvaluationTime().Add(-10 * time.Hour), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := freshDB(t)
			owner, station := newAccount(t, pool), newStation(t, pool)
			ruleID := newid(t)
			store := NewStore(pool)
			ctx := context.Background()
			if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{{OperationID: newid(t), RuleID: ruleID, Payload: rulePayload(t, station)}}}); err != nil {
				t.Fatal(err)
			}
			insertCommunityPrice(t, pool, station, "ETHANOL", "L", 6000, "STANDARD", tc.now.Add(-time.Hour), true)
			got, err := store.Evaluate(ctx, owner, ruleID, "", 100, tc.now)
			if err != nil || got.Candidates != tc.want {
				t.Fatalf("schedule %+v %v; want %d", got, err, tc.want)
			}
		})
	}
}
