package com.anpfuel.domain.enterprise.sync

import java.io.File
import java.security.MessageDigest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.TestFactory

class EnterpriseSyncVectorsTest {
    @TestFactory fun `consume every independent canonical fixture`(): List<DynamicTest> {
        val path = listOf(File("contracts/testdata/enterprise/sync/canonical-v1.psv"), File("../contracts/testdata/enterprise/sync/canonical-v1.psv")).first { it.isFile }
        val rows = fixture(path, 58, mapOf("CANONICAL" to 58))
        return rows.map { row -> DynamicTest.dynamicTest(row[0]) {
            val args = fields(row[2])
            val actual = try {
                val canonical = CanonicalJson.encode(CanonicalJson.parse(unhex(args.getValue("raw_hex"))))
                mapOf("canonical_hex" to hex(canonical), "sha256" to hex(MessageDigest.getInstance("SHA-256").digest(canonical)))
            } catch (failure: SyncProtocolException) { mapOf("error" to failure.code) }
            assertEquals(fields(row[3]), actual)
        } }
    }

    @TestFactory fun `consume every independent operation fixture`(): List<DynamicTest> {
        val path = listOf(File("contracts/testdata/enterprise/sync/operations-v1.psv"), File("../contracts/testdata/enterprise/sync/operations-v1.psv")).first { it.isFile }
        val rows = fixture(path, 202, mapOf("SEAL" to 69, "VERIFY" to 18, "ORIGIN" to 43, "BATCH" to 17, "SCAN" to 4, "ACK" to 48, "REPLAY" to 3))
        return rows.map { row -> DynamicTest.dynamicTest(row[0]) {
            val actual = try { execute(row[1], fields(row[2])) } catch (failure: SyncProtocolException) { mapOf("error" to failure.code) }
            assertEquals(fields(row[3]), actual)
        } }
    }

    private fun fixture(path: File, count: Int, operations: Map<String, Int>): List<List<String>> {
        val lines = path.readLines()
        require(lines.firstOrNull() == "id|operation|args|expected")
        val rows = lines.drop(1).map { line ->
            require(line.isNotEmpty())
            line.split('|').also { require(it.size == 4 && it[0].isNotEmpty()) }
        }
        assertEquals(count, rows.size)
        assertEquals(rows.size, rows.map { it[0] }.toSet().size)
        assertEquals(operations, rows.groupingBy { it[1] }.eachCount())
        return rows
    }

    private fun binding(alias: String): SyncBinding = SyncBinding.of(
        "7b0e2a54-0000-4000-8000-00000000000" + if (alias == "OWNER_B") "2" else "1",
        if (alias == "ORIGIN_B") "https://other.example.invalid" else "https://api.example.invalid",
        "7b0e2a54-0000-4000-8000-00000000006" + if (alias == "ORG_B") "6" else "5",
    ).also { require(alias in setOf("A", "OWNER_B", "ORG_B", "ORIGIN_B")) }

    private fun execute(operation: String, args: Map<String, String>): Map<String, String> = when (operation) {
        "ORIGIN" -> { canonicalOrigin(String(unhex(args.getValue("origin_hex")), Charsets.UTF_8)); mapOf("valid" to "true") }
        "SEAL", "VERIFY" -> {
            val raw = unhex(args.getValue("raw_hex"))
            val scope = binding(args.getValue("binding"))
            val value = if (operation == "SEAL") EnterpriseOperation.seal(raw, scope) else EnterpriseOperation.parse(raw, scope)
            mapOf("canonical_hex" to hex(value.canonicalBytes), "payload_digest" to value.payloadDigest, "supported" to value.supportsSchema.toString())
        }
        "BATCH" -> mapOf("operations" to SyncBatch.parse(unhex(args.getValue("raw_hex"))).operations.size.toString())
        "SCAN" -> {
            val scope = binding(args.getValue("binding"))
            val batch = SyncBatch.parse(unhex(args.getValue("raw_hex")))
            mapOf("outcomes" to batch.operations.joinToString(",") { candidate ->
                try { if (candidate.parse(scope).supportsSchema) "OK" else "UNSUPPORTED" } catch (failure: SyncProtocolException) { failure.code }
            })
        }
        "ACK" -> {
            val expected = EnterpriseOperation.parse(unhex(args.getValue("operation_hex")), binding(args.getValue("binding")))
            val ack = OperationAcknowledgement.parse(unhex(args.getValue("raw_hex")), expected)
            mapOf("final" to ack.isFinal.toString(), "result" to ack.result.name, "code" to (ack.code ?: "?"))
        }
        "REPLAY" -> {
            val scope = binding(args.getValue("binding"))
            val first = EnterpriseOperation.parse(unhex(args.getValue("first_hex")), scope)
            val second = EnterpriseOperation.parse(unhex(args.getValue("second_hex")), scope)
            mapOf("same" to first.sameIntent(second).toString())
        }
        else -> error("Unhandled shared operation")
    }

    private fun fields(raw: String): Map<String, String> {
        val pairs = raw.split(';').map { it.split('=', limit = 2).also { pair -> require(pair.size == 2) } }
        require(pairs.map { it[0] }.toSet().size == pairs.size)
        return pairs.associate { it[0] to it[1] }
    }
    private fun unhex(raw: String): ByteArray {
        require(raw.length % 2 == 0 && raw.all { it in '0'..'9' || it in 'a'..'f' })
        return raw.chunked(2).map { it.toInt(16).toByte() }.toByteArray()
    }
    private fun hex(raw: ByteArray): String = raw.joinToString("") { "%02x".format(it.toInt() and 255) }
}
