package application

import (
	"strings"
	"testing"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

func validInput() Input {
	return Input{
		Source: "pmqc", Stream: "pmqc-assay-v1", Edition: "2026-09",
		Resources:    []Resource{{URL: "https://example.invalid/p.zip", SHA256: strings.Repeat("a", 64), Bytes: 1}},
		Counts:       domain.Counts{Input: 2, Accepted: 2},
		ManifestHash: strings.Repeat("b", 64),
		Assays: []AssayInput{
			{SampleRef: "S-1", AssayRef: "A", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-04", Fuel: "ETHANOL", ResultText: "92,6", Unit: "%", Conformity: "CONFORMING", EvidenceRef: "f#S-1#A", Fingerprint: strings.Repeat("c", 64)},
			{SampleRef: "S-1", AssayRef: "B", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-04", Fuel: "ETHANOL", ResultText: "ok", Unit: "-", Conformity: "CONFORMING", EvidenceRef: "f#S-1#B", Fingerprint: strings.Repeat("d", 64)},
		},
		Links: []LinkInput{{RecordKey: "pmqc-assay-v1:S-1", Reason: "exact cnpj", Active: true}},
	}
}

func TestValidateAcceptsFrozenContract(t *testing.T) {
	if err := validInput().Validate(); err != nil {
		t.Fatalf("valid input refused: %v", err)
	}
}

func TestValidateRefusals(t *testing.T) {
	empty := validInput()
	empty.Resources = nil
	if err := empty.Validate(); err == nil {
		t.Error("resourceless publication accepted")
	}
	bad := validInput()
	bad.Counts.Input = 9
	if err := bad.Validate(); err == nil {
		t.Error("mismatched counts accepted")
	}
	hash := validInput()
	hash.ManifestHash = "xyz"
	if err := hash.Validate(); err == nil {
		t.Error("malformed manifest hash accepted")
	}
	identity := validInput()
	identity.Assays[0].AssayRef = "  "
	if err := identity.Validate(); err == nil {
		t.Error("missing assay identity accepted")
	}
	print := validInput()
	print.Assays[1].Fingerprint = strings.Repeat("d", 63) + "x"
	if err := print.Validate(); err == nil {
		t.Error("malformed fingerprint accepted")
	}
}
