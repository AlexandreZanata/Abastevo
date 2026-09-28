package com.anpfuel.data.parser.poi

import com.anpfuel.data.parser.AnpDateMapper
import com.anpfuel.data.parser.StationDetailSheetParser
import com.anpfuel.data.parser.WeeklySummarySheetParser
import com.anpfuel.data.parser.dto.AveragePriceRow
import com.anpfuel.data.parser.dto.StationPriceRow
import org.apache.poi.ss.usermodel.Cell
import org.apache.poi.ss.usermodel.CellType
import org.apache.poi.ss.usermodel.DateUtil
import org.apache.poi.ss.usermodel.Row
import org.apache.poi.ss.usermodel.WorkbookFactory
import java.io.File
import java.math.BigDecimal
import java.time.LocalDate

/**
 * Reads ANP sample XLSX sheets via Apache POI for cross-validation in tests only.
 */
internal object PoiAnpReferenceReader {

    fun readAveragePriceRows(
        file: File,
        sheetName: String = WeeklySummarySheetParser.SHEET_NAME,
        firstDataRowNumber: Int = WeeklySummarySheetParser.DEFAULT_FIRST_DATA_ROW_NUMBER,
    ): List<AveragePriceRow> {
        return file.inputStream().use { input ->
            WorkbookFactory.create(input).use { workbook ->
                val sheet = workbook.getSheet(sheetName)
                    ?: error("Sheet '$sheetName' not found in ${file.name}")
                buildList {
                    for (rowIndex in firstDataRowNumber - 1..sheet.lastRowNum) {
                        sheet.getRow(rowIndex)?.toAveragePriceRowOrNull()?.let(::add)
                    }
                }
            }
        }
    }

    fun readStationPriceRows(
        file: File,
        sheetName: String = StationDetailSheetParser.SHEET_NAME,
        firstDataRowNumber: Int = StationDetailSheetParser.DEFAULT_FIRST_DATA_ROW_NUMBER,
    ): List<StationPriceRow> {
        return file.inputStream().use { input ->
            WorkbookFactory.create(input).use { workbook ->
                val sheet = workbook.getSheet(sheetName)
                    ?: error("Sheet '$sheetName' not found in ${file.name}")
                buildList {
                    for (rowIndex in firstDataRowNumber - 1..sheet.lastRowNum) {
                        sheet.getRow(rowIndex)?.toStationPriceRowOrNull()?.let(::add)
                    }
                }
            }
        }
    }

    private fun Row.toAveragePriceRowOrNull(): AveragePriceRow? {
        val productLabel = cellString(COL_PRODUCT)?.takeIf { it.isNotBlank() } ?: return null
        val startDate = cellLocalDate(COL_START_DATE) ?: return null
        val endDate = cellLocalDate(COL_END_DATE) ?: return null
        return AveragePriceRow(
            startDate = startDate,
            endDate = endDate,
            state = cellString(COL_STATE).orEmpty(),
            municipality = cellString(COL_MUNICIPALITY).orEmpty(),
            productLabel = productLabel,
            stationCount = cellString(COL_STATION_COUNT)?.toIntOrNull(),
            unit = cellString(COL_UNIT),
            averagePrice = cellBigDecimal(COL_AVERAGE_PRICE),
            standardDeviation = cellBigDecimal(COL_STD_DEV),
            minimumPrice = cellBigDecimal(COL_MIN_PRICE),
            maximumPrice = cellBigDecimal(COL_MAX_PRICE),
            variationCoefficient = cellBigDecimal(COL_VARIATION),
        )
    }

