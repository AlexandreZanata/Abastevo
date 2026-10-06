package registry

import (
	"strings"
	"testing"
)

// P25-T01 executable source policy: header-name detection (never
// positions), kernel CNPJ text rules (letters/leading zeroes preserved),
// frozen state mapping and sufficient-evidence eligibility. Unknown
// headers or missing required names quarantine the run; a single bad
// row never poisons the batch.

func TestValidateHeaderRequiresNamesAndFlagsUnknown(t *testing.T) {
	names := []string{"CNPJ", "RAZAO_SOCIAL", "COD_IBGE", "UF", "SITUACAO", "COLUNA_DESCONHECIDA"}
	missing, unknown := ValidateHeader(names)
	if len(missing) != 0 {
		t.Fatalf("unexpected missing: %v", missing)
	}
	if len(unknown) != 1 || unknown[0] != "COLUNA_DESCONHECIDA" {
		t.Fatalf("unknown not flagged: %v", unknown)
	}
	if !QuarantineRun(missing, unknown) {
		t.Fatal("run with unknown columns must quarantine")
	}
}

func TestValidateHeaderMissingRequiredQuarantines(t *testing.T) {
	missing, _ := ValidateHeader([]string{"CNPJ", "UF"})
	if len(missing) == 0 {
		t.Fatal("missing required names must be reported")
	}
	if !QuarantineRun(missing, nil) {
		t.Fatal("run with missing names must quarantine")
	}
}

func TestValidCNPJTextPreservesLettersAndZeros(t *testing.T) {
	for _, raw := range []string{"04218406000104", "12ABC34501DE35", "00428184000195"} {
		cnpj, err := ValidCNPJText(raw)
		if err != nil {
			t.Fatalf("valid CNPJ rejected %q: %v", raw, err)
		}
		if cnpj != raw {
			t.Fatalf("CNPJ not preserved verbatim: %q != %q", cnpj, raw)
		}
	}
	for _, raw := range []string{"", "123", "0421840600010!", "042184060001040"} {
		if _, err := ValidCNPJText(raw); err == nil {
			t.Fatalf("invalid CNPJ accepted %q", raw)
		}
	}
}

func TestMapStatusFreezesWireValues(t *testing.T) {
	cases := map[string]Authorization{
		"ATIVA":        AuthorizationAuthorized,
		"SUSPENSA":     AuthorizationSuspended,
		"CANCELADA":    AuthorizationRevoked,
		"DESCONHECIDA": AuthorizationUnknown,
		"":             AuthorizationUnknown,
	}
	for raw, want := range cases {
		if got := MapStatus(raw); got != want {
			t.Fatalf("status %q mapped to %q, want %q", raw, got, want)
		}
	}
}

func TestEligibilityRequiresSufficientEvidence(t *testing.T) {
	eligible := Record{IdentityOK: true, AddressOK: true, Authorized: true, Withdrawn: false}
	if DecideEligibility(eligible) != EligibilityEligible {
		t.Fatal("sufficient record must be eligible")
	}
	denied := []Record{
		{IdentityOK: false, AddressOK: true, Authorized: true},
		{IdentityOK: true, AddressOK: false, Authorized: true},
		{IdentityOK: true, AddressOK: true, Authorized: false},
		{IdentityOK: true, AddressOK: true, Authorized: true, Withdrawn: true},
	}
	for i, rec := range denied {
		if got := DecideEligibility(rec); got == EligibilityEligible {
			t.Fatalf("case %d must not be eligible", i)
		}
	}
}

func TestFixtureManifestHasOwnedMarkersAndNoPII(t *testing.T) {
	entries := FixtureManifest(t)
	if len(entries) == 0 {
		t.Fatal("manifest lists no fixtures")
	}
	for _, path := range entries {
		body := ReadFixture(t, path)
		if !strings.Contains(body, "[P25-TEST]") {
			t.Fatalf("fixture %s lacks owned markers", path)
		}
		for _, pii := range []string{"@example.com", "@teste.com", "11999999999", "(11)"} {
			if strings.Contains(body, pii) {
				t.Fatalf("fixture %s contains PII pattern %q", path, pii)
			}
		}
	}
}
