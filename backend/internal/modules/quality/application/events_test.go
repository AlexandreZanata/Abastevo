package application

import (
	"strings"
	"testing"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

func validEventsInput() EventsInput {
	amount := int64(150000)
	return EventsInput{
		Source: "enforcement", Stream: "enforcement-v1", Edition: "2026-09",
		Resources:    []Resource{{URL: "https://example.invalid/e.zip", SHA256: strings.Repeat("a", 64), Bytes: 7}},
		Counts:       domain.Counts{Input: 2, Accepted: 2},
		ManifestHash: strings.Repeat("b", 64),
		Periods: []PeriodInput{
			{Label: "2026-08", Start: "2026-08-01", End: "2026-08-31"},
			{Label: "2026-09", Start: "2026-09-01", End: "2026-09-30"},
		},
		Enforcement: []EnforcementInput{
			{NativeKey: "AUTO-1", Scope: "municipal", Status: "autuado", SubjectCNPJ: "11222333000181", OccurredOn: "2026-08-02", AsOf: "2026-09-30", EvidenceRef: "e#AUTO-1"},
		},
		Sanctions: []SanctionInput{
			{NativeKey: "MULTA-1", ProceedingRef: "48000.001234/2026-11", PartyCNPJ: "04218406000104", AmountMinor: &amount, Status: "aplicada", DecidedOn: "2026-07-15", AsOf: "2026-09-30", EvidenceRef: "e#MULTA-1"},
		},
	}
}

func TestValidateEventsAcceptsFrozenContract(t *testing.T) {
	if err := validEventsInput().Validate(); err != nil {
		t.Fatalf("valid events input refused: %v", err)
	}
}

func TestValidateEventsRefusals(t *testing.T) {
	source := validEventsInput()
	source.Source = "prices"
	if err := source.Validate(); err == nil {
		t.Error("unknown event source accepted")
	}
	empty := validEventsInput()
	empty.Resources = nil
	if err := empty.Validate(); err == nil {
		t.Error("resourceless events publication accepted")
	}
	bad := validEventsInput()
	bad.Counts.Input = 9
	if err := bad.Validate(); err == nil {
		t.Error("mismatched counts accepted")
	}
	period := validEventsInput()
	period.Periods[0].End = "2026-07-01"
	if err := period.Validate(); err == nil {
		t.Error("inverted period accepted")
	}
	identity := validEventsInput()
	identity.Sanctions[0].NativeKey = " "
	if err := identity.Validate(); err == nil {
		t.Error("missing sanction identity accepted")
	}
}
