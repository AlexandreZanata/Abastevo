package com.anpfuel.data.parser

import com.anpfuel.data.parser.dto.AveragePriceRow
import java.io.File
import java.math.BigDecimal

/**
 * Streams municipality average rows from ANP weekly summary files.
 */
class WeeklySummarySheetParser(
    private val headerRowNumber: Int = DEFAULT_HEADER_ROW_NUMBER,
    private val firstDataRowNumber: Int = DEFAULT_FIRST_DATA_ROW_NUMBER,
) {

    fun parse(file: File, emit: (AveragePriceRow) -> Unit) {
        StreamingXlsxParser.open(file).use { parser ->
            parser.streamSheet(
                sheetName = SHEET_NAME,
                options = SheetParseOptions(
                    firstDataRowNumber = firstDataRowNumber,
                    headerRowNumber = headerRowNumber,
                ),
            ) { row ->
                if (row.rowNumber < firstDataRowNumber) return@streamSheet
                row.toAveragePriceRowOrNull()?.let(emit)
            }
        }
    }

    fun parseToList(file: File): List<AveragePriceRow> {
        val rows = mutableListOf<AveragePriceRow>()
        parse(file) { rows += it }
        return rows
    }

    private fun SheetRow.toAveragePriceRowOrNull(): AveragePriceRow? {
        val productLabel = cell(COL_PRODUCT)?.takeIf { it.isNotBlank() } ?: return null
        val startDate = AnpDateMapper.parseCellValue(cell(COL_START_DATE)) ?: return null
        val endDate = AnpDateMapper.parseCellValue(cell(COL_END_DATE)) ?: return null
        return AveragePriceRow(
            startDate = startDate,
            endDate = endDate,
            state = cell(COL_STATE).orEmpty(),
            municipality = cell(COL_MUNICIPALITY).orEmpty(),
            productLabel = productLabel,
            stationCount = cell(COL_STATION_COUNT)?.toIntOrNull(),
            unit = cell(COL_UNIT),
            averagePrice = cell(COL_AVERAGE_PRICE).toBigDecimalOrNull(),
            standardDeviation = cell(COL_STD_DEV).toBigDecimalOrNull(),
            minimumPrice = cell(COL_MIN_PRICE).toBigDecimalOrNull(),
            maximumPrice = cell(COL_MAX_PRICE).toBigDecimalOrNull(),
            variationCoefficient = cell(COL_VARIATION).toBigDecimalOrNull(),
        )
    }

    private fun SheetRow.cell(index: Int): String? = cells.getOrNull(index)

    private fun String?.toBigDecimalOrNull(): BigDecimal? =
        this?.takeIf { it.isNotBlank() }?.let { BigDecimal(it) }

    companion object {
        const val SHEET_NAME = "MUNICIPIOS"
        const val DEFAULT_HEADER_ROW_NUMBER = 10
        const val DEFAULT_FIRST_DATA_ROW_NUMBER = 11

        private const val COL_START_DATE = 0
        private const val COL_END_DATE = 1
        private const val COL_STATE = 2
        private const val COL_MUNICIPALITY = 3
        private const val COL_PRODUCT = 4
        private const val COL_STATION_COUNT = 5
        private const val COL_UNIT = 6
        private const val COL_AVERAGE_PRICE = 7
        private const val COL_STD_DEV = 8
        private const val COL_MIN_PRICE = 9
        private const val COL_MAX_PRICE = 10
        private const val COL_VARIATION = 11
    }
}
