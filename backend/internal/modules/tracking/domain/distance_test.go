package domain_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
)

// EMP-GPS-006 frozen oracle (B-BR-EMP-491..493, BR-GPS-027..029, §10.5,
// VAL-27, EDR-319/320).

func odo(m int64) domain.DistanceSource {
	return domain.DistanceSource{Kind: domain.KindOdometer, Status: domain.SourceOK, Metres: m}
}

func odoMissing(r domain.DistanceReason) domain.DistanceSource {
	return domain.DistanceSource{Kind: domain.KindOdometer, Status: domain.SourceInsufficient, Reason: r}
}

func gpsOK(m int64, q domain.Quality) domain.DistanceSource {
	return domain.DistanceSource{Kind: domain.KindGPS, Status: domain.SourceOK, Metres: m, Quality: q}
}

func gpsMissing(r domain.DistanceReason, q domain.Quality) domain.DistanceSource {
	return domain.DistanceSource{Kind: domain.KindGPS, Status: domain.SourceInsufficient, Reason: r, Quality: q}
}

var (
	gpsWithheld  = domain.DistanceSource{Kind: domain.KindGPS, Status: domain.SourceWithheld}
	noPlan       = domain.DistanceSource{Kind: domain.KindPlanned, Status: domain.SourceInsufficient, Reason: domain.ReasonNoRoutePlan}
	noEstimate   = domain.DistanceSource{Kind: domain.KindEstimate, Status: domain.SourceInsufficient, Reason: domain.ReasonNoRouteCompute}
	planned180   = domain.DistanceSource{Kind: domain.KindPlanned, Status: domain.SourceOK, Metres: 180_000}
	estimate181  = domain.DistanceSource{Kind: domain.KindEstimate, Status: domain.SourceOK, Metres: 181_000}
	defaultLimit = domain.Thresholds{Metres: 2_000, Percent: 10}
)

func evaluate(t *testing.T, o, g, p, e domain.DistanceSource, th domain.Thresholds) domain.Distances {
	t.Helper()
	got, err := domain.EvaluateDistances(o, g, p, e, th)
	if err != nil {
		t.Fatalf("evaluation refused: %v", err)
	}
	if got.Odometer != o || got.GPS != g || got.Planned != p || got.Estimate != e {
		t.Fatalf("a source was not kept apart unchanged: %+v", got)
	}
	return got
}

func TestDistanceThresholdsDefaultsAndRanges(t *testing.T) {
	if domain.DefaultThresholds() != defaultLimit {
		t.Fatal("BR-GPS-029 defaults", domain.DefaultThresholds())
	}
	for _, ok := range [][2]int64{{500, 5}, {10_000, 30}, {2_000, 10}, {500, 30}, {10_000, 5}} {
		th, err := domain.NewThresholds(ok[0], ok[1])
		if err != nil || th != (domain.Thresholds{Metres: ok[0], Percent: ok[1]}) {
			t.Fatal("threshold inside its range refused", ok, err)
		}
	}
	for _, bad := range [][2]int64{{499, 10}, {10_001, 10}, {2_000, 4}, {2_000, 31}, {0, 0}, {-1, 10}} {
		if _, err := domain.NewThresholds(bad[0], bad[1]); !errors.Is(err, domain.ErrDistanceInput) {
			t.Fatal("threshold outside its range accepted", bad)
		}
	}
	for _, bad := range []domain.Thresholds{{}, {Metres: 499, Percent: 10}, {Metres: 2_000, Percent: 31}} {
		if _, err := domain.EvaluateDistances(odo(1), gpsOK(1, domain.QualityPrecise), noPlan, noEstimate, bad); !errors.Is(err, domain.ErrDistanceInput) {
			t.Fatal("evaluation accepted thresholds outside their ranges", bad)
		}
	}
}

