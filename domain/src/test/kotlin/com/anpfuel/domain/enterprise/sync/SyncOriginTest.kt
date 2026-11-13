package com.anpfuel.domain.enterprise.sync

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class SyncOriginTest {
    private val account = "7b0e2a54-0000-4000-8000-000000000001"
    private val org = "7b0e2a54-0000-4000-8000-000000000002"
    @Test fun `binding accepts only captured canonical HTTPS origins without rewriting`() {
        listOf("https://example.test", "https://example.test.", "https://127.0.0.1", "https://[::1]", "https://[0:0:0:0:0:0:0:1]:8443", "https://example.test:1", "https://example.test:65535").forEach {
            assertEquals(it, SyncBinding.of(account, it, org).apiOrigin)
        }
        listOf("https://EXAMPLE.test", "HTTPS://example.test", "https://example.test/", "https://example.test:443", "https://example.test:08443", "https://example.test:0", "https://example.test:65536", "https://例.test", "https://example_test", "https://-bad.test", "https://bad-.test", "https://127.00.0.1", "https://999.0.0.1", "https://1.2.3", "https://[::AB]", "https://[fe80::1%25eth0]", "https://example.test?x", "https://user@example.test", "https://example.test#x", "https://example.test/path").forEach {
            assertEquals("BINDING", assertThrows(SyncProtocolException::class.java, { SyncBinding.of(account, it, org) }, it).code)
        }
        assertEquals("BINDING", assertThrows(SyncProtocolException::class.java) { SyncBinding.of("invalid", "https://example.test", org) }.code)
    }
}
