package com.anpfuel.data.remote

import com.anpfuel.domain.model.SurveyWeekCatalogEntry
import com.anpfuel.domain.valueobject.PriceTableType
import com.anpfuel.domain.valueobject.SurveyWeek
import org.jsoup.Jsoup
import org.jsoup.nodes.Element
import java.time.LocalDate

/**
 * Groups ANP listing links under week section headers (Phase 12.2.1–12.2.5).
 * Uses gov.br week-block boundaries (date-range header → next header) for reliable grouping.
 */
internal object AnpListingWeekCatalogParser {

    private val WEEK_HEADER_PATTERN = Regex(
        """(\d{1,2}/\d{1,2}/\d{4})\s+a\s+(\d{1,2}/\d{1,2}/\d{4})""",
        RegexOption.IGNORE_CASE,
    )

    fun parse(html: String, baseUrl: String): List<SurveyWeekCatalogEntry> {
        val document = Jsoup.parse(html, baseUrl)
        val bodyHtml = document.body()?.html() ?: return emptyList()
        val headers = WEEK_HEADER_PATTERN.findAll(bodyHtml).toList()
        if (headers.isEmpty()) {
            return emptyList()
        }

        return headers.mapIndexedNotNull { index, match ->
            parseWeekSection(
                surveyWeek = AnpListingWeekHeaderParser.parseWeekHeader(match.value)
                    ?: return@mapIndexedNotNull null,
                sectionHtml = sectionHtml(bodyHtml, headers, index),
                baseUrl = baseUrl,
            )
        }.sortedByDescending { it.surveyWeek.endDate }
    }

    private fun sectionHtml(bodyHtml: String, headers: List<MatchResult>, index: Int): String {
        val sectionStart = headers[index].range.last + 1
        val sectionEnd = headers.getOrNull(index + 1)?.range?.first ?: bodyHtml.length
        return bodyHtml.substring(sectionStart, sectionEnd)
    }

    private fun parseWeekSection(
        surveyWeek: SurveyWeek,
        sectionHtml: String,
        baseUrl: String,
    ): SurveyWeekCatalogEntry? {
        val section = Jsoup.parseBodyFragment(sectionHtml, baseUrl)
        var summaryUrl: String? = null
        var stationUrl: String? = null
        var summaryPublishedAt: LocalDate? = null
        var stationPublishedAt: LocalDate? = null

        section.select("a[href]").forEach { anchor ->
            val href = anchor.absUrl("href").ifBlank { anchor.attr("href") }
            if (!AnpPriceTableUrlParser.isPriceTableUrl(href)) return@forEach
            val priceTable = AnpPriceTableUrlParser.toPriceTable(href)
            val tableType = priceTable?.tableType
                ?: AnpPriceTableUrlParser.inferTableTypeFromLinkText(anchor.text())
                ?: return@forEach
            val sourceUrl = priceTable?.sourceUrl ?: href
            val updatedAt = AnpListingUpdatedAtParser.parseUpdatedAt(anchorContextText(anchor))
            when (tableType) {
                PriceTableType.WEEKLY_SUMMARY -> if (summaryUrl == null) {
                    summaryUrl = sourceUrl
                    summaryPublishedAt = updatedAt
                }
                PriceTableType.STATION_DETAIL -> if (stationUrl == null) {
                    stationUrl = sourceUrl
                    stationPublishedAt = updatedAt
                }
            }
        }

        val resolvedSummaryUrl = summaryUrl ?: return null
        val resolvedStationUrl = stationUrl ?: return null

        return SurveyWeekCatalogEntry.create(
            surveyWeek = surveyWeek,
            summaryUrl = resolvedSummaryUrl,
            stationUrl = resolvedStationUrl,
            publishedAt = listOfNotNull(summaryPublishedAt, stationPublishedAt).maxOrNull(),
            operationalNote = parseOperationalNote(section),
        )
    }

    private fun parseOperationalNote(section: Element): String? {
        section.select("p, div, em, small, span").forEach { element ->
            val note = AnpListingOperationalNoteParser.parseOperationalNote(element.text())
            if (note != null) {
                return note
            }
        }
        return AnpListingOperationalNoteParser.parseOperationalNote(section.text())
    }

    private fun anchorContextText(anchor: Element): String {
        val listItem = anchor.parent()?.takeIf { it.tagName() == "li" }
        return listItem?.text() ?: anchor.text()
    }
}
