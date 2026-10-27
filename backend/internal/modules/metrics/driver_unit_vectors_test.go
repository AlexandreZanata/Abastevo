package metrics

import (
	"bufio"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
)

// DRV-02-T03: the same unit/financial rows execute on Kotlin/JVM. Operations
// owned only by the client form or local review are listed explicitly so a
// new operation cannot be silently ignored by this consumer.
func TestDriverUnitVectors(t *testing.T) {
	f, err := os.Open("../../../../contracts/testdata/driver-suite/driver-unit-v1.psv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() || sc.Text() != "id|operation|args|expected" {
		t.Fatal("unexpected driver unit fixture schema")
	}
	kv := func(s string) map[string]string {
		m := map[string]string{}
		for _, part := range strings.Split(s, ";") {
			k, v, _ := strings.Cut(part, "=")
			m[k] = v
		}
		return m
	}
	num := func(t *testing.T, s string) int64 {
		t.Helper()
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	fill := func(t *testing.T, s string) Fill {
		t.Helper()
		p := strings.Split(s, ":")
		if len(p) != 3 {
			t.Fatalf("invalid fill %q", s)
		}
		return Fill{QuantityMilli: num(t, p[0]), TotalCentavos: num(t, p[1]), Full: p[2] == "F", Fuel: "GASOLINE_REGULAR", Unit: "L"}
	}
	seen := map[string]bool{}
	executed := 0
	for sc.Scan() {
		cols := strings.Split(sc.Text(), "|")
		if len(cols) != 4 || cols[0] == "" || seen[cols[0]] {
			t.Fatalf("invalid or duplicate row %q", sc.Text())
		}
		seen[cols[0]] = true
		id, op, args, want := cols[0], cols[1], kv(cols[2]), kv(cols[3])
		switch op {
		case "KM_INPUT", "LEGACY_REVIEW":
			continue // client-only: backend stores/receives integer metres
		case "INTERVAL", "COST", "SUM":
		default:
			t.Fatalf("%s: unknown operation %s", id, op)
		}
		executed++
		t.Run(id, func(t *testing.T) {
			got := map[string]string{}
			switch op {
			case "INTERVAL":
				in := Interval{StartingFillExcluded: fill(t, args["anchor"]), DistanceMetres: num(t, args["distance"])}
				for _, s := range strings.Split(args["fills"], ",") {
					in.Fills = append(in.Fills, fill(t, s))
				}
				r, err := in.Compute()
				if err != nil {
					t.Fatal(err)
				}
				got = map[string]string{"unknown": r.Unknown}
				if r.Unknown == "" {
					got["quantity"] = strconv.FormatInt(r.EligibleQuantityMilli, 10)
					got["cost"] = strconv.FormatInt(r.EligibleCostCentavos, 10)
					got["distance"] = strconv.FormatInt(r.DistanceMetres, 10)
					got["km_per_unit"], got["cost_per_km"] = r.KmPerUnit, r.CentavosPerKm
				}
			case "COST":
				r := FuelPurchasePerKm(num(t, args["total"]), num(t, args["distance"]))
				got = map[string]string{"unknown": r.Unknown, "cost_per_km": r.CentavosPerKm, "method": r.Method}
			case "SUM":
				var windows []Result
				for _, w := range strings.Split(args["windows"], ",") {
					p := strings.Split(w, ":")
					windows = append(windows, Result{DistanceMetres: num(t, p[0]), EligibleQuantityMilli: num(t, p[1])})
				}
				got = map[string]string{"km_per_unit": SumIntervals(windows).KmPerUnit}
			}
			for k, v := range want {
				if strings.HasPrefix(k, "display") {
					continue // presentation is owned by the Kotlin formatter
				}
				if got[k] != v {
					t.Fatalf("%s got %q want %q", k, got[k], v)
				}
			}
		})
	}
	if err := sc.Err(); err != nil || len(seen) != 19 || executed != 7 {
		t.Fatalf("fixture changed: rows=%d executed=%d err=%v", len(seen), executed, err)
	}
}

// The independent planned oracle remains unchanged: every executed ID that is
// also in the planned file keeps its planned expected value.
func TestDriverUnitVectorsMatchPlannedOracleUnchanged(t *testing.T) {
	raw, err := os.ReadFile("../../../../contracts/testdata/driver-suite/planned-financial-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Vectors []struct {
			ID       string                 `json:"id"`
			Expected map[string]interface{} `json:"expected"`
		} `json:"vectors"`
	}
	if err = json.Unmarshal(raw, &doc); err != nil || len(doc.Vectors) != 20 {
		t.Fatalf("planned oracle changed: %v", err)
	}
	want := map[string]map[string]string{
		"DRV-V02": {"km_per_l_rational": "25/2", "centavos_per_km_rational": "52", "display_brl_per_km": "0,52"},
		"DRV-V03": {"km_per_l_rational": "10", "centavos_per_km_rational": "61"},
		"DRV-V04": {"centavos_per_km_rational": "115/2", "display_brl_per_km": "0,58"},
		"DRV-V05": {"km_per_l_rational": "100/7"},
	}
	for _, v := range doc.Vectors {
		for k, expected := range want[v.ID] {
			if v.Expected[k] != expected {
				t.Fatalf("%s.%s planned oracle changed: %v", v.ID, k, v.Expected[k])
			}
		}
		delete(want, v.ID)
	}
	if len(want) != 0 {
		t.Fatalf("planned vectors missing: %v", want)
	}
}
