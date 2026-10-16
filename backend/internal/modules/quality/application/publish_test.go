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

func TestPublicationMemberIdentityAndPeriods(t *testing.T) {
	good := Resource{Member: "2025/PMQC_2025_01.csv", MemberSHA256: strings.Repeat("a", 64), MemberBytes: 3}
	if e := good.ValidateMember(); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"../escape.csv", "/root.csv", "2025/../file.csv", "a\\b.csv", "a\x00b.csv", "."} {
		bad := good
		bad.Member = name
		if e := bad.ValidateMember(); e == nil {
			t.Errorf("unsafe member accepted: %q", name)
		}
	}
	for _, change := range []func(*Resource){func(r *Resource) { r.Member = "" }, func(r *Resource) { r.MemberSHA256 = strings.Repeat("A", 64) }, func(r *Resource) { r.MemberBytes = 0 }, func(r *Resource) { r.MemberBytes = (100 << 20) + 1 }} {
		bad := good
		change(&bad)
		if e := bad.ValidateMember(); e == nil {
			t.Fatal("bad member identity accepted")
		}
	}
	in := validInput()
	in.Periods = []PeriodInput{{Label: "2026-02", Start: "2026-02-01", End: "2026-02-30"}}
	if e := in.Validate(); e == nil {
		t.Fatal("noncalendar PMQC period accepted")
	}
	in.Periods = []PeriodInput{{Label: "a", Start: "2026-02-01", End: "2026-02-28"}, {Label: "a", Start: "2026-02-01", End: "2026-02-28"}}
	if e := in.Validate(); e == nil {
		t.Fatal("duplicate period accepted")
	}
}

func TestPublicationRejectsMismatchedSourceClock(t *testing.T) {
	for _, clock := range []string{"2026-09-05 12:00:00", "2026-09-04", "2026-09-04 24:00:00"} {
		in := validInput()
		in.Assays[0].SourceObservedAt = clock
		if e := in.Validate(); e == nil {
			t.Fatal("bad source clock accepted", clock)
		}
	}
	in := validInput()
	in.Assays[0].SourceObservedAt = "2026-09-04 12:00:00"
	if e := in.Validate(); e != nil {
		t.Fatal(e)
	}
}
