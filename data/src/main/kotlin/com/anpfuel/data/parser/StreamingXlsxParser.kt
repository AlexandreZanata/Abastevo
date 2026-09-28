package com.anpfuel.data.parser

import org.xmlpull.v1.XmlPullParser
import org.xmlpull.v1.XmlPullParserFactory
import java.io.InputStream
import java.util.zip.ZipFile

private const val SPREADSHEET_MAIN_NAMESPACE =
    "http://schemas.openxmlformats.org/spreadsheetml/2006/main"

private fun XmlPullParser.isSpreadsheetTag(localName: String): Boolean =
    name == localName &&
        (namespace == SPREADSHEET_MAIN_NAMESPACE || namespace.isNullOrEmpty())

/**
 * Streams worksheet rows from an XLSX file (ZIP + sheet XML) with low memory use.
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
        zipFile.getInputStream(zipFile.getEntry(sheetPath)).use { input ->
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
        zipFile.getInputStream(zipFile.getEntry(WORKBOOK_ENTRY)).use { input ->
            val parser = createParser(input)
            while (parser.next() != XmlPullParser.END_DOCUMENT) {
                if (parser.eventType == XmlPullParser.START_TAG &&
                    parser.isSpreadsheetTag("sheet") &&
                    parser.getAttributeValue(null, "name") == sheetName
                ) {
                    return parser.getAttributeValue(REL_NAMESPACE, "id")
                        ?: error("Sheet '$sheetName' is missing relationship id")
                }
            }
        }
        error("Sheet '$sheetName' not found in workbook")
    }

    private fun readWorkbookRelationshipTarget(relationshipId: String): String {
        zipFile.getInputStream(zipFile.getEntry(WORKBOOK_RELS_ENTRY)).use { input ->
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
                        val columnIndex = XlsxCellReference.columnIndex(cellReference)
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

    private fun readInlineString(parser: XmlPullParser): String {
        val builder = StringBuilder()
        while (parser.next() != XmlPullParser.END_DOCUMENT) {
            when (parser.eventType) {
                XmlPullParser.START_TAG -> {
                    if (parser.isSpreadsheetTag("t")) {
                        builder.append(readText(parser))
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
            val sharedStrings = loadSharedStrings(zipFile)
            return StreamingXlsxParser(zipFile, sharedStrings)
        }

        private fun loadSharedStrings(zipFile: ZipFile): List<String> {
            val entry = zipFile.getEntry(SHARED_STRINGS_ENTRY) ?: return emptyList()
            val strings = mutableListOf<String>()
            zipFile.getInputStream(entry).use { input ->
                val parser = createParser(input)
                while (parser.next() != XmlPullParser.END_DOCUMENT) {
                    if (parser.eventType == XmlPullParser.START_TAG &&
                        parser.isSpreadsheetTag("si")
                    ) {
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
                            builder.append(readText(parser))
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
                    XmlPullParser.TEXT, XmlPullParser.CDSECT -> builder.append(parser.text)
                    XmlPullParser.END_TAG -> break
                }
            }
            return builder.toString()
        }
    }
}
