package domain_test

import (
	"encoding/csv"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fuelstock/domain"
)

func parseKV(s string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(s, ";") {
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			out[kv[0]] = kv[1]
		}
	}
	return out
}

func i64(t *testing.T, m map[string]string, k string) int64 {
	t.Helper()
	v, err := strconv.ParseInt(m[k], 10, 64)
	if err != nil {
		t.Fatalf("parse %s=%q: %v", k, m[k], err)
	}
	return v
}

func TestStockValuationGoldenVectors(t *testing.T) {
	f, err := os.Open("../../../../../contracts/testdata/enterprise/stock/stock-valuation-v1.psv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comma = '|'
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows[1:] {
		id, op, args, exp := row[0], row[1], parseKV(row[2]), parseKV(row[3])
		t.Run(id, func(t *testing.T) {
			switch op {
			case "RECEIVE":
				pool := domain.Pool{Product: args["tank_product"], Unit: "L", QuantityMilli: i64(t, args, "pool_ml"), ValueCentavos: i64(t, args, "pool_cents")}
				next, out, err := pool.Receive(args["product"], i64(t, args, "rx_ml"), i64(t, args, "rx_cents"))
				if exp["status"] == "REJECTED" {
					if !errors.Is(err, domain.ErrProductMismatch) {
						t.Fatalf("expected mismatch error, got %v", err)
					}
					if next.QuantityMilli != i64(t, exp, "after_ml") || next.ValueCentavos != i64(t, exp, "after_cents") {
						t.Fatalf("pool mutated on error: %+v", next)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if out.AfterQuantityMilli != i64(t, exp, "after_ml") || out.AfterValueCentavos != i64(t, exp, "after_cents") || out.RecognizedExpenseCentavos != i64(t, exp, "recognized_expense_cents") {
					t.Fatalf("unexpected outcome: %+v vs %+v", out, exp)
				}
			case "DISPENSE":
				pool := domain.Pool{Product: args["tank_product"], Unit: "L", QuantityMilli: i64(t, args, "pool_ml"), ValueCentavos: i64(t, args, "pool_cents")}
				next, out, err := pool.Dispense(args["product"], i64(t, args, "tx_ml"))
				if exp["status"] == "REJECTED" {
					if !errors.Is(err, domain.ErrInsufficientStock) {
						t.Fatalf("expected insufficient error, got %v", err)
					}
					if next.QuantityMilli != i64(t, exp, "after_ml") || next.ValueCentavos != i64(t, exp, "after_cents") {
						t.Fatalf("pool mutated on error: %+v", next)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if out.MovedCentavos != i64(t, exp, "dispense_cents") || out.AfterQuantityMilli != i64(t, exp, "after_ml") || out.AfterValueCentavos != i64(t, exp, "after_cents") || out.RecognizedExpenseCentavos != i64(t, exp, "recognized_expense_cents") {
					t.Fatalf("unexpected dispense outcome: %+v vs %+v", out, exp)
				}
			case "TRANSFER":
				src := domain.Pool{Product: args["src_product"], Unit: "L", QuantityMilli: i64(t, args, "src_ml"), ValueCentavos: i64(t, args, "src_cents")}
				dst := domain.Pool{Product: args["dst_product"], Unit: "L", QuantityMilli: i64(t, args, "dst_ml"), ValueCentavos: i64(t, args, "dst_cents")}
				ns, nd, out, err := domain.Transfer(src, dst, args["product"], i64(t, args, "tx_ml"))
				if err != nil {
					t.Fatal(err)
				}
				if out.MovedCentavos != i64(t, exp, "transfer_cents") || ns.QuantityMilli != i64(t, exp, "src_after_ml") || ns.ValueCentavos != i64(t, exp, "src_after_cents") || nd.QuantityMilli != i64(t, exp, "dst_after_ml") || nd.ValueCentavos != i64(t, exp, "dst_after_cents") || out.RecognizedExpenseCentavos != 0 {
					t.Fatalf("unexpected transfer outcome: ns=%+v nd=%+v out=%+v", ns, nd, out)
				}
			case "ADJUST":
				pool := domain.Pool{Product: "DIESEL_S10", Unit: "L", QuantityMilli: i64(t, args, "pool_ml"), ValueCentavos: i64(t, args, "pool_cents")}
				next, _, err := pool.Adjust(i64(t, args, "delta_ml"), args["reason"])
				if exp["status"] == "REJECTED" {
					if !errors.Is(err, domain.ErrInvalidInput) {
						t.Fatalf("expected invalid input error, got %v", err)
					}
					if next.QuantityMilli != i64(t, exp, "after_ml") || next.ValueCentavos != i64(t, exp, "after_cents") {
						t.Fatalf("pool mutated on error: %+v", next)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
			default:
				t.Fatalf("unknown op %s", op)
			}
		})
	}
}
