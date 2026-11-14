package com.anpfuel.domain.enterprise.sync

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class SyncBatchTest {
    private val id = "7b0e2a54-0000-4000-8000-000000000003"
    private val family = "7b0e2a54-0000-4000-8000-000000000010"
    private fun wrapper(operations: String, token: String = "replaceable-proof"): String = """{"schema_version":1,"family_id":"$family","access_token":"$token","operations":[$operations]}"""

    @Test fun `delivery wrapper preserves raw candidates without activating malformed operations`() {
        val raw = " { \"operation_id\" : \"$id\", \"private_value\": \"é😀\" } "
        val batch = SyncBatch.parse(wrapper("$raw,42").toByteArray())
        assertEquals(2, batch.operations.size)
        assertEquals(raw.trim(), String(batch.operations[0].rawBytes))
        assertEquals(family, batch.familyId)
        assertEquals("replaceable-proof", batch.accessToken)
        assertFalse(batch.toString().contains(batch.accessToken))
        val bytes = batch.operations[0].rawBytes
        bytes.fill(0)
        assertEquals(raw.trim(), String(batch.operations[0].rawBytes))
        assertThrows(SyncProtocolException::class.java) { SyncBatch.parse(wrapper("$raw,$raw").toByteArray()) }
        assertThrows(SyncProtocolException::class.java) { SyncBatch.parse(wrapper("0", " ").toByteArray()) }
        assertThrows(SyncProtocolException::class.java) { SyncBatch.parse(wrapper("0", "a".repeat(513)).toByteArray()) }
        assertEquals(50, SyncBatch.parse(wrapper(List(50) { "null" }.joinToString(",")).toByteArray()).operations.size)
        assertEquals("LIMIT", assertThrows(SyncProtocolException::class.java) { SyncBatch.parse(wrapper(List(51) { "null" }.joinToString(",")).toByteArray()) }.code)
    }
    @Test fun `delivery renewal cannot change creation identity or widen captured operation limits`() {
        val account = "7b0e2a54-0000-4000-8000-000000000001"
        val org = "7b0e2a54-0000-4000-8000-000000000002"
        val binding = SyncBinding.of(account, "https://example.test", org)
        val intent = EnterpriseOperation.seal(EnterpriseOperationTest().input().toByteArray(), binding)
        val encoded = String(intent.canonicalBytes, Charsets.UTF_8)
        val first = SyncBatch.parse(wrapper(encoded).toByteArray()).operations.single().parse(binding)
        val renewedDelivery = wrapper(encoded, "renewed-proof").replace(family, "7b0e2a54-0000-4000-8000-000000000011")
        val renewed = SyncBatch.parse(renewedDelivery.toByteArray()).operations.single().parse(binding)
        assertTrue(first.sameIntent(renewed))
        assertArrayEquals(first.canonicalBytes, renewed.canonicalBytes)
        val narrower = SyncLimits.trusted(maxOperationBytes = intent.canonicalBytes.size - 1)
        val captured = SyncBatch.parse(wrapper(encoded).toByteArray(), narrower).operations.single()
        assertEquals("LIMIT", assertThrows(SyncProtocolException::class.java) { captured.parse(binding) }.code)
        assertEquals("BINDING", assertThrows(SyncProtocolException::class.java) { first.let { EnterpriseOperation.parse(it.canonicalBytes, SyncBinding.of(org, "https://example.test", org)) } }.code)
    }

}
