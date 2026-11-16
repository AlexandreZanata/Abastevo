package com.anpfuel.domain.enterprise.meter

import java.io.File
import java.nio.ByteBuffer
import java.nio.charset.CodingErrorAction
import java.security.MessageDigest
import java.time.Instant
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestFactory

/** EMP-METER-003: the shared validation oracle supplies every expectation; this adapter only names typed inputs and outputs. */
class EnterpriseMeterValidationVectorsTest {
    private data class Vector(val id: String, val operation: String, val args: Map<String, String>, val expected: Map<String, String>)
    private val file = File("../contracts/testdata/enterprise/meter/validation-v1.psv")
    private val outputs = mapOf("POLICY" to setOf("rate", "skew"), "CHECK" to setOf("state", "flags", "retroactive", "previous", "next"), "REEVALUATE" to setOf("changes"))
    private val inputs = mapOf("POLICY" to setOf("rate", "skew"), "CHECK" to setOf("kind", "rate", "skew", "series", "candidate"), "REEVALUATE" to setOf("kind", "rate", "skew", "series"))
    private fun id(raw: String): String = Regex("R([0-9]+)").matchEntire(raw)?.let { "8c000000-0000-4000-8000-" + (200 + it.groupValues[1].toInt()).toString().padStart(12, '0') } ?: raw
    private fun label(raw: String?): String {
        if (raw == null) return "?"
        if (raw.startsWith("8c000000-0000-4000-8000-")) raw.substringAfterLast('-').toIntOrNull()?.takeIf { it >= 200 }?.let { return "R" + (it - 200) }
        return raw
    }
    private fun fields(raw: String): Map<String, String> {
        val pairs = raw.split(';').map { part ->
            val split = part.split('=', limit = 2)
            require(split.size == 2 && split[0].isNotEmpty() && split[0].trim() == split[0])
            split[0] to split[1]
        }
        require(pairs.map { it.first }.toSet().size == pairs.size)
        return pairs.toMap()
    }
    private fun load(raw: ByteArray): List<Vector> {
        val text = Charsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT)
            .onUnmappableCharacter(CodingErrorAction.REPORT).decode(ByteBuffer.wrap(raw)).toString()
        val rows = text.removeSuffix("\n").split('\n')
        require(rows.size == 105 && rows.first() == "id|operation|args|expected")
        val seen = mutableSetOf<String>()
        val vectors = rows.drop(1).map { line ->
            val parts = line.split('|'); require(parts.size == 4 && parts[0].isNotEmpty() && seen.add(parts[0]))
            val output = requireNotNull(outputs[parts[1]]) { "Unknown validation fixture operation" }
            val args = fields(parts[2]); val expected = fields(parts[3])
            require(args.keys.all { it in inputs.getValue(parts[1]) })
            if ("error" in expected) require(expected == mapOf("error" to "INVALID")) else require(expected.keys == output)
            Vector(parts[0], parts[1], args, expected)
        }
        require(vectors.groupingBy { it.operation }.eachCount() == mapOf("POLICY" to 8, "CHECK" to 79, "REEVALUATE" to 17))
        return vectors
    }
    private fun point(raw: String, size: Int): Pair<MeterPoint, List<String>> {
        val p = raw.split(','); require(p.size == size)
        require(p[2].endsWith("Z") && p[3].endsWith("Z"))
        return MeterPoint(id(p[0]), p[1].toLong(), Instant.parse(p[2]), Instant.parse(p[3])) to p
    }
    private fun series(raw: String): List<MeterSeriesEntry> = if (raw.isEmpty()) emptyList() else raw.split('~').map { item ->
        val (point, p) = point(item, 6)
        MeterSeriesEntry(point, ReadingState.parse(p[4]), if (p[5].isEmpty()) emptyList() else p[5].split('+').map { flag ->
            ReadingFlag.entries.firstOrNull { it.name == flag } ?: throw MeterDomainException()
        })
    }
    private fun flags(values: List<ReadingFlag>) = values.joinToString("+") { it.name }
    private fun apply(vector: Vector): Map<String, String> {
        val f = vector.args
        val policy = MeterPlausibility.of(f.getOrDefault("rate", "150000").toLong(), f.getOrDefault("skew", "300").toLong())
        if (vector.operation == "POLICY") return mapOf("rate" to policy.distancePerHour.toString(), "skew" to policy.clockSkewSeconds.toString())
        val kind = MeterKind.parse(f.getOrDefault("kind", "ODOMETER"))
        val members = series(f.getValue("series"))
        if (vector.operation == "CHECK") {
            val v = MeterValidation.check(policy, kind, members, point(f.getValue("candidate"), 4).first)
            return mapOf("state" to v.state.name, "flags" to flags(v.flags), "retroactive" to v.retroactive.toString(), "previous" to label(v.previous), "next" to label(v.next))
        }
        return mapOf("changes" to MeterValidation.reevaluate(policy, kind, members).joinToString("~") {
            label(it.id) + ":" + it.verdict.state.name + ":" + flags(it.verdict.flags) + ":" + if (it.verdict.retroactive) MeterValidation.NOTE_RETROACTIVE else ""
        })
    }

    @TestFactory fun `all shared validation cases`(): List<DynamicTest> {
        val raw = file.readBytes()
        assertEquals("65f0cc26b961c8f2cd661b11f89e8b242816a6d4426254efa8a942b4477bee95", MessageDigest.getInstance("SHA-256").digest(raw).joinToString("") { "%02x".format(it.toInt() and 255) })
        val vectors = load(raw)
        assertEquals(setOf("POLICY", "CHECK", "REEVALUATE"), vectors.map { it.operation }.toSet())
        val text = vectors.joinToString { it.expected.values.joinToString() }
        ReadingFlag.entries.filter { it != ReadingFlag.POSSIBLE_ROLLOVER }.forEach { assertTrue(it.name in text, "flag ${it.name} has no vector") } // POSSIBLE_ROLLOVER: segments-v1
        assertTrue(vectors.any { it.expected["error"] == "INVALID" })
        return vectors.map { vector -> DynamicTest.dynamicTest(vector.id) {
            val actual = try { apply(vector) } catch (failure: MeterDomainException) { mapOf("error" to failure.failure.name) }
            assertEquals(vector.expected, actual)
        } }
    }

    @Test fun `fixture grammar cannot skip reinterpret or repair input`(): Unit {
        val original = file.readText()
        for (bad in listOf(original.replaceFirst("id|operation|args|expected", "wrong"), original + "\n",
            original.replaceFirst("policy-min-rate-zero-skew|", "policy-default|"), original.replaceFirst("|POLICY|", "|UNKNOWN|"),
            original.replaceFirst("rate=150000;skew=300|rate", "speed=150000;skew=300|rate"), original.replaceFirst(";next=R2\n", "\n"),
            original.replaceFirst("changes=\n", "changes=;more=1\n"), original.replaceFirst("error=INVALID", "error=SCOPE"))) {
            assertThrows(IllegalArgumentException::class.java) { load(bad.toByteArray(Charsets.UTF_8)) }
        }
        assertThrows(java.nio.charset.CharacterCodingException::class.java) { load(byteArrayOf(0xff.toByte())) }
    }
}
