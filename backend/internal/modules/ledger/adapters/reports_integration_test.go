//go:build integration

package adapters

import (
	"context"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/application"
)

func reportExpense(t *testing.T, id, vehicle, occurred, category string) application.Command {
	t.Helper()
	payload := `{"id":"` + id + `","vehicle_id":"` + vehicle + `","occurred_at":"` + occurred + `","timezone":"America/Sao_Paulo","amount_centavos":1000,"category":"` + category + `"}`
	return application.Command{OperationID: newid(t), Dataset: "expense", ID: id, Payload: []byte(payload)}
}

func TestReportMonthLeapDSTCrossingEmptyAndPlans(t *testing.T) {
	pool := freshDB(t)
	owner := newAccount(t, pool)
	vehicle := newVehicle(t, pool, owner)
	store := NewStore(pool)
	ensureConsent(t, store, owner)
	ctx := context.Background()
	leap, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{reportExpense(t, newid(t), vehicle, "2024-02-29T10:00:00-03:00", "tolls")}})
	_ = leap
	if err != nil {
		t.Fatal(err)
	}
	feb24, err := store.BuildReport(ctx, owner, 2024, 2, "America/Sao_Paulo")
	if err != nil || feb24.Totals.ExpenseCentavos != 1000 {
		t.Fatalf("leap %+v %v", feb24, err)
	}
	feb23, err := store.BuildReport(ctx, owner, 2023, 2, "America/Sao_Paulo")
	if err != nil || feb23.Totals.ExpenseCentavos != 0 || feb23.Totals.RecordCount != 0 {
		t.Fatalf("empty %+v %v", feb23, err)
	}
	crossID := newid(t)
	if _, err = store.Apply(ctx, owner, application.Batch{Commands: []application.Command{reportExpense(t, crossID, vehicle, "2026-03-01T02:00:00Z", "parking")}}); err != nil {
		t.Fatal(err)
	}
	febSP, _ := store.BuildReport(ctx, owner, 2026, 2, "America/Sao_Paulo")
	marUTC, _ := store.BuildReport(ctx, owner, 2026, 3, "UTC")
	if febSP.Totals.ExpenseCentavos != 1000 || marUTC.Totals.ExpenseCentavos != 1000 {
		t.Fatalf("crossing feb=%d mar=%d", febSP.Totals.ExpenseCentavos, marUTC.Totals.ExpenseCentavos)
	}
	dstID := newid(t)
	if _, err = store.Apply(ctx, owner, application.Batch{Commands: []application.Command{reportExpense(t, dstID, vehicle, "2026-03-08T07:30:00Z", "tolls")}}); err != nil {
		t.Fatal("dst", err)
	}
	marNY, err := store.BuildReport(ctx, owner, 2026, 3, "America/New_York")
	if err != nil || marNY.Totals.RecordCount == 0 {
		t.Fatalf("dst month %+v %v", marNY, err)
	}
	if _, err = store.BuildReport(ctx, owner, 2026, 13, "America/Sao_Paulo"); err == nil {
		t.Fatal("invalid month admitted")
	}
	if _, err = store.BuildReport(ctx, owner, 2026, 3, "Mars/Olympus"); err == nil {
		t.Fatal("invalid timezone admitted")
	}
	foreign := newAccount(t, pool)
	if _, err = store.BuildReport(ctx, foreign, 2026, 3, "America/Sao_Paulo"); err != nil {
		t.Fatal("foreign empty month", err)
	}
	plan, err := pool.Query(ctx, "EXPLAIN SELECT dataset,id,vehicle_id,occurred_at,payload FROM ledger_records WHERE account_id=$1 AND NOT deleted AND NOT voided AND occurred_at>=$2 AND occurred_at<$3 ORDER BY occurred_at,id", owner, "2026-01-01T00:00:00Z", "2027-01-01T00:00:00Z")
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

func TestReportBackdatedStaleRestart(t *testing.T) {
	pool := freshDB(t)
	owner := newAccount(t, pool)
	vehicle := newVehicle(t, pool, owner)
	store := NewStore(pool)
	ensureConsent(t, store, owner)
	ctx := context.Background()
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{reportExpense(t, newid(t), vehicle, "2026-03-10T10:00:00-03:00", "tolls")}}); err != nil {
		t.Fatal(err)
	}
	first, err := store.BuildReport(ctx, owner, 2026, 3, "America/Sao_Paulo")
	if err != nil || first.Revision != 1 {
		t.Fatalf("first %+v %v", first, err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	staleErr := make(chan error, 1)
	go func() {
		defer wg.Done()
		_, e := store.BuildReport(ctx, owner, 2026, 3, "America/Sao_Paulo")
		staleErr <- e
	}()
	backdated := reportExpense(t, newid(t), vehicle, "2026-03-05T10:00:00-03:00", "parking")
	backdated.OperationID = newid(t)
	if _, err = store.Apply(ctx, owner, application.Batch{Commands: []application.Command{backdated}}); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	_ = <-staleErr
	second, err := store.BuildReport(ctx, owner, 2026, 3, "America/Sao_Paulo")
	if err != nil || second.Totals.ExpenseCentavos != 2000 {
		t.Fatalf("invalidated %+v %v", second, err)
	}
	var wg2 sync.WaitGroup
	revisions := make(chan int64, 2)
	for i := 0; i < 2; i++ {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			r, e := store.BuildReport(ctx, owner, 2026, 3, "America/Sao_Paulo")
			if e != nil {
				return
			}
			revisions <- r.Revision
		}()
	}
	wg2.Wait()
	close(revisions)
	for rev := range revisions {
		if rev != second.Revision {
			t.Fatalf("restart published superseded %d want %d", rev, second.Revision)
		}
	}
	got, err := store.GetReport(ctx, owner, 2026, 3, "America/Sao_Paulo")
	if err != nil || got.Revision != second.Revision || got.Totals.ExpenseCentavos != 2000 {
		t.Fatalf("coherent %+v %v", got, err)
	}
}
