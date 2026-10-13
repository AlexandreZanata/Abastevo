package domain

import (
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

const (
	stationA = "11111111-1111-4111-8111-111111111111"
	stationB = "22222222-2222-4222-8222-222222222222"
)

func baseRule(t *testing.T) Rule {
	t.Helper()
	return Rule{
		Predicates: []Predicate{{Kind: Threshold, Fuel: kernel.Ethanol, Unit: kernel.Liter, Condition: kernel.Standard, Source: Community, ThresholdMilli: 6200}},
		Operator:   "AND",
		Target:     Target{StationIDs: []string{stationA}},
		Schedule:   Schedule{Days: []int{0, 1, 2, 3, 4, 5, 6}, WindowStart: "00:00", WindowEnd: "00:00", Timezone: "America/Sao_Paulo", QuietStart: "00:00", QuietEnd: "00:00"},
	}
}

func TestPredicateBoundsAndNesting(t *testing.T) {
	r, err := NormalizeRule(baseRule(t))
	if err != nil {
		t.Fatal(err)
	}
	_ = r
	bad := baseRule(t)
	bad.Predicates = append(bad.Predicates, bad.Predicates[0], bad.Predicates[0], bad.Predicates[0])
	if _, err = NormalizeRule(bad); err == nil {
		t.Fatal("unbounded predicates admitted")
	}
	bad = baseRule(t)
	bad.Operator = "XOR"
	if _, err = NormalizeRule(bad); err == nil {
		t.Fatal("arbitrary operator admitted")
	}
	bad = baseRule(t)
	bad.Predicates[0].Fuel = kernel.LPGP13
	if _, err = NormalizeRule(bad); err == nil {
		t.Fatal("unsupported fuel admitted")
	}
	bad = baseRule(t)
	bad.Predicates[0].Unit = kernel.CubicMetre
	if _, err = NormalizeRule(bad); err == nil {
		t.Fatal("unit mismatch admitted")
	}
	bad = baseRule(t)
	bad.Predicates[0].Kind = Change
	bad.Predicates[0].ThresholdMilli = 0
	bad.Predicates[0].DropMilli = 0
	bad.Predicates[0].DropPercentBP = 0
	if _, err = NormalizeRule(bad); err == nil {
		t.Fatal("empty change admitted")
	}
	bad = baseRule(t)
	bad.Target = Target{StationIDs: []string{stationA}, City: "Sao Paulo"}
	if _, err = NormalizeRule(bad); err == nil {
		t.Fatal("dual target admitted")
	}
	bad = baseRule(t)
	bad.Target = Target{}
	if _, err = NormalizeRule(bad); err == nil {
		t.Fatal("empty target admitted")
	}
}

func TestMidnightWindowDSTFoldGap(t *testing.T) {
	r := baseRule(t)
	r.Schedule.WindowStart = "22:00"
	r.Schedule.WindowEnd = "06:00"
	r.Schedule.Timezone = "America/New_York"
	rule, err := NormalizeRule(r)
	if err != nil {
		t.Fatal(err)
	}
	at := func(s string) time.Time {
		parsed, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	price := Price{Source: Community, Fuel: kernel.Ethanol, Unit: kernel.Liter, Condition: kernel.Standard, Milli: 6000}
	for _, when := range []string{"2026-03-08T06:30:00Z", "2026-11-01T05:30:00Z", "2026-11-01T06:30:00Z"} {
		first, firstReasons := Evaluate(rule, price, nil, at(when), nil)
		second, secondReasons := Evaluate(rule, price, nil, at(when), nil)
		if first != second || len(firstReasons) != len(secondReasons) {
			t.Fatal("nondeterministic DST evaluation")
		}
	}
	night, _ := time.Parse(time.RFC3339, "2026-03-09T02:00:00-04:00")
	day, _ := time.Parse(time.RFC3339, "2026-03-09T14:00:00-04:00")
	if ok, _ := Evaluate(rule, price, nil, night, nil); !ok {
		t.Fatal("midnight window refused night")
	}
	if ok, _ := Evaluate(rule, price, nil, day, nil); ok {
		t.Fatal("midnight window admitted day")
	}
	r.Schedule.Timezone = "Mars/Olympus"
	if _, err = NormalizeRule(r); err == nil {
		t.Fatal("timezone change admitted")
	}
}

func TestSourcesConditionsBaselineAndCooldown(t *testing.T) {
	r := baseRule(t)
	rule, err := NormalizeRule(r)
	if err != nil {
		t.Fatal(err)
	}
	now, _ := time.Parse(time.RFC3339, "2026-03-09T12:00:00-03:00")
	price := Price{Source: Community, Fuel: kernel.Ethanol, Unit: kernel.Liter, Condition: kernel.Standard, Milli: 6000}
	if ok, reasons := Evaluate(rule, price, nil, now, nil); !ok || !containsReason(reasons, "threshold-met") {
		t.Fatalf("threshold %+v %v", ok, reasons)
	}
	official := price
	official.Source = Official
	if ok, reasons := Evaluate(rule, official, nil, now, nil); ok || !containsReason(reasons, "incompatible-source") {
		t.Fatalf("source mixed %+v %v", ok, reasons)
	}
	loyal := price
	loyal.Condition = kernel.Loyalty
	if ok, reasons := Evaluate(rule, loyal, nil, now, nil); ok || !containsReason(reasons, "incompatible-condition") {
		t.Fatalf("condition mixed %+v %v", ok, reasons)
	}
	change := baseRule(t)
	change.Predicates[0] = Predicate{Kind: Change, Fuel: kernel.Ethanol, Unit: kernel.Liter, Condition: kernel.Standard, Source: Community, DropMilli: 200, BaselineHours: 24}
	changeRule, err := NormalizeRule(change)
	if err != nil {
		t.Fatal(err)
	}
	if ok, reasons := Evaluate(changeRule, price, nil, now, nil); ok || !containsReason(reasons, "missing-baseline") {
		t.Fatalf("baseline missing %+v %v", ok, reasons)
	}
	baseline := &Price{Source: Community, Fuel: kernel.Ethanol, Unit: kernel.Liter, Condition: kernel.Standard, Milli: 6500}
	if ok, reasons := Evaluate(changeRule, price, baseline, now, nil); !ok || !containsReason(reasons, "change-met") {
		t.Fatalf("change %+v %v", ok, reasons)
	}
	recent := now.Add(-30 * time.Minute)
	cooled := baseRule(t)
	cooled.Schedule.CooldownHours = 1
	cooledRule, _ := NormalizeRule(cooled)
	if ok, reasons := Evaluate(cooledRule, price, nil, now, &recent); ok || !containsReason(reasons, "cooldown-active") {
		t.Fatalf("cooldown %+v %v", ok, reasons)
	}
	quiet := baseRule(t)
	quiet.Schedule.QuietStart = "00:00"
	quiet.Schedule.QuietEnd = "23:59"
	quietRule, _ := NormalizeRule(quiet)
	if ok, reasons := Evaluate(quietRule, price, nil, now, nil); ok || !containsReason(reasons, "quiet-hours") {
		t.Fatalf("quiet %+v %v", ok, reasons)
	}
}

func containsReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}
