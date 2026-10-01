package com.anpfuel.data.local.auth

import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import com.anpfuel.application.port.AnonymousDeviceKeyPort
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.Signature
import java.security.interfaces.ECPublicKey
import java.security.spec.ECGenParameterSpec
import javax.inject.Inject
import javax.inject.Singleton

/**
 * P10-T03 Android Keystore P-256 device-key custody.
 *
 * The EC key pair lives in AndroidKeyStore under [alias] and never leaves
 * it: no export, no backup, no user-authentication gate (device-unlock
 * gating would strand background contribution). Signing hashes the frozen
 * base with SHA-512 first, then signs the digest with `NONEwithECDSA` so
 * the bytes match the backend `ecdsa.Sign(digest)` path; the DER output is
 * converted to raw `R || S` by [AnonymousProofCrypto]. Null means the
 * keystore is unavailable and the flow fails closed.
 *
 * The ~60 lines of KeyStore boilerplate below cannot run on JVM unit
 * tests (no Android runtime): they are statically reviewed and
 * lint-guarded, while [AnonymousProofCrypto] and the application flow
 * carry the behavioral suites. Device interop stays deferred to
 * `connectedDebugAndroidTest`/phase exit, never claimed from unit runs.
 */
@Singleton
class AndroidAnonymousDeviceKeys @Inject constructor() : AnonymousDeviceKeyPort {

    override fun fingerprint(): String? {
        val jwk = publicJwk() ?: return null
        return AnonymousProofCrypto.fingerprint(jwk.first, jwk.second)
    }

    override fun publicJwk(): Pair<String, String>? {
        return try {
            val store = KeyStore.getInstance(ANDROID_KEYSTORE)
            store.load(null)
            val entry = store.getEntry(alias(), null) as? KeyStore.PrivateKeyEntry
                ?: return null
            val public = entry.certificate.publicKey as? ECPublicKey ?: return null
            val point = public.w
            val x = fixed32(point.affineX.toByteArray())
            val y = fixed32(point.affineY.toByteArray())
            Pair(
                AnonymousProofCrypto.b64urlEncode(x),
                AnonymousProofCrypto.b64urlEncode(y),
            )
        } catch (_: Exception) {
            null
        }
    }

    override fun signDigest(digestSha512: ByteArray): ByteArray? {
        if (digestSha512.size != 64) return null
        return try {
            val store = KeyStore.getInstance(ANDROID_KEYSTORE)
            store.load(null)
            val entry = store.getEntry(alias(), null) as? KeyStore.PrivateKeyEntry
                ?: return null
            val signature = Signature.getInstance("NONEwithECDSA")
            signature.initSign(entry.privateKey)
            signature.update(digestSha512)
            val der = signature.sign()
            AnonymousProofCrypto.derToRaw(der)
        } catch (_: Exception) {
            null
        }
    }

    override fun hasKey(): Boolean = publicJwk() != null

    override fun forgetKey() {
        try {
            val store = KeyStore.getInstance(ANDROID_KEYSTORE)
            store.load(null)
            if (store.containsAlias(alias())) {
                store.deleteEntry(alias())
            }
        } catch (_: Exception) {
            // Fails closed: the reference is gone from the caller's view.
        }
    }

    /**
     * Creates the P-256 key when absent. Returns true when a usable key
     * exists afterwards. Called lazily so browsing never pays the
     * Keystore cost until contribution is enabled.
     */
    fun ensureKey(): Boolean {
        return try {
            val store = KeyStore.getInstance(ANDROID_KEYSTORE)
            store.load(null)
            if (store.containsAlias(alias())) return true
            val generator = KeyPairGenerator.getInstance(
                KeyProperties.KEY_ALGORITHM_EC,
                ANDROID_KEYSTORE,
            )
            generator.initialize(
                KeyGenParameterSpec.Builder(
                    alias(),
                    KeyProperties.PURPOSE_SIGN,
                )
                    .setAlgorithmParameterSpec(ECGenParameterSpec("secp256r1"))
                    .setDigests(KeyProperties.DIGEST_NONE, KeyProperties.DIGEST_SHA512)
                    .setUserAuthenticationRequired(false)
                    .build(),
            )
            generator.generateKeyPair()
            true
        } catch (_: Exception) {
            false
        }
    }

    private fun alias(): String = DEFAULT_ALIAS

    private fun fixed32(bytes: ByteArray): ByteArray {
        if (bytes.size == 32) return bytes
        if (bytes.size > 32) return bytes.copyOfRange(bytes.size - 32, bytes.size)
        val out = ByteArray(32)
        bytes.copyInto(out, 32 - bytes.size)
        return out
    }

    companion object {
        const val ANDROID_KEYSTORE: String = "AndroidKeyStore"
        const val DEFAULT_ALIAS: String = "anpfuel_anon_p256_v1"
    }
}
