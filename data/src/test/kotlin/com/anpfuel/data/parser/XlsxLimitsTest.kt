package com.anpfuel.data.parser

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.io.ByteArrayOutputStream
import java.io.File
import java.util.zip.ZipEntry
import java.util.zip.ZipOutputStream

/**
 * SEC06-B adversarial ceilings (AUD-10): crafted columnar inputs fail
 * fast and small — never by catching OutOfMemoryError, never with a
 * live allocation. The 748-byte `ZZZZZZ1` audit repro inverts here.
 */
class XlsxLimitsTest {

    private val relNs = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"

    private fun workbookXml(sheets: List<String>): String {
        val tags = sheets.mapIndexed { i, name ->
            "<sheet name=\"$name\" sheetId=\"${i + 1}\" r:id=\"rId${i + 1}\"/>"
        }.joinToString("")
        return "<workbook xmlns:r=\"$relNs\"><sheets>$tags</sheets></workbook>"
    }

    private fun relsXml(count: Int): String {
        val tags = (1..count).joinToString("") { "<Relationship Id=\"rId$it\" Target=\"worksheets/sheet$it.xml\"/>" }
        return "<Relationships>$tags</Relationships>"
    }

    private fun sheetXml(cells: String, row: Int = 1): String =
        "<worksheet><sheetData><row r=\"$row\">$cells</row></sheetData></worksheet>"

    private fun craft(dir: File, name: String, entries: Map<String, ByteArray>): File {
        val file = File(dir, name)
        ZipOutputStream(file.outputStream()).use { zip ->
            for ((entryName, bytes) in entries) {
                zip.putNextEntry(ZipEntry(entryName))
                zip.write(bytes)
                zip.closeEntry()
            }
        }
        return file
    }

    private fun minimalSheet(
        dir: File,
        name: String,
        cells: String,
        sheetName: String = "S1",
    ): File = craft(
        dir, name, mapOf(
            "xl/workbook.xml" to workbookXml(listOf(sheetName)).toByteArray(),
            "xl/_rels/workbook.xml.rels" to relsXml(1).toByteArray(),
            "xl/worksheets/sheet1.xml" to sheetXml(cells).toByteArray(),
        ),
    )

    private fun parseCells(file: File): List<String?> {
        val out = mutableListOf<String?>()
        StreamingXlsxParser.open(file).use { parser ->
            parser.streamSheet("S1", SheetParseOptions(firstDataRowNumber = 1, headerRowNumber = 1)) { row ->
                out.addAll(row.cells)
            }
        }
        return out
    }

    @Test
    fun columnBombRefusesBeforeAllocation(@TempDir dir: File) {
        val file = minimalSheet(dir, "bomb.xlsx", "<c r=\"ZZZZZZ1\"><v>1</v></c>")
        // Audit repro shape: sub-KiB file, would-be 300M-entry row.
        assertTrue(file.length() < 2048, "fixture must stay tiny, was ${file.length()}")
        assertThrows(IllegalArgumentException::class.java) { parseCells(file) }
    }

    @Test
    fun gridEdgesRefuseBeyondXfd(@TempDir dir: File) {
        for (ref in listOf("XFE1", "AAAA1", "1", "")) {
            val file = minimalSheet(dir, "edge-$ref.xlsx", "<c r=\"$ref\"><v>1</v></c>")
            assertThrows(IllegalArgumentException::class.java, { parseCells(file) }, "ref $ref")
        }
        // Boundary rows refuse outside 1..1048576.
        for (row in listOf(0, 1048577)) {
            val file = craft(
                dir, "row-$row.xlsx", mapOf(
                    "xl/workbook.xml" to workbookXml(listOf("S1")).toByteArray(),
                    "xl/_rels/workbook.xml.rels" to relsXml(1).toByteArray(),
                    "xl/worksheets/sheet1.xml" to sheetXml("<c r=\"A1\"><v>1</v></c>", row).toByteArray(),
                ),
            )
            assertThrows(IllegalArgumentException::class.java, { parseCells(file) }, "row $row")
        }
    }

    @Test
    fun widestValidColumnParses(@TempDir dir: File) {
        assertEquals(16383, XlsxCellReference.columnIndex("XFD1048576"))
        assertEquals(1048576, XlsxCellReference.rowNumber("XFD1048576"))
        val file = minimalSheet(dir, "xfd.xlsx", "<c r=\"XFD1\"><v>7</v></c>")
        val cells = parseCells(file)
        assertEquals(16384, cells.size)
        assertEquals("7", cells[16383])
    }

