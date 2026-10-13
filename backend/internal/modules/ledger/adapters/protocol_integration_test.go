//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	ledgerhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/adapters/http"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type pgxTx = pgx.Tx

func newLedgerRouter(h ledgerhttp.Handler) *chi.Mux {
	router := chi.NewRouter()
	h.RegisterRoutes(router)
	return router
}

func itoaLedger(v int64) string { return strconv.FormatInt(v, 10) }

func ledgerHandler(t *testing.T, store *Store, owners map[string]string) ledgerhttp.Handler {
	t.Helper()
	return ledgerhttp.Handler{
		Service: store, Origin: "one.example.test", CursorSecret: []byte(strings.Repeat("s", 32)),
		Sessions: func(_ context.Context, family, token string) (string, error) {
			owner, ok := owners[family]
			if !ok || token != "synthetic" {
				return "", errors.New("ledger: authentication required")
			}
			return owner, nil
		},
		SignedOwner: func(_ *http.Request, _ string) (string, string, error) {
			return "33333333-3333-4333-8333-333333333333", "44444444-4444-4444-8444-444444444444", nil
		},
	}
}

func ledgerCall(h ledgerhttp.Handler, _, path, body string) *httptest.ResponseRecorder {
	router := newLedgerRouter(h)
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func TestLedgerProtocolConcurrentOutageEraseRevoke(t *testing.T) {
	pool := freshDB(t)
	owner := newAccount(t, pool)
	vehicle := newVehicle(t, pool, owner)
	store := NewStore(pool)
	ensureConsent(t, store, owner)
	store.Paid = func(context.Context, pgxTx, string) error { return nil }
	ctx := context.Background()
	_ = ctx
	family := newid(t)
	owners := map[string]string{family: owner}
	h := ledgerHandler(t, store, owners)
	refuelBody := func(op, id string, expected int64, quantity int64) string {
		raw, _ := json.Marshal(map[string]any{"id": id, "vehicle_id": vehicle, "occurred_at": "2026-03-15T10:30:00-03:00", "timezone": "America/Sao_Paulo", "fuel": "ETHANOL", "unit": "L", "quantity_milli": quantity, "total_centavos": 12400, "fill": "full"})
		return `{"family_id":"` + family + `","access_token":"synthetic","commands":[{"operation_id":"` + op + `","dataset":"refuel","id":"` + id + `","expected_revision":` + itoaLedger(expected) + `,"payload":` + string(raw) + `}]}`
	}
	baseID := newid(t)
	w := ledgerCall(h, "", "/v1/ledger/commands", refuelBody(newid(t), baseID, 0, 20000))
	if w.Code != 200 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for i, quantity := range []int64{21000, 22000} {
		wg.Add(1)
		go func(quantity int64) {
			defer wg.Done()
			body := refuelBody(newid(t), baseID, 1, quantity)
			codes <- ledgerCall(h, "", "/v1/ledger/commands", body).Code
		}(quantity)
		_ = i
	}
	wg.Wait()
	close(codes)
	successes := 0
	for code := range codes {
		if code == 200 {
			successes++
		} else if code != 409 {
			t.Fatalf("concurrent code %d", code)
		}
	}
	if successes < 1 {
		t.Fatal("concurrent lost accepted correction")
	}
	dupOp := newid(t)
	dupID := newid(t)
	dupBody := refuelBody(dupOp, dupID, 0, 20000)
	w = ledgerCall(h, "", "/v1/ledger/commands", dupBody)
	if w.Code != 200 {
		t.Fatalf("first duplicate %d", w.Code)
	}
	w = ledgerCall(h, "", "/v1/ledger/commands", dupBody)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"replayed":true`) {
		t.Fatalf("offline replay %d %s", w.Code, w.Body.String())
	}
	if _, err := pool.Exec(ctx, `CREATE FUNCTION reject_ledger_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic'; END $$; CREATE TRIGGER reject_ledger_insert BEFORE INSERT ON ledger_records FOR EACH ROW EXECUTE FUNCTION reject_ledger_insert()`); err != nil {
		t.Fatal(err)
	}
	beforeLedger, beforeOps, _ := counts(t, pool)
	w = ledgerCall(h, "", "/v1/ledger/commands", refuelBody(newid(t), newid(t), 0, 20000))
	if w.Code != 503 {
		t.Fatalf("outage code %d", w.Code)
	}
	afterLedger, afterOps, _ := counts(t, pool)
	if afterLedger != beforeLedger || afterOps != beforeOps {
		t.Fatal("outage left effects")
	}
	if _, err := pool.Exec(ctx, "DROP TRIGGER reject_ledger_insert ON ledger_records"); err != nil {
		t.Fatal(err)
	}
	restarted := NewStore(pool)
	restarted.Paid = store.Paid
	ensureConsent(t, restarted, owner)
	h2 := ledgerHandler(t, restarted, owners)
	w = ledgerCall(h2, "", "/v1/ledger/commands", refuelBody(newid(t), newid(t), 0, 20000))
	if w.Code != 200 {
		t.Fatalf("restart %d", w.Code)
	}
	eraseBody := `{"family_id":"` + family + `","access_token":"synthetic"}`
	w = ledgerCall(h2, "", "/v1/ledger/erase", eraseBody)
	if w.Code != 200 {
		t.Fatalf("erase %d", w.Code)
	}
	listBody := `{"family_id":"` + family + `","access_token":"synthetic","dataset":"refuel","limit":100}`
	w = ledgerCall(h2, "", "/v1/ledger/records", listBody)
	if w.Code != 200 || strings.Contains(w.Body.String(), baseID) {
		t.Fatalf("erasure listed %d %s", w.Code, w.Body.String())
	}
	w = ledgerCall(h2, "", "/v1/ledger/commands", dupBody)
	if w.Code == 200 && strings.Contains(w.Body.String(), `"replayed":true`) {
		t.Fatal("erased replay falsely restored")
	}
	store.Paid = func(context.Context, pgxTx, string) error { return errDenied() }
	restarted.Paid = store.Paid
	h3 := ledgerHandler(t, restarted, owners)
	w = ledgerCall(h3, "", "/v1/ledger/commands", refuelBody(newid(t), newid(t), 0, 20000))
	if w.Code != 403 {
		t.Fatalf("revoked commit admitted %d", w.Code)
	}
	w = ledgerCall(h3, "", "/v1/ledger/records", listBody)
	if w.Code != 200 {
		t.Fatalf("revoked read refused %d", w.Code)
	}
}

