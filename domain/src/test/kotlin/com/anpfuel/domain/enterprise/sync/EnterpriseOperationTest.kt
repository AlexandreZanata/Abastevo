package com.anpfuel.domain.enterprise.sync

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseOperationTest {
    private val account = "7b0e2a54-0000-4000-8000-000000000001"
    private val org = "7b0e2a54-0000-4000-8000-000000000002"
    private val operation = "7b0e2a54-0000-4000-8000-000000000003"
    private val binding get() = SyncBinding.of(account, "https://example.test", org)

    @Test fun `intent seal binds complete operation and digest survives caller byte mutation`() {
        val raw = input().toByteArray()
        val sealed = EnterpriseOperation.seal(raw, binding)
        raw.fill(0)
        assertEquals(operation, sealed.operationId)
        assertTrue(sealed.supportsSchema)
        assertEquals(1L, sealed.operationSchema)
        assertTrue(sealed.payloadDigest.matches(Regex("sha256:[0-9a-f]{64}")))
        val encoded = sealed.canonicalBytes
        val copy = EnterpriseOperation.parse(encoded, binding)
        encoded.fill(0)
        assertTrue(sealed.sameIntent(copy))
        assertEquals(sealed.payloadDigest, copy.payloadDigest)
        assertFalse(sealed.toString().contains(account))
        assertEquals("BINDING", assertThrows(SyncProtocolException::class.java) {
            EnterpriseOperation.parse(copy.canonicalBytes, SyncBinding.of(org, "https://example.test", org))
        }.code)
        val changed = EnterpriseOperation.seal(input("{\"a\":2}").toByteArray(), binding)
        assertEquals("REUSED", assertThrows(SyncProtocolException::class.java) { copy.sameIntent(changed) }.code)
        assertEquals("DIGEST", assertThrows(SyncProtocolException::class.java) {
            EnterpriseOperation.parse(String(copy.canonicalBytes).replace("\"a\":1", "\"a\":2").toByteArray(), binding)
        }.code)
    }

    @Test fun `schema is structural time remains declared and dependencies are immutable sets`() {
        val future = EnterpriseOperation.seal(input().replace("\"operation_schema\":1", "\"operation_schema\":2").toByteArray(), binding)
        assertFalse(future.supportsSchema)
        val mismatch = input().replace("\"device_offset_seconds\":-14400", "\"device_offset_seconds\":64800")
        assertNotNull(EnterpriseOperation.seal(mismatch.toByteArray(), binding))
        val invalids = listOf(
            input().replace("2026-11-13T12:00:00Z", "2026-11-13T12:00:00.0Z"),
            input().replace("America/Cuiaba", "Mars/Olympus"),
            input().replace("-14400", "64801"),
            input().replace("\"depends_on\":[]", "\"depends_on\":[\"$operation\"]"),
            input().replace("\"payload\":{\"a\":1}", "\"payload\":[]"),
            input().replace("\"schema_version\":1", "\"schema_version\":2"),
            input().replace("\"expected_revision\":null", "\"expected_revision\":0"),
        )
        invalids.forEach { assertThrows(SyncProtocolException::class.java, { EnterpriseOperation.seal(it.toByteArray(), binding) }, it) }
    }

    @Test fun `ACK finality comes from result and original intent is correlated`() {
        val intent = EnterpriseOperation.seal(input().toByteArray(), binding)
        val accepted = acknowledgement(intent.payloadDigest)
        val ack = OperationAcknowledgement.parse(accepted.toByteArray(), intent)
        assertTrue(ack.isFinal)
        assertTrue(ack.matches(intent))
        assertFalse(OperationAcknowledgement.parse(accepted.replace("ACCEPTED", "RETRY").replace("\"code\":null", "\"code\":\"UNAVAILABLE\"").replace("\"entity_revision\":1", "\"entity_revision\":null").replace("\"received_at\":\"2026-11-13T12:00:01Z\"", "\"received_at\":null").toByteArray(), intent).isFinal)
        assertThrows(SyncProtocolException::class.java) { OperationAcknowledgement.parse(accepted.replace("\"code\":null", "\"code\":\"UNAVAILABLE\"").toByteArray(), intent) }
        assertThrows(SyncProtocolException::class.java) { OperationAcknowledgement.parse(accepted.replace("\"warnings\":[]", "\"warnings\":[],\"final\":true").toByteArray(), intent) }
        assertEquals("DIGEST", assertThrows(SyncProtocolException::class.java) { OperationAcknowledgement.parse(accepted.replace(intent.payloadDigest, "sha256:" + "0".repeat(64)).toByteArray(), intent) }.code)
    }

    @Test fun `every immutable source field contributes to intent identity`() {
        val original = input().replace("\"depends_on\":[]", "\"depends_on\":[\"7b0e2a54-0000-4000-8000-000000000020\"]")
            .replace("\"soft_references\":[]", "\"soft_references\":[{\"type\":\"asset\",\"id\":\"7b0e2a54-0000-4000-8000-000000000021\"}]")
            .replace("\"attachments\":[]", "\"attachments\":[{\"id\":\"7b0e2a54-0000-4000-8000-000000000022\",\"sha256\":\"${"a".repeat(64)}\",\"bytes\":1,\"mime\":\"image/jpeg\"}]")
        val base = EnterpriseOperation.seal(original.toByteArray(), binding)
        val changes = listOf(
            operation to "7b0e2a54-0000-4000-8000-000000000030",
            "000000000004" to "000000000034", "000000000005" to "000000000035",
            "trip.start" to "trip.finish", "\"operation_schema\":1" to "\"operation_schema\":9223372036854775807",
            "\"type\":\"trip\"" to "\"type\":\"asset\"", "000000000006" to "000000000036",
            "\"expected_revision\":null" to "\"expected_revision\":9223372036854775807",
            "12:00:00Z" to "12:00:00.000000001Z", "America/Cuiaba" to "UTC", "-14400" to "0",
            "000000000020" to "000000000040", "\"type\":\"asset\"" to "\"type\":\"employee\"",
            "000000000021" to "000000000041", "000000000022" to "000000000042",
            "a".repeat(64) to "b".repeat(64), "\"bytes\":1" to "\"bytes\":9223372036854775807",
            "image/jpeg" to "image/png", "\"a\":1" to "\"a\":9223372036854775808",
        )
        changes.forEach { (old, new) ->
            assertTrue(original.contains(old))
            assertNotEquals(base.payloadDigest, EnterpriseOperation.seal(original.replace(old, new).toByteArray(), binding).payloadDigest, old)
        }
        listOf(
            Triple(account, "7b0e2a54-0000-4000-8000-000000000050", SyncBinding.of("7b0e2a54-0000-4000-8000-000000000050", "https://example.test", org)),
            Triple(org, "7b0e2a54-0000-4000-8000-000000000051", SyncBinding.of(account, "https://example.test", "7b0e2a54-0000-4000-8000-000000000051")),
            Triple("https://example.test", "https://other.test", SyncBinding.of(account, "https://other.test", org)),
        ).forEach { (old, new, scope) -> assertNotEquals(base.payloadDigest, EnterpriseOperation.seal(original.replace(old, new).toByteArray(), scope).payloadDigest) }
    }

    internal fun input(payload: String = "{\"a\":1}"): String = """{"schema_version":1,"operation_id":"$operation","owner_account_id":"$account","api_origin":"https://example.test","org_id":"$org","membership_id":"7b0e2a54-0000-4000-8000-000000000004","enrollment_id":"7b0e2a54-0000-4000-8000-000000000005","type":"trip.start","operation_schema":1,"entity":{"type":"trip","id":"7b0e2a54-0000-4000-8000-000000000006","expected_revision":null},"occurred_at":"2026-11-13T12:00:00Z","entry_zone":"America/Cuiaba","device_offset_seconds":-14400,"depends_on":[],"soft_references":[],"attachments":[],"payload":$payload}"""

    private fun acknowledgement(digest: String): String = """{"schema_version":1,"operation_id":"$operation","payload_digest":"$digest","result":"ACCEPTED","code":null,"entity_revision":1,"received_at":"2026-11-13T12:00:01Z","review_item_id":null,"warnings":[]}"""
}
