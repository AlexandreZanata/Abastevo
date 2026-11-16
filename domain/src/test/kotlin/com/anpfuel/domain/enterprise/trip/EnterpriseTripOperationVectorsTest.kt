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
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Assertions.fail
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestFactory

/**
 * The Kotlin consumer of trip-operations-v3 (EMP-TRIP-005, BUC-EMP-191, EDR-245; v2 adds the EMP-TRIP-019 checklist-run references,
 * EDR-331; v3 the EMP-TRIP-021 mission reference, lifecycle commands and mission answer, BUC-EMP-272, EDR-344); the Go intake
 * parsers consume the same rows.
 */
class EnterpriseTripOperationVectorsTest {
    private val hash = "5f4acc4b31c82580c9484a29e13baef92950d5ccfa3023cb58ba39d7eb629f54"
    private val binding = SyncBinding.of("7b0e2a54-0000-4000-8000-000000009501", "https://api.example.invalid", "7b0e2a54-0000-4000-8000-000000009502")
    private class Row(val id: String, val operation: String, val args: Map<String, String>, val outcome: String, val bytes: ByteArray)

    private fun rows(): List<Row> {
        val file = File("../contracts/testdata/enterprise/trip/trip-operations-v3.psv")
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
            reading(row), text(row, "reason"), clock(row), row.args["run"], row.args["mission"])
        "EVENT" -> TripOperations.event(envelope(row), row.args.getValue("event"), row.args.getValue("trip"), row.args.getValue("type"),
            text(row, "text"), row.args["dep"])
        "FINISH" -> TripOperations.finish(envelope(row), row.args.getValue("trip"), reading(row), text(row, "reason"), clock(row), row.args["dep"],
            row.args["run"])
        "SUBMIT" -> TripOperations.submit(envelope(row), row.args.getValue("trip"), row.args.getValue("revision").toLong(), row.args["dep"])
        "PAUSE", "RESUME", "CANCEL" -> TripOperations.lifecycle(envelope(row), TripCommand.valueOf(row.operation), row.args.getValue("trip"),
            row.args.getValue("revision").toLong(), text(row, "reason"), row.args["dep"])
        "RESPOND" -> TripMissionOperations.respond(envelope(row), row.args.getValue("mission"), row.args.getValue("revision").toLong(),
            row.args.getValue("response"), text(row, "reason"))
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
        if (row.operation == "RESPOND") {
            val answer = TripMissionOperations.read(EnterpriseOperation.parse(row.bytes, binding))
            assertEquals(listOf(row.args["op"], row.args["mission"], row.args["revision"], row.args["response"]),
                listOf(answer.operationId, answer.missionId, answer.revision.toString(), answer.response.name))
            assertEquals(text(row, "reason"), answer.reason)
            // An answer is never a trip operation.
            assertEquals(TripOperationFailure.MALFORMED, assertThrows(TripOperationException::class.java) {
                TripOperations.read(EnterpriseOperation.parse(row.bytes, binding)) }.failure)
            return
        }
        val fact = TripOperations.read(EnterpriseOperation.parse(row.bytes, binding))
        assertEquals(row.args.getValue("op"), fact.operationId)
        assertEquals(listOfNotNull(row.args["dep"]), fact.dependsOn)
        when (fact) {
            is TripOperationFact.Start -> {
                assertEquals(listOf(row.args["trip"], row.args["asset"], row.args["driver"]), listOf(fact.tripId, fact.assetId, fact.driverEmployeeId))
                evidence(row, fact.evidence); clockMatches(row, fact.clock); assertEquals(row.args["run"], fact.departureRunId)
                assertEquals(row.args["mission"], fact.missionId)
            }
            is TripOperationFact.Event -> {
                assertEquals(listOf(row.args["event"], row.args["trip"], row.args["type"]), listOf(fact.eventId, fact.tripId, fact.type.name))
                assertEquals(text(row, "text"), fact.text)
            }
            is TripOperationFact.Finish -> {
                assertEquals(row.args["trip"], fact.tripId); evidence(row, fact.evidence); clockMatches(row, fact.clock)
                assertEquals(row.args["run"], fact.returnRunId)
            }
            is TripOperationFact.Lifecycle -> {
                assertEquals(listOf(row.args["trip"], row.args["revision"], row.operation), listOf(fact.tripId, fact.revision.toString(), fact.command.name))
                assertEquals(text(row, "reason"), fact.reason)
            }
            is TripOperationFact.Submit -> {
                assertEquals(row.args["trip"], fact.tripId); assertEquals(row.args.getValue("revision").toLong(), fact.revision)
            }
        }
        assertEquals(row.operation, when (fact) { is TripOperationFact.Start -> "START"; is TripOperationFact.Event -> "EVENT"
            is TripOperationFact.Finish -> "FINISH"; is TripOperationFact.Submit -> "SUBMIT"; is TripOperationFact.Lifecycle -> fact.command.name })
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
        val decoded = if (row.operation == "RESPOND") {
            try { TripMissionOperations.read(sealed); null } catch (refusal: TripOperationException) { refusal.failure }
        } else try { TripOperations.read(sealed); null } catch (refusal: TripOperationException) { refusal.failure }
        assertEquals(expected, decoded, "decoder refusal class")
    }

    @Test fun `the corpus is complete and every operation and outcome is covered`() {
        val all = rows()
        assertEquals(94, all.size)
        assertEquals(all.size, all.map { it.id }.distinct().size)
        assertEquals(mapOf("START/OK" to 13, "START/MALFORMED" to 18, "START/PROTOCOL" to 3, "EVENT/OK" to 7, "EVENT/MALFORMED" to 7,
            "EVENT/UNSUPPORTED" to 3, "FINISH/OK" to 6, "FINISH/MALFORMED" to 4, "FINISH/PROTOCOL" to 1, "SUBMIT/OK" to 2, "SUBMIT/MALFORMED" to 1,
            "SUBMIT/PROTOCOL" to 1, "PAUSE/OK" to 4, "PAUSE/MALFORMED" to 3, "PAUSE/PROTOCOL" to 1, "RESUME/OK" to 2, "RESUME/MALFORMED" to 1,
            "CANCEL/OK" to 3, "CANCEL/MALFORMED" to 3, "RESPOND/OK" to 4, "RESPOND/MALFORMED" to 6, "RESPOND/PROTOCOL" to 1),
            all.groupingBy { it.operation + "/" + it.outcome }.eachCount())
        assertEquals(TripOperations.TYPES, setOf("trip.start", "trip.finish", "trip.event.record", "trip.submit", "trip.pause", "trip.resume", "trip.cancel"))
        assertEquals(TripOperations.LIFECYCLE.keys, setOf(TripCommand.PAUSE, TripCommand.RESUME, TripCommand.CANCEL))
        // Both answers, the mission reference alone and with a departure run are produced; a reserved PAUSE event stays refused.
        assertEquals(setOf("ACCEPT", "DECLINE"), all.filter { it.operation == "RESPOND" && it.outcome == "OK" }.map { it.args.getValue("response") }.toSet())
        assertTrue(all.any { it.operation == "START" && it.outcome == "OK" && "mission" in it.args && "run" !in it.args })
        assertTrue(all.any { it.operation == "START" && it.outcome == "OK" && "mission" in it.args && "run" in it.args })
        assertTrue(all.any { it.operation == "EVENT" && it.args["type"] == "PAUSE" && it.outcome == "UNSUPPORTED" })
        val types = all.filter { it.operation == "EVENT" }.map { it.args.getValue("type") }.toSet()
        assertTrue(TripLogbookType.entries.filter { it.supported }.all { it.name in types })
    }

    /** EMP-TRIP-019 (B-BR-EMP-511): the device reads back exactly one `checklist_run` reference; anything else stays UNSUPPORTED. */
    @Test fun `a second reference of one type or another reference type stays unsupported`() {
        for (id in listOf("start-with-departure-run", "finish-with-return-run")) {
            val row = rows().first { it.id == id }
            val run = row.args.getValue("run")
            val raw = row.bytes.toString(Charsets.UTF_8).replace(Regex(",\"payload_digest\":\"sha256:[0-9a-f]{64}\""), "")
            val single = "\"soft_references\":[{\"id\":\"$run\",\"type\":\"checklist_run\"}]"
            assertTrue(raw.contains(single))
            // EMP-TRIP-021 (EDR-344): a start may also name its mission, like the Go ParseStart; a finish never names one.
            val mission = "[{\"id\":\"$run\",\"type\":\"checklist_run\"},{\"id\":\"$run\",\"type\":\"mission\"}]"
            if (row.operation == "START") {
                val both = TripOperations.read(EnterpriseOperation.seal(raw.replace(single, "\"soft_references\":$mission").toByteArray(Charsets.UTF_8), binding))
                    as TripOperationFact.Start
                assertEquals(run to run, both.departureRunId to both.missionId)
            }
            for (refs in listOf("[{\"id\":\"$run\",\"type\":\"checklist_run\"},{\"id\":\"7b0e2a54-0000-4000-8000-000000009663\",\"type\":\"checklist_run\"}]",
                "[{\"id\":\"$run\",\"type\":\"fueling\"}]",
                "[{\"id\":\"$run\",\"type\":\"mission\"},{\"id\":\"7b0e2a54-0000-4000-8000-000000009663\",\"type\":\"mission\"}]") +
                listOfNotNull(mission.takeIf { row.operation == "FINISH" })) {
                val sealed = EnterpriseOperation.seal(raw.replace(single, "\"soft_references\":$refs").toByteArray(Charsets.UTF_8), binding)
                val failure = try { TripOperations.read(sealed); null } catch (refusal: TripOperationException) { refusal.failure }
                assertEquals(TripOperationFailure.UNSUPPORTED, failure, "$id $refs")
            }
        }
    }

    @TestFactory fun vectors(): List<DynamicTest> = rows().map { row ->
        DynamicTest.dynamicTest(row.id) { if (row.outcome == "OK") accepted(row) else refused(row) }
    }
}