// §10.5 worked table, rows 1..6, with the planned 180 000 m (MANUAL) and the
// engine estimate 181 000 m beside them: neither enters any row.
func TestDistanceWorkedExamples(t *testing.T) {
	cases := []struct {
		name       string
		o, g       domain.DistanceSource
		travelled  domain.Travelled
		divergence domain.Divergence
	}{
		{"1 above 2 000 m but 1.3 %", odo(186_400), gpsOK(183_900, domain.QualityPrecise),
			domain.Travelled{Status: domain.SourceOK, Basis: domain.BasisOdometer, Metres: 186_400},
			domain.Divergence{Status: domain.DivergenceNotRaised, Odometer: 186_400, GPS: 183_900, Difference: 2_500, Thresholds: defaultLimit}},
		{"2 signal 15.1 %", odo(186_400), gpsOK(158_300, domain.QualityPrecise),
			domain.Travelled{Status: domain.SourceOK, Basis: domain.BasisOdometer, Metres: 186_400},
			domain.Divergence{Status: domain.DivergenceRaised, Odometer: 186_400, GPS: 158_300, Difference: 28_100, Thresholds: defaultLimit}},
		{"3 13.3 % but not above 2 000 m", odo(12_000), gpsOK(10_400, domain.QualityPrecise),
			domain.Travelled{Status: domain.SourceOK, Basis: domain.BasisOdometer, Metres: 12_000},
			domain.Divergence{Status: domain.DivergenceNotRaised, Odometer: 12_000, GPS: 10_400, Difference: 1_600, Thresholds: defaultLimit}},
		{"4 approximate is not evaluated", odo(186_400), gpsOK(120_000, domain.QualityApproximate),
			domain.Travelled{Status: domain.SourceOK, Basis: domain.BasisOdometer, Metres: 186_400},
			domain.Divergence{Status: domain.DivergenceNotEvaluated, Reason: domain.ReasonGPSNotPrecise, Thresholds: defaultLimit}},
		{"5 no final reading: por GPS", odoMissing(domain.ReasonNoFinalReading), gpsOK(171_900, domain.QualityPrecise),
			domain.Travelled{Status: domain.SourceOK, Basis: domain.BasisGPS, Metres: 171_900},
			domain.Divergence{Status: domain.DivergenceNotEvaluated, Reason: domain.ReasonNoOdometerDistance, Thresholds: defaultLimit}},
		{"6 nothing usable", odoMissing(domain.ReasonNoFinalReading), gpsOK(120_000, domain.QualityApproximate),
			domain.Travelled{Status: domain.SourceInsufficient, Reason: domain.ReasonNoFinalReading},
			domain.Divergence{Status: domain.DivergenceNotEvaluated, Reason: domain.ReasonNoOdometerDistance, Thresholds: defaultLimit}},
	}
	for _, c := range cases {
		for _, beside := range [][2]domain.DistanceSource{{noPlan, noEstimate}, {planned180, estimate181}} {
			got := evaluate(t, c.o, c.g, beside[0], beside[1], defaultLimit)
			if got.Travelled != c.travelled || got.Divergence != c.divergence {
				t.Fatalf("%s: got %+v %+v", c.name, got.Travelled, got.Divergence)
			}
		}
	}
}

// VAL-27 with the fixture limit of 5 %: 28 000 m of 290 000 m (9.66 %) raises
// the signal; 6 000 m (2.07 %) does not; both distances stay separate.
func TestDistanceVal27(t *testing.T) {
	five, err := domain.NewThresholds(2_000, 5)
	if err != nil {
		t.Fatal(err)
	}
	first := evaluate(t, odo(290_000), gpsOK(262_000, domain.QualityPrecise), noPlan, noEstimate, five)
	if first.Divergence != (domain.Divergence{Status: domain.DivergenceRaised, Odometer: 290_000, GPS: 262_000, Difference: 28_000, Thresholds: five}) ||
		first.Travelled != (domain.Travelled{Status: domain.SourceOK, Basis: domain.BasisOdometer, Metres: 290_000}) {
		t.Fatalf("VAL-27 first run %+v", first)
	}
	second := evaluate(t, odo(290_000), gpsOK(284_000, domain.QualityPrecise), noPlan, noEstimate, five)
	if second.Divergence.Status != domain.DivergenceNotRaised || second.Divergence.Difference != 6_000 || second.Travelled.Metres != 290_000 {
		t.Fatalf("VAL-27 second run %+v", second)
	}
	none := evaluate(t, odo(290_000), gpsMissing(domain.ReasonNoGPSDistance, domain.QualityInsufficient), noPlan, noEstimate, five)
	if none.Divergence.Status != domain.DivergenceNotEvaluated || none.Divergence.Reason != domain.ReasonNoGPSDistance || none.GPS.Metres != 0 || none.GPS.Status != domain.SourceInsufficient {
		t.Fatalf("a trip with no usable track shows INSUFFICIENT_DATA %+v", none)
	}
}

