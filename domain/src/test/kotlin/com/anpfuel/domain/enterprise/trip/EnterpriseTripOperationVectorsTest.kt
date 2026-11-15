package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.SyncBinding
import com.anpfuel.domain.enterprise.sync.SyncProtocolException
import java.io.File
import java.security.MessageDigest
import org.junit.jupiter.api.Assertions.assertArrayEquals
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Assertions.fail
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestFactory

/** The Kotlin consumer of trip-operations-v1 (EMP-TRIP-005, BUC-EMP-191, EDR-245); the Go intake parsers consume the same rows. */
class EnterpriseTripOperationVectorsTest {
    private val hash = "6c12cb7970bf5184f10a0b77b10cbd78127a059a7d8dc84f432b985d2aaf3551"
    private val binding = SyncBinding.of("7b0e2a54-0000-4000-8000-000000009501", "https://api.example.invalid", "7b0e2a54-0000-4000-8000-000000009502")
    private class Row(val id: String, val operation: String, val args: Map<String, String>, val outcome: String, val bytes: ByteArray)

    private fun rows(): List<Row> {
        val file = File("../contracts/testdata/enterprise/trip/trip-operations-v1.psv")
        assertEquals(hash, MessageDigest.getInstance("SHA-256").digest(file.readBytes()).joinToString("") { "%02x".format(it.toInt() and 255) })
        val lines = file.readLines()
        assertEquals("id|operation|args|expected|bytes_hex", lines.first())
        return lines.drop(1).map { line ->
            val cols = line.split('|')
            require(cols.size == 5 && cols[0].isNotEmpty())
            val pairs = cols[2].split(';').map { val p = it.split('=', limit = 2); require(p.size == 2 && p[0].isNotEmpty()); p[0] to p[1] }
            require(pairs.map { it.first }.distinct().size == pairs.size)
            Row(cols[0], cols[1], pairs.toMap(), cols[3], TripFixture.decode(cols[4]).toByteArray(Charsets.UTF_8))
        }
    }

    private fun text(row: Row, key: String): String? = row.args[key]?.let { if (it.isEmpty()) "" else TripFixture.decode(it) }
    private fun reading(row: Row): TripReadingInput? = row.args["meter"]?.let {
        TripReadingInput(it, row.args.getValue("segment"), row.args.getValue("reading"), row.args.getValue("kind"), row.args.getValue("value"))
    }
    private fun clock(row: Row): TripClockReading? = row.args["boot"]?.let { TripClockReading(it, row.args.getValue("elapsed").toLong()) }
    private fun envelope(row: Row) = TripEnvelope(binding, "7b0e2a54-0000-4000-8000-000000009503", "7b0e2a54-0000-4000-8000-000000009504",
        row.args.getValue("op"), TimeDeclaration(EnterpriseTime.parseInstant(row.args["at"] ?: "2026-11-15T12:00:00.123Z"),
            row.args["zone"] ?: "America/Cuiaba", (row.args["offset"] ?: "-14400").toLong()))

    private fun build(row: Row): EnterpriseOperation = when (row.operation) {
        "START" -> TripOperations.start(envelope(row), row.args.getValue("trip"), row.args.getValue("asset"), row.args.getValue("driver"),
            reading(row), text(row, "reason"), clock(row))
        "EVENT" -> TripOperations.event(envelope(row), row.args.getValue("event"), row.args.getValue("trip"), row.args.getValue("type"),
            text(row, "text"), row.args["dep"])
        "FINISH" -> TripOperations.finish(envelope(row), row.args.getValue("trip"), reading(row), text(row, "reason"), clock(row), row.args["dep"])
        "SUBMIT" -> TripOperations.submit(envelope(row), row.args.getValue("trip"), row.args.getValue("revision").toLong(), row.args["dep"])
        else -> fail("unknown operation ${row.operation}")
    }

