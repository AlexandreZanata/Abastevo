package com.anpfuel.data.remote

import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * SEC06-A origin policy matrix (AUD-10): only the gov.br ANP hierarchy
 * supplies bytes — HTTPS, exact host, approved path. Everything else
 * refuses before any fetch (no network in this suite).
 */
class AnpOriginPolicyTest {

    @Test
    fun approvesCanonicalListingAndDownload() {
        assertTrue(AnpEndpoints.isApprovedUrl(AnpEndpoints.LISTING_PAGE_URL))
        assertTrue(AnpEndpoints.isApprovedUrl(AnpEndpoints.DOWNLOAD_BASE_PATH + "2026/resumo_semanal_lpc_2026-06-07_2026-06-13.xlsx"))
    }

    @Test
    fun refusesForeignPrivatePlaintextAndUserinfo() {
        val bad = listOf(
            "https://example.com/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/2026/a.xlsx",
            "https://evil-gov-br.example.com/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/2026/a.xlsx",
            "https://www.gov.br.evil.com/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/2026/a.xlsx",
            "http://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/2026/a.xlsx",
            "https://user:pass@www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/2026/a.xlsx",
            "https://127.0.0.1/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/2026/a.xlsx",
            "https://10.0.0.5/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/2026/a.xlsx",
            "https://[::1]/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/2026/a.xlsx",
            "https://www.gov.br/other/path/a.xlsx",
            "https://www.gov.br/",
            "not a url",
            "",
            "ftp://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/2026/a.xlsx",
        )
        for (url in bad) {
            assertFalse(AnpEndpoints.isApprovedUrl(url), "approved: $url")
        }
    }
}
