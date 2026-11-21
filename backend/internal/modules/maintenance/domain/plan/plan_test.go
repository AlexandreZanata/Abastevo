package plan_test

import (
	"encoding/csv"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/plan"
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

func day(t *testing.T, raw string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", raw)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func i64(t *testing.T, raw string) int64 {
	t.Helper()
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestMaintenanceGoldenVectorsVAL31(t *testing.T) {
	f, err := os.Open("../../../../../../contracts/testdata/enterprise/maintenance/maintenance-due-v1.psv")
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
			case "EVAL_PLAN", "PROJECT_DUE":
				baseM := i64(t, args["base_m"])
				intM := i64(t, args["int_m"])
				leadM := i64(t, args["lead_m"])
				months := int(i64(t, args["int_months"]))
				leadDays := int(i64(t, args["lead_days"]))
				evalM := i64(t, args["eval_m"])
				in := plan.PlanEvalInput{
					Plan: plan.PlanSpec{
						DistanceIntervalM:  &intM,
						DistanceLeadM:      &leadM,
						TimeIntervalMonths: &months,
						TimeLeadDays:       &leadDays,
						BaselineDay:        day(t, args["base_day"]),
						BaselineOdometerM:  &baseM,
					},
					EvalDay:     day(t, args["eval_day"]),
					CurrentOdoM: &evalM,
				}
				if op == "PROJECT_DUE" {
					wd := day(t, args["win_start_day"])
					wm := i64(t, args["win_start_m"])
					in.WindowStartDay = &wd
					in.WindowStartOdoM = &wm
				}
				res, err := plan.Evaluate(in)
				if err != nil {
					t.Fatal(err)
				}
				if *res.UsedM != i64(t, exp["used_m"]) || *res.RemainingM != i64(t, exp["remaining_m"]) || string(res.State) != exp["state"] {
					t.Fatalf("unexpected plan eval: %+v vs %+v", res, exp)
				}
				if op == "EVAL_PLAN" {
					if *res.DaysToDue != int(i64(t, exp["days_to_due"])) || string(res.DecidingTrigger) != exp["trigger"] {
						t.Fatalf("unexpected trigger/days: %+v vs %+v", res, exp)
					}
				} else {
					if res.ProjectedDueDay == nil || res.ProjectedDueDay.Format("2006-01-02") != exp["projected_day"] {
						t.Fatalf("unexpected projected day: %v vs %s", res.ProjectedDueDay, exp["projected_day"])
					}
				}
			case "EVAL_HOURS":
				baseS := i64(t, args["base_s"])
				intS := i64(t, args["int_s"])
				leadS := i64(t, args["lead_s"])
				evalS := i64(t, args["eval_s"])
				res, err := plan.Evaluate(plan.PlanEvalInput{
					Plan: plan.PlanSpec{
						HourIntervalS: &intS,
						HourLeadS:     &leadS,
						BaselineDay:   time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC),
						BaselineHourS: &baseS,
					},
					EvalDay:      time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC),
					CurrentHourS: &evalS,
				})
				if err != nil {
					t.Fatal(err)
				}
				if *res.UsedS != i64(t, exp["used_s"]) || *res.RemainingS != i64(t, exp["remaining_s"]) || string(res.State) != exp["state"] || string(res.DecidingTrigger) != exp["trigger"] {
					t.Fatalf("unexpected hours eval: %+v vs %+v", res, exp)
				}
			case "REBASE_HOURS":
				dueS := i64(t, args["due_s"])
				doneS := i64(t, args["done_s"])
				intS := i64(t, args["int_s"])
				if plan.RebaseNextMeter(dueS, doneS, intS, plan.RebaseActual) != i64(t, exp["next_actual"]) {
					t.Fatal("actual rebase mismatch")
				}
				if plan.RebaseNextMeter(dueS, doneS, intS, plan.RebaseScheduled) != i64(t, exp["next_scheduled"]) {
					t.Fatal("scheduled rebase mismatch")
				}
			case "EVAL_WO":
				lines := []plan.WorkOrderLine{
					{Kind: plan.LinePart, QuantityMilli: i64(t, args["oil_ml"]), UnitPriceMilli: i64(t, args["oil_price"])},
					{Kind: plan.LinePart, QuantityMilli: i64(t, args["filter_u"]), UnitPriceMilli: i64(t, args["filter_price"])},
					{Kind: plan.LineLabor, QuantityMilli: i64(t, args["labor_s"]), UnitPriceMilli: i64(t, args["labor_rate"])},
				}
				rec, err := plan.EvaluateWorkOrder(lines, i64(t, args["approved"]), int(i64(t, args["tol_pct"])), i64(t, args["discount"]), i64(t, args["paid"]))
				if err != nil {
					t.Fatal(err)
				}
				if rec.OverrunCeiling != i64(t, exp["overrun_ceiling"]) || rec.PartsCentavos != i64(t, exp["parts_cents"]) || rec.ServiceCentavos != i64(t, exp["service_cents"]) || rec.SumCentavos != i64(t, exp["sum_cents"]) || strconv.FormatBool(rec.ArithmeticMismatch) != exp["mismatch"] || strconv.FormatBool(rec.OverrunExceeded) != exp["overrun"] {
					t.Fatalf("unexpected WO reconciliation: %+v vs %+v", rec, exp)
				}
			default:
				t.Fatalf("unknown op %s", op)
			}
		})
	}

	if err := plan.ValidateReview("m-1", "m-1", nil, time.Now()); !errors.Is(err, plan.ErrSelfApproval) {
		t.Fatal("expected self-approval rejection")
	}
	expired := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := plan.ValidateReview("m-1", "m-2", &expired, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)); !errors.Is(err, plan.ErrQuoteExpired) {
		t.Fatal("expected expired quote rejection")
	}
	unavail := plan.UnavailableSeconds([]plan.HoldWindow{
		{Start: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 10, 3, 6, 0, 0, 0, time.UTC)},
		{Start: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 10, 3, 6, 0, 0, 0, time.UTC)},
	}, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
	if unavail != 194400 {
		t.Fatalf("expected merged unavailability 194400s, got %d", unavail)
	}
}
