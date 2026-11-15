package com.anpfuel.domain.enterprise.fleet

import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.sync.CanonicalJson
import com.anpfuel.domain.enterprise.sync.SyncJson
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

/** Kotlin consumer of the shared reference feed oracle (EDR-227); the Go server consumes the same bytes. */
class ReferenceFeedVectorsTest {
    private data class Vector(val id: String, val operation: String, val input: ByteArray, val context: ByteArray?,
        val outcome: String, val expected: ByteArray?)

    private fun bytes(): ByteArray {
        val file = generateSequence(Path.of("").toAbsolutePath()) { it.parent }
            .map { it.resolve("contracts/testdata/enterprise/sync/reference-feed-v1.psv") }.first(Files::exists)
        return Files.readAllBytes(file)
    }

    private fun hex(raw: String): ByteArray {
        require(raw.isNotEmpty() && raw.length % 2 == 0 && raw.all { it in '0'..'9' || it in 'a'..'f' })
        return ByteArray(raw.length / 2) { raw.substring(it * 2, it * 2 + 2).toInt(16).toByte() }
    }

    private val outcomes = mapOf(
        "REQUEST" to setOf("OK", "INVALID", "LIMIT"),
        "RESPONSE" to setOf("OK", "INVALID", "LIMIT", "BINDING"),
        "APPLY" to setOf("OK", "INVALID", "BINDING"),
        "CACHE" to setOf("OK", "INVALID"),
    )

    private fun load(raw: ByteArray): List<Vector> {
        val lines = String(raw, StandardCharsets.UTF_8).split('\n')
        require(lines.size == 131 && lines.first() == "id|operation|input_hex|context_hex|outcome|expected_hex" && lines.last().isEmpty())
        val ids = mutableSetOf<String>()
        val values = lines.subList(1, lines.lastIndex).map { line ->
            val fields = line.split('|')
            require(fields.size == 6 && fields[0].isNotEmpty() && ids.add(fields[0]))
            require(fields[4] in requireNotNull(outcomes[fields[1]]))
            val context = if (fields[1] == "RESPONSE" || fields[1] == "APPLY") hex(fields[3]) else { require(fields[3] == "-"); null }
            val expected = if (fields[4] == "OK") hex(fields[5]) else { require(fields[5] == "-"); null }
            Vector(fields[0], fields[1], hex(fields[2]), context, fields[4], expected)
        }
        require(values.groupingBy { it.operation }.eachCount() == mapOf("REQUEST" to 23, "RESPONSE" to 79, "APPLY" to 16, "CACHE" to 11))
        for ((operation, allowed) in outcomes) for (outcome in allowed) {
            require(values.any { it.operation == operation && it.outcome == outcome }) { "no vector for $operation $outcome" }
        }
        return values
    }

    private fun field(root: SyncJson.Object, key: String): SyncJson = requireNotNull(root.fields[key])
    private fun objectOf(value: SyncJson): SyncJson.Object = value as SyncJson.Object
    private fun request(root: SyncJson.Object): ReferenceFeedRequest = ReferenceFeedRequest.parse(CanonicalJson.encode(field(root, "request")))
    private fun clock(value: SyncJson): EnterpriseDeviceClock {
        val fields = objectOf(value).fields
        return EnterpriseDeviceClock((fields.getValue("boot") as SyncJson.Text).value,
            (fields.getValue("elapsed_millis") as SyncJson.Integer).lexeme.toLong(),
            (fields.getValue("wall_millis") as SyncJson.Integer).lexeme.toLong())
    }

    private fun execute(vector: Vector): () -> ByteArray = when (vector.operation) {
        "REQUEST" -> { { ReferenceFeedRequest.parse(vector.input).canonicalBytes } }
        "RESPONSE" -> {
            val root = objectOf(CanonicalJson.parse(requireNotNull(vector.context)))
            require(root.fields.keys == setOf("held_revision", "request"))
            val held = field(root, "held_revision").let { if (it === SyncJson.Null) null else (it as SyncJson.Integer).lexeme.toLong() }
            val bound = request(root)
            ({ ReferenceFeedResponse.parse(vector.input, bound, held).canonicalBytes })
        }
        "APPLY" -> {
            val root = objectOf(CanonicalJson.parse(requireNotNull(vector.context)))
            require(root.fields.keys == setOf("cache", "received", "request"))
            val bound = request(root)
            val prior = field(root, "cache").let { if (it === SyncJson.Null) null else ReferenceCache.decode(CanonicalJson.encode(it)) }
            val received = clock(field(root, "received"))
            ({
                val response = ReferenceFeedResponse.parse(vector.input, bound, ReferenceCache.heldRevision(prior, bound))
                ReferenceCache.apply(prior, bound, response, received).canonicalBytes
            })
        }
        "CACHE" -> { { ReferenceCache.decode(vector.input).canonicalBytes } }
        else -> error("invalid fixture namespace")
    }

    @TestFactory fun shared(): List<DynamicTest> {
        val raw = bytes()
        val hash = MessageDigest.getInstance("SHA-256").digest(raw).joinToString("") { "%02x".format(it.toInt() and 255) }
        require(hash == "ec39616cd9d58d0b54bb7ed9129e023fe91f8cb868c8d4a8546cf73e0ffd2098")
        return load(raw).map { vector ->
            DynamicTest.dynamicTest(vector.id) {
                // Fixture context errors stay outside the domain failure mapper.
                val run = execute(vector)
                if (vector.outcome == "OK") assertArrayEquals(vector.expected, run())
                else assertEquals(vector.outcome, assertThrows(SyncProtocolException::class.java) { run() }.code)
            }
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
