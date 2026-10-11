package com.anpfuel.data.parser

import org.xmlpull.v1.XmlPullParser
import org.xmlpull.v1.XmlPullParserFactory
import java.io.IOException
import java.io.InputStream
import java.util.zip.ZipFile

private const val SPREADSHEET_MAIN_NAMESPACE =
    "http://schemas.openxmlformats.org/spreadsheetml/2006/main"

private fun XmlPullParser.isSpreadsheetTag(localName: String): Boolean =
    name == localName &&
        (namespace == SPREADSHEET_MAIN_NAMESPACE || namespace.isNullOrEmpty())

/**
 * Resource ceilings for untrusted XLSX input (SEC06-B, AUD-10).
 * Every bound refuses before the corresponding allocation — never by
 * catching OutOfMemoryError. Excel grid maxima are hard caps; byte
 * and count ceilings sit far above observed ANP weeklies while
 * blocking input amplification (a 748-byte single-cell file must
 * never drive a 300M-entry row). Honest large ANP samples run
 * against these ceilings in SEC06-E.
 */
object XlsxLimits {
    /** Largest single inflated ZIP entry read (sheet/sharedStrings). */
    const val MAX_ENTRY_BYTES: Long = 256L * 1024L * 1024L
    /** Largest shared-strings table: count cap. */
    const val MAX_SHARED_STRINGS: Int = 1_000_000
    /** Largest single text run (shared/inline/value). */
    const val MAX_TEXT_CHARS: Int = 32_768
    /** Largest worksheet catalog scanned for one sheet lookup. */
    const val MAX_WORKSHEETS: Int = 1_024
}

/**
 * Streams worksheet rows from an XLSX file (ZIP + sheet XML) with low memory use.
 *
 * Untrusted-input posture: coordinates validate before allocation,
 * entry bytes cap before inflation, shared strings cap by count and
 * per-text length, worksheets cap by catalog size. Initialization
 * failure closes the ZIP (no handle leaks); callers close via `use`.
 */
