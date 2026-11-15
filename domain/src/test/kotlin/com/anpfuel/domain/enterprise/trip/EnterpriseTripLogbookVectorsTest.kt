package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.trip.TripFixture.decode
import com.anpfuel.domain.enterprise.trip.TripFixture.fields
import com.anpfuel.domain.enterprise.trip.TripFixture.hex
import com.anpfuel.domain.enterprise.trip.TripFixture.present
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.TestFactory

/** Loads the same logbook-v1 oracle as Go (EMP-TRIP-003); expectations are never computed here. */
class EnterpriseTripLogbookVectorsTest {
    private val corpusHash = "051b1a3a6947d8b7b620be5316ca6747b6f4fed86cde3ce7f7c9eb19171ef614"
    private val wantCounts = mapOf("TYPE" to 27, "RECORD" to 66, "ORDER" to 10)
    private val errorCodes = TripLogbookFailure.entries.map { it.name }.toSet()
    private val allowedArgs = mapOf("TYPE" to "value_hex",
        "RECORD" to "finished at fin_at ev_scope ev_id ev_id_org ev_author ev_author_org ev_type_hex ev_text_hex ev_at ev_entry ev_offset ev_omit",
        "ORDER" to "items").mapValues { it.value.split(' ').toSet() }
    private val eventOmittable = setOf("id", "author", "declaration")
    private val eventAliases = mapOf("E1" to 81, "E2" to 82, "E3" to 83)

    private fun alias(raw: String): String =
        eventAliases[raw]?.let { "5e000000-0000-4000-8000-" + it.toString().padStart(12, '0') } ?: TripFixture.alias(raw)
    private fun label(raw: String): String =
        eventAliases.entries.firstOrNull { alias(it.key) == raw }?.key ?: TripFixture.label(raw)

    private fun trip(a: Map<String, String>): Trip {
        val tripArgs = a.filterKeys { it in setOf("at", "fin_at") }
        return when (a["finished"] ?: "false") {
            "true" -> TripFixture.finish(tripArgs)
            "false" -> TripFixture.start(TripFixture.startFields(tripArgs))
            else -> error("Malformed finished fixture flag")
        }
    }

    private fun record(a: Map<String, String>): Map<String, String> {
        val trip = trip(a)
        val before = TripFixture.tripFields(trip)
        val org = "A"
        val omit = TripFixture.omitted(a["ev_omit"], eventOmittable)
        val reference = if ("id" in omit) null else TripLogbookRef(alias(a["ev_id_org"] ?: org), alias(a["ev_id"] ?: "E1"))
        val author = if ("author" in omit) null else MembershipRef(alias(a["ev_author_org"] ?: org), alias(a["ev_author"] ?: "M"))
        val type = decode(a["ev_type_hex"] ?: hex("NOTE"))
        val text = present(a["ev_text_hex"] ?: hex("Cliente ausente"))?.let(::decode)
        val declared = if ("declaration" in omit) null else TimeDeclaration(EnterpriseTime.parseInstant(a["ev_at"] ?: "2026-11-14T13:00:00Z"),
            a["ev_entry"] ?: "America/Cuiaba", requireNotNull((a["ev_offset"] ?: "-14400").toLongOrNull()))
        val entry = try {
            trip.log(alias(a["ev_scope"] ?: org), TripLogbookInput(reference, author, type, text, declared))
        } finally {
            assertEquals(before, TripFixture.tripFields(trip), "a logbook event changed its trip")
        }
        return mapOf("trip" to label(entry.trip.id), "id" to label(entry.reference.id), "type" to entry.type.name,
            "text_hex" to (entry.text?.let(::hex) ?: "?"), "author" to label(entry.author.id),
            "at" to EnterpriseTime.formatInstant(entry.declared.occurredAt), "entry" to entry.declared.entryZone,
            "offset" to entry.declared.deviceOffsetSeconds.toString(), "trip_state" to trip.state.name)
    }

    private fun order(raw: String): Map<String, String> {
        val items = raw.split(',').map {
            val parts = it.split('/')
            require(parts.size == 3)
            TripTimelineItem(TripTimelineKind.valueOf(parts[0]), alias(parts[1]), EnterpriseTime.parseInstant(parts[2]))
        }
        val sorted = TripTimeline.sort(items)
        assertEquals(sorted, TripTimeline.sort(items.reversed()), "timeline order depends on delivery order")
        assertEquals(sorted, items.sortedWith(TripTimeline.order))
        return mapOf("order" to sorted.joinToString(",") { "${it.kind.name}/${label(it.id)}" })
    }

    private fun execute(operation: String, a: Map<String, String>): Map<String, String> = when (operation) {
        "TYPE" -> TripLogbookType.parse(decode(a.getValue("value_hex"))).let {
            mapOf("type" to it.name, "status" to if (it.supported) "SUPPORTED" else "RESERVED")
        }
        "RECORD" -> record(a)
        "ORDER" -> order(a.getValue("items"))
        else -> error("Unknown fixture operation")
    }

    private fun failure(error: IllegalArgumentException): Map<String, String> = when (error) {
        is TripLogbookException -> mapOf("error" to error.failure.name)
        else -> TripFixture.failure(error)
    }

    @TestFactory fun allSharedTripLogbookVectors(): List<DynamicTest> {
        val rows = TripFixture.corpus("logbook-v1.psv", corpusHash)
        assertEquals(103, rows.size)
        assertEquals(wantCounts, rows.groupingBy { it[1] }.eachCount())
        val codes = mutableSetOf<String>()
        val types = mutableSetOf<String>()
        for (row in rows) {
            val args = fields(row[2])
            require(args.keys.all { it in requireNotNull(allowedArgs[row[1]]) })
            val expected = fields(row[3])
            expected["error"]?.let { require(expected.size == 1); codes += it }
            if (row[1] == "TYPE") expected["type"]?.let { types += it }
        }
        assertEquals(errorCodes, codes, "every error code needs a shared vector and no other code may appear")
        assertEquals(TripLogbookType.entries.map { it.name }.toSet(), types, "every type needs a TYPE vector")
        return rows.map { (id, operation, rawArgs, expected) ->
            DynamicTest.dynamicTest(id) {
                val actual = try { execute(operation, fields(rawArgs)) } catch (failure: IllegalArgumentException) { failure(failure) }
                assertEquals(fields(expected), actual)
            }
        }
    }
}
