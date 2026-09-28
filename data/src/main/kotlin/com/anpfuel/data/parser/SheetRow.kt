package com.anpfuel.data.parser

/**
 * One parsed row from an XLSX worksheet (`row` @r is 1-based Excel row number).
 */
data class SheetRow(
    val rowNumber: Int,
    val cells: List<String?>,
)

/**
 * Controls which rows are emitted by [StreamingXlsxParser].
 */
data class SheetParseOptions(
    val firstDataRowNumber: Int,
    val headerRowNumber: Int? = null,
) {
    init {
        require(firstDataRowNumber > 0) { "firstDataRowNumber must be positive" }
        headerRowNumber?.let { require(it > 0) { "headerRowNumber must be positive" } }
    }

    fun shouldEmit(rowNumber: Int): Boolean = rowNumber >= firstDataRowNumber

    fun isHeaderRow(rowNumber: Int): Boolean = headerRowNumber == rowNumber
}
