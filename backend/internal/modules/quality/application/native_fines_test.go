package application

import (
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

func validNativeFineInput() NativeFineInput {
	in := NativeFineInput{Edition: "2025", Clock: NativeFineClock{ReportYear: 2025, ExtractionMonth: "2026-01"}, Resources: []Resource{{URL: "https://www.gov.br/anp/fines-2025.csv", SHA256: strings.Repeat("a", 64), Bytes: 100}}, Counts: domain.Counts{Input: 1, Accepted: 1}, ManifestHash: strings.Repeat("b", 64), Observations: []FineObservation{{ObservationKey: "observation:" + strings.Repeat("c", 64), SourceCNPJ: "11222333000181", ProceedingRef: "000000000000001", DUFRef: "SYNTHETIC/1", Department: "SYNTHETIC", Status: "UNKNOWN source status", DueOn: "2025-02-28", AmountText: "150000,00", AmountMinor: 15000000, PaidText: "180000,01", PaidMinor: 18000001, ReportYear: 2025, ExtractionMonth: "2026-01", EvidenceRef: "synthetic.csv:8"}}}
	in.Observations[0].ObservationKey = in.Observations[0].ObservationIdentity()
	return in
}
func TestNativeFinePublicationInputPreservesClockAndStrictCounts(t *testing.T) {
	if e := validNativeFineInput().Validate(); e != nil {
		t.Fatal(e)
	}
	for name, change := range map[string]func(*NativeFineInput){
		"wrong edition":          func(a *NativeFineInput) { a.Edition = "2026" },
		"bad clock":              func(a *NativeFineInput) { a.Clock.ExtractionMonth = "2026-01-01" },
		"different record month": func(a *NativeFineInput) { a.Observations[0].ExtractionMonth = "2026-02" },
		"different report year":  func(a *NativeFineInput) { a.Observations[0].ReportYear = 2024 },
		"CPF":                    func(a *NativeFineInput) { a.Observations[0].SourceCNPJ = "12345678900" },
		"invented identity":      func(a *NativeFineInput) { a.Observations[0].ObservationKey = "MULTA-UNKNOWN" },
		"invalid calendar":       func(a *NativeFineInput) { a.Observations[0].DueOn = "2025-02-30" },
		"money mismatch":         func(a *NativeFineInput) { a.Observations[0].AmountMinor++ },
		"paid mismatch":          func(a *NativeFineInput) { a.Observations[0].PaidMinor++ },
		"duplicate observation": func(a *NativeFineInput) {
			a.Observations = append(a.Observations, a.Observations[0])
			a.Counts.Input++
			a.Counts.Accepted++
		},
		"balanced negative counts": func(a *NativeFineInput) { a.Counts.Duplicates = -1; a.Counts.Quarantined = 1 },
		"accepted count":           func(a *NativeFineInput) { a.Counts.Input++; a.Counts.Accepted++ },
		"missing evidence":         func(a *NativeFineInput) { a.Observations[0].EvidenceRef = "" },
		"control":                  func(a *NativeFineInput) { a.Observations[0].Status = "bad\nstatus" },
	} {
		t.Run(name, func(t *testing.T) {
			a := validNativeFineInput()
			change(&a)
			if a.Validate() == nil {
				t.Fatal("invalid native publication accepted")
			}
		})
	}
	empty := validNativeFineInput()
	empty.Counts = domain.Counts{}
	empty.Observations = nil
	if e := empty.Validate(); e != nil {
		t.Fatal("verified empty native revision refused", e)
	}
}