// Exact integer boundaries of both conditions, both directions of the
// difference and configured thresholds.
func TestDistanceDivergenceBoundaries(t *testing.T) {
	strict, _ := domain.NewThresholds(500, 5)
	cases := []struct {
		name   string
		o, g   int64
		th     domain.Thresholds
		status domain.DivergenceStatus
		diff   int64
	}{
		{"ten percent exactly is not raised", 100_000, 110_000, defaultLimit, domain.DivergenceNotRaised, 10_000},
		{"one metre above ten percent", 100_000, 110_001, defaultLimit, domain.DivergenceRaised, 10_001},
		{"two kilometres exactly is not raised", 10_000, 12_000, defaultLimit, domain.DivergenceNotRaised, 2_000},
		{"both thresholds just passed below", 10_000, 7_999, defaultLimit, domain.DivergenceRaised, 2_001},
		{"gps above odometer", 50_000, 60_000, defaultLimit, domain.DivergenceRaised, 10_000},
		{"equal distances", 50_000, 50_000, defaultLimit, domain.DivergenceNotRaised, 0},
		{"measured zero odometer is a value", 0, 2_001, defaultLimit, domain.DivergenceRaised, 2_001},
		{"measured zero both", 0, 0, defaultLimit, domain.DivergenceNotRaised, 0},
		{"strict thresholds", 10_000, 10_501, strict, domain.DivergenceRaised, 501},
		{"strict percent boundary", 20_000, 21_000, strict, domain.DivergenceNotRaised, 1_000},
		{"ceiling without overflow", domain.MaxDistanceMetres, 0, defaultLimit, domain.DivergenceRaised, domain.MaxDistanceMetres},
		{"ceiling gps", 0, domain.MaxDistanceMetres, domain.Thresholds{Metres: 10_000, Percent: 30}, domain.DivergenceRaised, domain.MaxDistanceMetres},
		{"ceiling both", domain.MaxDistanceMetres, domain.MaxDistanceMetres - 1, defaultLimit, domain.DivergenceNotRaised, 1},
	}
	for _, c := range cases {
		got := evaluate(t, odo(c.o), gpsOK(c.g, domain.QualityPrecise), noPlan, noEstimate, c.th)
		want := domain.Divergence{Status: c.status, Odometer: c.o, GPS: c.g, Difference: c.diff, Thresholds: c.th}
		if got.Divergence != want {
			t.Fatalf("%s: got %+v want %+v", c.name, got.Divergence, want)
		}
		if got.Travelled != (domain.Travelled{Status: domain.SourceOK, Basis: domain.BasisOdometer, Metres: c.o}) {
			t.Fatalf("%s: the odometer is the basis whatever the divergence %+v", c.name, got.Travelled)
		}
	}
}

