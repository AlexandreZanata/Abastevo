package com.anpfuel.data.remote

import okhttp3.HttpUrl.Companion.toHttpUrlOrNull

/**
 * ANP public endpoints (UC-001 discovery).
 * See `docs/data-sources.md`.
 *
 * SEC06-A approved origins: only the gov.br ANP hierarchy may supply
 * listing/download bytes — HTTPS, exact host, path under the listing
 * or arquivos hierarchy. Anything else (foreign hosts, private IPs,
 * plaintext, userinfo, off-hierarchy paths) refuses before any fetch,
 * and the downloader re-checks the FINAL URL after redirects for the
 * same policy, so a listing compromise cannot bounce the client
 * elsewhere. Malformed URLs refuse closed.
 */
object AnpEndpoints {

    const val LISTING_PAGE_URL =
        "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/" +
            "levantamento-de-precos-de-combustiveis-ultimas-semanas-pesquisadas"

    const val DOWNLOAD_BASE_PATH =
        "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/"

    val APPROVED_HOSTS: Set<String> = setOf("www.gov.br")

    /** Path prefixes (decoded) approved for listing and download fetches. */
    val APPROVED_PATH_PREFIXES: List<String> = listOf(
        "/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/",
    )

    fun isApprovedUrl(rawUrl: String): Boolean {
        val url = rawUrl.trim().toHttpUrlOrNull() ?: return false
        if (url.scheme != "https") return false
        if (url.username.isNotEmpty() || url.password.isNotEmpty()) return false
        if (url.host !in APPROVED_HOSTS) return false
        val path = url.encodedPath
        return APPROVED_PATH_PREFIXES.any { prefix -> path.startsWith(prefix) }
    }
}
