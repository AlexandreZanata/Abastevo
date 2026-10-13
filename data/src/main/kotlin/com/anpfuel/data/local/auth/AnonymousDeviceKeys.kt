package com.anpfuel.data.local.auth

import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.data.remote.ApiEnvironment
import javax.inject.Named
import java.security.MessageDigest
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
class AndroidAnonymousDeviceKeys @Inject constructor(
    private val auth:AuthFlow,
    @Named("apiOrigin") private val environment:ApiEnvironment,
) : AnonymousDeviceKeyPort {

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

    @Synchronized
    override fun forgetKey() {
        try {
            val store = KeyStore.getInstance(ANDROID_KEYSTORE)
            store.load(null)
            val selectedAlias=alias()
            if (store.containsAlias(selectedAlias)) {
                store.deleteEntry(selectedAlias)
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
    @Synchronized
    fun ensureKey(): Boolean {
        return try {
            val store = KeyStore.getInstance(ANDROID_KEYSTORE)
            store.load(null)
            val selectedAlias=alias()
            if (store.containsAlias(selectedAlias)) return true
            if(selectedAlias.startsWith(ACCOUNT_ALIAS_PREFIX) && store.aliases().toList().count {it.startsWith(ACCOUNT_ALIAS_PREFIX)}>=32) return false
            val generator = KeyPairGenerator.getInstance(
                KeyProperties.KEY_ALGORITHM_EC,
                ANDROID_KEYSTORE,
            )
            generator.initialize(
                KeyGenParameterSpec.Builder(
                    selectedAlias,
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

    private fun alias(): String = aliasFor(auth.rehydrate(),environment.origin)


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
        internal const val ACCOUNT_ALIAS_PREFIX="anpfuel_account_p256_v2_"
        internal fun aliasFor(state:AuthFlow.AuthState,origin:String):String {
            val account=when(state) {
                is AuthFlow.AuthState.Active -> state.session.accountId
                is AuthFlow.AuthState.NeedsRefresh -> state.session.accountId
                AuthFlow.AuthState.LoggedOut -> null
                is AuthFlow.AuthState.PendingKeyAccount -> throw IllegalStateException("identity recovery pending")
            }
            if(account==null && origin==ApiEnvironment.STAGING.origin) return DEFAULT_ALIAS
            val hash=MessageDigest.getInstance("SHA-256").digest((origin+"\n"+(account ?: "anonymous")).toByteArray(Charsets.UTF_8))
                .joinToString("") {"%02x".format(it)}
            return (if(account==null) "anpfuel_anon_p256_v2_" else ACCOUNT_ALIAS_PREFIX)+hash
        }
    }
}