func TestLedgerProtocolBoundedMonthVehicleScans(t *testing.T) {
	pool := freshDB(t)
	owner := newAccount(t, pool)
	vehicleA := newVehicle(t, pool, owner)
	vehicleB := newVehicle(t, pool, owner)
	store := NewStore(pool)
	ensureConsent(t, store, owner)
	store.Paid = func(context.Context, pgxTx, string) error { return nil }
	family := newid(t)
	owners := map[string]string{family: owner}
	h := ledgerHandler(t, store, owners)
	for month := 1; month <= 3; month++ {
		for i, vehicle := range []string{vehicleA, vehicleB} {
			for n := 0; n < 6; n++ {
				occurred := "2026-0" + itoaLedger(int64(month)) + "-15T10:30:00-03:00"
				raw, _ := json.Marshal(map[string]any{"id": newid(t), "vehicle_id": vehicle, "occurred_at": occurred, "timezone": "America/Sao_Paulo", "amount_centavos": 1000, "category": "tolls"})
				body := `{"family_id":"` + family + `","access_token":"synthetic","commands":[{"operation_id":"` + newid(t) + `","dataset":"expense","id":"` + newid(t) + `","expected_revision":0,"payload":` + string(raw) + `}]}`
				_ = i
				_ = n
				w := ledgerCall(h, "", "/v1/ledger/commands", body)
				if w.Code != 200 {
					t.Fatalf("seed %d %s", w.Code, w.Body.String())
				}
			}
		}
	}
	_ = vehicleA
	_ = vehicleB
	list := func(extra string) *httptest.ResponseRecorder {
		base := `{"family_id":"` + family + `","access_token":"synthetic","dataset":"expense","limit":10`
		if extra != "" {
			base += `,` + extra
		}
		return ledgerCall(h, "", "/v1/ledger/records", base+`}`)
	}
	w := list(`"from":"2026-02-01T00:00:00-03:00","to":"2026-03-01T00:00:00-03:00"`)
	if w.Code != 200 {
		t.Fatalf("month scan %d", w.Code)
	}
	var page struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		Next string `json:"next_cursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Items) != 10 || page.Next == "" {
		t.Fatalf("bounded page %+v %v", page, err)
	}
	seen := map[string]bool{}
	for _, item := range page.Items {
		seen[item.ID] = true
	}
	w = list(`"from":"2026-02-01T00:00:00-03:00","to":"2026-03-01T00:00:00-03:00","cursor":"` + page.Next + `"`)
	if w.Code != 200 {
		t.Fatalf("second page %d", w.Code)
	}
	var second struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		Next string `json:"next_cursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	for _, item := range second.Items {
		if seen[item.ID] {
			t.Fatal("duplicate across pages")
		}
	}
}
