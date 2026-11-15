package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.trip.TripFixture.fields
import com.anpfuel.domain.enterprise.trip.TripFixture.hex
import com.anpfuel.domain.enterprise.trip.TripFixture.label
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.TestFactory

/** Loads the same trip-time-v1 oracle as Go; expectations are never computed here. */
class EnterpriseTripTimeVectorsTest {
    private val corpusHash = "c5054633152e33112d7345b5302bd264ee6a5235f399617397f3971ae45c6a22"
    private val wantCounts = mapOf("CLOCK" to 12, "CROSSING" to 17, "ELAPSED" to 37)
    private val errorCodes = setOf("INVALID", "FINISH_BEFORE_START")
    private val allowedArgs = mapOf("CLOCK" to "boot_hex elapsed",
        "CROSSING" to "at entry offset zone unit_zone fin_at fin_entry fin_offset finished",
        "ELAPSED" to "at entry offset zone fin_at fin_entry fin_offset boot_hex mono fin_boot_hex fin_mono tolerance finished")
        .mapValues { it.value.split(' ').toSet() }

    private fun timedTrip(a: Map<String, String>): Trip = when (a["finished"] ?: "true") {
        "true" -> TripFixture.finish(a)
        "false" -> TripFixture.start(TripFixture.startFields(a))
        else -> error("Malformed finished fixture flag")
    }

    private fun clockField(clock: TripRunClock?): String = clock?.let { "${hex(it.bootId)}:${it.elapsedMillis}" } ?: "?"

    private fun crossingFields(trip: Trip): Map<String, String> {
        val start = trip.started.declared
        val end = trip.finished?.declared
        val crossing = trip.crossing
        assertEquals(trip.reporting.day, crossing.day, "the crossing view must report the start reporting day")
        val done = crossing.finished
        return mapOf("id" to label(trip.reference.id), "state" to trip.state.name, "day" to trip.reporting.day,
            "zone" to trip.reporting.zone, "reporting_source" to trip.reporting.source,
            "start_at" to EnterpriseTime.formatInstant(start.occurredAt), "start_entry" to start.entryZone,
            "start_offset" to start.deviceOffsetSeconds.toString(),
            "finish_at" to (end?.occurredAt?.let(EnterpriseTime::formatInstant) ?: "?"), "finish_entry" to (end?.entryZone ?: "?"),
            "finish_offset" to (end?.deviceOffsetSeconds?.toString() ?: "?"), "end_day" to (crossing.endDay ?: "?"),
            "crosses_midnight" to (if (done) crossing.crossesMidnight.toString() else "?"),
            "entry_zone_changed" to (if (done) crossing.entryZoneChanged.toString() else "?"),
            "offset_changed" to (if (done) crossing.offsetChanged.toString() else "?"))
    }

    private fun elapsedFields(trip: Trip, a: Map<String, String>): Map<String, String> {
        val elapsed = trip.elapsed(requireNotNull((a["tolerance"] ?: "2000").toLongOrNull()))
        return mapOf("state" to trip.state.name, "status" to elapsed.status.name, "seconds" to (elapsed.seconds?.toString() ?: "?"),
            "nanos" to (elapsed.nanos?.toString() ?: "?"), "gap" to (elapsed.gap?.name ?: "?"),
            "start_clock" to clockField(trip.started.clock), "finish_clock" to (trip.finished?.let { clockField(it.clock) } ?: "?"))
    }

    private fun execute(operation: String, a: Map<String, String>): Map<String, String> = when (operation) {
        "CLOCK" -> TripFixture.clock(a.getValue("boot_hex"), a.getValue("elapsed")).let {
            mapOf("boot_hex" to hex(it.bootId), "elapsed" to it.elapsedMillis.toString())
        }
        "CROSSING" -> crossingFields(timedTrip(a))
        "ELAPSED" -> elapsedFields(timedTrip(a), a)
        else -> error("Unknown fixture operation")
    }

    @TestFactory fun allSharedTripTimeVectors(): List<DynamicTest> {
        val rows = TripFixture.corpus("trip-time-v1.psv", corpusHash)
        assertEquals(66, rows.size)
        assertEquals(wantCounts, rows.groupingBy { it[1] }.eachCount())
        val codes = mutableSetOf<String>()
        for (row in rows) {
            val args = fields(row[2])
            require(args.keys.all { it in requireNotNull(allowedArgs[row[1]]) })
            if (row[1] == "ELAPSED") for (prefix in listOf("", "fin_")) require(("${prefix}boot_hex" in args) == ("${prefix}mono" in args))
            val expected = fields(row[3])
            expected["error"]?.let { require(expected.size == 1); codes += it }
        }
        assertEquals(errorCodes, codes, "every error code needs a shared vector and no other code may appear")
        return rows.map { (id, operation, rawArgs, expected) ->
            DynamicTest.dynamicTest(id) {
                val actual = try { execute(operation, fields(rawArgs)) } catch (failure: IllegalArgumentException) { TripFixture.failure(failure) }
                assertEquals(fields(expected), actual)
            }
        }
    }
}
