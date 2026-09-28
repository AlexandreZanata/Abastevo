package com.anpfuel.data.remote

import com.anpfuel.domain.model.PriceTable
import com.anpfuel.domain.model.SurveyWeekCatalogEntry
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request
import org.jsoup.Jsoup
import javax.inject.Inject

/**
 * Discovers ANP [PriceTable] download URLs from the public listing page (UC-001).
 */
class AnpListingScraper @Inject constructor(
    private val okHttpClient: OkHttpClient,
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
        val request = Request.Builder()
            .url(listingPageUrl)
            .get()
            .build()

        okHttpClient.newCall(request).execute().use { response ->
            check(response.isSuccessful) {
                "ANP listing request failed with HTTP ${response.code}"
            }
            return requireNotNull(response.body).string()
        }
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
    }
}
