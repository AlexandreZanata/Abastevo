package domain_test

import (
	"encoding/csv"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/intelligence/domain"
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

func i64(t *testing.T, raw string) int64 {
	t.Helper()
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		t.Fatal(raw, err)
	}
	return v
}

func TestIntelligenceGoldenVectorsVAL10_11_15_20(t *testing.T) {
	if specs := domain.Indicators(); len(specs) != 20 {
		t.Fatalf("expected 20 indicators IND-001..020, got %d", len(specs))
	}

	f, err := os.Open("../../../../../contracts/testdata/enterprise/intelligence/kpi-findings-v1.psv")
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
			case "RATIO_GROUP":
				res := domain.ComputeDistanceConsumption([]domain.RatioInterval{
					{Product: "DIESEL_S10", DistanceM: i64(t, args["i1_m"]), QuantityMilli: i64(t, args["i1_ml"]), CostCentavos: i64(t, args["i1_c"])},
					{Product: "DIESEL_S10", DistanceM: i64(t, args["i2_m"]), QuantityMilli: i64(t, args["i2_ml"]), CostCentavos: i64(t, args["i2_c"])},
					{Product: "DIESEL_S10", DistanceM: i64(t, args["i3_m"]), QuantityMilli: i64(t, args["i3_ml"]), CostCentavos: i64(t, args["i3_c"])},
				})
				if res.Status != "COMPUTED" || res.KmPerUnitRat != exp["km_per_l_rat"] || res.KmPerUnit2DP != exp["km_per_l_2dp"] || res.LitresPer100Km2DP != exp["l_per_100_2dp"] || res.CentsPerKmRounded != i64(t, exp["c_per_km_2dp"]) {
					t.Fatalf("ratio group mismatch: %+v vs %+v", res, exp)
				}
			case "RATIO_HOURS":
				res := domain.ComputeHourConsumption([]domain.RatioInterval{
					{Product: "DIESEL_S10", HourSeconds: i64(t, args["seconds"]), QuantityMilli: i64(t, args["ml"]), CostCentavos: i64(t, args["cents"])},
				})
				if res.Status != "COMPUTED" || res.LitresPerHourRat != exp["l_per_h_rat"] || res.LitresPerHour2DP != exp["l_per_h_2dp"] || res.CentsPerHourRounded != i64(t, exp["c_per_h"]) {
					t.Fatalf("ratio hours mismatch: %+v vs %+v", res, exp)
				}
			case "OP_COST":
				total := i64(t, args["fuel"]) + i64(t, args["maint"]) + i64(t, args["tolls"]) + i64(t, args["parking"])
				res := domain.ComputeDistanceConsumption([]domain.RatioInterval{
					{Product: "DIESEL_S10", DistanceM: i64(t, args["dist_m"]), QuantityMilli: 1, CostCentavos: total},
				})
				if total != i64(t, exp["total_cents"]) || res.CentsPerKmRounded != i64(t, exp["c_per_km_2dp"]) {
					t.Fatalf("operating cost mismatch: total=%d res=%+v", total, res)
				}
			case "AVAIL":
				av := domain.ComputeAvailability(i64(t, args["active_s"]), i64(t, args["unavail_s"]))
				if av.Status != "COMPUTED" || av.AvailableSeconds != i64(t, exp["avail_s"]) || av.Ratio != exp["avail_rat"] || av.Percent1DP != exp["avail_pct_1dp"] {
					t.Fatalf("availability mismatch: %+v vs %+v", av, exp)
				}
			case "PRICE_CMP":
				cmp := domain.EvaluatePriceComparison(domain.PriceComparisonInput{
					FuelingProduct:      "DIESEL_S10",
					FuelingUnit:         "L",
					PlaceKnown:          true,
					PlaceRule:           "SAME_MUNICIPALITY",
					FuelingCondition:    args["fuel_cond"],
					QuantityMilli:       i64(t, args["qty_ml"]),
					PaidCents:           i64(t, args["paid_cents"]),
					HasReference:        true,
					ReferenceSource:     "OFFICIAL",
					ReferenceProduct:    "DIESEL_S10",
					ReferenceUnit:       "L",
					ReferenceCondition:  args["ref_cond"],
					ReferencePriceMilli: i64(t, args["ref_milli"]),
					ReferenceAgeDays:    int(i64(t, args["age_days"])),
					MaxAgeDays:          int(i64(t, args["max_age"])),
					CoverageStations:    int(i64(t, args["cov"])),
					MinCoverageStations: int(i64(t, args["min_cov"])),
				})
				if cmp.Status != exp["status"] {
					t.Fatalf("price status mismatch: %+v vs %+v", cmp, exp)
				}
				if cmp.Status == "COMPUTED" {
					if cmp.ReferenceCents != i64(t, exp["ref_cents"]) || cmp.DifferenceCents != i64(t, exp["diff_cents"]) || cmp.DifferencePerUnitMilli != i64(t, exp["diff_per_unit_milli"]) {
						t.Fatalf("price diff mismatch: %+v vs %+v", cmp, exp)
					}
					if strings.Contains(strings.ToLower(cmp.DisplayNote), "prejuízo") || strings.Contains(strings.ToLower(cmp.DisplayNote), "economia") {
						t.Fatal("prohibited loss/savings wording")
					}
				} else if string(cmp.Reason) != exp["reason"] {
					t.Fatalf("price reason mismatch: %+v vs %+v", cmp, exp)
				}
			case "FIND_FT001":
				sig := domain.EvaluateFT001Duplicate(int(i64(t, args["delta_min"])), int(i64(t, args["window_min"])), args["same_key"] == "true")
				if !sig.Raised || string(sig.Severity) != exp["severity"] {
					t.Fatalf("ft001 mismatch: %+v", sig)
				}
			case "FIND_FT002":
				sig := domain.EvaluateFT002IncoherentMeter(false, i64(t, args["delta_m"]), i64(t, args["delta_s"]), int(i64(t, args["max_kmh"])), false)
				if !sig.Raised || string(sig.Severity) != exp["severity"] {
					t.Fatalf("ft002 mismatch: %+v", sig)
				}
			case "FIND_FT003":
				sig := domain.EvaluateFT003TankCapacity(i64(t, args["qty_ml"]), i64(t, args["cap_ml"]), int(i64(t, args["tol_pct"])), false)
				if !sig.Raised || string(sig.Severity) != exp["severity"] {
					t.Fatalf("ft003 mismatch: %+v", sig)
				}
			case "FIND_FT004":
				sig := domain.EvaluateFT004ConsumptionOutlier(i64(t, args["base_m"]), i64(t, args["base_ml"]), i64(t, args["test_m"]), i64(t, args["test_ml"]), int(i64(t, args["thresh_pct"])))
				if !sig.Raised || string(sig.Severity) != exp["severity"] {
					t.Fatalf("ft004 mismatch: %+v", sig)
				}
			case "FIND_FT005":
				sig := domain.EvaluateFT005DocDivergence(i64(t, args["rec_cents"]), i64(t, args["doc_cents"]), false)
				if !sig.Raised || string(sig.Severity) != exp["severity"] {
					t.Fatalf("ft005 mismatch: %+v", sig)
				}
			case "FIND_FT006":
				sig := domain.EvaluateFT006PriceAboveRef(i64(t, args["unit_milli"]), i64(t, args["ref_milli"]), i64(t, args["diff_cents"]), int(i64(t, args["pct"])), i64(t, args["min_cents"]))
				if !sig.Raised || string(sig.Severity) != exp["severity"] {
					t.Fatalf("ft006 mismatch: %+v", sig)
				}
			default:
				t.Fatalf("unknown op %s", op)
			}
		})
	}

	// Allocation check (EMP-FUEL-010)
	allocs, unassigned, err := domain.AllocateIntervalToTrips(100000, 60000, 500000, []domain.TripShareInput{
		{TripID: "t-1", DistanceM: 300000},
		{TripID: "t-2", DistanceM: 150000},
	})
	if err != nil || len(allocs) != 2 || allocs[0].QuantityMilli+allocs[1].QuantityMilli+unassigned.QuantityMilli != 100000 {
		t.Fatalf("allocation failed: %+v %+v %v", allocs, unassigned, err)
	}
	if err := domain.ValidateFindingDecision(domain.FindingOpen, "m-1", "m-1", "ok"); !errors.Is(err, domain.ErrFindingSelfApproval) {
		t.Fatal("expected self-approval rejection")
	}
}

func BenchmarkSyntheticFleet1000VehiclesKPIAndFindings(b *testing.B) {
	intervals := make([]domain.RatioInterval, 1000)
	for i := range intervals {
		intervals[i] = domain.RatioInterval{
			AssetID:       "asset-" + strconv.Itoa(i),
			Product:       "DIESEL_S10",
			DistanceM:     612000 + int64(i),
			QuantityMilli: 180000 + int64(i%5000),
			CostCentavos:  116820,
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		summary := domain.ComputeDistanceConsumption(intervals)
		if summary.Status != "COMPUTED" {
			b.Fatal("unexpected benchmark status", summary.Status)
		}
		_ = domain.ComputeAvailability(1000*30*86400, 194400)
		_ = domain.EvaluateFT006PriceAboveRef(6490, 6090, 6000, 5, 1000)
	}
}
