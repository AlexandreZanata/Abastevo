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
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Assertions.fail
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestFactory

/** The Kotlin consumer of session-operations-v1 (EMP-GPS-002, B-BR-EMP-497, EDR-322); the Go ParseSession consumes the same rows. */
class TrackingSessionOperationVectorsTest {
    private val hash = "e0ab1391496c811440e107e2f89fc5035798b566d8b52dc67c78b985c1181416"
    private val binding = SyncBinding.of("7b0e2a54-0000-4000-8000-000000009501", "https://api.example.invalid", "7b0e2a54-0000-4000-8000-000000009502")
    private class Row(val id: String, val operation: String, val args: Map<String, String>, val outcome: String, val bytes: ByteArray)

    private fun rows(): List<Row> {
        val file = File("../contracts/testdata/enterprise/tracking/session-operations-v1.psv")
        assertEquals(hash, MessageDigest.getInstance("SHA-256").digest(file.readBytes()).joinToString("") { "%02x".format(it.toInt() and 255) })
        val lines = file.readLines()
        assertEquals("id|operation|args|expected|bytes_hex", lines.first())
        return lines.drop(1).map { line ->
            val cols = line.split('|')
            require(cols.size == 5 && cols[0].isNotEmpty())
            val pairs = cols[2].split(';').map { val p = it.split('=', limit = 2); require(p.size == 2 && p[0].isNotEmpty()); p[0] to p[1] }
            require(pairs.map { it.first }.distinct().size == pairs.size)
            Row(cols[0], cols[1], pairs.toMap(), cols[3], cols[4].chunked(2).map { it.toInt(16).toByte() }.toByteArray())
        }
    }

    private val types = mapOf("START" to TrackingOperations.START, "RESUME" to TrackingOperations.RESUME, "SUSPEND" to TrackingOperations.SUSPEND,
        "INTERRUPT" to TrackingOperations.INTERRUPT, "REDUCE" to TrackingOperations.REDUCE, "END" to TrackingOperations.END)
    private fun dependencies(row: Row) = listOfNotNull(row.args["dep"], row.args["dep2"])
    private fun envelope(row: Row) = TrackingEnvelope(binding, "7b0e2a54-0000-4000-8000-000000009503", "7b0e2a54-0000-4000-8000-000000009504",
        row.args.getValue("op"), TimeDeclaration(EnterpriseTime.parseInstant(row.args["at"] ?: "2026-11-15T12:00:00.123Z"),
            row.args["zone"] ?: "America/Cuiaba", (row.args["offset"] ?: "-14400").toLong()))

    private fun build(row: Row): EnterpriseOperation = when (row.operation) {
        "START" -> TrackingOperations.start(envelope(row), row.args.getValue("session"), row.args.getValue("trip"), row.args.getValue("origin"),
            row.args.getValue("permission"), row.args.getValue("visible") == "true", dependencies(row))
        "RESUME" -> TrackingOperations.resume(envelope(row), row.args.getValue("session"), row.args.getValue("trip"), row.args.getValue("origin"),
            row.args.getValue("permission"), row.args.getValue("visible") == "true", dependencies(row))
        else -> TrackingOperations.event(types.getValue(row.operation), envelope(row), row.args.getValue("session"), row.args.getValue("trip"),
            row.args.getValue("cause"), dependencies(row))
    }

    private fun accepted(row: Row) {
        val operation = build(row)
        assertArrayEquals(row.bytes, operation.canonicalBytes, "builder bytes")
        val fact = TrackingOperations.read(EnterpriseOperation.parse(row.bytes, binding))
        assertEquals(listOf(types[row.operation], row.args["op"], row.args["session"], row.args["trip"]),
            listOf(fact.type, fact.operationId, fact.sessionId, fact.tripId))
        assertEquals(dependencies(row).sorted(), fact.dependsOn)
        assertEquals(EnterpriseTime.parseInstant(row.args["at"] ?: "2026-11-15T12:00:00.123Z"), fact.declared.occurredAt)
        when (fact) {
            is TrackingOperationFact.DriverAction -> assertEquals(listOf(row.args["origin"], row.args["permission"], row.args["visible"]),
                listOf(fact.origin.name, fact.permission.name, fact.notificationVisible.toString()))
            is TrackingOperationFact.Event -> assertEquals(row.args["cause"], fact.cause.name)
        }
    }

