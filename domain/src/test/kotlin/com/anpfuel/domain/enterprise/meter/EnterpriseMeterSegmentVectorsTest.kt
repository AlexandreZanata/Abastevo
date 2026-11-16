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

/** EMP-METER-005: the shared segment oracle supplies every expectation; this adapter only names typed inputs and outputs. */
class EnterpriseMeterSegmentVectorsTest {
    private data class Vector(val id: String, val operation: String, val args: Map<String, String>, val expected: Map<String, String>)
    private val file = File("../contracts/testdata/enterprise/meter/segments-v1.psv")
    private val outputs = mapOf(
        "CHECK_WRAP" to setOf("state", "flags", "retroactive", "previous", "next"), "REEVALUATE_WRAP" to setOf("changes"),
        "CUMULATIVE" to setOf("reason", "usage", "equivalent", "anchor"), "USAGE" to setOf("reason", "usage", "from", "to"),
        "REPLACE" to setOf("previous", "close", "open", "implied", "reason"), "RESET" to setOf("previous", "close", "open", "implied", "reason"),
        "ROLLOVER" to setOf("previous", "close", "open", "implied", "reason"))
    private val inputs = mapOf(
        "CHECK_WRAP" to setOf("kind", "rate", "skew", "modulus", "series", "candidate"), "REEVALUATE_WRAP" to setOf("kind", "rate", "skew", "modulus", "series"),
        "CUMULATIVE" to setOf("kind", "spans", "readings", "at"), "USAGE" to setOf("kind", "spans", "readings", "start", "end"),
        "REPLACE" to setOf("kind", "modulus", "series", "at", "final", "new_modulus", "opening"), "RESET" to setOf("kind", "modulus", "series", "at", "before", "after"),
        "ROLLOVER" to setOf("kind", "modulus", "series", "target"))
    private val counts = mapOf("CHECK_WRAP" to 31, "REEVALUATE_WRAP" to 13, "CUMULATIVE" to 35, "USAGE" to 19, "REPLACE" to 17, "RESET" to 12, "ROLLOVER" to 16)
    private val bases = listOf('M' to 500, 'S' to 400, 'R' to 200)
    private fun id(raw: String): String = Regex("([RSM])([0-9]+)").matchEntire(raw)?.let { m ->
        "8c000000-0000-4000-8000-" + (bases.first { it.first == m.groupValues[1][0] }.second + m.groupValues[2].toInt()).toString().padStart(12, '0')
    } ?: raw
    private fun label(raw: String?): String {
        if (raw == null) return "?"
        if (raw.startsWith("8c000000-0000-4000-8000-")) raw.substringAfterLast('-').toIntOrNull()?.let { n ->
            bases.firstOrNull { n >= it.second }?.let { return it.first + (n - it.second).toString() }
        }
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
        require(rows.size == 144 && rows.first() == "id|operation|args|expected")
        val seen = mutableSetOf<String>()
        val vectors = rows.drop(1).map { line ->
            val parts = line.split('|'); require(parts.size == 4 && parts[0].isNotEmpty() && seen.add(parts[0]))
            val output = requireNotNull(outputs[parts[1]]) { "Unknown segment fixture operation" }
            val args = fields(parts[2]); val expected = fields(parts[3])
            require(args.keys.all { it in inputs.getValue(parts[1]) })
            if ("error" in expected) require(expected == mapOf("error" to "INVALID")) else require(expected.keys == output)
            Vector(parts[0], parts[1], args, expected)
        }
        require(vectors.groupingBy { it.operation }.eachCount() == counts)
        return vectors
    }
    private fun instant(raw: String): Instant { require(raw.endsWith("Z")); return Instant.parse(raw) }
    private fun point(raw: String, size: Int): Pair<MeterPoint, List<String>> {
        val p = raw.split(','); require(p.size == size)
        return MeterPoint(id(p[0]), p[1].toLong(), instant(p[2]), instant(p[3])) to p
    }
    private fun series(raw: String): List<MeterSeriesEntry> = if (raw.isEmpty()) emptyList() else raw.split('~').map { item ->
        val (point, p) = point(item, 6)
        MeterSeriesEntry(point, ReadingState.parse(p[4]), if (p[5].isEmpty()) emptyList() else p[5].split('+').map { flag ->
            ReadingFlag.entries.firstOrNull { it.name == flag } ?: throw MeterDomainException()
        })
    }
    private fun spans(raw: String): List<MeterSpan> = if (raw.isEmpty()) emptyList() else raw.split('~').map { item ->
        val p = item.split(','); require(p.size == 7)
        if (p[5] == "-") require(p[6] == "-")
        MeterSpan(id(p[0]), id(p[1]), SegmentReason.entries.firstOrNull { it.name == p[2] } ?: throw MeterDomainException(), instant(p[3]), p[4].toLong(),
            if (p[5] == "-") null else instant(p[5]), if (p[6] == "-" || p[6] == "?") null else p[6].toLong())
    }
    private fun anchors(raw: String): List<MeterAnchor> = if (raw.isEmpty()) emptyList() else raw.split('~').map { item ->
        val p = item.split(','); require(p.size == 5)
        MeterAnchor(id(p[0]), id(p[1]), p[2].toLong(), instant(p[3]), instant(p[4]))
    }
    private fun flags(values: List<ReadingFlag>) = values.joinToString("+") { it.name }
    private fun value(known: Boolean, v: Long) = if (known) v.toString() else "?"
    private fun boundary(b: MeterBoundary) = mapOf("previous" to label(b.previous), "close" to (b.close?.toString() ?: "?"), "open" to b.open.toString(),
        "implied" to value(b.reason == null, b.implied), "reason" to (b.reason?.name ?: ""))
    private fun apply(vector: Vector): Map<String, String> {
        val f = vector.args
        val kind = MeterKind.parse(f.getOrDefault("kind", "ODOMETER"))
        val modulus = f["modulus"]?.toLong()
        when (vector.operation) {
            "CHECK_WRAP", "REEVALUATE_WRAP" -> {
                val policy = MeterPlausibility.of(f.getOrDefault("rate", "150000").toLong(), f.getOrDefault("skew", "300").toLong())
                val members = series(f.getValue("series"))
                if (vector.operation == "CHECK_WRAP") {
                    val v = MeterValidation.checkMeter(policy, kind, modulus, members, point(f.getValue("candidate"), 4).first)
                    return mapOf("state" to v.state.name, "flags" to flags(v.flags), "retroactive" to v.retroactive.toString(), "previous" to label(v.previous), "next" to label(v.next))
                }
                return mapOf("changes" to MeterValidation.reevaluateMeter(policy, kind, modulus, members).joinToString("~") {
                    label(it.id) + ":" + it.verdict.state.name + ":" + flags(it.verdict.flags) + ":" + if (it.verdict.retroactive) MeterValidation.NOTE_RETROACTIVE else ""
                })
            }
            "CUMULATIVE" -> {
                val c = MeterTrack.of(kind, spans(f.getValue("spans")), anchors(f.getValue("readings"))).cumulative(instant(f.getValue("at")))
                return mapOf("reason" to (c.reason?.name ?: ""), "usage" to value(c.known, c.value), "equivalent" to value(c.known, c.equivalent), "anchor" to label(c.to))
            }
            "USAGE" -> {
                val u = MeterTrack.of(kind, spans(f.getValue("spans")), anchors(f.getValue("readings"))).between(instant(f.getValue("start")), instant(f.getValue("end")))
                return mapOf("reason" to (u.reason?.name ?: ""), "usage" to value(u.known, u.value), "from" to label(u.from), "to" to label(u.to))
            }
            "REPLACE" -> return boundary(MeterBoundaries.replacement(kind, modulus, series(f.getValue("series")), instant(f.getValue("at")),
                f.getValue("final").takeIf { it != "?" }?.toLong(), f["new_modulus"]?.toLong(), f.getValue("opening").toLong()))
            "RESET" -> return boundary(MeterBoundaries.reset(kind, modulus, series(f.getValue("series")), instant(f.getValue("at")), f.getValue("before").toLong(), f.getValue("after").toLong()))
            else -> return boundary(MeterBoundaries.rollover(kind, modulus, series(f.getValue("series")), id(f.getValue("target"))))
        }
    }

