package com.anpfuel.data.remote

import kotlinx.coroutines.runBlocking
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class AnpListingScraperCacheTest {

    private lateinit var server: MockWebServer
    private lateinit var scraper: AnpListingScraper

    @BeforeEach
    fun setUp() {
        server = MockWebServer()
        server.start()
        scraper = AnpListingScraper(OkHttpClient(), extraAllowedHosts = setOf("localhost"))
    }

    @AfterEach
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun catalogDiscoveryFollowedByPriceTableDiscoveryUsesSingleListingRequest() = runBlocking {
        val listingUrl = server.url("/listing").toString()
        server.enqueue(MockResponse().setBody(AnpFixtureFiles.readListingHtml()))

        scraper.discoverSurveyWeekCatalog(listingUrl)
        scraper.discoverPriceTables(listingUrl)

        assertEquals(1, server.requestCount)
    }

    @Test
    fun expiredListingCacheTriggersFreshNetworkRequest() = runBlocking {
        val listingUrl = server.url("/listing").toString()
        val html = AnpFixtureFiles.readListingHtml()
        server.enqueue(MockResponse().setBody(html))
        server.enqueue(MockResponse().setBody(html))

        scraper.discoverSurveyWeekCatalog(listingUrl)
        scraper.clearListingCache()
        scraper.discoverPriceTables(listingUrl)

        assertEquals(2, server.requestCount)
    }

    @Test
    fun cachedListingHtmlProducesMatchingCatalogAndPriceTables() = runBlocking {
        val listingUrl = server.url("/listing").toString()
        server.enqueue(MockResponse().setBody(AnpFixtureFiles.readListingHtml()))

        val catalog = scraper.discoverSurveyWeekCatalog(listingUrl)
        val priceTables = scraper.discoverPriceTables(listingUrl)

        assertTrue(catalog.isNotEmpty())
        assertTrue(priceTables.isNotEmpty())
        assertEquals(catalog.first().surveyWeek, priceTables.first().surveyWeek)
    }

    // SEC06-A: foreign listings refuse before any fetch; oversized
    // listings refuse capped with nothing parsed.
    @Test
    fun refusesForeignListingWithoutFetching() = runBlocking {
        val before = server.requestCount
        try {
            scraper.discoverPriceTables("https://example.com/listing.html")
            throw AssertionError("foreign listing fetched")
        } catch (_: IllegalArgumentException) {
        }
        assertEquals(before, server.requestCount)
    }

    @Test
    fun refusesOversizedListing() = runBlocking {
        val listingUrl = server.url("/listing").toString()
        server.enqueue(MockResponse().setHeader("Content-Length", AnpListingScraper.MAX_LISTING_BYTES + 1))
        try {
            scraper.discoverPriceTables(listingUrl)
            throw AssertionError("oversize listing parsed")
        } catch (_: IllegalArgumentException) {
        }
    }
}
