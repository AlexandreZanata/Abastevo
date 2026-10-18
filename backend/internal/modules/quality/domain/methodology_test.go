package domain

import (
	"strings"
	"testing"
)

func assay(ensaio, result string, conformity Conformity) AssayObservation {
	return AssayObservation{Ensaio: ensaio, Result: result, Conformity: conformity}
}

// TestJudgeSampleFollowsOfficialPMQCMethodology freezes the sample judgment
// contract from the official 2026 guidance: the file unit is the assay, the
// judgment unit is the sample, Aspecto has fuel-specific complementary
// exceptions, and nothing is converted to a number.
func TestJudgeSampleFollowsOfficialPMQCMethodology(t *testing.T) {
	at := func(day string) SampleFacts {
		return SampleFacts{SampleKey: "pmqc-assay-v1:s-1", SourceCNPJ: "11222333000181", ObservedOn: day, Fuel: FuelGasolineRegular}
	}
	cases := []struct {
		name   string
		facts  SampleFacts
		assays []AssayObservation
		want   SampleVerdict
		reason string
	}{
		{
			"gasoline all conforming", at("2026-09-04"),
			[]AssayObservation{assay("Aspecto", "Límpido e isento de impurezas (LII)", Conforming), assay("Teor de Etanol", "27", Conforming)},
			SampleConforming, "",
		},
		{
			"gasoline aspecto failure has no exception", at("2026-09-04"),
			[]AssayObservation{assay("Aspecto", "Límpido com material particulado", Nonconforming), assay("Teor de Etanol", "27", Conforming)},
			SampleNonconforming, "",
		},
		{
			"gasoline blank result still fails on conformity", at("2026-09-04"),
			[]AssayObservation{assay("Massa Específica a 20°C", "", Nonconforming)},
			SampleNonconforming, "",
		},
		{
			"ethanol aspecto with nonconforming non-volatile material", SampleFacts{SampleKey: "pmqc-assay-v1:s-2", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-04", Fuel: FuelEthanol},
			[]AssayObservation{assay("Aspecto", "Límpido com material particulado", Nonconforming), assay("Material Não Volátil", "7", Nonconforming)},
			SampleNonconforming, "",
		},
		{
			"ethanol aspecto cleared by conforming non-volatile material", SampleFacts{SampleKey: "pmqc-assay-v1:s-3", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-04", Fuel: FuelEthanol},
			[]AssayObservation{assay("Aspecto", "Límpido com material particulado", Nonconforming), assay("Material Não Volátil", "3", Conforming)},
			SampleConforming, ReasonEthanolAspectoCleared,
		},
		{
			"ethanol aspecto cleared by conforming evaporation residue", SampleFacts{SampleKey: "pmqc-assay-v1:s-4", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-04", Fuel: FuelEthanol},
			[]AssayObservation{assay("Aspecto", "Turvo", Nonconforming), assay("Resíduo por Evaporação", "4", Conforming)},
			SampleConforming, ReasonEthanolAspectoCleared,
		},
		{
			"ethanol aspecto without complementaries stays conforming", SampleFacts{SampleKey: "pmqc-assay-v1:s-5", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-04", Fuel: FuelEthanol},
			[]AssayObservation{assay("Aspecto", "Turvo", Nonconforming), assay("Potencial Hidrogeniônico (pH)", "6.5", Conforming)},
			SampleConforming, ReasonAspectoUnconfirmed,
		},
		{
			"ethanol failure outside aspecto", SampleFacts{SampleKey: "pmqc-assay-v1:s-6", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-04", Fuel: FuelEthanol},
			[]AssayObservation{assay("Aspecto", "Límpido e isento de impurezas (LII)", Conforming), assay("Potencial Hidrogeniônico (pH)", "5.4", Nonconforming)},
			SampleNonconforming, "",
		},
		{
			"diesel particles confirmed by total contamination", SampleFacts{SampleKey: "pmqc-assay-v1:s-7", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-04", Fuel: FuelDieselS10},
			[]AssayObservation{assay("Aspecto", "Límpido com material particulado", Nonconforming), assay("Contaminação total", "10", Nonconforming)},
			SampleNonconforming, "",
		},
		{
			"diesel particles cleared by total contamination", SampleFacts{SampleKey: "pmqc-assay-v1:s-8", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-04", Fuel: FuelDieselS10},
			[]AssayObservation{assay("Aspecto", "Límpido com material particulado", Nonconforming), assay("Contaminação total", "10", Conforming)},
			SampleConforming, ReasonDieselAspectoCleared,
		},
		{
			"diesel particles without complement stays conforming", SampleFacts{SampleKey: "pmqc-assay-v1:s-9", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-04", Fuel: FuelDieselS10},
			[]AssayObservation{assay("Aspecto", "Límpido com material particulado", Nonconforming)},
			SampleConforming, ReasonAspectoUnconfirmed,
		},
		{
			"diesel turbidity confirmed by water", SampleFacts{SampleKey: "pmqc-assay-v1:s-10", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-04", Fuel: FuelDieselS10},
			[]AssayObservation{assay("Aspecto", "Turvo", Nonconforming), assay("Teor de Água", "464", Nonconforming)},
			SampleNonconforming, "",
		},
		{
			"diesel turbidity cleared when required complement conforms", SampleFacts{SampleKey: "pmqc-assay-v1:s-11", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-04", Fuel: FuelDieselS10},
			[]AssayObservation{assay("Aspecto", "Turvo", Nonconforming), assay("Teor de Água", "100", Conforming), assay("Contaminação total", "5", Conforming)},
			SampleConforming, ReasonDieselAspectoCleared,
		},
		{
			"diesel heterogeneous fails directly", SampleFacts{SampleKey: "pmqc-assay-v1:s-12", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-04", Fuel: FuelDieselS500},
			[]AssayObservation{assay("Aspecto", "Heterogêneo", Nonconforming)},
			SampleNonconforming, "",
		},
		{
			"diesel unknown defect is unresolved", SampleFacts{SampleKey: "pmqc-assay-v1:s-13", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-04", Fuel: FuelDieselS10},
			[]AssayObservation{assay("Aspecto", "Escuro e turvo com sedimento", Nonconforming)},
			SampleUnresolved, ReasonDefectUnclassified,
		},
		{
			"diesel rule disagreement is unresolved", SampleFacts{SampleKey: "pmqc-assay-v1:s-14", SourceCNPJ: "11222333000181", ObservedOn: "2025-01-15", Fuel: FuelDieselS500},
			[]AssayObservation{assay("Aspecto", "Heterogêneo", Nonconforming)},
			SampleUnresolved, ReasonDieselRuleEpoch,
		},
		{
			"diesel old-rule water and sediments agreeing conforms", SampleFacts{SampleKey: "pmqc-assay-v1:s-15", SourceCNPJ: "11222333000181", ObservedOn: "2019-05-02", Fuel: FuelDieselS500},
			[]AssayObservation{assay("Aspecto", "Turvo", Nonconforming), assay("Teor de Água e Sedimentos", "0.06", Conforming)},
			SampleConforming, ReasonDieselAspectoCleared,
		},
		{
			"replicas collapse into one distinct assay", at("2026-09-04"),
			[]AssayObservation{
				assay("Teor de Etanol", "36", Nonconforming), assay("Teor de Etanol", "36", Nonconforming),
				assay("Aspecto", "Límpido e isento de impurezas (LII)", Conforming),
			},
			SampleNonconforming, "",
		},
		{
			"conflicting replicas are unresolved", at("2026-09-04"),
			[]AssayObservation{assay("Teor de Etanol", "27", Conforming), assay("Teor de Etanol", "36", Nonconforming)},
			SampleUnresolved, ReasonReplicaConflict,
		},
		{
			"unknown non-aspecto status is unresolved", at("2026-09-04"),
			[]AssayObservation{assay("Teor de Etanol", "", UnknownStatus)},
			SampleUnresolved, ReasonUnknownAssay,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := JudgeSample(tc.facts, tc.assays)
			if got.Verdict != tc.want {
				t.Fatalf("verdict = %s (%v), want %s", got.Verdict, got.Reasons, tc.want)
			}
			if tc.reason != "" && !contains(got.Reasons, tc.reason) {
				t.Fatalf("reasons = %v, want %s", got.Reasons, tc.reason)
			}
			if len(got.NonconformingAssays) > 1 {
				for i := 1; i < len(got.NonconformingAssays); i++ {
					if got.NonconformingAssays[i] < got.NonconformingAssays[i-1] {
						t.Fatal("nonconforming assays not canonical", got.NonconformingAssays)
					}
				}
			}
		})
	}
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// TestFuelDimensionsStayUnscoredWithoutCalibration freezes the nullable
// score state: dimensions are counts and days, never an invented number.
func TestFuelDimensionsStayUnscoredWithoutCalibration(t *testing.T) {
	samples := []SampleFacts{
		{SampleKey: "k1", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-01", Fuel: FuelGasolineRegular, StationID: "station-1", Judgment: SampleJudgment{Verdict: SampleConforming}},
		{SampleKey: "k2", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-01", Fuel: FuelGasolineRegular, StationID: "station-1", Judgment: SampleJudgment{Verdict: SampleNonconforming}},
		{SampleKey: "k3", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-10", Fuel: FuelGasolineRegular, StationID: "station-2", Judgment: SampleJudgment{Verdict: SampleUnresolved}},
		{SampleKey: "k4", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-11", Fuel: FuelEthanol, StationID: "station-1", Judgment: SampleJudgment{Verdict: SampleConforming}},
	}
	all := ProjectFuelDimensions(samples)
	if len(all) != 2 {
		t.Fatal("fuels not separated", all)
	}
	gas := all[FuelGasolineRegular]
	if gas.Samples != 3 || gas.Visits != 2 || gas.Conforming != 1 || gas.Nonconforming != 1 || gas.Unresolved != 1 || gas.NewestObservedOn != "2026-09-10" || gas.OldestObservedOn != "2026-09-01" {
		t.Fatal("gasoline dimensions wrong", gas)
	}
	if gas.Score != ScoreUnscored || gas.MethodVersion != MethodologyVersion {
		t.Fatal("score invented without calibration", gas)
	}
	eth := all[FuelEthanol]
	if eth.Samples != 1 || eth.Nonconforming != 0 {
		t.Fatal("ethanol dimensions wrong", eth)
	}
	if !strings.Contains(MethodologyLimitation, "no numeric") {
		t.Fatal("methodology limitation not documented", MethodologyLimitation)
	}
}
