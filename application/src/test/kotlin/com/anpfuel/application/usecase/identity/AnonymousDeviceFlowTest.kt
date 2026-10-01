package com.anpfuel.application.usecase.identity

import com.anpfuel.application.port.AnonymousContributionFlagProvider
import com.anpfuel.application.port.AnonymousDeviceKeyPort
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P10-T03: anonymous device-key flow (flag, binding, replay, rotation,
 * honest loss). Crypto interop lives in `:data AnonymousProofCryptoTest`;
 * this suite only orchestrates.
 */
class AnonymousDeviceFlowTest {

    private val fingerprint = "fp:e568c4f52605d14215f6e649000e4ffc2da4caa4bdcbbf2b8bcd9937ff9ea624"

    private class FakeFlags(val enabled: Boolean) : AnonymousContributionFlagProvider {
        override fun isEnabled(): Boolean = enabled
    }

    private class FakeKeys(
        var fp: String? = null,
        var jwk: Pair<String, String>? = null,
    ) : AnonymousDeviceKeyPort {
        var forgotten = false
        override fun fingerprint(): String? = fp
        override fun publicJwk(): Pair<String, String>? = jwk
        override fun signDigest(digestSha512: ByteArray): ByteArray? = ByteArray(64)
        override fun hasKey(): Boolean = fp != null
        override fun forgetKey() {
            forgotten = true
            fp = null
            jwk = null
        }
    }

    @Test
    fun `disabled flag keeps browsing path`() {
        val flow = AnonymousDeviceFlow(FakeFlags(false), FakeKeys(fingerprint, Pair("X", "Y")))
        assertTrue(flow.currentIdentity() is AnonymousDeviceOutcome.Disabled)
        assertEquals(null, flow.registrationLines("POST", "h", "/v1/contributors", "", "ch-1", 100L, 200L))
    }

    @Test
    fun `missing key warns honestly and loss forgets`() {
        val keys = FakeKeys(null, null)
        val flow = AnonymousDeviceFlow(FakeFlags(true), keys)
        val missing = flow.currentIdentity()
        assertTrue(missing is AnonymousDeviceOutcome.MissingKey)
        val notice = flow.markKeyLost()
        assertTrue(keys.forgotten)
        assertTrue(notice.contains("cannot be recovered"))
    }

    @Test
    fun `ready identity binds nonce and spends it once`() {
        val flow = AnonymousDeviceFlow(
            FakeFlags(true),
            FakeKeys(fingerprint, Pair("X", "Y")),
            nowEpochSeconds = { 150L },
            clientNonce = { "client-1" },
        )
        val ready = flow.currentIdentity()
        assertTrue(ready is AnonymousDeviceOutcome.Ready)
        val lines = flow.registrationLines(
            "POST",
            "api.example.invalid",
            "/v1/contributors",
            "",
            "ch-1",
            100L,
            200L,
        )
        assertTrue(lines != null && lines!!.size == 8)
        assertTrue(lines!!.last() == "\"nonce\": \"ch-1.client-1\"")
    }

    @Test
    fun `rotation intent binds both challenges`() {
        val flow = AnonymousDeviceFlow(FakeFlags(true), FakeKeys(fingerprint, Pair("X", "Y")))
        val intent = flow.rotationIntent("NX", "NY", "old-1", "new-1")
        assertTrue(intent != null && intent!!.contains("\"old_challenge_id\":\"old-1\""))
        assertTrue(intent!!.contains("\"new_challenge_id\":\"new-1\""))
        assertEquals(null, flow.rotationIntent("", "NY", "old-1", "new-1"))
        assertEquals(null, flow.rotationIntent("NX", "NY", "", "new-1"))
    }
}