// Named negative "mixed sources": planned and estimated distances are never
// travelled distance, never compared, and a source in another source's place
// is refused; the travelled value is exactly one source's value.
func TestDistanceMixedSourcesAreRefusedOrKeptApart(t *testing.T) {
	got := evaluate(t, odoMissing(domain.ReasonTripNotFinished), gpsMissing(domain.ReasonNoTrack, ""), planned180, estimate181, defaultLimit)
	if got.Travelled != (domain.Travelled{Status: domain.SourceInsufficient, Reason: domain.ReasonTripNotFinished}) {
		t.Fatalf("a planned or estimated distance became travelled distance %+v", got.Travelled)
	}
	if got.Divergence.Status != domain.DivergenceNotEvaluated || got.Divergence.Odometer != 0 || got.Divergence.GPS != 0 {
		t.Fatalf("a planned or estimated distance entered the divergence %+v", got.Divergence)
	}
	got = evaluate(t, odo(84_000), gpsOK(90_000, domain.QualityPrecise), planned180, estimate181, defaultLimit)
	if got.Travelled.Metres != 84_000 || got.Travelled.Basis != domain.BasisOdometer {
		t.Fatalf("a GPS estimate replaced or was averaged with the odometer %+v", got.Travelled)
	}
	got = evaluate(t, odoMissing(domain.ReasonMeterGap), gpsOK(90_000, domain.QualityPrecise), planned180, estimate181, defaultLimit)
	if got.Travelled != (domain.Travelled{Status: domain.SourceOK, Basis: domain.BasisGPS, Metres: 90_000}) {
		t.Fatalf("por GPS basis %+v", got.Travelled)
	}
	swapped := map[string][4]domain.DistanceSource{
		"gps as odometer":               {gpsOK(1, domain.QualityPrecise), gpsOK(1, domain.QualityPrecise), noPlan, noEstimate},
		"odometer as gps":               {odo(1), odo(1), noPlan, noEstimate},
		"planned as odometer":           {planned180, gpsWithheld, noPlan, noEstimate},
		"estimate as planned":           {odo(1), gpsWithheld, estimate181, noEstimate},
		"planned as estimate":           {odo(1), gpsWithheld, noPlan, planned180},
		"odometer with quality":         {{Kind: domain.KindOdometer, Status: domain.SourceOK, Metres: 1, Quality: domain.QualityPrecise}, gpsWithheld, noPlan, noEstimate},
		"planned with quality":          {odo(1), gpsWithheld, {Kind: domain.KindPlanned, Status: domain.SourceOK, Metres: 1, Quality: domain.QualityPrecise}, noEstimate},
		"odometer withheld":             {{Kind: domain.KindOdometer, Status: domain.SourceWithheld}, gpsWithheld, noPlan, noEstimate},
		"planned withheld":              {odo(1), gpsWithheld, {Kind: domain.KindPlanned, Status: domain.SourceWithheld}, noEstimate},
		"gps reason on odometer":        {odoMissing(domain.ReasonNoTrack), gpsWithheld, noPlan, noEstimate},
		"odometer reason on gps":        {odo(1), gpsMissing(domain.ReasonMeterGap, ""), noPlan, noEstimate},
		"estimate reason on planned":    {odo(1), gpsWithheld, {Kind: domain.KindPlanned, Status: domain.SourceInsufficient, Reason: domain.ReasonNoRouteCompute}, noEstimate},
		"divergence reason on odometer": {odoMissing(domain.ReasonNoOdometerDistance), gpsWithheld, noPlan, noEstimate},
	}
	for name, in := range swapped {
		if _, err := domain.EvaluateDistances(in[0], in[1], in[2], in[3], defaultLimit); !errors.Is(err, domain.ErrDistanceInput) {
			t.Fatal("mixed sources accepted:", name)
		}
	}
}

