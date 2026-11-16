package com.anpfuel.domain.enterprise.expense

import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.SyncBinding
import com.anpfuel.domain.enterprise.sync.SyncProtocolException
import java.io.File
import java.security.MessageDigest
import org.junit.jupiter.api.Assertions.assertArrayEquals
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Assertions.fail
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestFactory

/** The Kotlin consumer of expense-submit-v1 (EMP-EXP-002, B-BR-EMP-447, EDR-289); the Go intake parser consumes the same rows. */
class EnterpriseExpenseOperationVectorsTest {
    private val hash = "5c69de3c71bdeb9495ddef9ae9a39ae6597f0e791cdf1500abebdffc100de6e6"
    private val binding = SyncBinding.of("9e5b0000-0000-4000-8000-000000009501", "https://api.example.invalid", "9e5b0000-0000-4000-8000-000000009502")
    private val keys = setOf("op", "expense", "asset", "category", "paid", "company", "payer", "trip", "at", "zone", "offset")
    private class Row(val id: String, val args: Map<String, String>, val outcome: String, val bytes: ByteArray)

    private fun hex(raw: String): ByteArray = ByteArray(raw.length / 2) { raw.substring(it * 2, it * 2 + 2).toInt(16).toByte() }

    private fun rows(): List<Row> {
        val file = File("../contracts/testdata/enterprise/expense/expense-submit-v1.psv")
        assertEquals(hash, MessageDigest.getInstance("SHA-256").digest(file.readBytes()).joinToString("") { "%02x".format(it.toInt() and 255) })
        val lines = file.readLines(Charsets.UTF_8)
        assertEquals("id|operation|args|expected|bytes_hex", lines.first())
        return lines.drop(1).map { line ->
            val cols = line.split('|')
            require(cols.size == 5 && cols[0].isNotEmpty() && cols[1] == "SUBMIT")
            val pairs = cols[2].split(';').map { val p = it.split('=', limit = 2); require(p.size == 2 && p[0] in keys); p[0] to p[1] }
            require(pairs.map { it.first }.distinct().size == pairs.size)
            Row(cols[0], pairs.toMap(), cols[3], hex(cols[4]))
        }
    }

    private fun envelope(row: Row) = ExpenseEnvelope(binding, "9e5b0000-0000-4000-8000-000000009503", "9e5b0000-0000-4000-8000-000000009504",
        row.args.getValue("op"), TimeDeclaration(EnterpriseTime.parseInstant(row.args["at"] ?: "2026-11-15T12:00:00.123Z"),
            row.args["zone"] ?: "America/Cuiaba", (row.args["offset"] ?: "-14400").toLong()))

    private fun build(row: Row): EnterpriseOperation = ExpenseOperations.submit(envelope(row), row.args.getValue("expense"), row.args.getValue("asset"),
        row.args.getValue("category"), row.args.getValue("paid"), row.args.getValue("company"), row.args.getValue("payer"), row.args["trip"])

    private fun accepted(row: Row) {
        val operation = build(row)
        assertArrayEquals(row.bytes, operation.canonicalBytes, "builder bytes")
        val fact = ExpenseOperations.read(EnterpriseOperation.parse(row.bytes, binding))
        assertEquals(listOf(row.args["op"], row.args["expense"], row.args["asset"], row.args["category"], row.args["paid"], row.args["company"], row.args["payer"]),
            listOf(fact.operationId, fact.expenseId, fact.assetId, fact.category, fact.paidCentavos.toString(), fact.companyCentavos.toString(), fact.payer))
        assertEquals(EnterpriseTime.parseInstant(row.args["at"] ?: "2026-11-15T12:00:00.123Z"), fact.declared.occurredAt)
        assertFalse(fact.toString().contains(fact.assetId))
    }

    private fun refused(row: Row) {
        val built = try { build(row); null } catch (refusal: ExpenseOperationException) { refusal.failure }
        if (row.outcome == "PROTOCOL") {
            assertTrue(built != null, "the builder accepted an envelope-profile refusal")
            try { EnterpriseOperation.seal(row.bytes, binding); fail("the envelope profile accepted ${row.id}") } catch (_: SyncProtocolException) { }
            return
        }
        val expected = ExpenseOperationFailure.valueOf(row.outcome)
        assertEquals(expected, built, "builder refusal class")
        val sealed = EnterpriseOperation.seal(row.bytes, binding)
        val decoded = try { ExpenseOperations.read(sealed); null } catch (refusal: ExpenseOperationException) { refusal.failure }
        assertEquals(expected, decoded, "decoder refusal class")
    }

    @Test fun `the corpus is complete and every category and outcome is covered`() {
        val all = rows()
        assertEquals(40, all.size)
        assertEquals(all.size, all.map { it.id }.distinct().size)
        assertEquals(mapOf("OK" to 7, "MALFORMED" to 28, "UNSUPPORTED" to 4, "PROTOCOL" to 1), all.groupingBy { it.outcome }.eachCount())
        assertEquals(ExpenseOperations.DIRECT_CATEGORIES.toSet(), all.filter { it.outcome == "OK" }.map { it.args.getValue("category") }.toSet())
        assertEquals(listOf("FINE", "INSURANCE", "LEASE", "MAINTENANCE", "OTHER", "PARKING", "TOLL"), ExpenseOperations.DIRECT_CATEGORIES)
    }

    @TestFactory fun vectors(): List<DynamicTest> = rows().map { row ->
        DynamicTest.dynamicTest(row.id) { if (row.outcome == "OK") accepted(row) else refused(row) }
    }
}
