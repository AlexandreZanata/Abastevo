package com.anpfuel.data.map

import androidx.test.ext.junit.runners.AndroidJUnit4
import java.security.KeyPairGenerator
import java.security.Signature
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Proves the owned Monocypher bridge verifies exact Ed25519 bytes on-device,
 * including Go-signed fixtures. This is the API-26 fallback path: the test
 * fails if the JNI library is missing or the verification disagrees with the
 * platform provider.
 */
@RunWith(AndroidJUnit4::class)
class MonocypherEd25519InstrumentedTest {
    @Test
    fun monocypherVerifiesGoSignedBytesAndRejectsMutations() {
        assertTrue(MonocypherEd25519.isAvailable())

        // Go `crypto/ed25519` fixture with seed 00..1f (see MAP-15 evidence).
        val pub = hex("03a107bff3ce10be1d70dd18e74bc09967e4d6309ba50d5f1ddc8664125531b8")
        val msg = "abastevo-map-test".toByteArray()
        val sig = hex("61f67425cf4fdbeb11c0186788866fe375c7054762f5b0ffc7fa0821718297eea58c9bddc27dba1e4da228c27c7f54628caaea4f3feeee4c8658673646c0ec09")
        assertTrue(MonocypherEd25519.verifyRaw(pub, msg, sig))

        // Empty-message Go fixture with the same seed.
        val emptySig = hex("9ca53579530654d5c3df77089ef45eda613e2fedf670e96bedac4639504e5845ef4b95d5793077233dd16817b2532e9c5525872a73a4ad74b759369a9e05c102")
        assertTrue(MonocypherEd25519.verifyRaw(pub, ByteArray(0), emptySig))

        // Mutations fail closed.
        val badSig = sig.copyOf().also { it[0] = (it[0].toInt() xor 1).toByte() }
        assertFalse(MonocypherEd25519.verifyRaw(pub, msg, badSig))
        val badMsg = "abastevo-map-tesu".toByteArray()
        assertFalse(MonocypherEd25519.verifyRaw(pub, badMsg, sig))
        val badKey = pub.copyOf().also { it[0] = (it[0].toInt() xor 1).toByte() }
        assertFalse(MonocypherEd25519.verifyRaw(badKey, msg, sig))

        // Sizes fail closed.
        assertFalse(MonocypherEd25519.verifyRaw(ByteArray(16), msg, sig))
        assertFalse(MonocypherEd25519.verifyRaw(pub, msg, ByteArray(32)))
        assertFalse(MonocypherEd25519.verifyRaw(ByteArray(0), msg, sig))

        // Platform round-trip agrees with Monocypher on-device.
        val keys = KeyPairGenerator.getInstance("Ed25519").generateKeyPair()
        val platformMsg = "abastevo-map-instrumented".toByteArray()
        val platformSig = Signature.getInstance("Ed25519").run {
            initSign(keys.private)
            update(platformMsg)
            sign()
        }
        assertTrue(
            Signature.getInstance("Ed25519").run {
                initVerify(keys.public)
                update(platformMsg)
                verify(platformSig)
            },
        )
        val raw = MonocypherEd25519.rawPublicKey(keys.public)!!
        assertTrue(MonocypherEd25519.verifyRaw(raw, platformMsg, platformSig))
        val mutatedPlatform = platformSig.copyOf().also { it[10] = (it[10].toInt() xor 2).toByte() }
        assertFalse(MonocypherEd25519.verifyRaw(raw, platformMsg, mutatedPlatform))
    }

    private fun hex(s: String): ByteArray {
        val out = ByteArray(s.length / 2)
        for (i in out.indices) out[i] = s.substring(i * 2, i * 2 + 2).toInt(16).toByte()
        return out
    }
}
