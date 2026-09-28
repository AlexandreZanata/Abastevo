package com.anpfuel.data.remote

import com.anpfuel.domain.valueobject.PriceTableType
import com.anpfuel.domain.valueobject.SurveyWeek
import okhttp3.OkHttpClient
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.time.LocalDate

class AnpListingScraperTest {

    private val scraper = AnpListingScraper(OkHttpClient())

    @Test
    fun parsesFixtureAndFindsLatestWeekSummaryAndStationUrls() {
        val html = AnpFixtureFiles.readListingHtml()

        val priceTables = scraper.parsePriceTablesFromHtml(html)

        assertTrue(priceTables.size >= 4)
        val latestWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
        val latestWeekTables = priceTables.filter { it.surveyWeek == latestWeek }

        assertEquals(2, latestWeekTables.size)
        assertTrue(latestWeekTables.any { it.tableType == PriceTableType.WEEKLY_SUMMARY })
        assertTrue(latestWeekTables.any { it.tableType == PriceTableType.STATION_DETAIL })
    }

    @Test
    fun resolvesRelativeHrefUsingListingBaseUrl() {
        val html = AnpFixtureFiles.readListingHtml()

        val priceTables = scraper.parsePriceTablesFromHtml(html)

        assertTrue(
            priceTables.any {
                it.sourceUrl.endsWith("resumo_semanal_lpc_2026-04-12-2026-04-18.xlsx")
            },
        )
    }

    @Test
    fun mapsFixtureLinksToValidSurveyWeekMetadata() {
        val html = AnpFixtureFiles.readListingHtml()

        val priceTables = scraper.parsePriceTablesFromHtml(html)

        priceTables.forEach { priceTable ->
            assertTrue(priceTable.surveyWeek.inclusiveDayCount <= 7)
            assertTrue(priceTable.sourceUrl.startsWith("https://"))
        }
    }

    @Test
    fun parseSurveyWeekCatalogGroupsUrlsUnderSectionHeaders() {
        val html = AnpFixtureFiles.readListingHtml()

        val catalog = scraper.parseSurveyWeekCatalogFromHtml(html)

        assertEquals(2, catalog.size)
        val latestWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
        val latestEntry = catalog.first()

        assertEquals(latestWeek, latestEntry.surveyWeek)
        assertTrue(latestEntry.summaryUrl.contains("resumo_semanal_lpc_2026-06-07_2026-06-13"))
        assertTrue(latestEntry.stationUrl.contains("revendas_lpc_2026-06-07_2026-06-13"))
    }

    @Test
    fun parseSurveyWeekCatalogOrdersNewestWeekFirst() {
        val html = AnpFixtureFiles.readListingHtml()

        val catalog = scraper.parseSurveyWeekCatalogFromHtml(html)

        assertTrue(
            catalog.zipWithNext().all { (newer, older) ->
                newer.surveyWeek.endDate >= older.surveyWeek.endDate
            },
        )
    }

    @Test
    fun parseSurveyWeekCatalogOmitsIncompleteWeekBlocks() {
        val html = AnpFixtureFiles.readListingHtml()

        val catalog = scraper.parseSurveyWeekCatalogFromHtml(html)

        assertTrue(catalog.none { it.surveyWeek == SurveyWeek.fromIsoDates("2026-04-12", "2026-04-18") })
    }

    @Test
    fun parseSurveyWeekCatalogStoresLatestUpdatedAtPerWeek() {
        val html = AnpFixtureFiles.readListingHtml()

        val catalog = scraper.parseSurveyWeekCatalogFromHtml(html)
        val latestEntry = catalog.first()
        val olderEntry = catalog.last()

        assertEquals(LocalDate.of(2026, 6, 12), latestEntry.publishedAt)
        assertNull(olderEntry.publishedAt)
    }

    @Test
    fun parseSurveyWeekCatalogStoresOperationalNoteOnWeekBlock() {
        val html = AnpFixtureFiles.readListingHtml()

        val catalog = scraper.parseSurveyWeekCatalogFromHtml(html)
        val weekWithNote = catalog.first { it.surveyWeek == SurveyWeek.fromIsoDates("2026-05-31", "2026-06-06") }

        assertEquals(
            "Os preços médios de Belo Horizonte não foram publicados entre 26/04/2026 e 16/05/2026.",
            weekWithNote.operationalNote,
        )
        assertNull(catalog.first().operationalNote)
    }
}
