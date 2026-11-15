package com.anpfuel.domain.enterprise.maintenance.incident

import java.io.File
import java.nio.ByteBuffer
import java.nio.charset.CharacterCodingException
import java.nio.charset.CodingErrorAction
import java.security.MessageDigest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestFactory

/** EMP-CHECK-003: independent consumer of the shared incident-v1 oracle; expectations come only from the corpus. */
class EnterpriseIncidentVectorsTest {
    private data class Vector(val id: String, val operation: String, val args: Map<String, String>, val expected: Map<String, String>)
    private class Fixture(message: String) : IllegalStateException(message)

    private val file = File("../contracts/testdata/enterprise/checklist/incident-v1.psv")
    private val sha256 = "401ba762533077b059ffd5c39161451622adb6cf1fcf40b7de8ca8b235549119"
    private val outputs = mapOf(
        "REPORT" to setOf("type", "kind", "state", "revision", "blocks"),
        "DECIDE" to setOf("from", "to", "revision", "action"),
    )
    private val inputs = mapOf(
        "REPORT" to setOf("type", "kind", "description", "location", "blocks"),
        "DECIDE" to setOf("state", "revision", "blocks", "report", "reporter", "action", "expected", "note", "duplicate", "handler"),
    )
    private val counts = mapOf("REPORT" to 35, "DECIDE" to 63)
    private val codes = IncidentFailure.entries.map { it.name }.toSet()

    private fun fields(raw: String): Map<String, String> {
        if (raw.isEmpty()) return emptyMap()
        val pairs = raw.split(';').map {
            val p = it.split('=', limit = 2)
            if (p.size != 2 || p[0].isEmpty() || p[0] != p[0].trim()) throw Fixture("malformed field")
            p[0] to p[1]
        }
        if (pairs.map { it.first }.toSet().size != pairs.size) throw Fixture("duplicate field")
        return pairs.toMap()
    }

