package com.anpfuel.domain.enterprise.meter

import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.fleet.AssetId
import com.anpfuel.domain.enterprise.fleet.AssetRef
import java.io.File
import java.nio.ByteBuffer
import java.nio.charset.CodingErrorAction
import java.security.MessageDigest
import java.time.Instant
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestFactory

class EnterpriseMeterCaptureVectorsTest {
    private data class Vector(val id: String, val operation: String, val args: Map<String, String>, val expected: Map<String, String>)
    private val file = File("../contracts/testdata/enterprise/meter/capture-v1.psv")
    private val aliases = mapOf("A" to 1, "B" to 2, "D" to 51, "W" to 71, "E" to 41, "O" to 101)
    private fun id(n: Int) = "8c000000-0000-4000-8000-" + n.toString().padStart(12, '0')
    private fun alias(raw: String): String = if (raw == "?") "" else aliases[raw]?.let(::id) ?: raw
    private fun label(raw: String): String = aliases.entries.firstOrNull { id(it.value) == raw }?.key ?: raw
    private fun fields(raw: String): Map<String, String> {
        val values = raw.split(';').map { part ->
            val split = part.split('=', limit = 2)
            require(split.size == 2 && split[0].isNotEmpty() && split[0].trim() == split[0])
            split[0] to split[1]
        }
        require(values.map { it.first }.toSet().size == values.size)
        return values.toMap()
    }
    private fun load(raw: ByteArray): List<Vector> {
        val text = Charsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT)
            .onUnmappableCharacter(CodingErrorAction.REPORT).decode(ByteBuffer.wrap(raw)).toString()
        val rows = text.removeSuffix("\n").split('\n')
        require(rows.size == 41 && rows.first() == "id|operation|args|expected")
        val seen = mutableSetOf<String>()
        val vectors = rows.drop(1).map { line ->
            val parts = line.split('|'); require(parts.size == 4 && parts[0].isNotEmpty() && seen.add(parts[0]))
            val args = fields(parts[2]); val expected = fields(parts[3])
            val source = setOf("kind", "org", "id")
            val output: Set<String>
            val allowed = when (parts[1]) {
                "SOURCE" -> { require(args.keys == source); output = source; source }
                "READING" -> { output = source + setOf("value", "state"); setOf("legacy", "legacy_org", "capture", "capture_org", "id", "author", "author_org", "operation", "operation_org", "value", "state") }
                else -> throw IllegalArgumentException("Unknown capture fixture operation")
            }
            require(args.keys.all { it in allowed })
            if ("error" in expected) require(expected.keys == setOf("error") && expected["error"] in setOf("INVALID", "SCOPE"))
            else require(expected.keys == output)
            Vector(parts[0], parts[1], args, expected)
        }
        require(vectors.groupingBy { it.operation }.eachCount() == mapOf("SOURCE" to 17, "READING" to 23))
        return vectors
    }
    private fun output(source: CaptureSource) = mapOf("kind" to source.kind.name, "org" to label(source.organizationId), "id" to label(source.id))
    private fun apply(vector: Vector): Map<String, String> {
        val f = vector.args
        if (vector.operation == "SOURCE") return output(CaptureSource(alias(f.getValue("org")), CaptureKind.parse(f.getValue("kind")), alias(f.getValue("id"))))
        val org = alias("A")
        val segment = SegmentRef(org, MeterRef(org, AssetRef(org, AssetId.parse(id(11))), id(21), MeterKind.ODOMETER), id(31))
        val author = MembershipRef(alias(f.getOrDefault("author_org", "A")), alias(f.getOrDefault("author", "E")))
        val operation = OperationRef(alias(f.getOrDefault("operation_org", "A")), alias(f.getOrDefault("operation", "O")))
        val device = f.getOrDefault("legacy", "D").let { if (it == "?") null else DeviceRef(alias(f.getOrDefault("legacy_org", "A")), alias(it)) }
        val source = when (val raw = f.getOrDefault("capture", "NONE")) {
            "NONE" -> null
            "ZERO" -> CaptureSource(org, CaptureKind.ENROLLED_DEVICE, "")
            else -> CaptureSource(alias(f.getOrDefault("capture_org", "A")), CaptureKind.parse(raw), alias(f.getOrDefault("id", "D")))
        }
        val reading = Reading(ReadingRef(org, segment, id(201)), MeterValue.of(MeterKind.ODOMETER, f.getOrDefault("value", "17").toLong()),
            TimeDeclaration(Instant.parse("2026-11-02T00:00:00Z"), "UTC", 0), Instant.parse("2026-11-03T00:00:00Z"),
            author, device, ReadingOrigin.STANDALONE, ReadingMethod.TYPED, operation,
            classification = ReadingState.parse(f.getOrDefault("state", "VALID")), capture = source)
        return output(reading.capture) + mapOf("value" to reading.value.units.toString(), "state" to reading.classification.name)
    }

    @TestFactory fun `all shared capture cases`(): List<DynamicTest> {
        val raw = file.readBytes()
        assertEquals("358c73f5b624e8529f5e130889fb34f3d4a3090232318449bd262fd6056e5020", MessageDigest.getInstance("SHA-256").digest(raw).joinToString("") { "%02x".format(it.toInt() and 255) })
        return load(raw).map { vector -> DynamicTest.dynamicTest(vector.id) {
            val actual = try { apply(vector) } catch (failure: MeterDomainException) { mapOf("error" to failure.failure.name) }
            assertEquals(vector.expected, actual)
        } }
    }

    @Test fun `fixture grammar cannot skip reinterpret or repair input`(): Unit {
        val original = file.readText()
        for (bad in listOf(original.replaceFirst("id|operation|args|expected", "wrong"), original + "\n",
            original.replaceFirst("source-web|SOURCE", "source-device|SOURCE"), original.replaceFirst("|SOURCE|", "|UNKNOWN|"),
            original.replaceFirst("org=A;kind=", "unknown=A;kind="), original.replaceFirst("kind=ENROLLED_DEVICE;org=A;id=D\n", "kind=ENROLLED_DEVICE;id=D\n"))) {
            assertThrows(IllegalArgumentException::class.java) { load(bad.toByteArray(Charsets.UTF_8)) }
        }
        assertThrows(java.nio.charset.CharacterCodingException::class.java) { load(byteArrayOf(0xff.toByte())) }
    }
}
