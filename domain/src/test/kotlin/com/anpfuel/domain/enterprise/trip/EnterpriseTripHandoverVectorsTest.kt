package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.EnterpriseTimeException
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.meter.MeterPresence
import com.anpfuel.domain.enterprise.trip.TripFixture.decode
import com.anpfuel.domain.enterprise.trip.TripFixture.fields
import com.anpfuel.domain.enterprise.trip.TripFixture.hex
import com.anpfuel.domain.enterprise.trip.TripFixture.omitted
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.TestFactory

/** Loads the same EMP-TRIP-012 handover oracle as Go (EDR-254); expectations are never computed here. */
class EnterpriseTripHandoverVectorsTest {
    private val corpusHash = "5dd11b0510c63fc8081e6c7a4b743e2a5af580a03ec233092cc3573768bd456a"
    private val wantCounts = mapOf("HANDOVER" to 47, "RESPONSIBLE" to 12, "ACT" to 28)
    private val errorCodes = setOf("INVALID", "SCOPE", "INCOMPLETE", "FORBIDDEN", "INVALID_TRANSITION", "CONFLICT", "EVENT_BEFORE_START", "EVENT_AFTER_FINISH")
    private val allowedArgs = mapOf(
        "HANDOVER" to "scope history recorder recorder_org receiver receiver_org receiver_employee receiver_employee_org expected reason_hex at omit again",
        "RESPONSIBLE" to "scope history at",
        "ACT" to "scope history act by act_at expected",
    ).mapValues { it.value.split(' ').toSet() }
    private val handoverOmittable = setOf("recorder", "receiver", "receiver_employee", "reason", "at")
    private val extra = mapOf("M3" to 43, "D3" to 33)
    private val employees = mapOf("M" to "D", "M2" to "D2", "M3" to "D3")
    private val reasonText = "Troca de turno"
    private val org get() = TripFixture.alias("A")

    private fun id(index: Int) = "5e000000-0000-4000-8000-" + index.toString().padStart(12, '0')
    private fun alias(raw: String): String = extra[raw]?.let(::id) ?: TripFixture.alias(raw)
    private fun label(raw: String): String = extra.entries.firstOrNull { id(it.value) == raw }?.key ?: TripFixture.label(raw)
    private fun member(raw: String, orgAlias: String = "A") = MembershipRef(alias(orgAlias), alias(raw))
    private fun declared(raw: String) = TimeDeclaration(EnterpriseTime.parseInstant(raw), "America/Cuiaba", -14400)
    private fun absent() = TripMeterEvidence.Absent(MeterPresence.missing("Sem leitura final"))

    private fun lifecycle(trip: Trip, command: String, input: TripLifecycleInput): Trip = when (command) {
        "PAUSE" -> trip.pause(org, input)
        "RESUME" -> trip.resume(org, input)
        "CANCEL" -> trip.cancel(org, input)
        else -> error("Not a lifecycle fixture command")
    }

    /** The default trip-v2 trip with the fixture history: step i at 12:i0Z by the current responsible member. */
    private fun trip(f: Map<String, String>): Trip {
        var trip = TripFixture.start(TripFixture.startFields(emptyMap()))
        val raw = f["history"] ?: return trip
        val steps = raw.split(',')
        require(steps.size <= 9)
        steps.forEachIndexed { i, step ->
            val at = "2026-11-14T12:${i + 1}0:00Z"
            trip = when {
                step.startsWith("H:") -> {
                    val target = step.removePrefix("H:")
                    val employee = requireNotNull(employees[target])
                    trip.handover(org, TripHandoverInput(member("M3"), member(target), EmployeeRef(org, alias(employee)), trip.revision, reasonText,
                        EnterpriseTime.parseInstant(at)))
                }
                step == "FINISH" -> trip.finish(org, TripFinishInput(trip.responsible, absent(), declared(at)))
                step == "PAUSE" || step == "RESUME" || step == "CANCEL" ->
                    lifecycle(trip, step, TripLifecycleInput(trip.responsible, trip.revision, if (step == "CANCEL") "Cliente cancelou" else null, declared(at)))
                else -> error("Unknown handover history step")
            }
        }
        return trip
    }

    private fun facts(trip: Trip): String = trip.handovers.joinToString(",") {
        "${it.revision}:${label(it.from.id)}>${label(it.to.id)}/${label(it.receiverEmployee.id)}@${EnterpriseTime.formatInstant(it.at)}"
    }.ifEmpty { "?" }

    private fun result(trip: Trip): Map<String, String> {
        val last = trip.handovers.lastOrNull()
        return mapOf("state" to trip.state.name, "revision" to trip.revision.toString(), "responsible" to label(trip.responsible.id),
            "start_author" to label(trip.started.author.id), "driver" to label(trip.driver.id), "handovers" to facts(trip),
            "recorder" to (last?.recorder?.id?.let(::label) ?: "?"), "reason_hex" to (last?.reason?.let(::hex) ?: "?"))
    }

