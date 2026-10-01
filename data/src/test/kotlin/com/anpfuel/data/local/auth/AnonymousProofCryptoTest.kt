package com.anpfuel.data.local.auth

import java.io.File
import java.security.KeyPairGenerator
import java.security.Signature
import java.security.spec.ECGenParameterSpec
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P10-T03: anonymous-proof crypto interop with the frozen backend vectors.
 *
 * Replays every identity vector under contracts testdata identity (one JSON
 * per vector) through the JVM verifier: the two valid proofs pass, all tamper/expiry/reorder/key
 * vectors fail. Device Keystore custody cannot run on JVM unit tests; the
 * key lifecycle below uses a generated P-256 key to prove the
 * sign/verify round-trip and the reinstall/lost-key shape instead.
 */
class AnonymousProofCryptoTest {

    private val nowEpochSeconds: Long = 1735689700L

    @Test
    fun `frozen vectors verify exactly like the backend`() {
        val vectors = identityVectors()
        assertTrue(vectors.size >= 11, "identity tamper matrix needs broad coverage")
        var valid = 0
        for (raw in vectors) {
            val doc = JSONObject(raw)
            val id = doc.getString("id")
            val expected = doc.getBoolean("valid")
            val lines = doc.getJSONArray("base_lines").let { array ->
                (0 until array.length()).map { array.getString(it) }
            }
            val ok = AnonymousProofCrypto.verify(
                jwkX = doc.getString("jwk_x"),
                jwkY = doc.getString("jwk_y"),
                lines = lines,
                signatureB64Url = doc.getString("signature"),
                nowEpochSeconds = nowEpochSeconds,
            )
            if (expected) {
                valid++
                assertTrue(ok, "valid vector refused: $id")
            } else {
                assertFalse(ok, "tampered vector accepted: $id")
            }
        }
        assertTrue(valid >= 2, "need positive and body coverage")
    }

    @Test
    fun `fingerprint binds the vector key and der is refused`() {
        val register = identityVectors()
            .map { JSONObject(it) }
            .first { it.getString("id") == "auth-valid-register" }
        val fingerprint = AnonymousProofCrypto.fingerprint(
            register.getString("jwk_x"),
            register.getString("jwk_y"),
        )
        val lines = (0 until register.getJSONArray("base_lines").length())
            .map { register.getJSONArray("base_lines").getString(it) }
        val keyId = lines.first { it.startsWith("\"keyid\":") }
        assertEquals("\"keyid\": \"$fingerprint\"", keyId)
        // DER blobs are never 64 raw bytes: the vector must fail verification.
        val der = identityVectors()
            .map { JSONObject(it) }
            .first { it.getString("id") == "auth-der-encoding" }
        val derLines = (0 until der.getJSONArray("base_lines").length())
            .map { der.getJSONArray("base_lines").getString(it) }
        assertFalse(
            AnonymousProofCrypto.verify(
                der.getString("jwk_x"),
                der.getString("jwk_y"),
                derLines,
                der.getString("signature"),
                nowEpochSeconds,
            ),
        )
    }

    @Test
    fun `generated p256 key signs and verifies with raw wire format`() {
        val generator = KeyPairGenerator.getInstance("EC")
        generator.initialize(ECGenParameterSpec("secp256r1"))
        val pair = generator.generateKeyPair()
        val public = pair.public as java.security.interfaces.ECPublicKey
        val x = fixed32(public.w.affineX.toByteArray())
        val y = fixed32(public.w.affineY.toByteArray())
        val jwkX = AnonymousProofCrypto.b64urlEncode(x)
        val jwkY = AnonymousProofCrypto.b64urlEncode(y)
        val fingerprint = AnonymousProofCrypto.fingerprint(jwkX, jwkY)
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
        val signer = Signature.getInstance("NONEwithECDSA")
        signer.initSign(pair.private)
        signer.update(digest)
        val raw = AnonymousProofCrypto.derToRaw(signer.sign())
        assertTrue(raw != null && raw!!.size == 64)
        assertTrue(
            AnonymousProofCrypto.verify(
                jwkX,
                jwkY,
                lines,
                AnonymousProofCrypto.b64urlEncode(raw!!),
                nowEpochSeconds,
            ),
        )
        // A fresh key (reinstall shape) cannot verify the old proof: the
        // keyid binding fails and the identity is honestly unrecoverable.
        val other = KeyPairGenerator.getInstance("EC").apply {
            initialize(ECGenParameterSpec("secp256r1"))
        }.generateKeyPair().public as java.security.interfaces.ECPublicKey
        val otherX = AnonymousProofCrypto.b64urlEncode(fixed32(other.w.affineX.toByteArray()))
        val otherY = AnonymousProofCrypto.b64urlEncode(fixed32(other.w.affineY.toByteArray()))
        assertFalse(
            AnonymousProofCrypto.verify(
                otherX,
                otherY,
                lines,
                AnonymousProofCrypto.b64urlEncode(raw),
                nowEpochSeconds,
            ),
        )
    }

    @Test
    fun `der round trip preserves raw signatures`() {
        val generator = KeyPairGenerator.getInstance("EC")
        generator.initialize(ECGenParameterSpec("secp256r1"))
        val pair = generator.generateKeyPair()
        val signer = Signature.getInstance("NONEwithECDSA")
        signer.initSign(pair.private)
        val digest = ByteArray(64) { it.toByte() }
        signer.update(digest)
        val der = signer.sign()
        val raw = AnonymousProofCrypto.derToRaw(der)
        assertTrue(raw != null && raw!!.size == 64)
        assertEquals(raw!!.toList(), AnonymousProofCrypto.derToRaw(
            AnonymousProofCrypto.rawToDer(raw)!!,
        )!!.toList())
    }

    private fun identityVectors(): List<String> {
        val dir = File(repoRoot(), "contracts/testdata/identity")
        val files = dir.listFiles { file -> file.extension == "json" }
            ?: throw AssertionError("missing identity fixtures")
        assertTrue(files.isNotEmpty(), "missing identity fixtures")
        return files.sortedBy { it.name }.map { it.readText() }
    }

    private fun repoRoot(): File {
        var dir = File(System.getProperty("user.dir") ?: ".")
        while (true) {
            if (File(dir, "contracts/testdata/identity/auth-valid-register.json").exists()) return dir
            dir = dir.parentFile ?: throw AssertionError("repo root not found")
        }
    }

    private fun fixed32(bytes: ByteArray): ByteArray {
        if (bytes.size == 32) return bytes
        if (bytes.size > 32) return bytes.copyOfRange(bytes.size - 32, bytes.size)
        val out = ByteArray(32)
        bytes.copyInto(out, 32 - bytes.size)
        return out
    }
}
