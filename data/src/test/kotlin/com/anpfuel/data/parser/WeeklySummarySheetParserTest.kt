package com.anpfuel.data.parser

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.math.BigDecimal
import java.time.LocalDate

class WeeklySummarySheetParserTest {

    private val parser = WeeklySummarySheetParser()

    @Test
    fun parsesMunicipiosSheetFromSampleFile() {
        val rows = parser.parseToList(SampleXlsxFiles.resolve(SampleXlsxFiles.SUMMARY_SAMPLE))

        assertTrue(rows.size in 2300..2400, "Expected ~2344 rows but got ${rows.size}")
    }

    @Test
    fun firstDataRowHasExpectedDatesAndProductLabel() {
        val firstRow = parser.parseToList(SampleXlsxFiles.resolve(SampleXlsxFiles.SUMMARY_SAMPLE)).first()

        assertEquals(LocalDate.parse("2026-06-07"), firstRow.startDate)
        assertEquals(LocalDate.parse("2026-06-13"), firstRow.endDate)
        assertEquals("ETANOL HIDRATADO", firstRow.productLabel)
        assertEquals(BigDecimal("3.42"), firstRow.averagePrice)
    }

    @Test
    fun configurableHeaderSkipsMetadataRows() {
        val customParser = WeeklySummarySheetParser(
            headerRowNumber = 10,
            firstDataRowNumber = 11,
        )
        val rows = customParser.parseToList(SampleXlsxFiles.resolve(SampleXlsxFiles.SUMMARY_SAMPLE))

        assertTrue(rows.isNotEmpty())
        assertEquals("ETANOL HIDRATADO", rows.first().productLabel)
    }
}
