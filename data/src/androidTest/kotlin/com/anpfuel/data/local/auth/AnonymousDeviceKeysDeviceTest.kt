package com.anpfuel.data.local.auth

import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/**
 * P10-T03 — Device Keystore P-256 generation and frozen-profile proof.
 *
 * Runs on `connectedDebugAndroidTest` (API 26+ device/CI emulator): the
 * key is non-exportable, signs the SHA-512 digest, and the raw signature
 * verifies against the frozen profile. JVM unit tests never claim this;
 * they only replay the frozen vectors.
 */
@RunWith(AndroidJUnit4::class)
class AnonymousDeviceKeysDeviceTest {

    @Test
    fun keystoreKeySignsFrozenProfile() {
        val keys = AndroidAnonymousDeviceKeys()
        assertTrue(keys.ensureKey())
        assertTrue(keys.hasKey())
        val jwk = keys.publicJwk()
        assertTrue(jwk != null)
        val fingerprint = keys.fingerprint()
        assertTrue(fingerprint != null && fingerprint!!.startsWith("fp:"))
        val lines = listOf(
            "\"@method\": POST",
            "\"@authority\": api.example.invalid",
            "\"@path\": /v1/contributors",
            "\"@query\": ",
            "\"created\": 1735689600",
            "\"expires\": 1735689900",
            "\"keyid\": \"$fingerprint\"",
            "\"nonce\": \"ch-1.client-1\"",
        )
        val digest = AnonymousProofCrypto.sha512(
            lines.joinToString("\n").toByteArray(Charsets.UTF_8),
        )
        val raw = keys.signDigest(digest)
        assertTrue(raw != null && raw!!.size == 64)
        assertTrue(
            AnonymousProofCrypto.verify(
                jwk!!.first,
                jwk.second,
                lines,
                AnonymousProofCrypto.b64urlEncode(raw!!),
                1735689700L,
            ),
        )
        keys.forgetKey()
    }
}
