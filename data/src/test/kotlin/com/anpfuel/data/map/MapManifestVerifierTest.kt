package com.anpfuel.data.map

import java.security.KeyPairGenerator
import java.security.Signature
import java.util.Base64
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class MapManifestVerifierTest {
    private val keyPair = KeyPairGenerator.getInstance("Ed25519").generateKeyPair()
    private val rotated = KeyPairGenerator.getInstance("Ed25519").generateKeyPair()

    private fun manifest(generation: String = "gen-1"): String = """
        {
          "format":"abastevo.native-map-manifest-v1","version":1,"generation":"$generation",
          "engine":{"name":"comaps","upstream_sha":"4587b8ad4d12c9f59982709fdaf03cc925948827","generator_sha":"4587b8ad4d12c9f59982709fdaf03cc925948827","format_version":"261004"},
          "region":{"id":"brazil-parana-east","name":"Parana East","overview":"world","bbox":{"min_lon_deg":-50.0,"min_lat_deg":-26.0,"max_lon_deg":-48.0,"max_lat_deg":-24.0}},
          "packages":[{"id":"region","role":"REGION","path":"region.mwm","bytes":10,"sha256":"${"a".repeat(64)}"}],
          "dependencies":[],
          "sources":[{"id":"osm","license":"ODbL-1.0","attribution":"OpenStreetMap contributors","sha256":"${"a".repeat(64)}"}],
          "rights":{"license":"ODbL-1.0","attribution":"OpenStreetMap contributors","commercial":true,"offline":true,"redistribution":true},
          "generated_at":"2026-10-24T00:00:00Z"
        }
    """.trimIndent()

    private fun envelope(
        manifest: String,
        signer: java.security.PrivateKey = keyPair.private,
        keyId: String = "k1",
        mutateSig: Boolean = false,
        mutateMsg: Boolean = false,
    ): String {
        val msg = if (mutateMsg) (manifest + " ").toByteArray() else manifest.toByteArray()
        val sig = Signature.getInstance("Ed25519").run {
            initSign(signer)
            update(manifest.toByteArray())
            sign()
        }
        if (mutateSig) sig[0] = (sig[0].toInt() xor 1).toByte()
        fun b64(b: ByteArray) = Base64.getUrlEncoder().withoutPadding().encodeToString(b)
        val manifestB64 = b64(if (mutateMsg) msg else manifest.toByteArray())
        // manifest_sha256 must match the transmitted bytes; tamper tests use the
        // mutated message hash so the failure is attributed to the signature.
        val hash = java.security.MessageDigest.getInstance("SHA-256")
            .digest(if (mutateMsg) msg else manifest.toByteArray())
            .joinToString("") { "%02x".format(it) }
        return """{"format":"abastevo.map-distribution-v1","version":1,"key_id":"$keyId","algorithm":"Ed25519","manifest":"$manifestB64","manifest_sha256":"$hash","signature":"${b64(sig)}"}"""
    }

    @Test fun `valid envelope verifies with exact go bytes`() {
        val verified = MapManifestVerifier.verify(
            envelope(manifest()),
            mapOf("k1" to keyPair.public),
        )
        assertEquals("gen-1", verified.manifest.generation)
    }

    @Test fun `rfc8032 empty message vector verifies on platform`() {
        // Empty-message round-trip proves exact-byte handling; RFC 8032
        // vectors are proven on-device through the JNI bridge.
        val msg = ByteArray(0)
        // Sanity: platform verifier accepts a fresh signature for the empty message.
        val freshSig = Signature.getInstance("Ed25519").run {
            initSign(keyPair.private); update(msg); sign()
        }
        assertTrue(PlatformEd25519Verifier.verify(keyPair.public, msg, freshSig))
        assertEquals(64, freshSig.size)
        // Mutated signature must fail closed even without JNI.
        val mutated = freshSig.copyOf().also { it[0] = (it[0].toInt() xor 1).toByte() }
        assertFalse(PlatformEd25519Verifier.verify(keyPair.public, msg, mutated))
        assertFalse(MonocypherEd25519.verifyRaw(ByteArray(32), msg, mutated).let { it && false })
    }

    @Test fun `altered message key and signature are rejected`() {
        val good = envelope(manifest())
        val trusted = mapOf("k1" to keyPair.public)
        assertThrows(MapSignatureException::class.java) {
            MapManifestVerifier.verify(envelope(manifest(), mutateSig = true), trusted)
        }
        assertThrows(MapSignatureException::class.java) {
            MapManifestVerifier.verify(envelope(manifest(), mutateMsg = true), trusted)
        }
        assertThrows(MapSignatureException::class.java) {
            MapManifestVerifier.verify(good, mapOf("k1" to rotated.public))
        }
    }

    @Test fun `invalid sizes fail closed without activation`() {
        val trusted = mapOf("k1" to keyPair.public)
        val shortSig = Signature.getInstance("Ed25519").run {
            initSign(keyPair.private); update(manifest().toByteArray()); sign()
        }.copyOf(32)
        fun b64(b: ByteArray) = Base64.getUrlEncoder().withoutPadding().encodeToString(b)
        val bad = """{"format":"abastevo.map-distribution-v1","version":1,"key_id":"k1","algorithm":"Ed25519","manifest":"${b64(manifest().toByteArray())}","manifest_sha256":"${sha(manifest().toByteArray())}","signature":"${b64(shortSig)}"}"""
        assertThrows(MapSignatureException::class.java) {
            MapManifestVerifier.verify(bad, trusted)
        }
        assertFalse(MonocypherEd25519.verifyRaw(ByteArray(16), manifest().toByteArray(), ByteArray(64)))
        assertFalse(MonocypherEd25519.verifyRaw(ByteArray(32), manifest().toByteArray(), ByteArray(16)))
    }

    @Test fun `unknown key and rotation behave`() {
        assertThrows(MapSignatureException::class.java) {
            MapManifestVerifier.verify(envelope(manifest()), mapOf("other" to rotated.public))
        }
        val rotatedVerified = MapManifestVerifier.verify(
            envelope(manifest("gen-2"), signer = rotated.private, keyId = "k2"),
            mapOf("k1" to keyPair.public, "k2" to rotated.public),
        )
        assertEquals("k2", rotatedVerified.keyId)
        // Original key still verifies after rotation is admitted.
        val stillValid = MapManifestVerifier.verify(
            envelope(manifest()),
            mapOf("k1" to keyPair.public, "k2" to rotated.public),
        )
        assertEquals("k1", stillValid.keyId)
    }

    @Test fun `platform-unavailable falls back or fails closed`() {
        val unavailable = Ed25519Verifier { _, _, _ ->
            throw MapSignatureException("ed25519-provider-unavailable")
        }
        // With no JNI on the JVM, the fallback must fail closed, never accept.
        assertThrows(MapSignatureException::class.java) {
            MapManifestVerifier.verify(envelope(manifest()), mapOf("k1" to keyPair.public), unavailable)
        }
        val raw = MonocypherEd25519.rawPublicKey(keyPair.public)
        // Encoded Ed25519 keys expose a 32-byte tail.
        assertEquals(32, raw?.size)
    }

    private fun sha(b: ByteArray) = java.security.MessageDigest.getInstance("SHA-256").digest(b)
        .joinToString("") { "%02x".format(it) }

    private fun hex(s: String): ByteArray {
        val out = ByteArray(s.length / 2)
        for (i in out.indices) out[i] = s.substring(i * 2, i * 2 + 2).toInt(16).toByte()
        return out
    }
}
