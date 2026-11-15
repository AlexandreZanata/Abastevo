package com.anpfuel.domain.enterprise.maintenance.checklist

import java.io.File
import java.nio.ByteBuffer
import java.nio.charset.CodingErrorAction
import java.security.MessageDigest
import java.time.Instant
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestFactory

/** EMP-CHECK-001: independent consumer of the shared checklist-v1 oracle; expectations come only from the corpus. */
class EnterpriseChecklistVectorsTest {
    private data class Vector(val id: String, val operation: String, val args: Map<String, String>, val expected: Map<String, String>)
    private class Fixture(message: String) : IllegalStateException(message)

    private val file = File("../contracts/testdata/enterprise/checklist/checklist-v1.psv")
    private val sha256 = "4231d227e82782a2dedf03655bc69e48168664bd8ef7a10a733ce3d3140165e2"
    private val outputs = mapOf(
        "TEMPLATE" to setOf("context", "target", "items", "critical", "optional", "photo"),
        "POLICY" to setOf("mode", "validity"),
        "EVALUATE" to setOf("outcome", "issues", "critical", "acknowledge"),
        "RESOLVE" to setOf("template", "match"),
        "POLICY_AT" to setOf("revision", "mode", "validity"),
    )
    private val inputs = mapOf(
        "TEMPLATE" to setOf("name", "context", "target", "items"),
        "POLICY" to setOf("mode", "validity"),
        "EVALUATE" to setOf("items", "mode", "validity", "answers"),
        "RESOLVE" to setOf("context", "asset", "category", "candidates"),
        "POLICY_AT" to setOf("history", "at"),
    )
    private val counts = mapOf("TEMPLATE" to 40, "POLICY" to 11, "EVALUATE" to 38, "RESOLVE" to 22, "POLICY_AT" to 16)
    private val codes = ChecklistFailure.entries.map { it.name }.toSet()

    private fun fields(raw: String): Map<String, String> {
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
        if (rows.size != 128 || rows.first() != "id|operation|args|expected") throw Fixture("header or size")
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

    private fun text(raw: String): String = when {
        raw.startsWith("hex:") -> {
            val hex = raw.removePrefix("hex:")
            if (hex.length % 2 != 0 || hex.any { it !in "0123456789abcdef" }) throw Fixture("hex")
            val bytes = ByteArray(hex.length / 2) { hex.substring(it * 2, it * 2 + 2).toInt(16).toByte() }
            Charsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT).decode(ByteBuffer.wrap(bytes)).toString()
        }
        raw.startsWith("rep:") -> {
            val rest = raw.removePrefix("rep:")
            val count = rest.substringBefore(':').toIntOrNull() ?: throw Fixture("repeat")
            if (count < 1 || !rest.contains(':')) throw Fixture("repeat")
            text(rest.substringAfter(':')).repeat(count)
        }
        else -> raw
    }

    private fun alias(raw: String): String {
        val n = raw.drop(1).toIntOrNull()
        if (raw.length > 1 && n != null && n >= 0 && n.toString() == raw.drop(1)) {
            val base = when (raw[0]) { 'C' -> 1000; 'X' -> 2000; 'T' -> 3000; else -> return raw }
            return "9c000000-0000-4000-8000-" + (base + n).toString().padStart(12, '0')
        }
        return raw
    }

    private fun label(id: String?): String {
        if (id == null) return "?"
        if (id.startsWith("9c000000-0000-4000-8000-")) {
            val n = id.substringAfterLast('-').toIntOrNull()
            if (n != null) when (n) {
                in 1001..1999 -> return "C" + (n - 1000)
                in 2001..2999 -> return "X" + (n - 2000)
                in 3001..3999 -> return "T" + (n - 3000)
            }
        }
        return id
    }

    private fun target(raw: String): ChecklistTarget {
        if (!raw.contains(':')) return ChecklistTarget(raw, null)
        val kind = raw.substringBefore(':')
        val rest = raw.substringAfter(':')
        if (rest.startsWith("gen:")) {
            val n = rest.removePrefix("gen:").toIntOrNull() ?: throw Fixture("target")
            val prefix = if (kind == "CATEGORIES") "C" else "X"
            return ChecklistTarget(kind, (1..n).map { alias(prefix + it) })
        }
        return ChecklistTarget(kind, if (rest.isEmpty()) emptyList() else rest.split(',').map(::alias))
    }

    private fun items(raw: String): List<ChecklistItem> {
        if (raw.isEmpty()) return emptyList()
        return raw.split('~').flatMap { token ->
            if (token.startsWith("gen:")) {
                val n = token.removePrefix("gen:").toIntOrNull() ?: throw Fixture("items")
                (1..n).map { ChecklistItem("Item $it") }
            } else {
                if (!token.contains(':')) throw Fixture("item")
                val flags = token.substringBefore(':')
                val name = text(token.substringAfter(':'))
                if (flags == "-") listOf(ChecklistItem(name)) else {
                    if (flags.isEmpty() || flags.any { it !in "COP" } || flags.toSet().size != flags.length) throw Fixture("flags")
                    listOf(ChecklistItem(name, critical = 'C' in flags, optional = 'O' in flags, photoOnNotOk = 'P' in flags))
                }
            }
        }
    }

    private fun answers(raw: String): List<ChecklistResponse> {
        if (raw.isEmpty()) return emptyList()
        return raw.split('~').flatMap { original ->
            var token = original
            var repeat = 1
            if (token.startsWith("gen:")) {
                val rest = token.removePrefix("gen:")
                repeat = rest.substringBefore(':').toIntOrNull() ?: throw Fixture("answers")
                token = rest.substringAfter(':')
            }
            val p = token.split(',', limit = 3)
            val photo = when (p.getOrNull(1)) { null, "0" -> false; "1" -> true; else -> throw Fixture("photo") }
            val response = ChecklistResponse(if (p[0] == "?") null else p[0], if (p.size > 2) text(p[2]) else "", photo)
            List(repeat) { response }
        }
    }

