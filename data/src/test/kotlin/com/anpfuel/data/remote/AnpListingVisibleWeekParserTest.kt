package com.anpfuel.data.remote

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable
import java.io.File

class AnpListingVisibleWeekParserTest {

    @Test
    fun countsWeekHeadersInMinimalFixture() {
        val html = AnpFixtureFiles.readListingHtml()

        assertEquals(2, AnpListingVisibleWeekParser.countVisibleWeekHeaders(html))
        assertEquals(2, AnpListingVisibleWeekParser.countCompleteVisibleWeekBlocks(html))
    }

    @Test
    fun fullFixtureCatalogCountMatchesCompleteVisibleWeekBlocks() {
        val html = AnpFixtureFiles.readFullListingHtml()
        val comparison = AnpListingWeekCatalogComparator.compare(html)

        assertTrue(comparison.catalogEntryCount >= 30)
        assertTrue(comparison.visibleWeekHeaderCount >= comparison.catalogEntryCount)
        assertEquals(comparison.catalogEntryCount, comparison.completeVisibleWeekBlockCount)
    }
}

@EnabledIfEnvironmentVariable(named = "ANP_LIVE_POC", matches = "true")
class AnpListingLiveCatalogValidationTest {

    @Test
    fun liveListingCatalogMatchesCompleteVisibleWeekBlocks() {
        val htmlPath = System.getenv("ANP_LIVE_HTML_PATH")
            ?: error("ANP_LIVE_HTML_PATH must point to a live listing HTML snapshot")
        val html = File(htmlPath).readText()
        val comparison = AnpListingWeekCatalogComparator.compare(html)

        comparison.requireValidDiscovery()

        System.getenv("ANP_LIVE_POC_OUTPUT")?.let { outputPath ->
            File(outputPath).writeText(formatWeekCatalogPocMarkdown(comparison))
        }
    }
}
