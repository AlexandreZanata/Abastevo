package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.trip.TripFixture.decode
import com.anpfuel.domain.enterprise.trip.TripFixture.fields
import com.anpfuel.domain.enterprise.trip.TripFixture.omitted
import java.nio.charset.CharacterCodingException
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestFactory

/** Loads the same trip-v2 oracle as Go (it supersedes trip-v1, EDR-252); expectations are never computed here. */
class EnterpriseTripVectorsTest {
    private val corpusHash = "43aecf00903fffaff2ab17c3530595c3647e4040953fe2ff5e28b104b0bef41e"
    private val wantCounts = mapOf("ENUM" to 49, "TRANSITION" to 40, "STANDING" to 14, "START" to 57, "FINISH" to 47, "PAUSE" to 40,
        "RESUME" to 16, "CANCEL" to 22, "HISTORY" to 12)
    private val errorCodes = TripFailure.entries.map { it.name }.toSet()
    private val startKeys = ("scope org id id_org author author_org asset asset_org driver driver_org dep dep_org dep_asset " +
        "dep_meter dep_segment dep_reading dep_kind dep_value_kind dep_value dep_modulus dep_reason_hex at entry offset zone unit_zone omit")
    private val finishKeys = "$startKeys fin_scope fin_author fin_author_org fin fin_org fin_asset fin_meter fin_segment fin_reading " +
        "fin_kind fin_value_kind fin_value fin_modulus fin_reason_hex fin_at fin_entry fin_offset fin_omit again"
    private val commandKeys = "$startKeys history cmd_scope by by_org expected reason_hex cmd_at cmd_entry cmd_offset cmd_omit again"
    private val allowedArgs = mapOf("ENUM" to "kind value_hex", "TRANSITION" to "state command", "STANDING" to "operational review",
        "START" to startKeys, "FINISH" to finishKeys, "PAUSE" to commandKeys, "RESUME" to commandKeys, "CANCEL" to commandKeys,
        "HISTORY" to "$startKeys history").mapValues { it.value.split(' ').toSet() }
    private val enumSelectors = setOf("STATE", "COMMAND", "REVIEW")

    private fun caseShape(operation: String, args: Map<String, String>) {
        val allowed = requireNotNull(allowedArgs[operation])
        require(args.keys.all { it in allowed })
        if (operation == "ENUM") require(args.getValue("kind") in enumSelectors)
    }

    @Test fun `malformed fixture grammar is rejected before domain dispatch`() {
        assertThrows(IllegalArgumentException::class.java) { caseShape("ENUM", fields("kind=OTHER;value_hex=")) }
        assertThrows(IllegalArgumentException::class.java) { caseShape("OTHER", fields("state=PAUSED")) }
        assertThrows(IllegalArgumentException::class.java) { caseShape("STANDING", fields("operational=COMPLETED;extra=1")) }
        assertThrows(IllegalArgumentException::class.java) { fields("a=1;a=2") }
        assertThrows(IllegalArgumentException::class.java) { fields("novalue") }
        assertThrows(IllegalArgumentException::class.java) { fields("") }
        assertThrows(IllegalArgumentException::class.java) { omitted("asset,asset", TripFixture.startOmittable) }
        assertThrows(IllegalArgumentException::class.java) { omitted("final", TripFixture.startOmittable) }
        assertThrows(IllegalArgumentException::class.java) { decode("4F") }
        assertThrows(CharacterCodingException::class.java) { decode("ff") }
        assertThrows(CharacterCodingException::class.java) { decode("eda080") }
    }

    private fun execute(operation: String, a: Map<String, String>): Map<String, String> = when (operation) {
        "ENUM" -> {
            val value = decode(a.getValue("value_hex"))
            when (a.getValue("kind")) {
                "STATE" -> TripState.parse(value).let { mapOf("value" to it.name, "terminal" to it.terminal.toString()) }
                "COMMAND" -> mapOf("value" to TripCommand.parse(value).name)
                "REVIEW" -> mapOf("value" to TripReviewState.parse(value).name)
                else -> error("Unknown fixture enum namespace")
            }
        }
        "TRANSITION" -> {
            val state = TripState.parse(a.getValue("state"))
            mapOf("state" to TripLifecycle.transition(state, TripCommand.parse(a.getValue("command"))).name)
        }
        "STANDING" -> TripStanding(TripState.parse(a.getValue("operational")), TripReviewState.parse(a.getValue("review"))).let {
            mapOf("operational" to it.operational.name, "review" to it.review.name, "completed" to it.completed.toString(),
                "approved" to it.approved.toString())
        }
        "START" -> TripFixture.tripFields(TripFixture.start(TripFixture.startFields(a)))
        "FINISH" -> TripFixture.tripFields(TripFixture.finish(a))
        "PAUSE", "RESUME", "CANCEL" -> TripFixture.command(TripCommand.parse(operation), a)
        "HISTORY" -> TripFixture.history(a)
        else -> error("Unknown fixture operation")
    }

    @TestFactory fun allSharedTripVectors(): List<DynamicTest> {
        val rows = TripFixture.corpus("trip-v2.psv", corpusHash)
        assertEquals(297, rows.size)
        assertEquals(wantCounts, rows.groupingBy { it[1] }.eachCount())
        val codes = mutableSetOf<String>()
        val pairs = mutableSetOf<String>()
        for (row in rows) {
            val args = fields(row[2])
            caseShape(row[1], args)
            val expected = fields(row[3])
            expected["error"]?.let { code ->
                require(expected.size == 1 || code == "INCOMPLETE" && expected.size == 2 && "missing" in expected)
                codes += code
            }
            if (row[1] == "TRANSITION") {
                val state = TripState.entries.firstOrNull { it.name == args["state"] }
                val command = TripCommand.entries.firstOrNull { it.name == args["command"] }
                if (state != null && command != null) pairs += "$state:$command"
            }
        }
        assertEquals(errorCodes, codes, "every error code needs a shared vector and no other code may appear")
        assertEquals(35, pairs.size, "the state and command table must be complete")
        return rows.map { (id, operation, rawArgs, expected) ->
            DynamicTest.dynamicTest(id) {
                val actual = try { execute(operation, fields(rawArgs)) } catch (failure: IllegalArgumentException) { TripFixture.failure(failure) }
                assertEquals(fields(expected), actual)
            }
        }
    }
}
