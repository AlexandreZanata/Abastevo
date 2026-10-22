package metrics

import (
	"encoding/csv"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The same input rows and exact expected outputs execute on Kotlin/JVM.
func TestFinancialExecutionVectors(t *testing.T) {
	f, err := os.Open("../../../../contracts/testdata/premium/financial-execution-v1.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	wantHeader := strings.Split("id,operation,anchor,fills,distance,fuel_cost,expenses,window_start_odo,window_end_odo,window_start_ms,window_end_ms,has_gap,crosses_segment,odometer_missing,expected_quantity,expected_cost,expected_distance,expected_efficiency,expected_cost_per_km,expected_change,expected_unknown", ",")
	if len(rows) != 37 || !reflect.DeepEqual(rows[0], wantHeader) {
		t.Fatal("unexpected financial fixture schema/count")
	}
	seen := map[string]bool{}
	for _, row := range rows[1:] {
		if row[0] == "" || seen[row[0]] {
			t.Fatalf("duplicate/empty vector ID %q", row[0])
		}
		seen[row[0]] = true
		t.Run(row[0], func(t *testing.T) {
			v := map[string]string{}
			for i, name := range wantHeader {
				v[name] = row[i]
			}
			n := func(s string) int64 { return vectorInt(t, s) }
			var got Result
			change := ""
			switch v["operation"] {
			case "INTERVAL":
				in := Interval{StartingFillExcluded: vectorFill(t, v["anchor"]), DistanceMetres: n(v["distance"]), HasGap: v["has_gap"] == "1", CrossesSegment: v["crosses_segment"] == "1", OdometerMissing: v["odometer_missing"] == "1"}
				for _, s := range vectorItems(t, v["fills"]) {
					in.Fills = append(in.Fills, vectorFill(t, s))
				}
				var err error
				got, err = in.Compute()
				if (err != nil) != (got.Unknown == "input-out-of-bounds") {
					t.Fatalf("bounds error inconsistent: %v %+v", err, got)
				}
			case "SUM", "COMPARE":
				var inputs []Result
				for _, s := range vectorItems(t, v["fills"]) {
					p := vectorParts(t, s, 4)
					inputs = append(inputs, Result{DistanceMetres: n(p[0]), EligibleQuantityMilli: n(p[1]), EligibleCostCentavos: n(p[2]), Unknown: strings.ReplaceAll(p[3], "_", "")})
				}
				if v["operation"] == "SUM" {
					a := SumIntervals(inputs)
					got = Result{DistanceMetres: a.DistanceMetres, EligibleQuantityMilli: a.EligibleQuantityMilli, EligibleCostCentavos: a.EligibleCostCentavos, KmPerUnit: a.KmPerUnit, CentavosPerKm: a.CentavosPerKm, Unknown: a.Unknown}
				} else {
					if len(inputs) != 2 {
						t.Fatal("comparison requires two results")
					}
					a := CompareConsumption(inputs[0], inputs[1])
					change = a.ChangePercent
					got.Unknown = a.Unknown
				}
			case "MATCHED", "COST":
				var a CostResult
				if v["operation"] == "COST" {
					a = FuelPurchasePerKm(n(v["fuel_cost"]), n(v["distance"]))
				} else {
					var expenses []ExpenseEntry
					for _, s := range vectorItems(t, v["expenses"]) {
						p := vectorParts(t, s, 4)
						e := ExpenseEntry{ID: p[0], AmountCentavos: n(p[1]), OccurredAt: time.UnixMilli(n(p[2]))}
						if p[3] != "_" {
							odo := n(p[3])
							e.OdometerMetres = &odo
						}
						expenses = append(expenses, e)
					}
					a = MatchedWindowCost(n(v["fuel_cost"]), n(v["distance"]), expenses, Window{StartOdometer: n(v["window_start_odo"]), EndOdometer: n(v["window_end_odo"]), StartTime: time.UnixMilli(n(v["window_start_ms"])), EndTime: time.UnixMilli(n(v["window_end_ms"])), HasGap: v["has_gap"] == "1", CrossesSegment: v["crosses_segment"] == "1"})
				}
				got = Result{EligibleCostCentavos: a.TotalCentavos, DistanceMetres: a.DistanceMetres, CentavosPerKm: a.CentavosPerKm, Unknown: a.Unknown}
			default:
				t.Fatalf("unknown operation %q", v["operation"])
			}
			want := Result{EligibleQuantityMilli: n(v["expected_quantity"]), EligibleCostCentavos: n(v["expected_cost"]), DistanceMetres: n(v["expected_distance"]), KmPerUnit: v["expected_efficiency"], CentavosPerKm: v["expected_cost_per_km"], Unknown: v["expected_unknown"]}
			if got != want || change != v["expected_change"] {
				t.Fatalf("got %+v change=%q; want %+v change=%q", got, change, want, v["expected_change"])
			}
		})
	}
}

func vectorInt(t *testing.T, s string) int64 {
	t.Helper()
	if s == "" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func vectorParts(t *testing.T, s string, count int) []string {
	t.Helper()
	p := strings.Split(s, ":")
	if len(p) != count {
		t.Fatalf("invalid tuple %q", s)
	}
	return p
}

func vectorFill(t *testing.T, s string) Fill {
	t.Helper()
	p := vectorParts(t, s, 5)
	if p[2] != "0" && p[2] != "1" {
		t.Fatal("invalid full flag")
	}
	return Fill{QuantityMilli: vectorInt(t, p[0]), TotalCentavos: vectorInt(t, p[1]), Full: p[2] == "1", Fuel: p[3], Unit: p[4]}
}

func vectorItems(t *testing.T, s string) []string {
	t.Helper()
	var items []string
	if s == "" {
		return items
	}
	for _, item := range strings.Split(s, ";") {
		count := int64(1)
		if prefix, body, ok := strings.Cut(item, "*"); ok {
			count = vectorInt(t, prefix)
			item = body
		}
		if count < 1 || count > 20000 || int64(len(items))+count > 20000 {
			t.Fatal("fixture repeat exceeds bound")
		}
		for i := int64(0); i < count; i++ {
			items = append(items, strings.ReplaceAll(item, "#", strconv.FormatInt(i, 10)))
		}
	}
	return items
}
