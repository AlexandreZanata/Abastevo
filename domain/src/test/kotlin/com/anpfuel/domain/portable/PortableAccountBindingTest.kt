package com.anpfuel.domain.portable

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class PortableAccountBindingTest {

    private val fingerprint = "fp:" + "0123456789abcdef".repeat(4)

    @Test
    fun `statement matches the backend key prover bind message`() {
        assertEquals(
            "anpfuel-bind.v1\nacc-1\ncontrib-1\n1798765432",
            PortableAccountBinding.statement("acc-1", "contrib-1", 1798765432L),
        )
    }

    @Test
    fun `proof joins fingerprint expiry and base64url signature`() {
        assertEquals(
            "$fingerprint.1798765432.c2ln-_A",
            PortableAccountBinding.proof(fingerprint, 1798765432L, "c2ln-_A"),
        )
    }

    @Test
    fun `proof refuses malformed parts`() {
        assertNull(PortableAccountBinding.proof("a".repeat(64), 1L, "c2ln"))
        assertNull(PortableAccountBinding.proof(fingerprint, 0L, "c2ln"))
        assertNull(PortableAccountBinding.proof(fingerprint, 1L, ""))
        assertNull(PortableAccountBinding.proof(fingerprint, 1L, "c2ln.AA"))
        assertNull(PortableAccountBinding.proof(fingerprint, 1L, "c2ln=="))
    }

    @Test
    fun `requested lifetime stays inside the server freshness window`() {
        assertTrue(PortableAccountBinding.PROOF_LIFETIME_SECONDS in 1 until PortableAccountBinding.PROOF_TTL_SECONDS)
    }
}