class StreamingXlsxParser private constructor(
    private val zipFile: ZipFile,
    private val sharedStrings: List<String>,
) : AutoCloseable {

    fun streamSheet(
        sheetName: String,
        options: SheetParseOptions,
        onRow: (SheetRow) -> Unit,
    ) {
        val sheetPath = resolveSheetEntryPath(sheetName)
        val entry = zipFile.getEntry(sheetPath)
            ?: error("Sheet '$sheetName' has no data entry")
        cappedStream(entry).use { input ->
            streamSheetXml(input, options, onRow)
        }
    }

    override fun close() {
        zipFile.close()
    }

    private fun resolveSheetEntryPath(sheetName: String): String {
        val relationshipId = readWorkbookRelationshipId(sheetName)
        val target = readWorkbookRelationshipTarget(relationshipId)
        return "xl/$target"
    }

    private fun readWorkbookRelationshipId(sheetName: String): String {
        // The catalog is tiny (capped below), so collect it whole: the
        // size bound must hold regardless of where the target sits.
        val sheets = mutableListOf<Pair<String, String>>()
        entryStream(WORKBOOK_ENTRY)?.use { input ->
            val parser = createParser(input)
            while (parser.next() != XmlPullParser.END_DOCUMENT) {
                if (parser.eventType == XmlPullParser.START_TAG &&
                    parser.isSpreadsheetTag("sheet")
                ) {
                    sheets += (parser.getAttributeValue(null, "name") ?: "") to
                        (parser.getAttributeValue(REL_NAMESPACE, "id") ?: "")
                    require(sheets.size <= XlsxLimits.MAX_WORKSHEETS) {
                        "Workbook exceeds ${XlsxLimits.MAX_WORKSHEETS} worksheets"
                    }
                }
            }
        }
        return sheets.firstOrNull { it.first == sheetName }?.second
            ?.ifBlank { error("Sheet '$sheetName' is missing relationship id") }
            ?: error("Sheet '$sheetName' not found in workbook")
    }

    private fun readWorkbookRelationshipTarget(relationshipId: String): String {
        entryStream(WORKBOOK_RELS_ENTRY)?.use { input ->
            val parser = createParser(input)
            while (parser.next() != XmlPullParser.END_DOCUMENT) {
                if (parser.eventType == XmlPullParser.START_TAG &&
                    parser.name == "Relationship" &&
                    parser.getAttributeValue(null, "Id") == relationshipId
                ) {
                    return parser.getAttributeValue(null, "Target")
                        ?: error("Missing Target for $relationshipId")
                }
            }
        }
        error("Relationship '$relationshipId' not found")
    }

    /** Capped entry stream, or null when the entry is absent. */
    private fun entryStream(entryName: String): InputStream? {
        val entry = zipFile.getEntry(entryName) ?: return null
        return cappedStream(entry)
    }

    private fun streamSheetXml(
        input: InputStream,
        options: SheetParseOptions,
        onRow: (SheetRow) -> Unit,
    ) {
        val parser = createParser(input)
        while (parser.next() != XmlPullParser.END_DOCUMENT) {
            if (parser.eventType == XmlPullParser.START_TAG &&
                parser.isSpreadsheetTag("row")
            ) {
                val rowNumber = parser.getAttributeValue(null, "r")?.toIntOrNull()
                    ?: error("Row element missing r attribute")
                require(rowNumber in 1..XlsxCellReference.MAX_ROW_NUMBER) {
                    "Row number is outside the Excel grid: $rowNumber"
                }
                if (options.shouldEmit(rowNumber) || options.isHeaderRow(rowNumber)) {
                    onRow(readRow(parser, rowNumber))
                }
            }
        }
    }

    private fun readRow(parser: XmlPullParser, rowNumber: Int): SheetRow {
        val cells = mutableMapOf<Int, String?>()
        while (parser.next() != XmlPullParser.END_DOCUMENT) {
            when (parser.eventType) {
                XmlPullParser.START_TAG -> {
                    if (parser.isSpreadsheetTag("c")) {
                        val cellReference = parser.getAttributeValue(null, "r")
                            ?: error("Cell missing r attribute")
                        // Validates before any allocation: XFD is the
                        // widest key, so the dense row below is bounded.
                        val columnIndex = XlsxCellReference.columnIndex(cellReference)
                        require(cells.size <= XlsxCellReference.MAX_COLUMN_INDEX) {
                            "Row $rowNumber exceeds the Excel grid"
                        }
                        val cellType = parser.getAttributeValue(null, "t")
                        cells[columnIndex] = readCellValue(parser, cellType)
                    }
                }
                XmlPullParser.END_TAG -> {
                    if (parser.isSpreadsheetTag("row")) {
                        break
                    }
                }
            }
        }
        val maxIndex = cells.keys.maxOrNull() ?: -1
        return SheetRow(rowNumber = rowNumber, cells = List(maxIndex + 1) { cells[it] })
    }

    private fun readCellValue(parser: XmlPullParser, cellType: String?): String? {
        var value: String? = null
        var inlineText: String? = null
        while (parser.next() != XmlPullParser.END_DOCUMENT) {
            when (parser.eventType) {
                XmlPullParser.START_TAG -> {
                    when {
                        parser.isSpreadsheetTag("v") -> value = readText(parser)
                        parser.isSpreadsheetTag("t") -> inlineText = readText(parser)
                        parser.isSpreadsheetTag("is") -> inlineText = readInlineString(parser)
                    }
                }
                XmlPullParser.END_TAG -> {
                    if (parser.isSpreadsheetTag("c")) {
                        break
                    }
                }
            }
        }
        return when (cellType) {
            "s" -> value?.toIntOrNull()?.let(sharedStrings::get)
            "inlineStr" -> inlineText
            "str" -> value ?: inlineText
            else -> value ?: inlineText
        }
    }

    /** Capped entry stream for sheet data (see companion). */
    private fun cappedStream(entry: java.util.zip.ZipEntry): InputStream =
        Companion.cappedStream(zipFile, entry)

    private fun readInlineString(parser: XmlPullParser): String {
        val builder = StringBuilder()
        while (parser.next() != XmlPullParser.END_DOCUMENT) {
            when (parser.eventType) {
                XmlPullParser.START_TAG -> {
                    if (parser.isSpreadsheetTag("t")) {
                        val run = readText(parser)
                        require(builder.length + run.length <= XlsxLimits.MAX_TEXT_CHARS) {
                            "Inline string exceeds ${XlsxLimits.MAX_TEXT_CHARS} characters"
                        }
                        builder.append(run)
                    }
                }
                XmlPullParser.END_TAG -> {
                    if (parser.isSpreadsheetTag("is")) {
                        break
                    }
                }
            }
        }
        return builder.toString()
    }

    companion object {
        private const val WORKBOOK_ENTRY = "xl/workbook.xml"
        private const val WORKBOOK_RELS_ENTRY = "xl/_rels/workbook.xml.rels"
        private const val SHARED_STRINGS_ENTRY = "xl/sharedStrings.xml"
        private const val REL_NAMESPACE = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"

        fun open(file: java.io.File): StreamingXlsxParser {
            val zipFile = ZipFile(file)
            try {
                val sharedStrings = loadSharedStrings(zipFile)
                return StreamingXlsxParser(zipFile, sharedStrings)
            } catch (e: Exception) {
                // Initialization failure closes the handle: no leak,
                // and callers still see the original refusal.
                try {
                    zipFile.close()
                } catch (_: Exception) {
                }
                throw e
            }
        }

        private fun loadSharedStrings(zipFile: ZipFile): List<String> {
            val entry = zipFile.getEntry(SHARED_STRINGS_ENTRY) ?: return emptyList()
            val strings = mutableListOf<String>()
            cappedStream(zipFile, entry).use { input ->
                val parser = createParser(input)
                while (parser.next() != XmlPullParser.END_DOCUMENT) {
                    if (parser.eventType == XmlPullParser.START_TAG &&
                        parser.isSpreadsheetTag("si")
                    ) {
                        require(strings.size < XlsxLimits.MAX_SHARED_STRINGS) {
                            "Shared strings exceed ${XlsxLimits.MAX_SHARED_STRINGS}"
                        }
                        strings += readSharedStringItem(parser)
                    }
                }
            }
            return strings
        }

        private fun readSharedStringItem(parser: XmlPullParser): String {
            val builder = StringBuilder()
            while (parser.next() != XmlPullParser.END_DOCUMENT) {
                when (parser.eventType) {
                    XmlPullParser.START_TAG -> {
                        if (parser.isSpreadsheetTag("t")) {
                            val run = readText(parser)
                            require(builder.length + run.length <= XlsxLimits.MAX_TEXT_CHARS) {
                                "Shared string exceeds ${XlsxLimits.MAX_TEXT_CHARS} characters"
                            }
                            builder.append(run)
                        }
                    }
                    XmlPullParser.END_TAG -> {
                        if (parser.isSpreadsheetTag("si")) {
                            break
                        }
                    }
                }
            }
            return builder.toString()
        }

        private fun createParser(input: InputStream): XmlPullParser =
            XmlPullParserFactory.newInstance().apply {
                isNamespaceAware = true
            }.newPullParser().apply {
                setInput(input, null)
            }

        private fun readText(parser: XmlPullParser): String {
            val builder = StringBuilder()
            while (parser.next() != XmlPullParser.END_DOCUMENT) {
                when (parser.eventType) {
                    XmlPullParser.TEXT, XmlPullParser.CDSECT -> {
                        require(builder.length + parser.text.length <= XlsxLimits.MAX_TEXT_CHARS) {
                            "Text run exceeds ${XlsxLimits.MAX_TEXT_CHARS} characters"
                        }
                        builder.append(parser.text)
                    }
                    XmlPullParser.END_TAG -> break
                }
            }
            return builder.toString()
        }

        /**
         * Entry stream capped two ways: the declared inflated size
         * refuses without reading, and a counting wrapper caps actual
         * bytes (lying headers cannot smuggle expansion past it).
         */
        private fun cappedStream(zipFile: ZipFile, entry: java.util.zip.ZipEntry): InputStream {
            val declared = entry.size
            require(declared < 0 || declared <= XlsxLimits.MAX_ENTRY_BYTES) {
                "ZIP entry exceeds ${XlsxLimits.MAX_ENTRY_BYTES} bytes"
            }
            return CappedInputStream(zipFile.getInputStream(entry), XlsxLimits.MAX_ENTRY_BYTES)
        }
    }
}