    @TestFactory fun `all shared segment cases`(): List<DynamicTest> {
        val raw = file.readBytes()
        assertEquals("296afeb474264ba4305004cbcc13c56761c192eca8c7ab9b9eede6e9cf15d21f", MessageDigest.getInstance("SHA-256").digest(raw).joinToString("") { "%02x".format(it.toInt() and 255) })
        val vectors = load(raw)
        assertEquals(outputs.keys, vectors.map { it.operation }.toSet())
        val text = vectors.joinToString { it.expected.values.joinToString() }
        listOf("POSSIBLE_ROLLOVER", "NO_READING_BEFORE", "METER_GAP", "INVALID").forEach { assertTrue(it in text, "$it has no vector") }
        return vectors.map { vector -> DynamicTest.dynamicTest(vector.id) {
            val actual = try { apply(vector) } catch (failure: MeterDomainException) { mapOf("error" to failure.failure.name) }
            assertEquals(vector.expected, actual)
        } }
    }

    @Test fun `segment fixture grammar cannot skip reinterpret or repair input`(): Unit {
        val original = file.readText()
        for (bad in listOf(original.replaceFirst("id|operation|args|expected", "wrong"), original + "\n",
            original.replaceFirst("d-without-modulus-below|", "d-wrap-candidate|"), original.replaceFirst("|CHECK_WRAP|", "|CHECK|"),
            original.replaceFirst("|CUMULATIVE|spans=", "|CUMULATIVE|segments="), original.replaceFirst(";anchor=R8\n", "\n"),
            original.replaceFirst("changes=\n", "changes=;more=1\n"), original.replaceFirst("error=INVALID", "error=SCOPE"))) {
            assertThrows(IllegalArgumentException::class.java) { load(bad.toByteArray(Charsets.UTF_8)) }
        }
        assertThrows(java.nio.charset.CharacterCodingException::class.java) { load(byteArrayOf(0xff.toByte())) }
    }
}