// Named negative "absent basis as zero": an unknown value carries a reason
// and no number, a number never hides behind an unknown status, and the
// travelled value is unreadable without its basis.
func TestDistanceAbsentBasisIsNeverZero(t *testing.T) {
	reasons := []domain.DistanceReason{domain.ReasonTripNotFinished, domain.ReasonTripCancelled, domain.ReasonNoMeter, domain.ReasonNoStartReading,
		domain.ReasonNoFinalReading, domain.ReasonNoReadingBefore, domain.ReasonMeterGap, domain.ReasonMeterUnavailable}
	gpsCases := []domain.DistanceSource{gpsWithheld, gpsMissing(domain.ReasonNoTrack, ""), gpsMissing(domain.ReasonNotProcessed, ""),
		gpsMissing(domain.ReasonNoGPSDistance, domain.QualityInsufficient), gpsMissing(domain.ReasonNoGPSDistance, domain.QualityApproximate),
		gpsMissing(domain.ReasonNoGPSDistance, domain.QualityPrecise), gpsOK(50_000, domain.QualityApproximate)}
	for _, r := range reasons {
		for _, g := range gpsCases {
			got := evaluate(t, odoMissing(r), g, noPlan, noEstimate, defaultLimit)
			if got.Travelled != (domain.Travelled{Status: domain.SourceInsufficient, Reason: r}) {
				t.Fatalf("%s/%+v: travelled %+v", r, g, got.Travelled)
			}
			if m, b, ok := got.Travelled.Value(); ok || m != 0 || b != "" {
				t.Fatal("an absent basis reads as a value", r, m, b)
			}
			if got.Divergence != (domain.Divergence{Status: domain.DivergenceNotEvaluated, Reason: domain.ReasonNoOdometerDistance, Thresholds: defaultLimit}) {
				t.Fatalf("%s: divergence %+v", r, got.Divergence)
			}
		}
	}
	if m, b, ok := evaluate(t, odo(0), gpsWithheld, noPlan, noEstimate, defaultLimit).Travelled.Value(); !ok || m != 0 || b != domain.BasisOdometer {
		t.Fatal("a measured zero is a value with its basis", m, b, ok)
	}
	if m, b, ok := evaluate(t, odoMissing(domain.ReasonNoMeter), gpsOK(7, domain.QualityPrecise), noPlan, noEstimate, defaultLimit).Travelled.Value(); !ok || m != 7 || b != domain.BasisGPS {
		t.Fatal("por GPS value", m, b, ok)
	}
	withheld := evaluate(t, odo(50_000), gpsWithheld, noPlan, noEstimate, defaultLimit)
	if withheld.Divergence != (domain.Divergence{Status: domain.DivergenceNotEvaluated, Reason: domain.ReasonGPSWithheld, Thresholds: defaultLimit}) {
		t.Fatalf("withheld GPS compared %+v", withheld.Divergence)
	}
	for name, s := range map[string]domain.DistanceSource{"no track": gpsMissing(domain.ReasonNoTrack, ""), "not processed": gpsMissing(domain.ReasonNotProcessed, ""),
		"no distance": gpsMissing(domain.ReasonNoGPSDistance, domain.QualityInsufficient)} {
		if got := evaluate(t, odo(50_000), s, noPlan, noEstimate, defaultLimit).Divergence; got.Reason != domain.ReasonNoGPSDistance || got.Status != domain.DivergenceNotEvaluated {
			t.Fatalf("%s: divergence %+v", name, got)
		}
	}
	refused := map[string]domain.DistanceSource{
		"unknown with a number":    {Kind: domain.KindOdometer, Status: domain.SourceInsufficient, Reason: domain.ReasonNoMeter, Metres: 1},
		"unknown without a reason": {Kind: domain.KindOdometer, Status: domain.SourceInsufficient},
		"unknown reason":           {Kind: domain.KindOdometer, Status: domain.SourceInsufficient, Reason: "SOMETHING"},
		"known with a reason":      {Kind: domain.KindOdometer, Status: domain.SourceOK, Reason: domain.ReasonNoMeter, Metres: 1},
		"negative":                 odo(-1),
		"above the ceiling":        odo(domain.MaxDistanceMetres + 1),
		"unknown status":           {Kind: domain.KindOdometer, Status: "ESTIMATED", Metres: 1},
		"no kind":                  {Status: domain.SourceOK, Metres: 1},
	}
	for name, o := range refused {
		if _, err := domain.EvaluateDistances(o, gpsWithheld, noPlan, noEstimate, defaultLimit); !errors.Is(err, domain.ErrDistanceInput) {
			t.Fatal("inconsistent odometer accepted:", name)
		}
	}
	gpsRefused := map[string]domain.DistanceSource{
		"withheld with a value":      {Kind: domain.KindGPS, Status: domain.SourceWithheld, Metres: 1},
		"withheld with a quality":    {Kind: domain.KindGPS, Status: domain.SourceWithheld, Quality: domain.QualityPrecise},
		"withheld with a reason":     {Kind: domain.KindGPS, Status: domain.SourceWithheld, Reason: domain.ReasonNoTrack},
		"insufficient quality ok":    gpsOK(1, domain.QualityInsufficient),
		"ok without quality":         gpsOK(1, ""),
		"unknown quality":            gpsOK(1, "GOOD"),
		"no track with a quality":    gpsMissing(domain.ReasonNoTrack, domain.QualityPrecise),
		"not processed with quality": gpsMissing(domain.ReasonNotProcessed, domain.QualityApproximate),
		"unknown with a number":      {Kind: domain.KindGPS, Status: domain.SourceInsufficient, Reason: domain.ReasonNoGPSDistance, Metres: 3},
		"negative":                   gpsOK(-5, domain.QualityPrecise),
		"above the ceiling":          gpsOK(domain.MaxDistanceMetres+1, domain.QualityPrecise),
	}
	for name, g := range gpsRefused {
		if _, err := domain.EvaluateDistances(odo(1), g, noPlan, noEstimate, defaultLimit); !errors.Is(err, domain.ErrDistanceInput) {
			t.Fatal("inconsistent GPS source accepted:", name)
		}
	}
}

