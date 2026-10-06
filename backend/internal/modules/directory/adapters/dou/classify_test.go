package dou

import (
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

// P26-T02 deterministic act classification: keyword rules over the act
// text, never free-text/LLM interpretation. Unknown wording quarantines
// (ambiguous). Grant dates never become inauguration dates — that
// mapping is refused structurally (no opening-date output exists).

func TestClassifyGrantsCorrectionsRevocations(t *testing.T) {
	grant := Act{ID: "A1", Text: "A ANP autoriza [P26-TEST] POSTO ALFA LTDA, CNPJ 04218406000104, a exercer a atividade de revenda varejista."}
	if got := Classify(grant); got != ClassGrant {
		t.Fatalf("grant = %q", got)
	}
	correction := Act{ID: "A2", Text: "Retifica o ato ANP-2026-0001: onde se lê 04218406000104, leia-se 00428184000195."}
	if got := Classify(correction); got != ClassCorrection {
		t.Fatalf("correction = %q", got)
	}
	revocation := Act{ID: "A3", Text: "Fica revogada a autorização de [P26-TEST] POSTO EPSILON LTDA, CNPJ 55881177000136."}
	if got := Classify(revocation); got != ClassRevocation {
		t.Fatalf("revocation = %q", got)
	}
}

func TestClassifyUnrelatedAndAmbiguousQuarantine(t *testing.T) {
	unrelated := Act{ID: "A4", Text: "Concede licença de operação para terminal portuário de granéis líquidos."}
	if got := Classify(unrelated); got != ClassUnrelated {
		t.Fatalf("unrelated = %q", got)
	}
	ambiguous := Act{ID: "A5", Text: "O posto mencionado anteriormente deverá observar o disposto no artigo 3º."}
	if got := Classify(ambiguous); got != ClassAmbiguous {
		t.Fatalf("ambiguous = %q", got)
	}
}

func TestExtractCNPJsValidatesAndDropsInvalid(t *testing.T) {
	got := ExtractCNPJs("CNPJ 04218406000104 e 12ABC34501DE35, inválido 123 e 99999999999999")
	if len(got) != 2 || got[0] != "04218406000104" || got[1] != "12ABC34501DE35" {
		t.Fatalf("cnpjs = %v", got)
	}
	if len(ExtractCNPJs("sem identificadores aqui")) != 0 {
		t.Fatal("spurious identifiers extracted")
	}
}

func TestExtractCNPJsAcceptsFormatted(t *testing.T) {
	got := ExtractCNPJs("CNPJ 04.218.406/0001-04 regularizado")
	if len(got) != 1 || got[0] != "04218406000104" {
		t.Fatalf("cnpjs = %v", got)
	}
	_ = kernel.ErrInvalidCNPJ
}

func TestStageActBuildsAssertionWithoutOpeningDate(t *testing.T) {
	edition := Edition{Date: "2026-10-01", Number: "190", Checksum: "ed-sha"}
	act := Act{ID: "ANP-2026-0001", Kind: "autorizacao", Title: "Autoriza revenda", Text: "autoriza [P26-TEST] POSTO ALFA LTDA, CNPJ 04218406000104"}
	assertions, skipped := StageActs(edition, []Act{act})
	if skipped != 0 || len(assertions) != 1 {
		t.Fatalf("assertions = %d, skipped = %d", len(assertions), skipped)
	}
	a := assertions[0]
	if a.SourceKey != "04218406000104" || a.AuthState != "authorized" {
		t.Fatalf("assertion = %+v", a)
	}
	if !a.EffectiveDate.Valid || a.EffectiveDate.Time.Format("2006-01-02") != "2026-10-01" {
		t.Fatalf("effective date not the edition date: %+v", a.EffectiveDate)
	}
}

func TestStageActQuarantinesAmbiguousAndMultiCNPJ(t *testing.T) {
	edition := Edition{Date: "2026-10-01", Checksum: "ed-sha"}
	ambiguous := Act{ID: "A5", Text: "O posto mencionado anteriormente deverá observar o disposto."}
	multi := Act{ID: "A6", Text: "autoriza ALFA CNPJ 04218406000104 e BETA CNPJ 00428184000195 na mesma republicação"}
	_, skipped := StageActs(edition, []Act{ambiguous, multi})
	if skipped != 2 {
		t.Fatalf("skipped = %d, want 2 (ambiguous + multi-establishment needs review)", skipped)
	}
}
