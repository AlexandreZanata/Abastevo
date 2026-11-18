package domain

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func parseKV(raw string) map[string]string {
	m := map[string]string{}
	for _, part := range strings.Split(raw, ";") {
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			m[kv[0]] = kv[1]
		}
	}
	return m
}

func optIntStr(v *int64) string {
	if v == nil {
		return "NONE"
	}
	return strconv.FormatInt(*v, 10)
}

func TestRouteSuggestionsSharedVectors(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "..", "contracts", "testdata", "enterprise", "route", "suggestions-v1.psv")
	f, err := os.Open(path)
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
	if len(rows) != 8 {
		t.Fatalf("expected header + 7 rows, got %d", len(rows))
	}
	for _, row := range rows[1:] {
		id, op, args, want := row[0], row[1], parseKV(row[2]), parseKV(row[3])
		t.Run(id, func(t *testing.T) {
			vol, _ := strconv.ParseInt(args["volume_ml"], 10, 64)
			baseM, _ := strconv.ParseInt(args["base_m"], 10, 64)
			baseS, _ := strconv.ParseInt(args["base_s"], 10, 64)
			freshLimit, _ := strconv.ParseInt(args["fresh_limit_s"], 10, 64)
			var cons *AssetConsumptionRef
			if args["cons_m"] != "NONE" {
				cm, _ := strconv.ParseInt(args["cons_m"], 10, 64)
				cml, _ := strconv.ParseInt(args["cons_ml"], 10, 64)
				cons = &AssetConsumptionRef{DistanceMetres: cm, VolumeMilli: cml}
			}

			switch op {
			case "EVALUATE":
				refPrice, _ := strconv.ParseInt(args["ref_price"], 10, 64)
				refAge, _ := strconv.ParseInt(args["ref_age_s"], 10, 64)
				candPrice, _ := strconv.ParseInt(args["cand_price"], 10, 64)
				candAge, _ := strconv.ParseInt(args["cand_age_s"], 10, 64)
				candM, _ := strconv.ParseInt(args["cand_m"], 10, 64)
				candS, _ := strconv.ParseInt(args["cand_s"], 10, 64)
				ref := &FuelPriceReference{
					SourceKind:       PriceSourceKind(args["ref_source"]),
					Product:          args["ref_product"],
					Unit:             args["ref_unit"],
					PaymentCondition: args["ref_pay"],
					PriceMilliBRL:    refPrice,
					AgeSeconds:       refAge,
				}
				cand := StationCandidate{
					StationID:   id,
					StationName: "Posto " + id,
					Price: &FuelPriceReference{
						SourceKind:       PriceSourceKind(args["cand_source"]),
						Product:          args["cand_product"],
						Unit:             args["cand_unit"],
						PaymentCondition: args["cand_pay"],
						PriceMilliBRL:    candPrice,
						AgeSeconds:       candAge,
					},
					RouteTotalMetres:   candM,
					RouteTotalSeconds:  candS,
					HasPaidPlan:        args["paid_plan"] == "true",
					HasActiveAgreement: args["agreement"] == "true",
				}
				got, err := EvaluateSuggestion(args["product"], vol, baseM, baseS, cons, freshLimit, ref, cand)
				if err != nil {
					t.Fatalf("evaluate failed: %v", err)
				}
				if strconv.FormatBool(got.Comparable) != want["comparable"] ||
					strconv.FormatInt(got.DetourMetres, 10) != want["detour_m"] ||
					strconv.FormatInt(got.DetourSeconds, 10) != want["detour_s"] ||
					optIntStr(got.DetourFuelMilli) != want["detour_fuel_ml"] ||
					optIntStr(got.DetourCostCentavos) != want["detour_cost_centavos"] ||
					optIntStr(got.ReferenceCentavos) != want["ref_cost_centavos"] ||
					optIntStr(got.CandidateCentavos) != want["cand_cost_centavos"] ||
					optIntStr(got.GrossDiffCentavos) != want["gross_diff_centavos"] ||
					optIntStr(got.NetDiffCentavos) != want["net_diff_centavos"] ||
					strconv.FormatBool(got.DetourExceedsDiff) != want["detour_exceeds"] {
					t.Fatalf("vector mismatch got=%+v want=%v", got, want)
				}
			case "RANK":
				ref := &FuelPriceReference{SourceKind: PriceSourceCommunity, Product: args["product"], Unit: "LITRE", PaymentCondition: "CASH", PriceMilliBRL: 6490, AgeSeconds: 3600}
				ap, _ := strconv.ParseInt(args["a_price"], 10, 64)
				am, _ := strconv.ParseInt(args["a_m"], 10, 64)
				bp, _ := strconv.ParseInt(args["b_price"], 10, 64)
				bm, _ := strconv.ParseInt(args["b_m"], 10, 64)
				evA, err := EvaluateSuggestion(args["product"], vol, baseM, baseS, cons, freshLimit, ref, StationCandidate{
					StationID: args["a_id"], StationName: "A", RouteTotalMetres: am, RouteTotalSeconds: baseS,
					Price:              &FuelPriceReference{SourceKind: PriceSourceCommunity, Product: args["product"], Unit: "LITRE", PaymentCondition: "CASH", PriceMilliBRL: ap, AgeSeconds: 3600},
					HasPaidPlan:        args["a_paid"] == "true",
					HasActiveAgreement: args["a_agree"] == "true",
				})
				if err != nil {
					t.Fatal(err)
				}
				evB, err := EvaluateSuggestion(args["product"], vol, baseM, baseS, cons, freshLimit, ref, StationCandidate{
					StationID: args["b_id"], StationName: "B", RouteTotalMetres: bm, RouteTotalSeconds: baseS,
					Price:              &FuelPriceReference{SourceKind: PriceSourceCommunity, Product: args["product"], Unit: "LITRE", PaymentCondition: "CASH", PriceMilliBRL: bp, AgeSeconds: 3600},
					HasPaidPlan:        args["b_paid"] == "true",
					HasActiveAgreement: args["b_agree"] == "true",
				})
				if err != nil {
					t.Fatal(err)
				}
				ranked := RankSuggestions([]SuggestionEvaluation{evA, evB})
				if ranked[0].StationID != want["first"] || ranked[1].StationID != want["second"] {
					t.Fatalf("rank order wrong: got %s, %s; want %s, %s", ranked[0].StationID, ranked[1].StationID, want["first"], want["second"])
				}
			default:
				t.Fatalf("unknown op %s", op)
			}
		})
	}
}