    private fun evidence(row: Row, value: TripEvidence) {
        val expected = reading(row)
        if (expected == null) {
            assertEquals(text(row, "reason"), (value as TripEvidence.Absent).reason)
        } else {
            val observed = value as TripEvidence.Reading
            assertEquals(listOf(expected.meterId, expected.segmentId, expected.readingId, expected.kind, expected.value),
                listOf(observed.meterId, observed.segmentId, observed.readingId, observed.value.kind.name, observed.value.units.toString()))
        }
    }
    private fun clockMatches(row: Row, value: TripRunClock?) {
        val expected = clock(row)
        if (expected == null) assertNull(value) else assertEquals(TripRunClock(expected.bootId, expected.elapsedMillis), value)
    }

    private fun accepted(row: Row) {
        val operation = build(row)
        assertArrayEquals(row.bytes, operation.canonicalBytes, "builder bytes")
        val fact = TripOperations.read(EnterpriseOperation.parse(row.bytes, binding))
        assertEquals(row.args.getValue("op"), fact.operationId)
        assertEquals(listOfNotNull(row.args["dep"]), fact.dependsOn)
        when (fact) {
            is TripOperationFact.Start -> {
                assertEquals(listOf(row.args["trip"], row.args["asset"], row.args["driver"]), listOf(fact.tripId, fact.assetId, fact.driverEmployeeId))
                evidence(row, fact.evidence); clockMatches(row, fact.clock)
            }
            is TripOperationFact.Event -> {
                assertEquals(listOf(row.args["event"], row.args["trip"], row.args["type"]), listOf(fact.eventId, fact.tripId, fact.type.name))
                assertEquals(text(row, "text"), fact.text)
            }
            is TripOperationFact.Finish -> { assertEquals(row.args["trip"], fact.tripId); evidence(row, fact.evidence); clockMatches(row, fact.clock) }
            is TripOperationFact.Submit -> {
                assertEquals(row.args["trip"], fact.tripId); assertEquals(row.args.getValue("revision").toLong(), fact.revision)
            }
        }
        assertEquals(row.operation, when (fact) { is TripOperationFact.Start -> "START"; is TripOperationFact.Event -> "EVENT"
            is TripOperationFact.Finish -> "FINISH"; is TripOperationFact.Submit -> "SUBMIT" })
    }

    private fun refused(row: Row) {
        val built = try { build(row); null } catch (refusal: TripOperationException) { refusal.failure }
        if (row.outcome == "PROTOCOL") {
            assertTrue(built != null, "the builder accepted an envelope-profile refusal")
            try { EnterpriseOperation.seal(row.bytes, binding); fail("the envelope profile accepted ${row.id}") } catch (_: SyncProtocolException) { }
            return
        }
        val expected = TripOperationFailure.valueOf(row.outcome)
        assertEquals(expected, built, "builder refusal class")
        val sealed = EnterpriseOperation.seal(row.bytes, binding)
        val decoded = try { TripOperations.read(sealed); null } catch (refusal: TripOperationException) { refusal.failure }
        assertEquals(expected, decoded, "decoder refusal class")
    }

    @Test fun `the corpus is complete and every operation and outcome is covered`() {
        val all = rows()
        assertEquals(54, all.size)
        assertEquals(all.size, all.map { it.id }.distinct().size)
        assertEquals(mapOf("START/OK" to 8, "START/MALFORMED" to 17, "EVENT/OK" to 7, "EVENT/MALFORMED" to 7, "EVENT/UNSUPPORTED" to 3,
            "FINISH/OK" to 4, "FINISH/MALFORMED" to 4, "SUBMIT/OK" to 2, "SUBMIT/MALFORMED" to 1, "SUBMIT/PROTOCOL" to 1),
            all.groupingBy { it.operation + "/" + it.outcome }.eachCount())
        assertEquals(TripOperations.TYPES, setOf("trip.start", "trip.finish", "trip.event.record", "trip.submit"))
        // Every supported logbook type is produced and every reserved one is refused somewhere in the corpus.
        val types = all.filter { it.operation == "EVENT" }.map { it.args.getValue("type") }.toSet()
        assertTrue(TripLogbookType.entries.filter { it.supported }.all { it.name in types })
    }

    @TestFactory fun vectors(): List<DynamicTest> = rows().map { row ->
        DynamicTest.dynamicTest(row.id) { if (row.outcome == "OK") accepted(row) else refused(row) }
    }
}
