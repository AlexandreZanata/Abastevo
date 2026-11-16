package com.anpfuel.domain.enterprise.tracking

import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.SyncBinding
import com.anpfuel.domain.enterprise.sync.SyncProtocolException
import java.io.File
import java.security.MessageDigest
import org.junit.jupiter.api.Assertions.assertArrayEquals
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.fail
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestFactory

/** The Kotlin consumer of track-batch-v1 (EMP-GPS-003, B-BR-EMP-504, EDR-326); the Go ParseBatch consumes the same rows. */
class TrackingBatchVectorsTest {
    private val hash = "9cbdd508ff2843dda9df0b3eb64a8f0aa4c30f82b4e30d281922a0aa38ccf4f6"
    private val binding = SyncBinding.of("7b0e2a54-0000-4000-8000-000000009501", "https://api.example.invalid", "7b0e2a54-0000-4000-8000-000000009502")
    private class Row(val id: String, val args: Map<String, String>, val outcome: String, val bytes: ByteArray)

    private fun rows(): List<Row> {
        val file = File("../contracts/testdata/enterprise/tracking/track-batch-v1.psv")
        assertEquals(hash, MessageDigest.getInstance("SHA-256").digest(file.readBytes()).joinToString("") { "%02x".format(it.toInt() and 255) })
        val lines = file.readLines()
        assertEquals("id|operation|args|expected|bytes_hex", lines.first())
        return lines.drop(1).map { line ->
            val cols = line.split('|')
            require(cols.size == 5 && cols[0].isNotEmpty() && cols[1] == "BATCH")
            val pairs = cols[2].split(';').map { val p = it.split('=', limit = 2); require(p.size == 2 && p[0].isNotEmpty()); p[0] to p[1] }
            require(pairs.map { it.first }.distinct().size == pairs.size)
            Row(cols[0], pairs.toMap(), cols[3], cols[4].chunked(2).map { it.toInt(16).toByte() }.toByteArray())
        }
    }
    private fun fixes(row: Row): List<TrackingBatchFix> = row.args.getValue("fixes").split(',').map { item ->
        val p = item.split(':'); require(p.size == 7)
        TrackingBatchFix(p[0].toLong(), p[1].toLong().toInt(), p[2].toLong().toInt(), p[3].toInt(), p[4].takeIf { it != "n" }?.toInt(),
            p[5].takeIf { it != "n" }?.toInt(), p[6].toInt())
    }
    private fun dropped(row: Row) = (row.args["dropped"] ?: "0:0:0:0").split(':').map { it.toLong() }.let { TrackingDropped(it[0], it[1], it[2], it[3]) }
    private fun envelope(row: Row) = TrackingEnvelope(binding, "7b0e2a54-0000-4000-8000-000000009503", "7b0e2a54-0000-4000-8000-000000009504",
        row.args.getValue("op"), TimeDeclaration(EnterpriseTime.parseInstant(row.args["at"] ?: "2026-11-15T12:00:00.123Z"), "America/Cuiaba", -14_400))
    private fun build(row: Row): EnterpriseOperation = TrackingBatchOperations.batch(envelope(row), row.args.getValue("session"), row.args.getValue("trip"),
        row.args.getValue("seq").toLong(), row.args["boot"], row.args["elapsed"]?.toLong(), fixes(row), dropped(row), listOfNotNull(row.args["dep"]))

    @Test fun `the corpus is complete`() {
        val all = rows()
        assertEquals(24, all.size)
        assertEquals(all.size, all.map { it.id }.distinct().size)
        assertEquals(mapOf("OK" to 11, "MALFORMED" to 12, "PROTOCOL" to 1), all.groupingBy { it.outcome }.eachCount())
        assertEquals(200, fixes(all.single { it.id == "batch-200-fixes" }).size)
    }

    @TestFactory fun `every row is built, decoded or refused exactly like the Go intake parser`(): List<DynamicTest> = rows().map { row ->
        DynamicTest.dynamicTest(row.id) {
            when (row.outcome) {
                "OK" -> {
                    assertArrayEquals(row.bytes, build(row).canonicalBytes, "builder bytes")
                    val fact = TrackingBatchOperations.read(EnterpriseOperation.parse(row.bytes, binding))
                    assertEquals(fixes(row), fact.fixes)
                    assertEquals(listOf(row.args["op"], row.args["session"], row.args["trip"], row.args["seq"], row.args["boot"], row.args["elapsed"]),
                        listOf(fact.operationId, fact.sessionId, fact.tripId, fact.sequence.toString(), fact.bootId, fact.baseElapsedMillis?.toString()))
                    assertEquals(dropped(row), fact.dropped)
                    assertEquals(listOfNotNull(row.args["dep"]), fact.dependsOn)
                }
                "MALFORMED" -> {
                    assertThrows(TrackingOperationException::class.java, { build(row) }, "builder refusal")
                    assertThrows(TrackingOperationException::class.java, { TrackingBatchOperations.read(EnterpriseOperation.seal(row.bytes, binding)) }, "decoder refusal")
                }
                "PROTOCOL" -> {
                    assertThrows(TrackingOperationException::class.java, { build(row) }, "builder refusal")
                    try { EnterpriseOperation.seal(row.bytes, binding); fail("the envelope profile accepted ${row.id}") } catch (_: SyncProtocolException) { }
                }
                else -> fail("unknown outcome")
            }
        }
    }
}
