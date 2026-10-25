package com.anpfuel.data.map

import com.anpfuel.data.mapper.MapPoiCodec
import com.anpfuel.domain.map.MapContractException
import com.anpfuel.domain.map.NativeMapManifest
import java.security.PublicKey
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
    ): VerifiedMapManifest {
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
