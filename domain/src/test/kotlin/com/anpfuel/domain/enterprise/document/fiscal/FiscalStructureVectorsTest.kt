package com.anpfuel.domain.enterprise.document.fiscal

import com.anpfuel.domain.enterprise.document.DocumentDimension
import com.anpfuel.domain.enterprise.document.DocumentState
import com.anpfuel.domain.enterprise.document.DocumentTransition
import com.anpfuel.domain.enterprise.document.EnterpriseDocument
import java.io.ByteArrayOutputStream
import java.io.File
import java.security.MessageDigest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestFactory

/** Same shared oracle as the Go consumer; XML rows are Go-only (EDR-159). Expectations never come from the implementation. */
class FiscalStructureVectorsTest {
    private val corpusHash = "ad8cd1e15a4359b3e622741cd23030420612f506a0ff2238c40ce20502d84f08"
    private val goOnlyRows = 118
    private val maxRepeat = 2_000_000
    private val operations = mapOf("KEY" to setOf("text", "unit", "repeat", "tail"), "QR" to setOf("text", "unit", "repeat", "tail"),
        "XML" to setOf("text", "unit", "repeat", "tail"), "COMPARE" to setOf("left", "right"), "FEED" to setOf("input"))

    private fun fields(raw: String): Map<String, String> {
        val pairs = raw.split(';').map { val p = it.split('=', limit = 2); require(p.size == 2 && p[0].isNotBlank()); p[0] to p[1] }
        require(pairs.map { it.first }.distinct().size == pairs.size)
        return pairs.toMap()
    }

    private fun hex(b: Byte): Int = when (val c = b.toInt().toChar()) {
        in '0'..'9' -> c - '0'
        in 'A'..'F' -> c - 'A' + 10
        else -> throw IllegalArgumentException("malformed fixture escape")
    }

    /** Undoes the oracle's own %XX layer (uppercase hex only), over UTF-8 bytes like the Go consumer. */
    private fun decode(raw: String): String {
        val bytes = raw.toByteArray(Charsets.UTF_8)
        val out = ByteArrayOutputStream()
        var i = 0
        while (i < bytes.size) {
            if (bytes[i] != '%'.code.toByte()) { out.write(bytes[i].toInt()); i++; continue }
            require(i + 2 < bytes.size) { "malformed fixture escape" }
            out.write(hex(bytes[i + 1]) * 16 + hex(bytes[i + 2]))
            i += 3
        }
        return String(out.toByteArray(), Charsets.UTF_8)
    }

    private fun input(a: Map<String, String>): String {
        val text = decode(requireNotNull(a["text"]))
        val repeat = a["repeat"] ?: run { require("unit" !in a && "tail" !in a); return text }
        val n = repeat.toIntOrNull()
        require(n != null && n in 1..maxRepeat && n.toString() == repeat)
        val unit = decode(requireNotNull(a["unit"]))
        require(unit.isNotEmpty())
        return text + unit.repeat(n) + decode(a["tail"] ?: "")
    }

    private fun operand(raw: String): Pair<String, String> {
        val decoded = decode(raw)
        val cut = decoded.indexOf(':')
        require(cut > 0) { "malformed fixture operand" }
        val kind = decoded.substring(0, cut)
        require(kind == "KEY" || kind == "QR") { "unknown or Go-only fixture operand" }
        return kind to decoded.substring(cut + 1)
    }

    private fun check(kind: String, text: String) =
        if (kind == "KEY") FiscalStructure.checkAccessKey(text) else FiscalStructure.checkQrCode(text)

    private fun goOnly(operation: String, rawArgs: String) = operation == "XML" || rawArgs.contains("=XML:")

    @Test fun `malformed fixtures are rejected before domain dispatch`() {
        for (raw in listOf("%", "%7", "%7c", "%G0", "abc%")) assertThrows(IllegalArgumentException::class.java) { decode(raw) }
        for (args in listOf(mapOf("unit" to "a"), mapOf("text" to "", "repeat" to "2"), mapOf("text" to "", "unit" to "a", "repeat" to "0"),
            mapOf("text" to "", "unit" to "a", "repeat" to "02"), mapOf("text" to "", "unit" to "", "repeat" to "2"),
            mapOf("text" to "", "tail" to "x"), mapOf("text" to "", "unit" to "a", "repeat" to "2000001"))) {
            assertThrows(IllegalArgumentException::class.java) { input(args) }
        }
        for (raw in listOf("KEY", "PDF:x", "key:x", ":x", "XML:<a/>")) assertThrows(IllegalArgumentException::class.java) { operand(raw) }
        assertThrows(IllegalArgumentException::class.java) { fields("a=1;a=2") }
        assertThrows(IllegalArgumentException::class.java) { fields("novalue") }
    }