    private fun Row.toStationPriceRowOrNull(): StationPriceRow? {
        val productLabel = cellString(COL_STATION_PRODUCT)?.takeIf { it.isNotBlank() } ?: return null
        val price = cellBigDecimal(COL_STATION_PRICE) ?: return null
        return StationPriceRow(
            cnpj = cellString(COL_CNPJ).orEmpty(),
            legalName = cellString(COL_LEGAL_NAME),
            tradeName = cellString(COL_TRADE_NAME),
            address = cellString(COL_ADDRESS).orEmpty(),
            number = cellString(COL_NUMBER),
            complement = cellString(COL_COMPLEMENT),
            neighborhood = cellString(COL_NEIGHBORHOOD),
            zipCode = cellString(COL_ZIP_CODE),
            municipality = cellString(COL_STATION_MUNICIPALITY).orEmpty(),
            stateName = cellString(COL_STATION_STATE).orEmpty(),
            brand = cellString(COL_BRAND),
            productLabel = productLabel,
            unit = cellString(COL_STATION_UNIT),
            price = price,
            collectedAt = cellLocalDate(COL_COLLECTED_AT),
        )
    }

    private fun Row.cellString(index: Int): String? {
        val cell = getCell(index, Row.MissingCellPolicy.RETURN_BLANK_AS_NULL) ?: return null
        return when (cell.cellType) {
            CellType.STRING -> cell.stringCellValue.trim()
            CellType.NUMERIC -> {
                if (DateUtil.isCellDateFormatted(cell)) {
                    cell.localDateTimeCellValue.toLocalDate().toString()
                } else {
                    numericCellValueAsPlainString(cell)
                }
            }
            CellType.BOOLEAN -> cell.booleanCellValue.toString()
            CellType.FORMULA -> cellCachedString(cell)
            else -> null
        }?.takeIf { it.isNotBlank() }
    }

    private fun Row.cellBigDecimal(index: Int): BigDecimal? =
        cellString(index)?.let(::BigDecimal)

    private fun Row.cellLocalDate(index: Int): LocalDate? {
        val cell = getCell(index, Row.MissingCellPolicy.RETURN_BLANK_AS_NULL) ?: return null
        return when (cell.cellType) {
            CellType.NUMERIC -> {
                if (DateUtil.isCellDateFormatted(cell)) {
                    cell.localDateTimeCellValue.toLocalDate()
                } else {
                    AnpDateMapper.toLocalDate(cell.numericCellValue)
                }
            }
            CellType.STRING -> AnpDateMapper.parseCellValue(cell.stringCellValue.trim())
            CellType.FORMULA -> {
                when (cell.cachedFormulaResultType) {
                    CellType.NUMERIC -> {
                        if (DateUtil.isCellDateFormatted(cell)) {
                            cell.localDateTimeCellValue.toLocalDate()
                        } else {
                            AnpDateMapper.toLocalDate(cell.numericCellValue)
                        }
                    }
                    CellType.STRING -> AnpDateMapper.parseCellValue(cell.stringCellValue.trim())
                    else -> null
                }
            }
            else -> null
        }
    }

    private fun cellCachedString(cell: Cell): String? =
        when (cell.cachedFormulaResultType) {
            CellType.STRING -> cell.stringCellValue.trim()
            CellType.NUMERIC -> numericCellValueAsPlainString(cell)
            CellType.BOOLEAN -> cell.booleanCellValue.toString()
            else -> null
        }

    private fun numericCellValueAsPlainString(cell: Cell): String {
        val numericValue = cell.numericCellValue
        return if (numericValue == numericValue.toLong().toDouble()) {
            numericValue.toLong().toString()
        } else {
            numericValue.toString()
        }
    }

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

    private const val COL_CNPJ = 0
    private const val COL_LEGAL_NAME = 1
    private const val COL_TRADE_NAME = 2
    private const val COL_ADDRESS = 3
    private const val COL_NUMBER = 4
    private const val COL_COMPLEMENT = 5
    private const val COL_NEIGHBORHOOD = 6
    private const val COL_ZIP_CODE = 7
    private const val COL_STATION_MUNICIPALITY = 8
    private const val COL_STATION_STATE = 9
    private const val COL_BRAND = 10
    private const val COL_STATION_PRODUCT = 11
    private const val COL_STATION_UNIT = 12
    private const val COL_STATION_PRICE = 13
    private const val COL_COLLECTED_AT = 14
}
