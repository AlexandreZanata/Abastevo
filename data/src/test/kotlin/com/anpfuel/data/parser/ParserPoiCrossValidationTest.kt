package com.anpfuel.data.parser

import com.anpfuel.data.parser.dto.AveragePriceRow
import com.anpfuel.data.parser.dto.StationPriceRow
import com.anpfuel.data.parser.poi.PoiAnpReferenceReader
import com.anpfuel.domain.rule.FuelProductNormalizationRule
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.math.BigDecimal
import java.math.RoundingMode

class ParserPoiCrossValidationTest {

    private val summaryFile = SampleXlsxFiles.resolve(SampleXlsxFiles.SUMMARY_SAMPLE)
    private val stationFile = SampleXlsxFiles.resolve(SampleXlsxFiles.STATION_SAMPLE)
    private val summaryParser = WeeklySummarySheetParser()
    private val stationParser = StationDetailSheetParser()

    @Test
    fun poiReadsSummaryMunicipiosBaselineRowCount() {
        val rows = PoiAnpReferenceReader.readAveragePriceRows(summaryFile)

        assertEquals(2344, rows.size)
    }

    @Test
    fun summaryCustomParserRowCountMatchesPoi() {
        val poiRows = PoiAnpReferenceReader.readAveragePriceRows(summaryFile)
        val parserRows = summaryParser.parseToList(summaryFile)

        assertEquals(poiRows.size, parserRows.size)
    }

    @Test
    fun summaryFirstTenRowsMatchPoiFieldByField() {
        val poiRows = PoiAnpReferenceReader.readAveragePriceRows(summaryFile)
        val parserRows = summaryParser.parseToList(summaryFile)

        assertTrue(poiRows.size >= 10)
        assertTrue(parserRows.size >= 10)
        repeat(10) { index ->
            assertAveragePriceRowEqual(poiRows[index], parserRows[index], index)
        }
    }

    @Test
    fun poiReadsStationSheetBaselineRowCount() {
        val rows = PoiAnpReferenceReader.readStationPriceRows(stationFile)

        assertEquals(19676, rows.size)
    }

    @Test
    fun stationCustomParserRowCountMatchesPoi() {
        val poiRows = PoiAnpReferenceReader.readStationPriceRows(stationFile)
        val parserRows = stationParser.parseToList(stationFile)

        assertEquals(poiRows.size, parserRows.size)
    }

    @Test
    fun stationFirstTenRowsMatchPoiFieldByField() {
        val poiRows = PoiAnpReferenceReader.readStationPriceRows(stationFile)
        val parserRows = stationParser.parseToList(stationFile)

        assertTrue(poiRows.size >= 10)
        assertTrue(parserRows.size >= 10)
        repeat(10) { index ->
            assertStationPriceRowEqual(poiRows[index], parserRows[index], index)
        }
    }

    @Test
    fun allSampleProductLabelsMapViaBr002() {
        val labels = buildSet {
            summaryParser.parseToList(summaryFile).forEach { add(it.productLabel) }
            stationParser.parseToList(stationFile).forEach { add(it.productLabel) }
        }

        val unmapped = labels.filter { FuelProductNormalizationRule.normalize(it).isFailure }

        assertTrue(unmapped.isEmpty(), "Unmapped product labels: $unmapped")
    }

    private fun assertAveragePriceRowEqual(
        expected: AveragePriceRow,
        actual: AveragePriceRow,
        index: Int,
    ) {
        assertEquals(expected.startDate, actual.startDate, "startDate mismatch at row $index")
        assertEquals(expected.endDate, actual.endDate, "endDate mismatch at row $index")
        assertEquals(expected.state, actual.state, "state mismatch at row $index")
        assertEquals(expected.municipality, actual.municipality, "municipality mismatch at row $index")
        assertEquals(expected.productLabel, actual.productLabel, "productLabel mismatch at row $index")
        assertEquals(expected.stationCount, actual.stationCount, "stationCount mismatch at row $index")
        assertEquals(expected.unit, actual.unit, "unit mismatch at row $index")
        assertBigDecimalEqual(expected.averagePrice, actual.averagePrice, "averagePrice at row $index")
        assertBigDecimalEqual(expected.standardDeviation, actual.standardDeviation, "standardDeviation at row $index")
        assertBigDecimalEqual(expected.minimumPrice, actual.minimumPrice, "minimumPrice at row $index")
        assertBigDecimalEqual(expected.maximumPrice, actual.maximumPrice, "maximumPrice at row $index")
        assertBigDecimalEqual(
            expected.variationCoefficient,
            actual.variationCoefficient,
            "variationCoefficient at row $index",
        )
    }

    private fun assertStationPriceRowEqual(
        expected: StationPriceRow,
        actual: StationPriceRow,
        index: Int,
    ) {
        assertEquals(expected.cnpj, actual.cnpj, "cnpj mismatch at row $index")
        assertEquals(expected.legalName, actual.legalName, "legalName mismatch at row $index")
        assertEquals(expected.tradeName, actual.tradeName, "tradeName mismatch at row $index")
        assertEquals(expected.address, actual.address, "address mismatch at row $index")
        assertEquals(expected.number, actual.number, "number mismatch at row $index")
        assertEquals(expected.complement, actual.complement, "complement mismatch at row $index")
        assertEquals(expected.neighborhood, actual.neighborhood, "neighborhood mismatch at row $index")
        assertEquals(expected.zipCode, actual.zipCode, "zipCode mismatch at row $index")
        assertEquals(expected.municipality, actual.municipality, "municipality mismatch at row $index")
        assertEquals(expected.stateName, actual.stateName, "stateName mismatch at row $index")
        assertEquals(expected.brand, actual.brand, "brand mismatch at row $index")
        assertEquals(expected.productLabel, actual.productLabel, "productLabel mismatch at row $index")
        assertEquals(expected.unit, actual.unit, "unit mismatch at row $index")
        assertBigDecimalEqual(expected.price, actual.price, "price at row $index")
        assertEquals(expected.collectedAt, actual.collectedAt, "collectedAt mismatch at row $index")
    }

    private fun assertBigDecimalEqual(expected: BigDecimal?, actual: BigDecimal?, label: String) {
        when {
            expected == null && actual == null -> Unit
            expected == null || actual == null ->
                assertEquals(expected, actual, label)
            else -> {
                val normalizedExpected = expected.normalizedForComparison()
                val normalizedActual = actual.normalizedForComparison()
                assertEquals(
                    0,
                    normalizedExpected.compareTo(normalizedActual),
                    "$label: expected=$expected actual=$actual",
                )
            }
        }
    }

    private fun BigDecimal.normalizedForComparison(): BigDecimal =
        setScale(12, RoundingMode.HALF_UP).stripTrailingZeros()
}