    private fun positions(values: List<Int>) = values.joinToString("+")
    private fun instant(raw: String): Instant {
        if (!raw.endsWith("Z")) throw Fixture("instant")
        return Instant.parse(raw)
    }

    private fun apply(v: Vector): Map<String, String> {
        val a = v.args
        return when (v.operation) {
            "TEMPLATE" -> {
                val d = ChecklistDefinition.of(text(a["name"] ?: "Synthetic checklist"), a["context"] ?: "DEPARTURE", target(a["target"] ?: "ORGANIZATION"), items(a["items"] ?: "gen:1"))
                val ids = if (d.targetIds.isEmpty()) "" else ":" + d.targetIds.joinToString(",") { label(it) }
                mapOf("context" to d.context.name, "target" to d.targetKind.name + ids, "items" to d.items.size.toString(),
                    "critical" to positions(d.items.indices.filter { d.items[it].critical }.map { it + 1 }),
                    "optional" to positions(d.items.indices.filter { d.items[it].optional }.map { it + 1 }),
                    "photo" to positions(d.items.indices.filter { d.items[it].photoOnNotOk }.map { it + 1 }))
            }
            "POLICY" -> {
                val p = ChecklistPolicy.of(a["mode"] ?: "BLOCK", (a["validity"] ?: "43200").toLongOrNull() ?: throw Fixture("validity"))
                mapOf("mode" to p.mode.name, "validity" to p.validitySeconds.toString())
            }
            "EVALUATE" -> {
                val e = Checklist.evaluate(items(a["items"] ?: "gen:1"), a["mode"] ?: "BLOCK", (a["validity"] ?: "43200").toLongOrNull() ?: throw Fixture("validity"), answers(a.getValue("answers")))
                mapOf("outcome" to e.outcome.name, "issues" to positions(e.issues), "critical" to positions(e.critical), "acknowledge" to e.acknowledgementRequired.toString())
            }
            "RESOLVE" -> {
                val raw = a.getValue("candidates")
                val candidates = if (raw.isEmpty()) emptyList() else raw.split('~').map {
                    val p = it.split('/', limit = 3)
                    if (p.size != 3) throw Fixture("candidate")
                    ChecklistCandidate(alias(p[0]), p[1], target(p[2]))
                }
                val r = Checklist.resolve(candidates, a["context"] ?: "DEPARTURE", alias(a["asset"] ?: "X1"), alias(a["category"] ?: "C1"))
                mapOf("template" to label(r.templateId), "match" to r.match.name)
            }
            "POLICY_AT" -> {
                val raw = a.getValue("history")
                val history = if (raw.isEmpty()) emptyList() else raw.split('~').map {
                    val p = it.split(',')
                    if (p.size != 4) throw Fixture("revision")
                    ChecklistPolicyRevision(p[0].toLongOrNull() ?: throw Fixture("revision"), p[1], p[2].toLongOrNull() ?: throw Fixture("validity"), instant(p[3]))
                }
                val r = Checklist.policyAt(history, instant(a.getValue("at")))
                if (r == null) mapOf("revision" to "?", "mode" to "?", "validity" to "?")
                else mapOf("revision" to r.revision.toString(), "mode" to r.mode, "validity" to r.validitySeconds.toString())
            }
            else -> throw Fixture("operation")
        }
    }

    @TestFactory fun `all shared checklist cases`(): List<DynamicTest> {
        val raw = file.readBytes()
        assertEquals(sha256, MessageDigest.getInstance("SHA-256").digest(raw).joinToString("") { "%02x".format(it.toInt() and 255) })
        val vectors = load(raw)
        assertEquals(outputs.keys, vectors.map { it.operation }.toSet())
        val errors = vectors.mapNotNull { it.expected["error"] }.toSet()
        assertEquals(codes, errors)
        assertEquals(outputs.keys, vectors.filter { it.expected["error"] == "INVALID" }.map { it.operation }.toSet())
        return vectors.map { v ->
            DynamicTest.dynamicTest(v.id) {
                val got = try { apply(v) } catch (e: ChecklistDomainException) { mapOf("error" to e.failure.name) }
                assertEquals(v.expected, got, v.id)
            }
        }
    }

    @Test fun `the grammar cannot skip or reinterpret rows`() {
        val original = file.readText()
        listOf(
            original.replace("id|operation|args|expected", "wrong"),
            original + "\n",
            original.replaceFirst("tpl-categories-sorted|", "tpl-org-default|"),
            original.replaceFirst("|POLICY|", "|UNKNOWN|"),
            original.replaceFirst("|mode=WARN;validity=3600|", "|style=WARN;validity=3600|"),
            original.replaceFirst(";acknowledge=false\n", "\n"),
            original.replaceFirst("match=NONE\n", "match=NONE;more=1\n"),
            original.replaceFirst("error=INCOMPLETE", "error=LATER"),
            original.replaceFirst("error=PHOTO_REQUIRED", "error=PHOTO_REQUIRED;outcome=BLOCKING"),
            original.replaceFirst("pol-zero|POLICY|mode=BLOCK;validity=0|error=INVALID\n", ""),
        ).forEach { bad ->
            assertTrue(bad != original)
            assertThrows(Fixture::class.java) { load(bad.toByteArray()) }
        }
        assertThrows(Exception::class.java) { load(byteArrayOf(0xff.toByte())) }
    }
}
