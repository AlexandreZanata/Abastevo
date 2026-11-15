package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.trip.TripFixture.decode
import com.anpfuel.domain.enterprise.trip.TripFixture.fields
import com.anpfuel.domain.enterprise.trip.TripFixture.hex
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.TestFactory

/** Loads the same mission-v1 oracle as Go (EMP-TRIP-008); expectations are never computed here. */
class EnterpriseTripMissionVectorsTest {
    private val corpusHash = "d591d9e496cb686e24658f59f91896892e323afaa35f1d5ba4bf2c752021c21f"
    private val wantCounts = mapOf("STATE" to 20, "COMMAND" to 15, "RESPONSE" to 8, "TRANSITION" to 62, "CREATE" to 29, "RESTORE" to 34,
        "ASSIGN" to 23, "RESPOND" to 32, "CANCEL" to 15)
    private val errorCodes = TripMissionFailure.entries.map { it.name }.toSet()
    private val stored = "scope id id_org unit unit_org asset asset_org title_hex starts ends state revision assignee assignee_org member member_org answer answer_member answer_reason_hex"
    private val allowedArgs = mapOf("STATE" to "value_hex", "COMMAND" to "value_hex", "RESPONSE" to "value_hex", "TRANSITION" to "state command",
        "CREATE" to "scope id id_org unit unit_org asset asset_org title_hex starts ends omit", "RESTORE" to stored,
        "ASSIGN" to "$stored to to_org to_member to_member_org expected", "RESPOND" to "$stored by by_org kind_hex reason_hex expected",
        "CANCEL" to "$stored reason_hex expected").mapValues { it.value.split(' ').toSet() }
    private val missionAliases = mapOf("MI" to 91, "U" to 95)

    private fun alias(raw: String): String =
        missionAliases[raw]?.let { "5e000000-0000-4000-8000-" + it.toString().padStart(12, '0') } ?: TripFixture.alias(raw)
    private fun label(raw: String): String =
        missionAliases.entries.firstOrNull { alias(it.key) == raw }?.key ?: TripFixture.label(raw)
    private fun Map<String, String>.get(key: String, other: String): String = this[key] ?: other
    private fun instant(raw: String) = EnterpriseTime.parseInstant(raw)

    private fun content(a: Map<String, String>, omit: Set<String>): TripMissionInput {
        val reference = if ("id" in omit) null else MissionRef(alias(a.get("id_org", "A")), alias(a.get("id", "MI")))
        val unit = if ("unit" in omit) null else UnitRef(alias(a.get("unit_org", "A")), alias(a.get("unit", "U")))
        val asset = a.get("asset", "V").takeUnless { it == "?" }?.let { MissionAssetRef(alias(a.get("asset_org", "A")), alias(it)) }
        return TripMissionInput(reference, unit, asset, decode(a.get("title_hex", hex("Entrega no cliente"))),
            TripMissionWindow(instant(a.get("starts", "2026-11-20T11:00:00Z")), instant(a.get("ends", "2026-11-20T15:00:00Z"))))
    }

    private fun restore(a: Map<String, String>, scope: String): TripMission {
        val c = content(a, emptySet())
        val employee = a.get("assignee", "D")
        val member = a.get("member", "M")
        val assignee = if (employee == "?" && member == "?") null else TripMissionAssignee(
            employee.takeUnless { it == "?" }?.let { EmployeeRef(alias(a.get("assignee_org", "A")), alias(it)) },
            member.takeUnless { it == "?" }?.let { MembershipRef(alias(a.get("member_org", "A")), alias(it)) })
        val answer = a.get("answer", "?").takeUnless { it == "?" }?.let { kind ->
            TripMissionAnswer(kind, a.get("answer_member", "M").takeUnless { it == "?" }?.let { MembershipRef(alias("A"), alias(it)) },
                a.get("answer_reason_hex", "?").takeUnless { it == "?" }?.let(::decode))
        }
        val revision = requireNotNull(a.get("revision", "2").toLongOrNull())
        return TripMission.restore(alias(scope), TripMissionSnapshot(c.reference, c.unit, c.asset, c.title, c.window, a.get("state", "ASSIGNED"),
            revision, assignee, answer))
    }

    private fun fieldsOf(m: TripMission): Map<String, String> = mapOf("id" to label(m.reference.id), "unit" to label(m.unit.id),
        "asset" to (m.asset?.let { label(it.id) } ?: "?"), "title_hex" to hex(m.title), "starts" to EnterpriseTime.formatInstant(m.window.from),
        "ends" to EnterpriseTime.formatInstant(m.window.to), "state" to m.state.name, "revision" to m.revision.toString(),
        "assignee" to (m.assignee?.employee?.let { label(it.id) } ?: "?"), "member" to (m.assignee?.member?.let { label(it.id) } ?: "?"),
        "answer" to (m.answer?.kind ?: "?"), "answer_member" to (m.answer?.member?.let { label(it.id) } ?: "?"),
        "answer_reason_hex" to (m.answer?.reason?.let(::hex) ?: "?"))

