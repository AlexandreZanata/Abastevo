package com.anpfuel.domain.enterprise.fueling

import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.SyncBinding
import java.io.File
import java.security.MessageDigest
import org.junit.jupiter.api.Assertions.assertArrayEquals
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestFactory

/** The Kotlin consumer of fueling-submit-v1 (EMP-FUEL-011, BUC-EMP-233, EDR-294); the Go intake parser consumes the same rows. */
class EnterpriseFuelingSubmitVectorsTest {
    private val hash = "67068cfba629ff0fe6ec6299fb2c892bcc2c6e001424ebb0a012d6a1ecf5f0b8"
    private val binding = SyncBinding.of("7b0e2a54-0000-4000-8000-000000009501", "https://api.example.invalid", "7b0e2a54-0000-4000-8000-000000009502")
    private class Row(val id: String, val args: Map<String, String>, val outcome: String, val bytes: ByteArray)

    private fun hex(raw: String): String = String(ByteArray(raw.length / 2) { raw.substring(it * 2, it * 2 + 2).toInt(16).toByte() }, Charsets.UTF_8)

    private fun rows(): List<Row> {
        val file = File("../contracts/testdata/enterprise/fueling/fueling-submit-v1.psv")
        assertEquals(hash, MessageDigest.getInstance("SHA-256").digest(file.readBytes()).joinToString("") { "%02x".format(it.toInt() and 255) })
        val lines = file.readLines()
        assertEquals("id|operation|args|expected|bytes_hex", lines.first())
        return lines.drop(1).map { line ->
            val cols = line.split('|')
            require(cols.size == 5 && cols[0].isNotEmpty() && cols[1] == "SUBMIT")
            val pairs = cols[2].split(';').map { val p = it.split('=', limit = 2); require(p.size == 2 && p[0].isNotEmpty()); p[0] to p[1] }
            require(pairs.map { it.first }.distinct().size == pairs.size)
            Row(cols[0], pairs.toMap(), cols[3], hex(cols[4]).toByteArray(Charsets.UTF_8))
        }
    }

    private fun text(row: Row, key: String): String? = row.args[key]?.let { hex(it) }
    private fun input(row: Row): FuelingSubmitInput {
        val a = row.args
        val reading = a["meter"]?.let { FuelingReadingInput(it, a.getValue("segment"), a.getValue("reading"), a.getValue("kind"), a.getValue("value")) }
        return FuelingSubmitInput(a.getValue("asset"), a.getValue("responsible"), a.getValue("product"), a.getValue("unit"), a.getValue("quantity"),
            a.getValue("paid"), a["discount"], a["price"], text(row, "price_text"), a.getValue("fill"), a.getValue("payer"), a.getValue("payment"),
            reading, text(row, "reason"), a["place_kind"], text(row, "place_name"), a["trip"], a["supplier"])
    }
    private fun envelope(row: Row) = FuelingEnvelope(binding, "7b0e2a54-0000-4000-8000-000000009503", "7b0e2a54-0000-4000-8000-000000009504",
        row.args.getValue("op"), TimeDeclaration(EnterpriseTime.parseInstant(row.args["at"] ?: "2026-11-15T12:00:00.123Z"),
            row.args["zone"] ?: "America/Cuiaba", (row.args["offset"] ?: "-14400").toLong()))

    private fun accepted(row: Row) {
        val operation = FuelingOperations.submit(envelope(row), row.args.getValue("fueling"), input(row))
        assertArrayEquals(row.bytes, operation.canonicalBytes, "builder bytes")
        val fact = FuelingOperations.read(EnterpriseOperation.parse(row.bytes, binding))
        val a = row.args
        assertEquals(listOf(a["op"], a["fueling"], a["asset"], a["responsible"], a["product"], a["unit"], a["quantity"], a["paid"], a["discount"],
            a["price"], text(row, "price_text"), a["fill"], a["payer"], a["payment"], text(row, "place_name")),
            listOf(fact.operationId, fact.fuelingId, fact.assetId, fact.responsibleEmployeeId, fact.quantity.product.name, fact.quantity.unit.name,
                fact.quantity.amount.toString(), fact.amounts.paid.toString(), fact.amounts.discount?.toString(), fact.amounts.unitPrice?.toString(),
                fact.amounts.sourcePriceText, fact.fill.name, fact.payer.name, fact.payment.name, fact.placeName))
        when (val evidence = fact.evidence) {
            is FuelingEvidence.Absent -> assertEquals(text(row, "reason"), evidence.reason)
            is FuelingEvidence.Reading -> assertEquals(listOf(a["meter"], a["segment"], a["reading"], a["kind"], a["value"]),
                listOf(evidence.meterId, evidence.segmentId, evidence.readingId, evidence.value.kind.name, evidence.value.units.toString()))
        }
    }

    private fun refused(row: Row) {
        val expected = FuelingOperationFailure.valueOf(row.outcome)
        val built = try { FuelingOperations.submit(envelope(row), row.args.getValue("fueling"), input(row)); null }
            catch (refusal: FuelingOperationException) { refusal.failure }
        assertEquals(expected, built, "builder refusal class")
        val sealed = EnterpriseOperation.seal(row.bytes, binding)
        val decoded = try { FuelingOperations.read(sealed); null } catch (refusal: FuelingOperationException) { refusal.failure }
        assertEquals(expected, decoded, "decoder refusal class")
    }

    @Test fun `the corpus is complete and every product, method and outcome is covered`() {
        val all = rows()
        assertEquals(59, all.size)
        assertEquals(all.size, all.map { it.id }.distinct().size)
        assertEquals(mapOf("OK" to 15, "MALFORMED" to 39, "UNSUPPORTED" to 5), all.groupingBy { it.outcome }.eachCount())
        val ok = all.filter { it.outcome == "OK" }
        assertEquals(FuelProduct.entries.map { it.name }.toSet(), ok.map { it.args.getValue("product") }.toSet())
        assertEquals(PaymentMethod.entries.map { it.name }.toSet(), ok.map { it.args.getValue("payment") }.toSet())
        assertEquals(FillLevel.entries.map { it.name }.toSet(), ok.map { it.args.getValue("fill") }.toSet())
        assertEquals(Payer.entries.map { it.name }.toSet(), ok.map { it.args.getValue("payer") }.toSet())
        assertTrue(ok.any { "reason" in it.args } && ok.any { "meter" in it.args } && ok.any { "place_kind" in it.args })
    }

    @TestFactory fun vectors(): List<DynamicTest> = rows().map { row ->
        DynamicTest.dynamicTest(row.id) { if (row.outcome == "OK") accepted(row) else refused(row) }
    }
}
