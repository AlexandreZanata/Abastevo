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
        scraper = AnpListingScraper(OkHttpClient())
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
}
