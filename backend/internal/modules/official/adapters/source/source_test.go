package source

import (
	"net/netip"
	"testing"
)

func TestDefaultAllowlistMatchesDocumentedOrigins(t *testing.T) {
	// Anchors from docs/data-sources.md: these exact documented patterns
	// must stay fetchable; anything else stays refused.
	allow := DefaultAllowlist()
	approved := []struct {
		url  string
		kind string
	}{
		{"https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/levantamento-de-precos-de-combustiveis-ultimas-semanas-pesquisadas", KindListing},
		{"https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/2026/revendas_lpc_2026-06-07_2026-06-13.xlsx", KindStationDetail},
		{"https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/2026/resumo_semanal_lpc_2026-06-07_2026-06-13.xlsx", KindWeeklySummary},
		{"https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/serie-historica-de-precos-de-combustiveis", KindHistoricalCSV},
	}
	for _, c := range approved {
		e, err := Match(allow, c.url)
		if err != nil {
			t.Errorf("documented origin refused %q: %v", c.url, err)
			continue
		}
		if e.Kind != c.kind {
			t.Errorf("%q kind = %q, want %q", c.url, e.Kind, c.kind)
		}
	}
}

func TestAllowlistRefusals(t *testing.T) {
	allow := DefaultAllowlist()
	refused := map[string]string{
		"subdomain":  "https://evil.www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/2026/revendas_lpc_2026-06-07_2026-06-13.xlsx",
		"plain http": "http://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/2026/revendas_lpc_2026-06-07_2026-06-13.xlsx",
		"userinfo":   "https://user:pass@www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/2026/revendas_lpc_2026-06-07_2026-06-13.xlsx",
		"port":       "https://www.gov.br:8443/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/2026/revendas_lpc_2026-06-07_2026-06-13.xlsx",
		"query":      "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/2026/revendas_lpc_2026-06-07_2026-06-13.xlsx?x=1",
		"wrong path": "https://www.gov.br/anp/pt-br/outro/lugar/file.xlsx",
		"wrong name": "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/2026/pwned.exe",
		"other host": "https://example.com/anp/file.xlsx",
		"ip literal": "https://93.184.216.34/anp/file.xlsx",
		"traversal":  "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/../../etc/passwd",
		"ftp":        "ftp://www.gov.br/anp/file.xlsx",
		"empty":      "",
		"garbage":    "://missing-scheme",
	}
	for name, raw := range refused {
		t.Run(name, func(t *testing.T) {
			if _, err := Match(allow, raw); err == nil {
				t.Errorf("URL accepted: %q", raw)
			}
		})
	}
}

func TestPublicUnicastOnly(t *testing.T) {
	denied := []string{
		"10.0.0.1", "100.64.0.1", "127.0.0.1", "169.254.1.1", "172.16.0.1",
		"192.0.0.1", "192.0.2.1", "192.168.1.1", "198.18.0.1", "198.51.100.1",
		"203.0.113.1", "224.0.0.1", "0.0.0.0", "::", "::1", "fe80::1",
		"fc00::1", "2001:db8::1", "::ffff:127.0.0.1",
	}
	for _, s := range denied {
		addr := netip.MustParseAddr(s)
		if PublicUnicastOnly(addr) {
			t.Errorf("address allowed: %s", s)
		}
	}
	allowed := []string{"8.8.8.8", "1.1.1.1", "2001:4860:4860::8888", "2804::1"}
	for _, s := range allowed {
		addr := netip.MustParseAddr(s)
		if !PublicUnicastOnly(addr) {
			t.Errorf("public address refused: %s", s)
		}
	}
	if PublicUnicastOnly(netip.Addr{}) {
		t.Error("invalid address allowed")
	}
}
