package com.anpfuel.data.remote

import com.anpfuel.domain.valueobject.PriceTableType
import com.anpfuel.domain.valueobject.SurveyWeek
import org.jsoup.Jsoup
import org.jsoup.nodes.Element
import org.jsoup.nodes.Node
import org.jsoup.nodes.TextNode
import org.jsoup.select.NodeTraversor
import org.jsoup.select.NodeVisitor

/**
 * Counts visible week section headers on the ANP listing page (Phase 12.2.5).
 * Mirrors gov.br week blocks shown as date ranges (e.g. "07/06/2026 a 13/06/2026").
 */
internal object AnpListingVisibleWeekParser {

    private val HEADER_TAGS = setOf("h1", "h2", "h3", "h4", "h5", "h6", "strong", "p")
    private val WEEK_HEADER_PATTERN = Regex(
        """(\d{1,2}/\d{1,2}/\d{4})\s+a\s+(\d{1,2}/\d{1,2}/\d{4})""",
        RegexOption.IGNORE_CASE,
    )

    fun countVisibleWeekHeaders(html: String, baseUrl: String = AnpEndpoints.LISTING_PAGE_URL): Int =
        discoverVisibleWeeks(html, baseUrl).size

    /**
     * Week blocks on gov.br that expose both summary and station download links
     * between one date-range header and the next.
     */
    fun countCompleteVisibleWeekBlocks(
        html: String,
        baseUrl: String = AnpEndpoints.LISTING_PAGE_URL,
    ): Int {
        val document = Jsoup.parse(html, baseUrl)
        val bodyHtml = document.body()?.html() ?: return 0
        val headers = WEEK_HEADER_PATTERN.findAll(bodyHtml).toList()
        if (headers.isEmpty()) {
            return 0
        }

        return headers.indices.count { index ->
            sectionHasSummaryAndStation(sectionHtml(bodyHtml, headers, index), baseUrl)
        }
    }

    fun discoverVisibleWeeks(
        html: String,
        baseUrl: String = AnpEndpoints.LISTING_PAGE_URL,
    ): Set<SurveyWeek> {
        val document = Jsoup.parse(html, baseUrl)
        val body = document.body() ?: return emptySet()
        val weeks = linkedSetOf<SurveyWeek>()

        NodeTraversor.traverse(
            object : NodeVisitor {
                override fun head(node: Node, depth: Int) {
                    when (node) {
                        is TextNode -> collectWeek(node.wholeText, weeks)
                        is Element -> {
                            if (node.tagName() in HEADER_TAGS) {
                                collectWeek(node.ownText(), weeks)
                            }
                        }
                    }
                }

                override fun tail(node: Node, depth: Int) = Unit
            },
            body,
        )

        return weeks
    }

    private fun sectionHtml(bodyHtml: String, headers: List<MatchResult>, index: Int): String {
        val sectionStart = headers[index].range.last + 1
        val sectionEnd = headers.getOrNull(index + 1)?.range?.first ?: bodyHtml.length
        return bodyHtml.substring(sectionStart, sectionEnd)
    }

    private fun sectionHasSummaryAndStation(sectionHtml: String, baseUrl: String): Boolean {
        val section = Jsoup.parseBodyFragment(sectionHtml, baseUrl)
        var hasSummary = false
        var hasStation = false

        section.select("a[href]").forEach { anchor ->
            val href = anchor.absUrl("href").ifBlank { anchor.attr("href") }
            if (!AnpPriceTableUrlParser.isPriceTableUrl(href)) return@forEach
            val priceTable = AnpPriceTableUrlParser.toPriceTable(href)
            val tableType = priceTable?.tableType
                ?: AnpPriceTableUrlParser.inferTableTypeFromLinkText(anchor.text())
                ?: return@forEach
            when (tableType) {
                PriceTableType.WEEKLY_SUMMARY -> hasSummary = true
                PriceTableType.STATION_DETAIL -> hasStation = true
            }
        }

        return hasSummary && hasStation
    }

    private fun collectWeek(text: String, weeks: MutableSet<SurveyWeek>) {
        val week = AnpListingWeekHeaderParser.parseWeekHeader(text) ?: return
        weeks.add(week)
    }
}