    @Test
    fun sharedStringsCountBombRefuses(@TempDir dir: File) {
        val out = ByteArrayOutputStream()
        ZipOutputStream(out).use { zip ->
            zip.putNextEntry(ZipEntry("xl/workbook.xml"))
            zip.write(workbookXml(listOf("S1")).toByteArray())
            zip.closeEntry()
            zip.putNextEntry(ZipEntry("xl/_rels/workbook.xml.rels"))
            zip.write(relsXml(1).toByteArray())
            zip.closeEntry()
            zip.putNextEntry(ZipEntry("xl/sharedStrings.xml"))
            zip.write("<sst>".toByteArray())
            repeat(XlsxLimits.MAX_SHARED_STRINGS + 1) {
                zip.write("<si><t>x</t></si>".toByteArray())
            }
            zip.write("</sst>".toByteArray())
            zip.closeEntry()
            zip.putNextEntry(ZipEntry("xl/worksheets/sheet1.xml"))
            zip.write(sheetXml("<c r=\"A1\" t=\"s\"><v>0</v></c>").toByteArray())
            zip.closeEntry()
        }
        val file = File(dir, "strings-bomb.xlsx")
        file.writeBytes(out.toByteArray())
        assertThrows(IllegalArgumentException::class.java) { parseCells(file) }
    }

    @Test
    fun sharedStringTextBombRefuses(@TempDir dir: File) {
        val big = "y".repeat(XlsxLimits.MAX_TEXT_CHARS + 1)
        val file = craft(
            dir, "text-bomb.xlsx", mapOf(
                "xl/workbook.xml" to workbookXml(listOf("S1")).toByteArray(),
                "xl/_rels/workbook.xml.rels" to relsXml(1).toByteArray(),
                "xl/sharedStrings.xml" to "<sst><si><t>$big</t></si></sst>".toByteArray(),
                "xl/worksheets/sheet1.xml" to sheetXml("<c r=\"A1\" t=\"s\"><v>0</v></c>").toByteArray(),
            ),
        )
        assertThrows(IllegalArgumentException::class.java) { parseCells(file) }
    }

    @Test
    fun worksheetCatalogBombRefuses(@TempDir dir: File) {
        val names = (1..XlsxLimits.MAX_WORKSHEETS + 1).map { "S$it" }
        val file = craft(
            dir, "sheets-bomb.xlsx", mapOf(
                "xl/workbook.xml" to workbookXml(names).toByteArray(),
                "xl/_rels/workbook.xml.rels" to relsXml(names.size).toByteArray(),
                "xl/worksheets/sheet1.xml" to sheetXml("<c r=\"A1\"><v>1</v></c>").toByteArray(),
            ),
        )
        assertThrows(IllegalArgumentException::class.java) { parseCells(file) }
    }

    @Test
    fun corruptInitClosesTheHandle(@TempDir dir: File) {
        // Over-long shared text fails mid-load, after the ZIP opened:
        // the handle must close (file deletes) and callers still see
        // the original refusal.
        val big = "z".repeat(XlsxLimits.MAX_TEXT_CHARS + 1)
        val midFail = craft(
            dir, "midfail.xlsx", mapOf(
                "xl/workbook.xml" to workbookXml(listOf("S1")).toByteArray(),
                "xl/_rels/workbook.xml.rels" to relsXml(1).toByteArray(),
                "xl/sharedStrings.xml" to "<sst><si><t>$big</t></si></sst>".toByteArray(),
                "xl/worksheets/sheet1.xml" to sheetXml("<c r=\"A1\" t=\"s\"><v>0</v></c>").toByteArray(),
            ),
        )
        assertThrows(IllegalArgumentException::class.java) { StreamingXlsxParser.open(midFail) }
        assertTrue(midFail.delete())
        // A non-ZIP refuses at open with nothing to leak.
        val garbage = File(dir, "garbage.xlsx").also { it.writeBytes(byteArrayOf(1, 2, 3, 4)) }
        assertThrows(Exception::class.java) { StreamingXlsxParser.open(garbage) }
        assertTrue(garbage.delete())
    }

    @Test
    fun missingSheetEntryRefuses(@TempDir dir: File) {
        val file = craft(
            dir, "ghost.xlsx", mapOf(
                "xl/workbook.xml" to workbookXml(listOf("Ghost")).toByteArray(),
                "xl/_rels/workbook.xml.rels" to relsXml(1).toByteArray(),
            ),
        )
        assertThrows(Exception::class.java) {
            StreamingXlsxParser.open(file).use { parser ->
                parser.streamSheet("Ghost", SheetParseOptions(firstDataRowNumber = 1, headerRowNumber = 1)) {}
            }
        }
    }
}
