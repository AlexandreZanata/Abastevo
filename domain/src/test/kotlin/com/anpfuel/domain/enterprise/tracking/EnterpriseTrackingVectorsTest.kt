package com.anpfuel.domain.enterprise.tracking

import com.anpfuel.domain.enterprise.tracking.TrackingFixture.fields
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.TestFactory

/** Loads the same session-v1 oracle as Go; expectations are never computed here. */
class EnterpriseTrackingVectorsTest {
    private val corpusHash = "4ddac666121fe1183e067dd65589a57c3f97a82e230e3091454ce9b889f9d39e"
    private val wantCounts = mapOf("ENUM" to 64, "OPEN" to 55, "START" to 53, "RESUME" to 30, "SUSPEND" to 23, "INTERRUPT" to 28,
        "REDUCE" to 13, "END" to 23, "FOLLOW" to 22, "COLLECT" to 22, "QUALITY" to 26)
    private val errorCodes = TrackingFailure.entries.map { it.name }.toSet()
    private val tripKeys = "prior scope trip trip_org driver driver_org trip_state"
    private val allowedArgs = mapOf(
        "ENUM" to "kind value_hex",
        "OPEN" to "scope org session session_org trip trip_org driver driver_org trip_state author author_org origin policy at existing omit",
        "START" to "$tripKeys author author_org origin permission notification at omit",
        "RESUME" to "$tripKeys author author_org origin permission notification at omit",
        "SUSPEND" to "$tripKeys cause author author_org at omit", "INTERRUPT" to "$tripKeys cause author author_org at omit",
        "REDUCE" to "$tripKeys cause author author_org at omit", "END" to "$tripKeys cause author author_org at omit",
        "FOLLOW" to "$tripKeys at omit", "COLLECT" to "$tripKeys omit", "QUALITY" to "expected gap fixes coarse",
    ).mapValues { it.value.split(' ').toSet() }

    private fun enum(a: Map<String, String>): Map<String, String> {
        val raw = TrackingFixture.decode(a.getValue("value_hex"))
        return when (a.getValue("kind")) {
            "STATE" -> TrackingSessionState.parse(raw).let { mapOf("value" to it.name, "terminal" to it.terminal.toString()) }
            "PERMISSION" -> mapOf("value" to LocationPermission.parse(raw).name)
            "POLICY" -> mapOf("value" to TrackingPolicy.parse(raw).name)
            "ORIGIN" -> mapOf("value" to TrackingOrigin.parse(raw).name)
            "CAUSE" -> mapOf("value" to TrackingCause.parse(raw).name)
            "ACTOR" -> mapOf("value" to TrackingActor.parse(raw).name)
            "QUALITY" -> mapOf("value" to TrackQuality.parse(raw).name)
            else -> error("Unknown enum fixture kind")
        }
    }

    private fun quality(args: Map<String, String>): Map<String, String> {
        val f = mapOf("expected" to "20400", "gap" to "1020", "fixes" to "2160", "coarse" to "false") + args
        val count = { key: String -> requireNotNull(f.getValue(key).toLongOrNull()) }
        val c = TrackQualityRules.classify(TrackEvidence(count("expected"), count("gap"), count("fixes"), TrackingFixture.flag(f.getValue("coarse"))))
        return mapOf("quality" to c.quality.name, "coverage" to (c.coveragePermille?.toString() ?: "?"))
    }

    private fun execute(operation: String, a: Map<String, String>): Map<String, String> {
        when (operation) {
            "ENUM" -> return enum(a)
            "QUALITY" -> return quality(a)
            "OPEN" -> return TrackingFixture.sessionFields(TrackingFixture.openVector(a))
        }
        val session = TrackingFixture.priorSession(a["prior"])
        val before = TrackingFixture.sessionFields(session)
        try {
            if (operation == "COLLECT") {
                val f = TrackingFixture.commandFields(operation, a)
                val facts = TrackingFixture.facts(f, TrackingFixture.omitted(f["omit"], TrackingFixture.collectOmittable))
                val precision = session.collecting(TrackingFixture.alias(f.getValue("scope")), facts)
                return mapOf("collecting" to (precision != null).toString(), "precision" to (precision?.name ?: "?"))
            }
            return TrackingFixture.sessionFields(TrackingFixture.runCommand(session, operation, TrackingFixture.commandFields(operation, a)))
        } finally {
            assertEquals(before, TrackingFixture.sessionFields(session), "a command changed its input session")
        }
    }

    @TestFactory fun allSharedTrackingSessionVectors(): List<DynamicTest> {
        val rows = TrackingFixture.corpus("session-v1.psv", corpusHash)
        assertEquals(359, rows.size)
        assertEquals(wantCounts, rows.groupingBy { it[1] }.eachCount())
        val codes = mutableSetOf<String>()
        for (row in rows) {
            val args = fields(row[2])
            require(args.keys.all { it in requireNotNull(allowedArgs[row[1]]) }) { "Unknown fixture argument in ${row[0]}" }
            val expected = fields(row[3])
            expected["error"]?.let { require(expected.size == 1); codes += it }
        }
        assertEquals(errorCodes, codes, "every error code needs a shared vector and no other code may appear")
        return rows.map { (id, operation, rawArgs, expected) ->
            DynamicTest.dynamicTest(id) {
                val actual = try { execute(operation, fields(rawArgs)) } catch (failure: IllegalArgumentException) { TrackingFixture.failure(failure) }
                assertEquals(fields(expected), actual)
            }
        }
    }
}