// Named negative "divergence described as misconduct": the closed vocabulary
// and the signal's fields describe a difference and its evidence only.
func TestDistanceDivergenceStatesNoMisconduct(t *testing.T) {
	vocabulary := []string{string(domain.SourceOK), string(domain.SourceInsufficient), string(domain.SourceWithheld), string(domain.BasisOdometer), string(domain.BasisGPS),
		string(domain.DivergenceRaised), string(domain.DivergenceNotRaised), string(domain.DivergenceNotEvaluated), string(domain.KindOdometer), string(domain.KindGPS),
		string(domain.KindPlanned), string(domain.KindEstimate), domain.SourcesMethod,
		string(domain.ReasonTripNotFinished), string(domain.ReasonTripCancelled), string(domain.ReasonNoMeter), string(domain.ReasonNoStartReading),
		string(domain.ReasonNoFinalReading), string(domain.ReasonNoReadingBefore), string(domain.ReasonMeterGap), string(domain.ReasonMeterUnavailable),
		string(domain.ReasonNoTrack), string(domain.ReasonNotProcessed), string(domain.ReasonNoGPSDistance), string(domain.ReasonNoRoutePlan),
		string(domain.ReasonNoRouteCompute), string(domain.ReasonNoOdometerDistance), string(domain.ReasonGPSWithheld), string(domain.ReasonGPSNotPrecise)}
	for _, word := range vocabulary {
		upper := strings.ToUpper(word)
		for _, banned := range []string{"FRAUD", "THEFT", "MISCONDUCT", "SUSPICIOUS", "CHEAT", "LOSS", "GUILT", "VIOLATION", "DRIVER", "EMPLOYEE", "MEMBER"} {
			if strings.Contains(upper, banned) {
				t.Fatal("the vocabulary states more than a difference:", word)
			}
		}
	}
	divergence := reflect.TypeOf(domain.Divergence{})
	var fields []string
	for i := range divergence.NumField() {
		fields = append(fields, divergence.Field(i).Name)
	}
	if strings.Join(fields, ",") != "Status,Reason,Odometer,GPS,Difference,Thresholds" {
		t.Fatal("the divergence carries more than its evidence:", fields)
	}
	raised := evaluate(t, odo(186_400), gpsOK(158_300, domain.QualityPrecise), noPlan, noEstimate, defaultLimit).Divergence
	if raised.Status != domain.DivergenceRaised || raised.Odometer != 186_400 || raised.GPS != 158_300 || raised.Difference != 28_100 || raised.Thresholds != defaultLimit {
		t.Fatalf("the raised signal lacks its evidence %+v", raised)
	}
}
