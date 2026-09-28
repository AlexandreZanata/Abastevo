package com.anpfuel.data.parser

import com.anpfuel.data.parser.dto.StationPriceRow
import java.io.File
import java.math.BigDecimal

/**
 * Streams station-level price rows from ANP station detail files.
 */
class StationDetailSheetParser(
    private val headerRowNumber: Int = DEFAULT_HEADER_ROW_NUMBER,
    private val firstDataRowNumber: Int = DEFAULT_FIRST_DATA_ROW_NUMBER,
) {

    fun parse(file: File, emit: (StationPriceRow) -> Unit) {
        StreamingXlsxParser.open(file).use { parser ->
            parser.streamSheet(
                sheetName = SHEET_NAME,
                options = SheetParseOptions(
                    firstDataRowNumber = firstDataRowNumber,
                    headerRowNumber = headerRowNumber,
                ),
            ) { row ->
                if (row.rowNumber < firstDataRowNumber) return@streamSheet
                emit(row.toStationPriceRow())
            }
        }
    }

    fun parseToList(file: File): List<StationPriceRow> {
        val rows = mutableListOf<StationPriceRow>()
        parse(file) { rows += it }
        return rows
    }

    private fun SheetRow.toStationPriceRow(): StationPriceRow {
        val streetAddress = cell(COL_ADDRESS).orEmpty()
        return StationPriceRow(
            cnpj = cell(COL_CNPJ).orEmpty(),
            legalName = cell(COL_LEGAL_NAME),
            tradeName = cell(COL_TRADE_NAME),
            address = streetAddress,
            number = cell(COL_NUMBER),
            complement = cell(COL_COMPLEMENT),
            neighborhood = cell(COL_NEIGHBORHOOD),
            zipCode = cell(COL_ZIP_CODE),
            municipality = cell(COL_MUNICIPALITY).orEmpty(),
            stateName = cell(COL_STATE).orEmpty(),
            brand = cell(COL_BRAND),
            productLabel = cell(COL_PRODUCT).orEmpty(),
            unit = cell(COL_UNIT),
            price = cell(COL_PRICE)?.let(::BigDecimal)
                ?: error("Missing price at row $rowNumber"),
            collectedAt = AnpDateMapper.parseCellValue(cell(COL_COLLECTED_AT)),
        )
    }

    private fun SheetRow.cell(index: Int): String? = cells.getOrNull(index)

    companion object {
        const val SHEET_NAME = "POSTOS REVENDEDORES"
        const val DEFAULT_HEADER_ROW_NUMBER = 10
        const val DEFAULT_FIRST_DATA_ROW_NUMBER = 11

        private const val COL_CNPJ = 0
        private const val COL_LEGAL_NAME = 1
        private const val COL_TRADE_NAME = 2
        private const val COL_ADDRESS = 3
        private const val COL_NUMBER = 4
        private const val COL_COMPLEMENT = 5
        private const val COL_NEIGHBORHOOD = 6
        private const val COL_ZIP_CODE = 7
        private const val COL_MUNICIPALITY = 8
        private const val COL_STATE = 9
        private const val COL_BRAND = 10
        private const val COL_PRODUCT = 11
        private const val COL_UNIT = 12
        private const val COL_PRICE = 13
        private const val COL_COLLECTED_AT = 14
    }
}