    private fun refused(row: Row) {
        assertThrows(TrackingOperationException::class.java, { build(row) }, "builder refusal")
        if (row.outcome == "PROTOCOL") {
            try { EnterpriseOperation.seal(row.bytes, binding); fail("the envelope profile accepted ${row.id}") } catch (_: SyncProtocolException) { }
            return
        }
        val sealed = EnterpriseOperation.seal(row.bytes, binding)
        assertThrows(TrackingOperationException::class.java, { TrackingOperations.read(sealed) }, "decoder refusal")
    }

    @Test fun `the corpus is complete and every operation and outcome is covered`() {
        val all = rows()
        assertEquals(41, all.size)
        assertEquals(all.size, all.map { it.id }.distinct().size)
        assertEquals(mapOf("START/OK" to 9, "START/MALFORMED" to 8, "START/PROTOCOL" to 1, "RESUME/OK" to 4, "RESUME/MALFORMED" to 1,
            "SUSPEND/OK" to 2, "SUSPEND/MALFORMED" to 1, "INTERRUPT/OK" to 7, "INTERRUPT/MALFORMED" to 2, "REDUCE/OK" to 1,
            "REDUCE/MALFORMED" to 1, "END/OK" to 2, "END/MALFORMED" to 1, "END/PROTOCOL" to 1),
            all.groupingBy { it.operation + "/" + it.outcome }.eachCount())
        assertEquals(TrackingOperations.SESSION_TYPES, types.values.toSet())
        assertEquals(types.values.toSet(), all.filter { it.outcome == "OK" }.map { types.getValue(it.operation) }.toSet())
        // Every session cause except NOT_STARTED parses in some OK row; NOT_STARTED and a fix-level gap reason are refused.
        val okCauses = all.filter { it.outcome == "OK" }.mapNotNull { it.args["cause"] }.toSet()
        assertTrue(okCauses.containsAll(setOf("TRIP_PAUSED", "LONG_STATIONARY", "PROCESS_STOPPED", "DEVICE_RESTARTED", "STOPPED_BY_DRIVER",
            "OWNER_CHANGED", "PERMISSION_REVOKED", "NOTIFICATION_BLOCKED", "PRECISION_REDUCED", "TRIP_COMPLETED", "TRIP_CANCELLED")))
        assertTrue(LocationPermission.entries.all { p -> all.any { it.outcome == "OK" && it.args["permission"] == p.name } })
    }

    @TestFactory fun `every row is built, decoded or refused exactly like the Go intake parser`(): List<DynamicTest> = rows().map { row ->
        DynamicTest.dynamicTest(row.id) { if (row.outcome == "OK") accepted(row) else refused(row) }
    }

    @Test fun `a non-session or extended operation is refused by the decoder`() {
        val row = rows().first { it.id == "start-precise-visible-after-trip-start" }
        val operation = EnterpriseOperation.parse(row.bytes, binding)
        // Three dependencies are more than the device ever writes.
        assertThrows(TrackingOperationException::class.java) {
            TrackingOperations.start(envelope(row), row.args.getValue("session"), row.args.getValue("trip"), "TRIP_SCREEN", "PRECISE", true,
                listOf("7b0e2a54-0000-4000-8000-000000009601", "7b0e2a54-0000-4000-8000-000000009602", "7b0e2a54-0000-4000-8000-000000009603"))
        }
        assertThrows(TrackingOperationException::class.java) {
            TrackingOperations.event(TrackingOperations.START, envelope(row), row.args.getValue("session"), row.args.getValue("trip"), "PROCESS_STOPPED", emptyList())
        }
        assertEquals(TrackingOperations.START, TrackingOperations.read(operation).type)
    }
}
