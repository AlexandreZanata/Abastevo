package com.anpfuel.domain.enterprise.sync

import java.nio.ByteBuffer
import java.nio.charset.CodingErrorAction
import java.nio.file.Files
import java.nio.file.Path
import java.security.MessageDigest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestFactory

class EnterpriseReviewVectorsTest {
    private data class Vector(val id: String, val operation: String, val input: ByteArray,
        val context: ByteArray?, val outcome: String, val canonical: ByteArray?)
    private fun bytes(): ByteArray {
        val root = generateSequence(Path.of("").toAbsolutePath()) { it.parent }
            .map { it.resolve("contracts/testdata/enterprise/sync/review-v1.psv") }.first(Files::exists)
        return Files.readAllBytes(root)
    }
    private fun utf8(raw: ByteArray): String = Charsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT)
        .onUnmappableCharacter(CodingErrorAction.REPORT).decode(ByteBuffer.wrap(raw)).toString()
    private fun hex(raw: String): ByteArray {
        require(raw.isNotEmpty() && raw.length % 2 == 0 && raw.all { it in '0'..'9' || it in 'a'..'f' })
        return ByteArray(raw.length / 2) { raw.substring(it * 2, it * 2 + 2).toInt(16).toByte() }
    }
    private fun load(raw: ByteArray): List<Vector> {
        val lines = utf8(raw).split('\n')
        require(lines.size == 139 && lines.first() == "id|operation|input_hex|context_hex|outcome|canonical_hex" && lines.last().isEmpty())
        val ids = mutableSetOf<String>()
        val values = lines.subList(1, lines.lastIndex).map { line ->
            val fields = line.split('|')
            require(fields.size == 6 && fields[0].isNotEmpty() && ids.add(fields[0]))
            require(fields[1] in setOf("RESOLUTION", "REQUEST", "RESPONSE"))
            require(fields[4] in setOf("OK", "INVALID", "LIMIT", "BINDING", "DIGEST"))
            val context = if (fields[1] == "RESPONSE") hex(fields[3]) else { require(fields[3] == "-"); null }
            val canonical = if (fields[4] == "OK") hex(fields[5]) else { require(fields[5] == "-"); null }
            Vector(fields[0], fields[1], hex(fields[2]), context, fields[4], canonical)
        }
        require(values.groupingBy { it.operation }.eachCount() == mapOf("RESOLUTION" to 66, "REQUEST" to 33, "RESPONSE" to 38))
        return values
    }
    private fun response(vector: Vector): () -> ByteArray {
        val raw = requireNotNull(vector.context)
        val root = objectValue(CanonicalJson.parse(raw))
        require(CanonicalJson.encode(root).contentEquals(raw))
        exactKeys(root, setOf("request", "source", "expected"))
        val source = objectValue(root["source"])
        exactKeys(source, setOf("owner_account_id", "api_origin", "org_id", "membership_id", "enrollment_id"))
        val bound = ReviewSourceBinding(SyncBinding.of(text(source, "owner_account_id"), text(source, "api_origin"), text(source, "org_id")),
            text(source, "membership_id"), text(source, "enrollment_id"))
        val request = ReviewResultsRequest.parse(CanonicalJson.encode(requireNotNull(root["request"])))
        val expected = array(root, "expected").map {
            val item = objectValue(it); exactKeys(item, setOf("operation_id", "payload_digest", "review_item_id"))
            ReviewExpectation(ReviewSelector(text(item, "operation_id"), text(item, "payload_digest")), text(item, "review_item_id"))
        }
        return { ReviewResultsResponse.parse(vector.input, request, bound, expected).canonicalBytes }
    }
    @TestFactory fun shared(): List<DynamicTest> {
        val raw = bytes()
        val hash = MessageDigest.getInstance("SHA-256").digest(raw).joinToString("") { "%02x".format(it.toInt() and 255) }
        require(hash == "db1e7499590d6c5e226f77be4f5962ec43ad0411356fcb4c1cdecfb8a5683dd8")
        return load(raw).map { vector -> DynamicTest.dynamicTest(vector.id) {
            // Fixture context/dispatch errors stay outside the domain failure mapper.
            val execute: () -> ByteArray = when (vector.operation) {
                "RESOLUTION" -> { { ReviewResolution.parse(vector.input).canonicalBytes } }
                "REQUEST" -> { { ReviewResultsRequest.parse(vector.input).canonicalBytes } }
                "RESPONSE" -> response(vector)
                else -> error("invalid fixture namespace")
            }
            if (vector.outcome == "OK") assertArrayEquals(vector.canonical, execute())
            else {
                val failure = assertThrows(SyncProtocolException::class.java) { execute() }
                assertEquals(vector.outcome, failure.code)
            }
        } }
    }
    @Test fun loaderRejectsMalformedGrammarOutsideDomainMapper() {
        val original = utf8(bytes())
        val lines = original.split('\n').toMutableList()
        val first = lines[1].split('|'); val duplicate = lines[2].split('|').toMutableList()
        duplicate[0] = first[0]; lines[2] = duplicate.joinToString("|")
        val malformed = listOf(original.replaceFirst("id|operation", "id|kind"),
            original.replaceFirst("|RESOLUTION|", "|UNKNOWN|"), original.replaceFirst("\n", "\n\n"),
            original.replaceFirst("|7b", "|zb"), lines.joinToString("\n"))
        for (raw in malformed) assertThrows(IllegalArgumentException::class.java) { load(raw.toByteArray()) }
        assertThrows(java.nio.charset.CharacterCodingException::class.java) { load(byteArrayOf(0xff.toByte())) }
    }
}
