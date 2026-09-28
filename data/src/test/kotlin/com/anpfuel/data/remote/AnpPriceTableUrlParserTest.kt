package com.anpfuel.data.remote

import com.anpfuel.domain.valueobject.PriceTableType
import com.anpfuel.domain.valueobject.SurveyWeek
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class AnpPriceTableUrlParserTest {

    private val summaryUrl =
        "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/" +
            "arquivos-lpc/2026/resumo_semanal_lpc_2026-06-07_2026-06-13.xlsx"

    private val stationUrl =
        "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/" +
            "arquivos-lpc/2026/revendas_lpc_2026-06-07_2026-06-13.xlsx"

    @Test
    fun recognizesValidPriceTableUrls() {
        assertTrue(AnpPriceTableUrlParser.isPriceTableUrl(summaryUrl))
        assertTrue(AnpPriceTableUrlParser.isPriceTableUrl(stationUrl))
        assertFalse(AnpPriceTableUrlParser.isPriceTableUrl("https://example.com/file.pdf"))
    }

    @Test
    fun mapsSummaryUrlToWeeklySummaryPriceTable() {
        val priceTable = requireNotNull(AnpPriceTableUrlParser.toPriceTable(summaryUrl))

        assertEquals(PriceTableType.WEEKLY_SUMMARY, priceTable.tableType)
        assertEquals(SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13"), priceTable.surveyWeek)
        assertEquals(summaryUrl, priceTable.sourceUrl)
    }

    @Test
    fun mapsStationUrlToStationDetailPriceTable() {
        val priceTable = requireNotNull(AnpPriceTableUrlParser.toPriceTable(stationUrl))

        assertEquals(PriceTableType.STATION_DETAIL, priceTable.tableType)
        assertEquals(SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13"), priceTable.surveyWeek)
    }

    @Test
    fun acceptsHyphenSeparatedDatesInFilename() {
        val url =
            "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/" +
                "arquivos-lpc/2026/resumo_semanal_lpc_2026-04-12-2026-04-18.xlsx"

        val priceTable = requireNotNull(AnpPriceTableUrlParser.toPriceTable(url))

        assertEquals(SurveyWeek.fromIsoDates("2026-04-12", "2026-04-18"), priceTable.surveyWeek)
    }

    @Test
    fun rejectsInvalidSurveyWeekRange() {
        val url =
            "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/" +
                "arquivos-lpc/2026/resumo_semanal_lpc_2026-01-01_2026-01-31.xlsx"

        assertNull(AnpPriceTableUrlParser.toPriceTable(url))
    }

    @Test
    fun acceptsLegacyHyphenFilenameFormat() {
        val url =
            "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/" +
                "arquivos-lpc/2022/resumo-semanal-lpc-2022-12-25-2022-12-31.xlsx"

        val priceTable = requireNotNull(AnpPriceTableUrlParser.toPriceTable(url))

        assertEquals(SurveyWeek.fromIsoDates("2022-12-25", "2022-12-31"), priceTable.surveyWeek)
    }

    @Test
    fun acceptsDotSeparatedDatesInFilename() {
        val url =
            "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/" +
                "arquivos-lpc/2024/Resumo_SEMANAL_LPC_2024.12.22_2024.12.28.xlsx"

        val priceTable = requireNotNull(AnpPriceTableUrlParser.toPriceTable(url))

        assertEquals(SurveyWeek.fromIsoDates("2024-12-22", "2024-12-28"), priceTable.surveyWeek)
    }

    @Test
    fun acceptsDuplicateFileSuffixBeforeExtension() {
        val url =
            "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/" +
                "arquivos-lpc/2026/resumo_semanal_lpc_2026-03-08_2026-03-14-1.xlsx"

        val priceTable = requireNotNull(AnpPriceTableUrlParser.toPriceTable(url))

        assertEquals(SurveyWeek.fromIsoDates("2026-03-08", "2026-03-14"), priceTable.surveyWeek)
    }

    @Test
    fun acceptsCompactBrazilianDateRangeInFilename() {
        val url =
            "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/" +
                "arquivos-lpc/2025/resumo_semanal_lpc-17112025-a-23112025.xlsx"

        val priceTable = requireNotNull(AnpPriceTableUrlParser.toPriceTable(url))

        assertEquals(SurveyWeek.fromIsoDates("2025-11-17", "2025-11-23"), priceTable.surveyWeek)
    }

    @Test
    fun infersTableTypeFromPortugueseLinkText() {
        assertEquals(
            PriceTableType.WEEKLY_SUMMARY,
            AnpPriceTableUrlParser.inferTableTypeFromLinkText(
                "Preços médios semanais: Brasil, regiões, estados e municípios",
            ),
        )
        assertEquals(
            PriceTableType.STATION_DETAIL,
            AnpPriceTableUrlParser.inferTableTypeFromLinkText(
                "Preços por posto revendedor (combustíveis automotivos e GLP P13)",
            ),
        )
    }

    @Test
    fun priceTableIdIsDeterministicForSameUrl() {
        val first = requireNotNull(AnpPriceTableUrlParser.toPriceTable(summaryUrl))
        val second = requireNotNull(AnpPriceTableUrlParser.toPriceTable(summaryUrl))

        assertEquals(first.id, second.id)
    }
}
