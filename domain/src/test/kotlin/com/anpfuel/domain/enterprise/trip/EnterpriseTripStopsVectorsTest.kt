package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.trip.TripFixture.decode
import com.anpfuel.domain.enterprise.trip.TripFixture.fields
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestFactory

/** Loads the same stops-v1 oracle as Go (EMP-TRIP-010); expectations are never computed here. */
class EnterpriseTripStopsVectorsTest {
    private val corpusHash = "128d45271fef1b7f8b5c53003fdc96a74f6cd506d68f6869eea7d53d01105ae5"
    private val wantCounts = mapOf("PLANNED" to 21, "OBSERVE" to 33)
    private val errorCodes = setOf("INVALID")
    private val allowedArgs = mapOf("PLANNED" to "stops_hex", "OBSERVE" to "start end facts").mapValues { it.value.split(' ').toSet() }
    private val stopAliases = mapOf("C" to 91) + (1..9).associate { "E$it" to 80 + it }

    private fun alias(raw: String): String =
        stopAliases[raw]?.let { "5e000000-0000-4000-8000-" + it.toString().padStart(12, '0') } ?: TripFixture.alias(raw)
    private fun label(raw: String): String =
        stopAliases.entries.firstOrNull { alias(it.key) == raw }?.key ?: TripFixture.label(raw)

    private fun planned(raw: String): Map<String, String> {
        val places = when (raw) {
            "?" -> null
            "" -> emptyList()
            else -> raw.split(',').map(::decode)
        }
        val stops = TripPlannedStops.create(places)
        assertEquals(places ?: emptyList<String>(), stops.places)
        assertEquals(places?.size ?: 0, stops.size)
        fun point(p: TripPlannedPoint) = if (p.kind == TripPlannedPointKind.STOP) "STOP${p.stop}" else p.kind.name
        val legs = stops.legs
        legs.forEachIndexed { i, leg -> assertEquals(i + 1, leg.sequence) }
        return mapOf("count" to stops.size.toString(), "legs" to legs.joinToString(",") { point(it.from) + ">" + point(it.to) })
    }

    private fun fact(raw: String, parts: Int): TripStopFact {
        val p = raw.split('/')
        require(p.size == parts)
        val at = EnterpriseTime.parseInstant(p.last())
        return if (parts == 2) TripStopFact(TripStopMark.START, alias(p[0]), at) else TripStopFact(TripStopMark.parse(p[0]), alias(p[1]), at)
    }

    private fun boundary(f: TripStopFact?) = f?.let { "${it.mark.name}/${label(it.id)}" } ?: "-"

    private fun observe(a: Map<String, String>): Map<String, String> {
        val start = fact(a.getValue("start"), 2)
        val end = a["end"]?.let { fact(it, 3) }
        val facts = a["facts"]?.split(',')?.map { fact(it, 3) } ?: emptyList()
        val got = TripStops.observe(start, end, facts)
        assertEquals(got, TripStops.observe(start, end, facts.reversed()), "observed stops depend on the delivery order")
        got.stops.forEachIndexed { i, s -> assertEquals(i + 1, s.sequence) }
        got.legs.forEachIndexed { i, l -> assertEquals(i + 1, l.sequence) }
        return mapOf(
            "stops" to got.stops.joinToString(",") { "${it.sequence}:${boundary(it.arrival)}:${boundary(it.departure)}:${it.issue?.name ?: "-"}" },
            "legs" to got.legs.joinToString(",") { "${it.sequence}:${boundary(it.departure)}:${boundary(it.arrival)}:${it.issue?.name ?: "-"}" },
            "after_end" to got.afterEnd.joinToString(",") { boundary(it) },
        )
    }

    private fun execute(operation: String, a: Map<String, String>): Map<String, String> = when (operation) {
        "PLANNED" -> planned(a.getValue("stops_hex"))
        "OBSERVE" -> observe(a)
        else -> error("Unknown fixture operation")
    }

    @TestFactory fun allSharedTripStopsVectors(): List<DynamicTest> {
        val rows = TripFixture.corpus("stops-v1.psv", corpusHash)
        assertEquals(54, rows.size)
        assertEquals(wantCounts, rows.groupingBy { it[1] }.eachCount())
        val codes = mutableSetOf<String>()
        val issues = mutableSetOf<String>()
        for (row in rows) {
            val args = fields(row[2])
            require(args.keys.all { it in requireNotNull(allowedArgs[row[1]]) })
            val expected = fields(row[3])
            expected["error"]?.let { require(expected.size == 1); codes += it }
            TripStopIssue.entries.filter { ((expected["stops"] ?: "") + (expected["legs"] ?: "")).contains(it.name) }.forEach { issues += it.name }
        }
        assertEquals(errorCodes, codes, "every error code needs a shared vector and no other code may appear")
        assertEquals(TripStopIssue.entries.map { it.name }.toSet(), issues, "every issue needs a shared vector")
        return rows.map { (id, operation, rawArgs, expected) ->
            DynamicTest.dynamicTest(id) {
                val actual = try { execute(operation, fields(rawArgs)) } catch (_: TripStopsException) { mapOf("error" to "INVALID") }
                assertEquals(fields(expected), actual)
            }
        }
    }

    @Test fun plannedStopsAreRedactedAndIndependent() {
        val places = mutableListOf("Cliente Jardim Primavera", "Posto de apoio BR-163")
        val stops = TripPlannedStops.create(places)
        places[0] = "changed"
        assertEquals("Cliente Jardim Primavera", stops.places.first())
        assertFalse(stops.toString().contains("Cliente"))
        assertThrows(TripStopsException::class.java) { TripPlannedStops.create(List(TripPlannedStops.MAX_STOPS + 1) { "Parada" }) }
    }
}
