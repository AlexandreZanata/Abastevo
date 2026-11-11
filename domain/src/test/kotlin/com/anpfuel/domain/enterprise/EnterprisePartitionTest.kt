package com.anpfuel.domain.enterprise

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterprisePartitionTest {
    private val account = "7b0e2a54-0000-4000-8000-000000000001"
    private val org = "7b0e2a54-0000-4000-8000-000000000002"

    @Test
    fun `partition normalizes origin but never owner or organization`() {
        val a = EnterprisePartition.of(account, "https://API.example.invalid:443/", org)
        val b = EnterprisePartition.of(account, "https://api.example.invalid", org)
        assertEquals(a, b)
        assertEquals(a.hashCode(), b.hashCode())
        assertEquals("https://api.example.invalid", a.origin)
        assertNotEquals(a, EnterprisePartition.of(org, a.origin, account))
        assertNotEquals(a, EnterprisePartition.of(account, "https://other.example.invalid", org))
        assertNotEquals(a, EnterprisePartition.of(account, "https://api.example.invalid:8443", org))
        assertFalse(a.toString().contains(account))
        assertFalse(a.toString().contains(org))
    }

    @Test
    fun `malformed authority and noncanonical identifiers fail without echoing input`() {
        for (origin in listOf("", "http://api.example.invalid", "https://user:secret@api.example.invalid",
            "https://api.example.invalid/v1", "https://api.example.invalid?token=secret",
            "https://api.example.invalid/#secret", "https://api.example.invalid:99999",
            "https://api.example.invalid\\path", "https://api.example.invalid/%2f")) {
            val error = assertThrows(IllegalArgumentException::class.java) { EnterprisePartition.of(account, origin, org) }
            assertFalse(error.message.orEmpty().contains("secret"))
        }
        for (bad in listOf("", "private-account", account.uppercase(), "00000000-0000-0000-0000-000000000000")) {
            assertThrows(IllegalArgumentException::class.java) { EnterprisePartition.of(bad, "https://api.example.invalid", org) }
            assertThrows(IllegalArgumentException::class.java) { EnterprisePartition.of(account, "https://api.example.invalid", bad) }
        }
    }

    @Test
    fun `stored values snapshot opaque envelopes and redact references`() {
        val partition = EnterprisePartition.of(account, "https://api.example.invalid", org)
        val bytes = byteArrayOf(1,2,3)
        val record = EnterpriseLocalRecord(partition, account, bytes)
        val operation = EnterprisePendingOperation(partition, org, account, bytes)
        bytes[0] = 9
        record.envelope[1] = 9
        operation.envelope[2] = 9
        assertArrayEquals(byteArrayOf(1,2,3), record.envelope)
        assertArrayEquals(byteArrayOf(1,2,3), operation.envelope)
        for (value in listOf(record, operation)) {
            assertFalse(value.toString().contains(account))
            assertFalse(value.toString().contains(org))
        }
        assertThrows(IllegalArgumentException::class.java) { EnterpriseLocalRecord(partition, "", bytes) }
        assertThrows(IllegalArgumentException::class.java) { EnterprisePendingOperation(partition, org, "", bytes) }
        assertThrows(IllegalArgumentException::class.java) { EnterprisePendingOperation(partition, "", account, bytes) }
    }
}
