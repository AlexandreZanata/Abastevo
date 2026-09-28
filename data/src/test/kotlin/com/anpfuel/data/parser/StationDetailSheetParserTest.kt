package com.anpfuel.data.parser

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.math.BigDecimal

class StationDetailSheetParserTest {

    private val parser = StationDetailSheetParser()

    @Test
    fun parsesStationSheetFromSampleFile() {
        val rows = parser.parseToList(SampleXlsxFiles.resolve(SampleXlsxFiles.STATION_SAMPLE))

        assertTrue(rows.size in 19000..20000, "Expected ~19676 rows but got ${rows.size}")
    }

    @Test
    fun firstDataRowContainsCnpjProductAndPrice() {
        val firstRow = parser.parseToList(SampleXlsxFiles.resolve(SampleXlsxFiles.STATION_SAMPLE)).first()

        assertTrue(firstRow.cnpj.isNotBlank())
        assertEquals("GLP", firstRow.productLabel)
        assertTrue(firstRow.price >= BigDecimal.ZERO)
        assertTrue(firstRow.address.isNotBlank())
    }

    @Test
    fun configurableHeaderSkipsMetadataRows() {
        val customParser = StationDetailSheetParser(
            headerRowNumber = 10,
            firstDataRowNumber = 11,
        )
        val rows = customParser.parseToList(SampleXlsxFiles.resolve(SampleXlsxFiles.STATION_SAMPLE))

        assertTrue(rows.isNotEmpty())
        assertTrue(rows.first().municipality.isNotBlank())
    }
}