    private fun handover(args: Map<String, String>): Map<String, String> {
        val trip = trip(args)
        val f = mutableMapOf("scope" to "A", "recorder" to "M3", "recorder_org" to "A", "receiver" to "M2", "receiver_org" to "A",
            "receiver_employee" to "D2", "receiver_employee_org" to "A", "expected" to trip.revision.toString(), "reason_hex" to hex(reasonText),
            "at" to "2026-11-14T13:00:00Z")
        f.putAll(args)
        val omit = omitted(f["omit"], handoverOmittable)
        val recorder = if ("recorder" in omit) null else MembershipRef(alias(f.getValue("recorder_org")), alias(f.getValue("recorder")))
        val receiver = if ("receiver" in omit) null else MembershipRef(alias(f.getValue("receiver_org")), alias(f.getValue("receiver")))
        val employee = if ("receiver_employee" in omit) null else EmployeeRef(alias(f.getValue("receiver_employee_org")), alias(f.getValue("receiver_employee")))
        val reason = if ("reason" in omit) null else decode(f.getValue("reason_hex"))
        val at = if ("at" in omit) null else EnterpriseTime.parseInstant(f.getValue("at"))
        val input = TripHandoverInput(recorder, receiver, employee, requireNotNull(f.getValue("expected").toLongOrNull()), reason, at)
        val scope = alias(f.getValue("scope"))
        val again = f["again"] ?: "false"
        require(again == "true" || again == "false")
        val before = result(trip)
        var next = try { trip.handover(scope, input) } finally { assertEquals(before, result(trip), "a handover changed its input trip") }
        if (again == "true") next = next.handover(scope, input)
        return result(next)
    }

    private fun responsible(args: Map<String, String>): Map<String, String> {
        val trip = trip(args)
        val at = args["at"]?.let { EnterpriseTime.parseInstant(it) }
        return mapOf("responsible" to label((if (at == null) trip.responsible else trip.responsibleAt(at)).id))
    }

    private fun act(args: Map<String, String>): Map<String, String> {
        val trip = trip(args)
        val by = member(args.getValue("by"))
        val at = declared(args["act_at"] ?: "2026-11-14T14:00:00Z")
        val expected = args["expected"]?.let { requireNotNull(it.toLongOrNull()) } ?: trip.revision
        val next = when (val command = args.getValue("act")) {
            "PAUSE", "RESUME", "CANCEL" -> lifecycle(trip, command, TripLifecycleInput(by, expected, if (command == "CANCEL") "Cliente cancelou" else null, at))
            "FINISH" -> trip.finish(org, TripFinishInput(by, absent(), at))
            "LOG" -> {
                val entry = trip.log(org, TripLogbookInput(TripLogbookRef(org, id(81)), by, "NOTE", "Parada", at))
                return mapOf("logged" to "true", "author" to label(entry.author.id))
            }
            else -> error("Unknown act fixture")
        }
        return mapOf("state" to next.state.name, "revision" to next.revision.toString()) +
            (next.finished?.let { mapOf("finish_author" to label(it.author.id)) } ?: emptyMap())
    }

    private fun failure(error: Throwable): Map<String, String> = when (error) {
        is TripLogbookException -> mapOf("error" to error.failure.name)
        is EnterpriseTimeException -> mapOf("error" to "INVALID")
        else -> TripFixture.failure(error)
    }

    @TestFactory fun allSharedHandoverVectors(): List<DynamicTest> {
        val rows = TripFixture.corpus("handover-v1.psv", corpusHash)
        assertEquals(87, rows.size)
        assertEquals(wantCounts, rows.groupingBy { it[1] }.eachCount())
        val codes = mutableSetOf<String>()
        for (row in rows) {
            val args = fields(row[2])
            require(args.keys.all { it in requireNotNull(allowedArgs[row[1]]) })
            fields(row[3])["error"]?.let { codes += it }
        }
        assertEquals(errorCodes, codes, "every handover error code needs a shared vector and no other code may appear")
        return rows.map { (id, operation, rawArgs, expected) ->
            DynamicTest.dynamicTest(id) {
                val args = fields(rawArgs)
                val actual = try {
                    when (operation) {
                        "HANDOVER" -> handover(args)
                        "RESPONSIBLE" -> responsible(args)
                        "ACT" -> act(args)
                        else -> error("Unknown fixture operation")
                    }
                } catch (failure: IllegalArgumentException) { failure(failure) }
                assertEquals(fields(expected), actual)
            }
        }
    }

}