    private fun project(r: FiscalStructureResult): Map<String, String> {
        val out = linkedMapOf("kind" to r.kind.name, "outcome" to r.outcome.name, "reason" to (r.reason?.name ?: "-"),
            "checks" to r.checks.joinToString(",") { it.name })
        if (r.outcome != FiscalOutcome.STRUCTURE_OK) {
            if (r.accessKey != null || r.qrCode != null) out["leak"] = "true"
            return out
        }
        val k = r.accessKey ?: return out.also { it["shape"] = "unexpected" }
        if ((r.qrCode != null) != (r.kind == FiscalInputKind.QR_CODE)) out["shape"] = "unexpected"
        out["key"] = k.text
        when (r.kind) {
            FiscalInputKind.ACCESS_KEY -> {
                out["uf"] = k.state; out["year_month"] = k.yearMonth; out["issuer"] = k.issuer; out["model"] = k.model
                out["series"] = k.series; out["number"] = k.number; out["emission"] = k.emissionType
                out["code"] = k.numericCode; out["digit"] = k.checkDigit
            }
            FiscalInputKind.QR_CODE -> {
                val q = requireNotNull(r.qrCode)
                if (q.accessKey.text != k.text) out["shape"] = "key"
                out["version"] = q.version; out["environment"] = q.environment.name; out["layout"] = q.layout.name
                out["total"] = q.total?.text ?: "?"; out["centavos"] = q.total?.centavos?.toString() ?: "?"
            }
            FiscalInputKind.COMPARISON -> Unit
        }
        return out
    }

    private fun feed(r: FiscalStructureResult): Map<String, String> {
        val start = EnterpriseDocument.create("9d000000-0000-4000-8000-000000000001", "9d000000-0000-4000-8000-000000000201", "1a".repeat(32))
            .apply(DocumentTransition(DocumentDimension.STRUCTURE, DocumentState.NOT_APPLICABLE, DocumentState.UNCHECKED))
        val command = r.structureTransition()
        val after = command?.let { start.apply(it) } ?: start
        return mapOf("outcome" to r.outcome.name, "transition" to (command?.to?.token ?: "-"),
            "structure" to after.statuses.structure.token, "fiscal" to after.statuses.fiscal.token,
            "consulted_at" to (if (after.consultedAt == null) "?" else "present"))
    }

    private fun execute(operation: String, a: Map<String, String>): Map<String, String> = when (operation) {
        "KEY" -> project(FiscalStructure.checkAccessKey(input(a)))
        "QR" -> project(FiscalStructure.checkQrCode(input(a)))
        "COMPARE" -> {
            val (lk, lt) = operand(a.getValue("left"))
            val (rk, rt) = operand(a.getValue("right"))
            project(FiscalStructure.compare(check(lk, lt), check(rk, rt)))
        }
        "FEED" -> { val (kind, text) = operand(a.getValue("input")); feed(check(kind, text)) }
        else -> error("Unknown or Go-only fixture operation")
    }

    @TestFactory fun allSharedFiscalVectors(): List<DynamicTest> {
        val file = File("../contracts/testdata/enterprise/fiscal/fiscal-v1.psv")
        assertEquals(corpusHash, MessageDigest.getInstance("SHA-256").digest(file.readBytes()).joinToString("") { "%02x".format(it.toInt() and 255) })
        val lines = file.readLines(Charsets.UTF_8)
        assertEquals("id|operation|args|expected", lines.first())
        val rows = lines.drop(1).map { it.split('|') }
        require(rows.all { it.size == 4 && it[0].isNotBlank() })
        assertEquals(rows.size, rows.map { it[0] }.distinct().size)
        assertEquals(mapOf("KEY" to 64, "QR" to 110, "XML" to 111, "COMPARE" to 13, "FEED" to 9), rows.groupingBy { it[1] }.eachCount())
        for (row in rows) require(fields(row[2]).keys.all { it in requireNotNull(operations[row[1]]) })
        val mirrored = rows.filterNot { goOnly(it[1], it[2]) }
        assertEquals(goOnlyRows, rows.size - mirrored.size)
        val expectations = mirrored.map { fields(it[3]) }
        assertEquals(FiscalReason.entries.map { it.name }.toSet(), expectations.mapNotNull { it["reason"] }.filter { it != "-" }.toSet())
        assertEquals(FiscalCheck.entries.map { it.name }.toSet(), expectations.flatMap { it["checks"]?.split(',') ?: emptyList() }.toSet())
        assertEquals(FiscalOutcome.entries.map { it.name }.toSet(), expectations.mapNotNull { it["outcome"] }.toSet())
        assertEquals(setOf("NOT_COMPARABLE"), expectations.mapNotNull { it["error"] }.toSet())
        for (e in expectations) e["reason"]?.takeIf { it != "-" }?.let { assertEquals(FiscalReason.valueOf(it).outcome.name, e["outcome"]) }
        return mirrored.map { (id, operation, rawArgs, expected) -> DynamicTest.dynamicTest(id) {
            val actual = try { execute(operation, fields(rawArgs)) } catch (_: FiscalComparisonException) { mapOf("error" to "NOT_COMPARABLE") }
            assertEquals(fields(expected), actual)
        } }
    }
}