    private fun expected(a: Map<String, String>): Long = requireNotNull(a.get("expected", "2").toLongOrNull())

    private fun command(a: Map<String, String>, run: (TripMission) -> TripMission): Map<String, String> {
        val mission = restore(a, "A")
        val before = fieldsOf(mission)
        try {
            return fieldsOf(run(mission))
        } finally {
            assertEquals(before, fieldsOf(mission), "a mission command changed its input mission")
        }
    }

    private fun execute(operation: String, a: Map<String, String>): Map<String, String> = when (operation) {
        "STATE" -> TripMissionState.parse(decode(a.getValue("value_hex"))).let { mapOf("state" to it.name, "terminal" to it.terminal.toString()) }
        "COMMAND" -> mapOf("command" to TripMissionCommand.parse(decode(a.getValue("value_hex"))).name)
        "RESPONSE" -> mapOf("response" to TripMissionResponse.parse(decode(a.getValue("value_hex"))).name)
        "TRANSITION" -> mapOf("state" to TripMissionLifecycle.transition(TripMissionState.parse(a.getValue("state")), TripMissionCommand.parse(a.getValue("command"))).name)
        "CREATE" -> fieldsOf(TripMission.create(alias(a.get("scope", "A")), content(a, TripFixture.omitted(a["omit"], setOf("id", "unit")))))
        "RESTORE" -> fieldsOf(restore(a, a.get("scope", "A")))
        "ASSIGN" -> command(a) {
            val to = TripMissionAssignee(a.get("to", "D2").takeUnless { v -> v == "?" }?.let { v -> EmployeeRef(alias(a.get("to_org", "A")), alias(v)) },
                a.get("to_member", "M2").takeUnless { v -> v == "?" }?.let { v -> MembershipRef(alias(a.get("to_member_org", "A")), alias(v)) })
            it.assign(alias(a.get("scope", "A")), expected(a), to)
        }
        "RESPOND" -> command(a) {
            val by = a.get("by", "M").takeUnless { v -> v == "?" }?.let { v -> MembershipRef(alias(a.get("by_org", "A")), alias(v)) }
            it.respond(alias(a.get("scope", "A")), expected(a), by, decode(a.get("kind_hex", hex("ACCEPT"))),
                a.get("reason_hex", "?").takeUnless { v -> v == "?" }?.let(::decode))
        }
        "CANCEL" -> command(a) { it.cancel(alias(a.get("scope", "A")), expected(a), decode(a.get("reason_hex", hex("Cliente cancelou")))) }
        else -> error("Unknown fixture operation")
    }

    private fun failure(error: IllegalArgumentException): Map<String, String> = when (error) {
        is TripMissionException -> mapOf("error" to error.failure.name)
        is TripDomainException -> mapOf("error" to error.failure.name)
        else -> throw error
    }

    @TestFactory fun allSharedTripMissionVectors(): List<DynamicTest> {
        val rows = TripFixture.corpus("mission-v1.psv", corpusHash)
        assertEquals(238, rows.size)
        assertEquals(wantCounts, rows.groupingBy { it[1] }.eachCount())
        val codes = mutableSetOf<String>()
        val pairs = mutableSetOf<String>()
        val enums = mapOf("STATE" to mutableSetOf<String>(), "COMMAND" to mutableSetOf(), "RESPONSE" to mutableSetOf())
        for (row in rows) {
            val args = fields(row[2])
            require(args.keys.all { it in requireNotNull(allowedArgs[row[1]]) })
            val expected = fields(row[3])
            expected["error"]?.let { require(expected.size == 1); codes += it }
            if (row[1] == "TRANSITION") pairs += "${args["state"]}/${args["command"]}"
            enums[row[1]]?.let { set -> listOf("state", "command", "response").forEach { k -> expected[k]?.let(set::add) } }
        }
        assertEquals(errorCodes, codes, "every error code needs a shared vector and no other code may appear")
        assertEquals(TripMissionState.entries.map { it.name }.toSet(), enums.getValue("STATE"))
        assertEquals(TripMissionCommand.entries.map { it.name }.toSet(), enums.getValue("COMMAND"))
        assertEquals(TripMissionResponse.entries.map { it.name }.toSet(), enums.getValue("RESPONSE"))
        for (s in TripMissionState.entries) for (c in TripMissionCommand.entries) require("${s.name}/${c.name}" in pairs) { "pair without TRANSITION vector" }
        return rows.map { (id, operation, rawArgs, expected) ->
            DynamicTest.dynamicTest(id) {
                val actual = try { execute(operation, fields(rawArgs)) } catch (failure: IllegalArgumentException) { failure(failure) }
                assertEquals(fields(expected), actual)
            }
        }
    }
}
