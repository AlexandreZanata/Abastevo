package com.anpfuel.domain.enterprise.attachment

import com.anpfuel.domain.enterprise.sync.SyncProtocolException
import java.nio.charset.StandardCharsets
import java.nio.file.Files
import java.nio.file.Path
import java.security.MessageDigest
import org.junit.jupiter.api.Assertions.assertArrayEquals
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestFactory

/** Kotlin consumer of the shared attachment session oracle (EDR-270); the Go server consumes the same bytes. */
class AttachmentSessionVectorsTest {
    private data class Vector(val id: String, val operation: String, val input: ByteArray, val context: ByteArray?,
        val outcome: String, val expected: ByteArray?)

    private fun bytes(): ByteArray {
        val file = generateSequence(Path.of("").toAbsolutePath()) { it.parent }
            .map { it.resolve("contracts/testdata/enterprise/sync/attachment-session-v1.psv") }.first(Files::exists)
        return Files.readAllBytes(file)
    }

    private fun hex(raw: String): ByteArray {
        require(raw.isNotEmpty() && raw.length % 2 == 0 && raw.all { it in '0'..'9' || it in 'a'..'f' })
        return ByteArray(raw.length / 2) { raw.substring(it * 2, it * 2 + 2).toInt(16).toByte() }
    }

    private val outcomes = mapOf("REQUEST" to setOf("OK", "INVALID", "LIMIT"), "RESPONSE" to setOf("OK", "INVALID", "LIMIT", "BINDING"))

    private fun load(raw: ByteArray): List<Vector> {
        val lines = String(raw, StandardCharsets.UTF_8).split('\n')
        require(lines.size == 91 && lines.first() == "id|operation|input_hex|context_hex|outcome|expected_hex" && lines.last().isEmpty())
        val ids = mutableSetOf<String>()
        val values = lines.subList(1, lines.lastIndex).map { line ->
            val fields = line.split('|')
            require(fields.size == 6 && fields[0].isNotEmpty() && ids.add(fields[0]))
            require(fields[4] in requireNotNull(outcomes[fields[1]]))
            val context = if (fields[1] == "RESPONSE") hex(fields[3]) else { require(fields[3] == "-"); null }
            val expected = if (fields[4] == "OK") hex(fields[5]) else { require(fields[5] == "-"); null }
            Vector(fields[0], fields[1], hex(fields[2]), context, fields[4], expected)
        }
        require(values.groupingBy { it.operation + "/" + it.outcome }.eachCount() == mapOf("REQUEST/OK" to 15, "REQUEST/INVALID" to 37,
            "REQUEST/LIMIT" to 1, "RESPONSE/OK" to 11, "RESPONSE/INVALID" to 20, "RESPONSE/LIMIT" to 1, "RESPONSE/BINDING" to 4))
        return values
    }

    private fun execute(vector: Vector): () -> ByteArray = when (vector.operation) {
        "REQUEST" -> { { AttachmentSessionRequest.parse(vector.input).canonicalBytes } }
        "RESPONSE" -> {
            val bound = AttachmentSessionRequest.parse(requireNotNull(vector.context))
            ({ AttachmentSessionResponse.parse(vector.input, bound).canonicalBytes })
        }
        else -> error("invalid fixture namespace")
    }

    @TestFactory fun shared(): List<DynamicTest> {
        val raw = bytes()
        val hash = MessageDigest.getInstance("SHA-256").digest(raw).joinToString("") { "%02x".format(it.toInt() and 255) }
        require(hash == "1996b45fb008044a04df5715315ac0d82f82f5d942f36a8c64b89dbf4f170b9c")
        return load(raw).map { vector ->
            DynamicTest.dynamicTest(vector.id) {
                val run = execute(vector)
                if (vector.outcome == "OK") assertArrayEquals(vector.expected, run())
                else assertEquals(vector.outcome, assertThrows(SyncProtocolException::class.java) { run() }.code)
            }
        }
    }

    @Test fun requestBuilderProducesTheCanonicalRequestAndRefusesInvalidInput() {
        val vectors = load(bytes())
        val canonical = vectors.first { it.id == "request-meter-reading" }.expected!!
        val parsed = AttachmentSessionRequest.parse(canonical)
        assertArrayEquals(canonical, AttachmentSessionRequest.create(parsed.snapshot()).canonicalBytes)
        for (bad in listOf(parsed.snapshot().copy(kind = "INVOICE"), parsed.snapshot().copy(bytes = 0), parsed.snapshot().copy(linkId = parsed.snapshot().declarationId),
            parsed.snapshot().copy(accessToken = "a b"), parsed.snapshot().copy(targetKind = "meter_reading"), parsed.snapshot().copy(sha256 = "AB".repeat(32)))) {
            assertThrows(SyncProtocolException::class.java) { AttachmentSessionRequest.create(bad) }
        }
    }

    @Test fun loaderRejectsChangedGrammar() {
        val original = String(bytes(), StandardCharsets.UTF_8)
        val lines = original.split('\n').toMutableList()
        lines[1] = lines[1].replaceFirst("|REQUEST|", "|UNKNOWN|")
        assertThrows(IllegalArgumentException::class.java) { load(lines.joinToString("\n").toByteArray()) }
        val duplicate = original.split('\n').toMutableList().also { it[2] = it[1] }
        assertThrows(IllegalArgumentException::class.java) { load(duplicate.joinToString("\n").toByteArray()) }
    }
}
