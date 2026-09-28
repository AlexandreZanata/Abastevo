package com.anpfuel.data.remote

import com.anpfuel.domain.valueobject.PriceTableType
import okhttp3.OkHttpClient
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * Gate 12.2.4 — full gov.br listing HTML snapshot (Jun/2026).
 */
class AnpListingFullFixtureTest {

    private val scraper = AnpListingScraper(OkHttpClient())

    @Test
    fun fullListingFixtureYieldsAtLeastThirtyCompleteWeekBlocks() {
        val html = AnpFixtureFiles.readFullListingHtml()

        val catalog = scraper.parseSurveyWeekCatalogFromHtml(html)

        assertTrue(
            catalog.size >= 30,
            "Expected at least 30 complete week blocks, got ${catalog.size}",
        )
    }

    @Test
    fun latestWeekInFullFixtureHasSummaryAndStationUrls() {
        val html = AnpFixtureFiles.readFullListingHtml()

        val catalog = scraper.parseSurveyWeekCatalogFromHtml(html)
        val latest = catalog.first()

        assertTrue(latest.summaryUrl.contains("resumo_semanal_lpc"))
        assertTrue(latest.stationUrl.contains("revendas_lpc"))
    }

    @Test
    fun fullListingFixturePreservesOperationalNotesWhenPresent() {
        val html = AnpFixtureFiles.readFullListingHtml()

        val catalog = scraper.parseSurveyWeekCatalogFromHtml(html)
        val withNote = catalog.filter { it.operationalNote != null }

        assertTrue(withNote.isNotEmpty(), "Expected at least one operational note in full fixture")
    }

    @Test
    fun fullListingPriceTableDiscoveryCoversBothFileTypes() {
        val html = AnpFixtureFiles.readFullListingHtml()

        val priceTables = scraper.parsePriceTablesFromHtml(html)

        assertTrue(priceTables.count { it.tableType == PriceTableType.WEEKLY_SUMMARY } >= 30)
        assertTrue(priceTables.count { it.tableType == PriceTableType.STATION_DETAIL } >= 30)
    }

    @Test
    fun fullListingCatalogOrdersNewestWeekFirst() {
        val html = AnpFixtureFiles.readFullListingHtml()

        val catalog = scraper.parseSurveyWeekCatalogFromHtml(html)

        assertTrue(catalog.size >= 2)
        assertTrue(catalog.first().surveyWeek.endDate >= catalog.last().surveyWeek.endDate)
        assertNotNull(catalog.first().publishedAt)
    }
}
