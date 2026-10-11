package com.anpfuel.data.remote

import com.anpfuel.domain.model.PriceTable
import com.anpfuel.domain.model.SurveyWeekCatalogEntry
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import okhttp3.OkHttpClient
import okhttp3.Request
import org.jsoup.Jsoup

/**
 * Discovers ANP [PriceTable] download URLs from the public listing page (UC-001).
 *
 * SEC06-A bounded discovery: approved listing origins only, capped
 * listing bytes (the whole response never buffers unbounded), and the
 * same filename-pattern classification as before (origin policy is
 * separate from name patterns). Provided via NetworkModule (not
 * constructor injection: the test seam has no binding).
 */
class AnpListingScraper(
    private val okHttpClient: OkHttpClient,
    /**
     * Explicit test seam for offline suites (loopback MockWebServer).
     * Production wiring passes the default empty set (approved gov.br
     * origins over HTTPS only). Never populate from remote input.
     */
    private val extraAllowedHosts: Set<String> = emptySet(),
) {

    private val cacheLock = Any()
    private var cachedListingHtml: CachedListingHtml? = null

    suspend fun discoverPriceTables(
        listingPageUrl: String = AnpEndpoints.LISTING_PAGE_URL,
    ): List<PriceTable> = withContext(Dispatchers.IO) {
        val html = fetchListingHtml(listingPageUrl)
        parsePriceTablesFromHtml(html, listingPageUrl)
    }

    suspend fun discoverSurveyWeekCatalog(
        listingPageUrl: String = AnpEndpoints.LISTING_PAGE_URL,
    ): List<SurveyWeekCatalogEntry> = withContext(Dispatchers.IO) {
        val html = fetchListingHtml(listingPageUrl)
        parseSurveyWeekCatalogFromHtml(html, listingPageUrl)
    }

    fun parseSurveyWeekCatalogFromHtml(
        html: String,
        baseUrl: String = AnpEndpoints.LISTING_PAGE_URL,
    ): List<SurveyWeekCatalogEntry> = AnpListingWeekCatalogParser.parse(html, baseUrl)

    fun parsePriceTablesFromHtml(
        html: String,
        baseUrl: String = AnpEndpoints.LISTING_PAGE_URL,
    ): List<PriceTable> {
        val document = Jsoup.parse(html, baseUrl)
        return document.select("a[href]")
            .asSequence()
            .mapNotNull { anchor ->
                val href = anchor.attr("abs:href").ifBlank { anchor.attr("href") }
                href.takeIf { it.isNotBlank() }
            }
            .filter { AnpPriceTableUrlParser.isPriceTableUrl(it) }
            .distinct()
            .mapNotNull { AnpPriceTableUrlParser.toPriceTable(it) }
            .sortedWith(compareByDescending<PriceTable> { it.surveyWeek.endDate }.thenBy { it.tableType.name })
            .toList()
    }

    internal fun clearListingCache() {
        synchronized(cacheLock) {
            cachedListingHtml = null
        }
    }

    private fun fetchListingHtml(listingPageUrl: String): String {
        synchronized(cacheLock) {
            cachedListingHtml?.let { cached ->
                if (cached.listingPageUrl == listingPageUrl && !cached.isExpired()) {
                    return cached.html
                }
            }
        }

        val html = fetchListingHtmlFromNetwork(listingPageUrl)
        synchronized(cacheLock) {
            cachedListingHtml = CachedListingHtml(
                listingPageUrl = listingPageUrl,
                html = html,
                fetchedAtMillis = System.currentTimeMillis(),
            )
        }
        return html
    }

    private fun fetchListingHtmlFromNetwork(listingPageUrl: String): String {
        require(isFetchableListingUrl(listingPageUrl)) {
            "Listing URL is outside the approved ANP origins: $listingPageUrl"
        }
        val request = Request.Builder()
            .url(listingPageUrl)
            .get()
            .build()

        okHttpClient.newCall(request).execute().use { response ->
            check(response.isSuccessful) {
                "ANP listing request failed with HTTP ${response.code}"
            }
            val body = requireNotNull(response.body) { "ANP listing response body was empty" }
            // Declared length refuses without reading; the read itself
            // stays capped so unexpected files cannot amplify in memory.
            val declared = body.contentLength()
            require(declared < 0 || declared <= MAX_LISTING_BYTES) {
                "ANP listing exceeds $MAX_LISTING_BYTES bytes"
            }
            return readBounded(body.byteStream())
        }
    }

    internal fun isFetchableListingUrl(rawUrl: String): Boolean {
        if (AnpEndpoints.isApprovedUrl(rawUrl)) return true
        if (extraAllowedHosts.isEmpty()) return false
        val host = rawUrl.trim().toHttpUrlOrNull()?.host ?: return false
        return host in extraAllowedHosts
    }

    private fun readBounded(input: java.io.InputStream): String {
        val out = java.io.ByteArrayOutputStream()
        val buffer = ByteArray(8 * 1024)
        var total = 0L
        while (true) {
            val read = input.read(buffer)
            if (read < 0) break
            total += read
            require(total <= MAX_LISTING_BYTES) {
                "ANP listing exceeds $MAX_LISTING_BYTES bytes"
            }
            out.write(buffer, 0, read)
        }
        return out.toString(Charsets.UTF_8.name())
    }

    private data class CachedListingHtml(
        val listingPageUrl: String,
        val html: String,
        val fetchedAtMillis: Long,
    ) {
        fun isExpired(nowMillis: Long = System.currentTimeMillis()): Boolean =
            nowMillis - fetchedAtMillis >= LISTING_CACHE_TTL_MILLIS
    }

    companion object {
        internal const val LISTING_CACHE_TTL_MILLIS = 60_000L
        /**
         * Listing ceiling (SEC06-A): far above the observed ANP
         * listing page (hundreds of KiB) while blocking whole-response
         * buffering amplification.
         */
        const val MAX_LISTING_BYTES: Long = 4L * 1024L * 1024L
    }
}
