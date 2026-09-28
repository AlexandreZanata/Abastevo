package com.anpfuel.data.parser

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Test

class StreamingXlsxParserTest {

    @Test
    fun readsFirstMunicipiosDataRowCells() {
        StreamingXlsxParser.open(SampleXlsxFiles.resolve(SampleXlsxFiles.SUMMARY_SAMPLE)).use { parser ->
            var firstDataRow: SheetRow? = null
            parser.streamSheet(
                sheetName = WeeklySummarySheetParser.SHEET_NAME,
                options = SheetParseOptions(firstDataRowNumber = 11, headerRowNumber = 10),
            ) { row ->
                if (row.rowNumber == 11) {
                    firstDataRow = row
                }
            }

            val row = requireNotNull(firstDataRow)
            assertEquals("46180", row.cells.getOrNull(0))
            assertEquals("46186", row.cells.getOrNull(1))
            assertEquals("ETANOL HIDRATADO", row.cells.getOrNull(4))
            assertEquals("3.42", row.cells.getOrNull(7))
        }
    }
}
