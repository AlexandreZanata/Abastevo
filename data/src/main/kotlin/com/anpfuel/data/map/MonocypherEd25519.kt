package com.anpfuel.data.map

import java.security.PublicKey
import java.security.Signature

/**
 * Owned Monocypher/BSD-2-Clause Ed25519 fallback for API 26 devices without
 * a platform `Signature.getInstance("Ed25519")` provider.
 *
 * The math lives in the vendored audited C sources
 * (`data/src/main/jni/monocypher/`, Monocypher 4.0.2, BSD-2-Clause OR CC0-1.0);
 * this file is only a small JNI bridge. No cryptographic algorithm is
 * reimplemented here.
 */
object MonocypherEd25519 {
    private var loadAttempted = false
    private var loaded = false

    @Synchronized
    fun isAvailable(): Boolean {
        if (!loadAttempted) {
            loadAttempted = true
            loaded = try {
                System.loadLibrary("monocypher_ed25519")
                true
            } catch (_: UnsatisfiedLinkError) {
                false
            } catch (_: SecurityException) {
                false
            }
        }
        return loaded
    }

    /**
     * Verifies a raw 32-byte public key against a message and 64-byte
     * signature. Wrong sizes fail closed as false without touching JNI.
     */
    fun verifyRaw(publicKey32: ByteArray, message: ByteArray, signature64: ByteArray): Boolean {
        if (publicKey32.size != 32 || signature64.size != 64) return false
        if (message.size > 8 * 1024 * 1024) return false
        if (!isAvailable()) return false
        return try {
            verifyNative(publicKey32, message, signature64) == 1
        } catch (_: UnsatisfiedLinkError) {
            false
        }
    }

  /** Extracts the raw 32-byte Ed25519 key from a platform [PublicKey]. */
  fun rawPublicKey(publicKey: PublicKey): ByteArray? {
      val encoded = try {
          publicKey.encoded
      } catch (_: Exception) {
          return null
      } ?: return null
      // X.509 SubjectPublicKeyInfo for Ed25519 ends with the raw 32-byte key.
      // Accept a bare 32-byte encoding as well; otherwise take the tail.
      return when {
          encoded.size == 32 -> encoded.copyOf()
          encoded.size > 32 -> encoded.copyOfRange(encoded.size - 32, encoded.size)
          else -> null
      }
  }

    @JvmStatic
    private external fun verifyNative(publicKey32: ByteArray, message: ByteArray, signature64: ByteArray): Int
}

/** Platform provider verifier; throws when the provider is absent. */
object PlatformEd25519Verifier : Ed25519Verifier {
    override fun verify(publicKey: PublicKey, message: ByteArray, signature: ByteArray): Boolean {
        val verifier = try {
            Signature.getInstance("Ed25519")
        } catch (error: Exception) {
            throw MapSignatureException("ed25519-provider-unavailable", error)
        }
        try {
            verifier.initVerify(publicKey)
        } catch (error: Exception) {
            throw MapSignatureException("manifest-signature-invalid", error)
        }
        verifier.update(message)
        return try {
            verifier.verify(signature)
        } catch (error: MapSignatureException) {
            throw error
        } catch (error: Exception) {
            throw MapSignatureException("manifest-signature-invalid", error)
        }
    }
}

/**
 * Platform-first verifier with Monocypher fallback. Used on API 26 where the
 * platform provider may be absent. JNI absence still fails closed.
 */
object FallbackEd25519Verifier : Ed25519Verifier {
    override fun verify(publicKey: PublicKey, message: ByteArray, signature: ByteArray): Boolean {
        try {
            return PlatformEd25519Verifier.verify(publicKey, message, signature)
        } catch (error: MapSignatureException) {
            if (error.message != "ed25519-provider-unavailable") throw error
        }
        val raw = MonocypherEd25519.rawPublicKey(publicKey)
            ?: throw MapSignatureException("ed25519-provider-unavailable")
        if (signature.size != 64) throw MapSignatureException("manifest-signature-invalid")
        if (!MonocypherEd25519.isAvailable()) {
            throw MapSignatureException("ed25519-provider-unavailable")
        }
        return try {
            MonocypherEd25519.verifyRaw(raw, message, signature)
        } catch (error: Exception) {
            throw MapSignatureException("manifest-signature-invalid", error)
        }
    }
}
