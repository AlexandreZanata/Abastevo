package native

import (
	"strings"
	"testing"
)

const pmqcPageURL = "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/pmqc-programa-de-monitoramento-da-qualidade-dos-combustiveis"

// TestParseListingPageDerivesPeriodsFromHrefs freezes the discovery input
// contract: periods come from verified archive hrefs only, never from anchor
// text, and missing periods are never invented.
func TestParseListingPageDerivesPeriodsFromHrefs(t *testing.T) {
	page := `<!doctype html><html><body>
<a href="/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/pmqc/pmqc_2025.zip">2025</a>
<a href="https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/pmqc/pmqc_2026_03.zip">Marco</a>
<a href="/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/pmqc/pmqc_2026_07.zip">Fevereiro</a>
<a href="/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/pmqc/pmqc_2026_07.zip">Julho</a>
<a href="/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/pmqc/pmqc-metadados.pdf">Metadados</a>
<a href="#conteudo">Conteudo</a>
</body></html>`
	refs, e := ParseListingPage(pmqcPageURL, []byte(page))
	if e != nil {
		t.Fatal(e)
	}
	if len(refs) != 3 {
		t.Fatalf("hrefs not deduped by verified URL: %+v", refs)
	}
	byLabel := map[string]ArchiveRef{}
	for _, ref := range refs {
		byLabel[ref.Year+ref.Month] = ref
	}
	if ref, ok := byLabel["2025"]; !ok || ref.Month != "" || ref.URL != "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/pmqc/pmqc_2025.zip" {
		t.Fatal("annual archive lost", refs)
	}
	if _, ok := byLabel["202603"]; !ok {
		t.Fatal("monthly archive lost", refs)
	}
	// The February anchor text pointed at the July file; only the verified
	// href period exists and no 2026-02 entry may be fabricated.
	if _, ok := byLabel["202607"]; !ok {
		t.Fatal("mislabeled monthly archive lost", refs)
	}
	if _, ok := byLabel["202602"]; ok {
		t.Fatal("missing period invented from anchor text", refs)
	}
}

func TestParseListingPageRefusesUnusablePages(t *testing.T) {
	valid := `https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/pmqc/pmqc_2016_01.zip`
	cases := []struct {
		name string
		page string
		body string
	}{
		{"zero archives", pmqcPageURL, `<html><a href="#x">nothing</a></html>`},
		{"foreign host", pmqcPageURL, `<html><a href="https://evil.example/pmqc_2026_01.zip">x</a></html>`},
		{"wrong prefix", pmqcPageURL, `<html><a href="https://www.gov.br/outro/pmqc_2026_01.zip">x</a></html>`},
		{"query smuggling", pmqcPageURL, `<html><a href="https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/pmqc/pmqc_2026_01.zip?x=1">x</a></html>`},
		{"malformed page URL", "https://evil.example/", `<html><a href="` + valid + `">x</a></html>`},
		{"oversized body", pmqcPageURL, strings.Repeat("a", MaxListingBytes+1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, e := ParseListingPage(tc.page, []byte(tc.body)); e == nil {
				t.Fatal("unusable page accepted")
			}
		})
	}
	if _, e := ParseListingPage(pmqcPageURL, nil); e == nil {
		t.Fatal("empty page accepted")
	}
}
