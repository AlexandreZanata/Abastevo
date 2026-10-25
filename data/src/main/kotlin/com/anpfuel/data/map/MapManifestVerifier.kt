package com.anpfuel.data.map

import com.anpfuel.data.mapper.MapPoiCodec
import com.anpfuel.domain.map.MapContractException
import com.anpfuel.domain.map.NativeMapManifest
import java.security.KeyFactory
import java.security.NoSuchAlgorithmException
import java.security.PublicKey
import java.security.Signature
import java.security.spec.InvalidKeySpecException
import java.security.spec.X509EncodedKeySpec
import java.util.Base64
import org.json.JSONObject

data class VerifiedMapManifest(
    val manifest: NativeMapManifest,
    val manifestBytes: ByteArray,
    val manifestSha256: String,
    val keyId: String,
)

class MapSignatureException(message: String, cause: Throwable? = null) : Exception(message, cause)

/**
 * Verifies the detached MAP-12 envelope. The platform Ed25519 provider is
 * preferred; API levels without the provider fall back to the owned
 * Monocypher bridge. There is no insecure digest-only fallback: absent
 * providers fail closed with [MapSignatureException].
 */
object MapManifestVerifier {
    fun verify(envelopeJson: String, trustedKeys: Map<String, PublicKey>): VerifiedMapManifest =
        verify(envelopeJson, trustedKeys, FallbackEd25519Verifier)

    fun verify(
        envelopeJson: String,
        trustedKeys: Map<String, PublicKey>,
        verifier: Ed25519Verifier,
    ): VerifiedMapManifest {        val root = try {
            JSONObject(envelopeJson)
        } catch (error: Exception) {
            throw MapContractException("invalid-envelope")
        }
        val format = root.optString("format")
        val version = root.optInt("version", -1)
        val keyId = root.optString("key_id")
        val algorithm = root.optString("algorithm")
        if (format != "abastevo.map-distribution-v1" || version != 1 || algorithm != "Ed25519") {
            throw MapContractException("unsupported-envelope")
        }
        val publicKey = trustedKeys[keyId] ?: throw MapSignatureException("untrusted-map-key")
        val manifestBytes = decodeUrl(root.optString("manifest"))
        val signatureBytes = decodeUrl(root.optString("signature"))
        val expectedHash = root.optString("manifest_sha256")
        val actualHash = sha256(manifestBytes)
        if (expectedHash != actualHash) throw MapSignatureException("manifest-hash-mismatch")
        if (signatureBytes.size != 64) throw MapSignatureException("manifest-signature-invalid")
        val valid = verifier.verify(publicKey, manifestBytes, signatureBytes)
        if (!valid) throw MapSignatureException("manifest-signature-invalid")
        val manifest = MapPoiCodec.decodeManifest(String(manifestBytes, Charsets.UTF_8))
        return VerifiedMapManifest(manifest, manifestBytes, actualHash, keyId)
    }

    /**
     * Production verification against raw 32-byte pinned keys. Platform-first
     * via an X.509 wrapper; on provider absence (API 26) the owned Monocypher
     * bridge verifies. Unknown key ids, wrong key sizes and absent providers
     * fail closed; there is no digest-only fallback.
     */
    @JvmName("verifyRawKeys")
    fun verify(envelopeJson: String, trustedKeys: Map<String, ByteArray>): VerifiedMapManifest {
        val root = try {
            JSONObject(envelopeJson)
        } catch (error: Exception) {
            throw MapContractException("invalid-envelope")
        }
        val format = root.optString("format")
        val version = root.optInt("version", -1)
        val keyId = root.optString("key_id")
        val algorithm = root.optString("algorithm")
        if (format != "abastevo.map-distribution-v1" || version != 1 || algorithm != "Ed25519") {
            throw MapContractException("unsupported-envelope")
        }
        val rawKey = trustedKeys[keyId] ?: throw MapSignatureException("untrusted-map-key")
        if (rawKey.size != 32) throw MapSignatureException("untrusted-map-key")
        val manifestBytes = decodeUrl(root.optString("manifest"))
        val signatureBytes = decodeUrl(root.optString("signature"))
        val expectedHash = root.optString("manifest_sha256")
        val actualHash = sha256(manifestBytes)
        if (expectedHash != actualHash) throw MapSignatureException("manifest-hash-mismatch")
        if (signatureBytes.size != 64) throw MapSignatureException("manifest-signature-invalid")
        if (!verifyRawKey(rawKey, manifestBytes, signatureBytes)) {
            throw MapSignatureException("manifest-signature-invalid")
        }
        val manifest = MapPoiCodec.decodeManifest(String(manifestBytes, Charsets.UTF_8))
        return VerifiedMapManifest(manifest, manifestBytes, actualHash, keyId)
    }

    private val ED25519_SPKI_PREFIX = byteArrayOf(
        0x30, 0x2a, 0x30, 0x05, 0x06, 0x03, 0x2b, 0x65, 0x70, 0x03, 0x21, 0x00,
    )

    private fun verifyRawKey(rawKey: ByteArray, message: ByteArray, signature: ByteArray): Boolean {
        val platform = tryPlatformRawKey(rawKey, message, signature)
        if (platform != null) return platform
        if (!MonocypherEd25519.isAvailable()) throw MapSignatureException("ed25519-provider-unavailable")
        return try {
            MonocypherEd25519.verifyRaw(rawKey, message, signature)
        } catch (error: Exception) {
            throw MapSignatureException("manifest-signature-invalid", error)
        }
    }

    /**
     * Returns the platform verdict, or null when no Ed25519 provider exists
     * (API 26) so the caller falls back to Monocypher. Malformed platform
     * key material fails closed as invalid, never as unavailable.
     */
    private fun tryPlatformRawKey(rawKey: ByteArray, message: ByteArray, signature: ByteArray): Boolean? {
        try {
            val keyFactory = KeyFactory.getInstance("Ed25519")
            val publicKey = keyFactory.generatePublic(X509EncodedKeySpec(ED25519_SPKI_PREFIX + rawKey))
            val verifier = Signature.getInstance("Ed25519")
            verifier.initVerify(publicKey)
            verifier.update(message)
            return try {
                verifier.verify(signature)
            } catch (_: Exception) {
                false
            }
        } catch (_: NoSuchAlgorithmException) {
            return null
        } catch (error: InvalidKeySpecException) {
            throw MapSignatureException("manifest-signature-invalid", error)
        } catch (error: java.security.InvalidKeyException) {
            throw MapSignatureException("manifest-signature-invalid", error)
        }
    }
    private fun decodeUrl(value: String): ByteArray {
        if (value.isBlank()) throw MapContractException("invalid-envelope")
        val padded = value + "=".repeat((4 - value.length % 4) % 4)
        return try {
            Base64.getUrlDecoder().decode(padded)
        } catch (error: Exception) {
            throw MapContractException("invalid-envelope")
        }
    }

    private fun sha256(bytes: ByteArray): String =
        java.security.MessageDigest.getInstance("SHA-256").digest(bytes)
            .joinToString("") { "%02x".format(it) }
}