    private fun load(raw: ByteArray): List<Vector> {
        val text = Charsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT)
            .onUnmappableCharacter(CodingErrorAction.REPORT).decode(ByteBuffer.wrap(raw)).toString()
        val rows = text.removeSuffix("\n").split('\n')
        if (rows.size != 99 || rows.first() != "id|operation|args|expected") throw Fixture("header or size")
        val seen = mutableSetOf<String>()
        val vectors = rows.drop(1).map { line ->
            val p = line.split('|')
            if (p.size != 4 || p[0].isEmpty() || !seen.add(p[0])) throw Fixture("row")
            val output = outputs[p[1]] ?: throw Fixture("operation")
            val args = fields(p[2])
            val expected = fields(p[3])
            if (!inputs.getValue(p[1]).containsAll(args.keys)) throw Fixture("arg key")
            if ("error" in expected) {
                if (expected.size != 1 || expected.getValue("error") !in codes) throw Fixture("error map")
            } else if (expected.keys != output) {
                throw Fixture("expected keys")
            }
            Vector(p[0], p[1], args, expected)
        }
        if (vectors.groupingBy { it.operation }.eachCount() != counts) throw Fixture("counts")
        return vectors
    }

    /** hex: bytes that are not valid UTF-8 cannot form a Kotlin string; they become an unpaired surrogate (README). */
    private fun text(raw: String): String = when {
        raw.startsWith("hex:") -> {
            val hex = raw.removePrefix("hex:")
            if (hex.length % 2 != 0 || hex.any { it !in "0123456789abcdef" }) throw Fixture("hex")
            val bytes = ByteArray(hex.length / 2) { hex.substring(it * 2, it * 2 + 2).toInt(16).toByte() }
            try {
                Charsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT).decode(ByteBuffer.wrap(bytes)).toString()
            } catch (_: CharacterCodingException) {
                "\uD800"
            }
        }
        raw.startsWith("rep:") -> {
            val rest = raw.removePrefix("rep:")
            val count = rest.substringBefore(':').toIntOrNull() ?: throw Fixture("repeat")
            if (count < 1 || !rest.contains(':')) throw Fixture("repeat")
            text(rest.substringAfter(':')).repeat(count)
        }
        else -> raw
    }

    private fun optional(raw: String): String? = if (raw == "?") null else text(raw)

    private fun alias(raw: String): String {
        val n = raw.drop(1).toIntOrNull()
        if (raw.length > 1 && n != null && n >= 0 && n.toString() == raw.drop(1)) {
            val base = when (raw[0]) { 'R' -> 4000; 'A' -> 5000; 'M' -> 6000; 'E' -> 7000; else -> return raw }
            return "9c000000-0000-4000-8000-" + (base + n).toString().padStart(12, '0')
        }
        return raw
    }

    private fun identity(raw: String): List<String> = if (raw.isEmpty()) emptyList() else raw.split(',').map(::alias)

    private fun bool(raw: String): Boolean = when (raw) { "true" -> true; "false" -> false; else -> throw Fixture("boolean") }

    private fun long(raw: String): Long {
        val n = raw.toLongOrNull() ?: throw Fixture("integer")
        if (n.toString() != raw) throw Fixture("integer")
        return n
    }

    private fun apply(v: Vector): Map<String, String> {
        val a = v.args
        return when (v.operation) {
            "REPORT" -> {
                val kind = a["kind"] ?: "?"
                val r = IncidentReport.draft(a["type"] ?: "DAMAGE", if (kind == "?") null else text(kind), text(a["description"] ?: "Synthetic damage"),
                    optional(a["location"] ?: "?"), bool(a["blocks"] ?: "false"))
                mapOf("type" to r.type.name, "kind" to (r.kind?.name ?: "?"), "state" to r.state.name, "revision" to r.revision.toString(), "blocks" to r.blocksUse.toString())
            }
            "DECIDE" -> {
                val revision = long(a["revision"] ?: "1")
                val duplicate = a["duplicate"] ?: "?"
                val current = IncidentCurrent(a["state"] ?: "REPORTED", revision, bool(a["blocks"] ?: "false"), alias(a["report"] ?: "R1"), identity(a["reporter"] ?: "A1,M1,E1"))
                val command = IncidentCommand(a["action"] ?: "", long(a["expected"] ?: revision.toString()), optional(a["note"] ?: "?"),
                    if (duplicate == "?") null else alias(duplicate), identity(a["handler"] ?: "A2,M2,E2"))
                val step = Incident.decide(current, command)
                mapOf("from" to step.from.name, "to" to step.to.name, "revision" to step.revision.toString(), "action" to step.action.name)
            }
            else -> throw Fixture("operation")
        }
    }

    @TestFactory fun `all shared incident cases`(): List<DynamicTest> {
        val raw = file.readBytes()
        assertEquals(sha256, MessageDigest.getInstance("SHA-256").digest(raw).joinToString("") { "%02x".format(it.toInt() and 255) })
        val vectors = load(raw)
        assertEquals(outputs.keys, vectors.map { it.operation }.toSet())
        assertEquals(codes, vectors.mapNotNull { it.expected["error"] }.toSet())
        assertEquals(outputs.keys, vectors.filter { it.expected["error"] == "INVALID" }.map { it.operation }.toSet())
        val accepted = vectors.filter { it.operation == "DECIDE" && "error" !in it.expected }
        assertEquals(IncidentAction.entries.map { it.name }.toSet(), accepted.map { it.expected.getValue("action") }.toSet())
        assertEquals(IncidentState.entries.map { it.name }.toSet(), accepted.flatMap { listOf(it.expected.getValue("from"), it.expected.getValue("to")) }.toSet())
        return vectors.map { v ->
            DynamicTest.dynamicTest(v.id) {
                val got = try { apply(v) } catch (e: IncidentDomainException) { mapOf("error" to e.failure.name) }
                assertEquals(v.expected, got, v.id)
            }
        }
    }

    @Test fun `the grammar cannot skip or reinterpret rows`() {
        val original = file.readText()
        listOf(
            original.replace("id|operation|args|expected", "wrong"),
            original + "\n",
            original.replaceFirst("report-incident-breakdown|", "report-damage-defaults|"),
            original.replaceFirst("|DECIDE|action=OPEN|", "|CLOSE|action=OPEN|"),
            original.replaceFirst("|type=FIRE|", "|style=FIRE|"),
            original.replaceFirst(";action=ACKNOWLEDGE\n", "\n"),
            original.replaceFirst(";blocks=true\n", ";blocks=true;more=1\n"),
            original.replaceFirst("error=TRANSITION", "error=LATER"),
            original.replaceFirst("error=SELF_APPROVAL", "error=SELF_APPROVAL;to=RESOLVED"),
            original.replaceFirst("decide-action-empty|DECIDE|action=|error=INVALID\n", ""),
        ).forEach { bad ->
            assertTrue(bad != original)
            assertThrows(Fixture::class.java) { load(bad.toByteArray()) }
        }
        assertThrows(Exception::class.java) { load(byteArrayOf(0xff.toByte())) }
        assertEquals(setOf("INVALID", "CONFLICT", "TRANSITION", "SELF_APPROVAL"), codes)
    }
}
