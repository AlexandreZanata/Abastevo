package application

import (
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"strings"
	"testing"
)

func TestHistoricalFineUnknownSemanticsAndContentIdentity(t *testing.T) {
	row := HistoricalFineObservation{Schema: "anp-fine-history-observation-v1", Source: "sanctions", SourceCNPJ: "11222333000181", ProceedingRef: "000000000000001", ReferenceLabel: "Número do DUF", ReferenceRef: "SYNTHETIC/1", Status: "Unknown source label", DueOn: "2023-02-28", AmountText: " R$ 1.234,56 ", AmountMinor: 123456, PaidText: " R$ -   ", SourcePeriod: "2023", Clock: FineHistoryClock{Precision: "MONTH", Value: "2024-06"}, InputContract: "anp-sicom-cp850-2023-v1", EvidenceRef: "multas-aplicadas-2023.csv:7"}
	row.ObservationKey = row.ObservationIdentity()
	if !row.Valid() {
		t.Fatal("unknown values lost")
	}
	zero := int64(0)
	row.PaidMinor = &zero
	if row.Valid() {
		t.Fatal("unknown paid coerced to zero")
	}
	row.PaidText = " R$ 0,00 "
	row.ObservationKey = row.ObservationIdentity()
	if !row.Valid() {
		t.Fatal("actual zero refused")
	}
	if row.ObservationKey == "" {
		t.Fatal("missing identity")
	}
}

func historySample(period string) HistoricalFineObservation {
	p, _ := HistoricalFineProfile(period)
	paid := int64(0)
	clock := FineHistoryClock{Precision: p.ClockPrecision, Value: "2024-06"}
	if p.ClockPrecision == "DAY" {
		clock.Value = "2020-02-06"
	}
	r := HistoricalFineObservation{Schema: "anp-fine-history-observation-v1", Source: "sanctions", SourceCNPJ: "11222333000181", ProceedingRef: "000000000000001", ReferenceLabel: p.ReferenceLabel, ReferenceRef: "SYNTHETIC/1", Status: "Unknown source label", DueOn: "2020-02-28", AmountText: "1,00", AmountMinor: 100, PaidText: "0,00", PaidMinor: &paid, SourcePeriod: period, Clock: clock, InputContract: p.Contract, EvidenceRef: p.SourceFile + ":8"}
	r.ObservationKey = r.ObservationIdentity()
	return r
}
func TestHistoricalFineSourceSemanticsRefuseInventedMeaning(t *testing.T) {
	for _, period := range []string{"2016-2019", "2020", "2021", "2022", "2023", "2024"} {
		if !historySample(period).Valid() {
			t.Fatal("actual source shape refused", period)
		}
	}
	for name, change := range map[string]func(*HistoricalFineObservation){
		"CPF":                     func(r *HistoricalFineObservation) { r.SourceCNPJ = "00000000000" },
		"wrong AI label":          func(r *HistoricalFineObservation) { r.ReferenceLabel = "Número do DUF" },
		"invented month from day": func(r *HistoricalFineObservation) { r.Clock = FineHistoryClock{Precision: "MONTH", Value: "2020-02"} },
		"wrong period":            func(r *HistoricalFineObservation) { r.SourcePeriod = "2016" },
		"bad extraction day":      func(r *HistoricalFineObservation) { r.Clock.Value = "2020-02-30" },
		"bad transit day":         func(r *HistoricalFineObservation) { v := "2020-02-30"; r.SourceTransitOn = &v },
		"invented unknown paid":   func(r *HistoricalFineObservation) { r.PaidText = "R$ -"; r.PaidMinor = nil },
		"wrong value":             func(r *HistoricalFineObservation) { r.AmountMinor++ },
		"overflow":                func(r *HistoricalFineObservation) { r.AmountText = "92233720368547758,08" },
		"extra unlabelled":        func(r *HistoricalFineObservation) { r.UnlabelledField = "SIM" },
		"control":                 func(r *HistoricalFineObservation) { r.Status = "private\nstatus" },
		"line separator":          func(r *HistoricalFineObservation) { r.Status = "status\u2028tail" },
		"unknown schema":          func(r *HistoricalFineObservation) { r.Schema = "future" },
	} {
		t.Run(name, func(t *testing.T) {
			r := historySample("2016-2019")
			change(&r)
			r.ObservationKey = r.ObservationIdentity()
			if r.Valid() {
				t.Fatal("invalid semantics accepted")
			}
		})
	}
	r := historySample("2024")
	day := "2020-02-28"
	r.SourceTransitOn = &day
	r.ObservationKey = r.ObservationIdentity()
	if r.Valid() {
		t.Fatal("source without transit gained a transit fact")
	}
	for _, period := range []string{"2020", "2021"} {
		r := historySample(period)
		if !r.Clock.Valid(period) {
			t.Fatal("month clock refused")
		}
		r.Clock.Precision = "DAY"
		r.Clock.Value = "2024-06-01"
		if r.Clock.Valid(period) {
			t.Fatal("month gained an invented exact day")
		}
	}
}

func validHistoryInput() HistoricalFineInput {
	row := historySample("2020")
	profile, _ := HistoricalFineProfile("2020")
	return HistoricalFineInput{Context: FineHistoryContext{InputContract: profile.Contract, SourcePeriod: profile.Period, SourceFile: profile.SourceFile, HeaderRecord: profile.HeaderRecord, Clock: row.Clock, SourceRecordCount: 1}, Resources: []Resource{{URL: "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/mav/" + profile.SourceFile, SHA256: strings.Repeat("a", 64), Bytes: 100}}, Counts: domain.Counts{Input: 1, Accepted: 1}, ManifestHash: strings.Repeat("b", 64), Observations: []HistoricalFineObservation{row}}
}
func TestHistoricalFinePublicationInputValidatesBeforeIO(t *testing.T) {
	if e := validHistoryInput().Validate(); e != nil {
		t.Fatal("verified historical input refused", e)
	}
	for name, change := range map[string]func(*HistoricalFineInput){
		"wrong source URL":        func(in *HistoricalFineInput) { in.Resources[0].URL = "https://evil.example/source.csv" },
		"context clock":           func(in *HistoricalFineInput) { in.Context.Clock.Precision = "DAY" },
		"wrong count":             func(in *HistoricalFineInput) { in.Counts.Accepted++ },
		"negative balanced count": func(in *HistoricalFineInput) { in.Counts.Duplicates = -1; in.Counts.Quarantined = 1 },
		"wrong original records":  func(in *HistoricalFineInput) { in.Context.SourceRecordCount++ },
		"unannotated metadata":    func(in *HistoricalFineInput) { in.Context.MetadataRecords = 1 },
		"header locator":          func(in *HistoricalFineInput) { in.Observations[0].EvidenceRef = "multas-aplicadas-2020.csv:7" },
		"other profile row":       func(in *HistoricalFineInput) { in.Observations[0].SourcePeriod = "2021" },
		"missing field":           func(in *HistoricalFineInput) { in.Observations[0].ReferenceLabel = "" },
		"wrong manifest hash":     func(in *HistoricalFineInput) { in.ManifestHash = "bad" },
	} {
		t.Run(name, func(t *testing.T) {
			in := validHistoryInput()
			change(&in)
			if in.Validate() == nil {
				t.Fatal("invalid source publication input accepted")
			}
		})
	}
	empty := validHistoryInput()
	empty.Context.SourceRecordCount = 0
	empty.Counts = domain.Counts{}
	empty.Observations = nil
	if e := empty.Validate(); e != nil {
		t.Fatal("empty source lost its clock", e)
	}
}
