//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/application"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

func mexFill(t *testing.T, id, vehicle, at string, odo *int64) application.Command {
	t.Helper()
	c := refuelCommand(t, id, vehicle, 0)
	var p map[string]any
	_ = json.Unmarshal(c.Payload, &p)
	p["occurred_at"] = at
	if odo != nil {
		p["odometer_metres"] = *odo
	}
	c.Payload, _ = json.Marshal(p)
	return c
}
func TestMEX10ReportMoneyAndErase(t *testing.T) {
	pool := freshDB(t)
	owner := newAccount(t, pool)
	vehicle := newVehicle(t, pool, owner)
	s := NewStore(pool)
	ensureConsent(t, s, owner)
	ctx := context.Background()
	c := reportExpense(t, newid(t), vehicle, "2026-03-01T00:00:00Z", "tolls")
	if _, e := s.Apply(ctx, owner, application.Batch{Commands: []application.Command{c}}); e != nil {
		t.Fatal(e)
	}
	r, e := s.BuildReport(ctx, owner, 2026, 3, "UTC")
	if e != nil {
		t.Fatal(e)
	}
	if r.Totals.ByCategory["tolls"] != 1000 {
		t.Errorf("category is not centavos: %+v", r.Totals)
	}
	if e = s.EraseAll(ctx, owner); e != nil {
		t.Fatal(e)
	}
	if r, e = s.GetReport(ctx, owner, 2026, 3, "UTC"); e == nil {
		t.Errorf("erased private projection still readable: %+v", r)
	}
}
func TestMEX10ReportMissingOdometerAndDeclaredGap(t *testing.T) {
	for _, kind := range []string{"missing", "gap", "segment"} {
		t.Run(kind, func(t *testing.T) {
			pool := freshDB(t)
			owner := newAccount(t, pool)
			vehicle := newVehicle(t, pool, owner)
			s := NewStore(pool)
			ensureConsent(t, s, owner)
			ctx := context.Background()
			a, b, c := int64(100000), int64(200000), int64(300000)
			first := mexFill(t, newid(t), vehicle, "2026-03-01T10:00:00Z", &a)
			middle := mexFill(t, newid(t), vehicle, "2026-03-02T10:00:00Z", &b)
			last := mexFill(t, newid(t), vehicle, "2026-03-03T10:00:00Z", &c)
			if kind == "missing" {
				middle = mexFill(t, middle.ID, vehicle, "2026-03-02T10:00:00Z", nil)
			}
			if _, e := s.Apply(ctx, owner, application.Batch{Commands: []application.Command{first, middle}}); e != nil {
				t.Fatal(e)
			}
			if kind == "segment" {
				if _, e := s.OpenSegment(ctx, owner, vehicle, "reset"); e != nil {
					t.Fatal(e)
				}
			}
			if _, e := s.Apply(ctx, owner, application.Batch{Commands: []application.Command{last}}); e != nil {
				t.Fatal(e)
			}
			before, e := s.BuildReport(ctx, owner, 2026, 3, "UTC")
			if e != nil {
				t.Fatal(e)
			}
			if kind == "gap" {
				if _, e = s.DeclareGap(ctx, owner, vehicle, newid(t), first.ID, last.ID, ""); e != nil {
					t.Fatal(e)
				}
			}
			r, e := s.BuildReport(ctx, owner, 2026, 3, "UTC")
			if e != nil {
				t.Fatal(e)
			}
			want := map[string]string{"missing": "odometer-missing", "gap": "declared-fill-gap", "segment": "odometer-segment-boundary"}[kind]
			if r.Totals.Coverage[vehicle] != want {
				t.Errorf("coverage=%q want %q (before=%+v)", r.Totals.Coverage[vehicle], want, before)
			}
		})
	}
}
func TestMEX10GetRejectsStaleProjection(t *testing.T) {
	pool := freshDB(t)
	o := newAccount(t, pool)
	v := newVehicle(t, pool, o)
	s := NewStore(pool)
	ensureConsent(t, s, o)
	ctx := context.Background()
	c := reportExpense(t, newid(t), v, "2026-03-01T00:00:00Z", "tolls")
	_, e := s.Apply(ctx, o, application.Batch{Commands: []application.Command{c}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.BuildReport(ctx, o, 2026, 3, "UTC"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Delete(ctx, o, "expense", c.ID); e != nil {
		t.Fatal(e)
	}
	if r, e := s.GetReport(ctx, o, 2026, 3, "UTC"); e == nil {
		t.Fatalf("stale private totals returned: %+v", r)
	}
}

func TestMEX10ReportOwnerLockBlocksCorrectionUntilCommitOrCancel(t *testing.T) {
	for _, cancelBuild := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelBuild), func(t *testing.T) {
			pool := freshDB(t)
			o := newAccount(t, pool)
			v := newVehicle(t, pool, o)
			s := NewStore(pool)
			ensureConsent(t, s, o)
			ctx := context.Background()
			c := reportExpense(t, newid(t), v, "2026-03-01T00:00:00Z", "tolls")
			if _, e := s.Apply(ctx, o, application.Batch{Commands: []application.Command{c}}); e != nil {
				t.Fatal(e)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			reporter := NewStore(pool)
			calls := 0
			reporter.Paid = func(ctx context.Context, _ pgx.Tx, _ string) error {
				calls++
				if calls == 2 {
					close(entered)
					select {
					case <-release:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				return nil
			}
			buildCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			built := make(chan error, 1)
			go func() { _, e := reporter.BuildReport(buildCtx, o, 2026, 3, "UTC"); built <- e }()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("report barrier not reached")
			}
			c.OperationID = newid(t)
			c.ExpectedRevision = 1
			var payload map[string]any
			_ = json.Unmarshal(c.Payload, &payload)
			payload["amount_centavos"] = 2500
			c.Payload, _ = json.Marshal(payload)
			corrected := make(chan error, 1)
			go func() { _, e := s.Apply(ctx, o, application.Batch{Commands: []application.Command{c}}); corrected <- e }()
			deadline := time.NewTimer(3 * time.Second)
			defer deadline.Stop()
			tick := time.NewTicker(10 * time.Millisecond)
			defer tick.Stop()
			observed := false
			for !observed {
				select {
				case e := <-corrected:
					t.Fatalf("writer escaped report transaction: %v", e)
				case <-deadline.C:
					t.Fatal("writer lock was not observed")
				case <-tick.C:
					if e := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%SELECT revision FROM ledger_consent%')`).Scan(&observed); e != nil {
						t.Fatal(e)
					}
				}
			}
			if cancelBuild {
				cancel()
			} else {
				close(release)
			}
			select {
			case e := <-built:
				if (e != nil) != cancelBuild {
					t.Fatalf("build cancellation=%v err=%v", cancelBuild, e)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("report stuck")
			}
			select {
			case e := <-corrected:
				if e != nil {
					t.Fatal(e)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("cancel/commit leaked owner lock")
			}
			r, e := s.BuildReport(ctx, o, 2026, 3, "UTC")
			if e != nil || r.Totals.ExpenseCentavos != 2500 {
				t.Fatalf("correction not converged %+v %v", r, e)
			}
		})
	}
}

func TestMEX10ConsumptionProjectionBoundsOwnershipVoidGap(t *testing.T) {
	pool := freshDB(t)
	o := newAccount(t, pool)
	v := newVehicle(t, pool, o)
	s := NewStore(pool)
	ensureConsent(t, s, o)
	ctx := context.Background()
	a, b := int64(100000), int64(200000)
	first := mexFill(t, newid(t), v, "2026-03-01T00:00:00Z", &a)
	last := mexFill(t, newid(t), v, "2026-03-02T00:00:00Z", &b)
	if _, e := s.Apply(ctx, o, application.Batch{Commands: []application.Command{first, last}}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.ConsumptionRows(ctx, o, v, "2026-03-01T00:00:00Z", "2026-04-01T00:00:00Z", 1); e == nil {
		t.Fatal("silently truncated consumption")
	}
	rows, e := s.ConsumptionRows(ctx, o, v, "2026-03-01T00:00:00Z", "2026-04-01T00:00:00Z", 1000)
	if e != nil || len(rows) != 2 {
		t.Fatalf("bounded projection %v %v", len(rows), e)
	}
	foreign := newAccount(t, pool)
	rows, e = s.ConsumptionRows(ctx, foreign, v, "2026-03-01T00:00:00Z", "2026-04-01T00:00:00Z", 1000)
	if e != nil || len(rows) != 0 {
		t.Fatal("foreign rows leaked", e)
	}
	if _, e = s.DeclareGap(ctx, o, v, newid(t), first.ID, last.ID, ""); e != nil {
		t.Fatal(e)
	}
	rows, e = s.ConsumptionRows(ctx, o, v, "2026-03-01T00:00:00Z", "2026-04-01T00:00:00Z", 1000)
	if e != nil || !rows[0].HasGap {
		t.Fatal("gap evidence lost", e)
	}
	if _, e = s.Apply(ctx, o, application.Batch{Commands: []application.Command{{OperationID: newid(t), ID: last.ID, Dataset: "refuel", ExpectedRevision: 1, Void: true}}}); e != nil {
		t.Fatal(e)
	}
	rows, e = s.ConsumptionRows(ctx, o, v, "2026-03-01T00:00:00Z", "2026-04-01T00:00:00Z", 1000)
	if e != nil || len(rows) != 1 {
		t.Fatal("voided row counted", e)
	}
}

func TestMEX10ReportInvalidationDoesNotDependOnLargestWallClock(t *testing.T) {
	pool := freshDB(t)
	o := newAccount(t, pool)
	v := newVehicle(t, pool, o)
	s := NewStore(pool)
	ensureConsent(t, s, o)
	ctx := context.Background()
	a := reportExpense(t, newid(t), v, "2026-03-01T00:00:00Z", "tolls")
	b := reportExpense(t, newid(t), v, "2026-03-02T00:00:00Z", "tolls")
	if _, e := s.Apply(ctx, o, application.Batch{Commands: []application.Command{a, b}}); e != nil {
		t.Fatal(e)
	}
	if _, e := pool.Exec(ctx, "UPDATE ledger_records SET updated_at='2100-01-01' WHERE account_id=$1 AND id=$2", o, a.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := s.BuildReport(ctx, o, 2026, 3, "UTC"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Delete(ctx, o, "expense", b.ID); e != nil {
		t.Fatal(e)
	}
	r, e := s.BuildReport(ctx, o, 2026, 3, "UTC")
	if e != nil || r.Totals.ExpenseCentavos != 1000 {
		t.Fatalf("clock hid correction %+v %v", r, e)
	}
}
