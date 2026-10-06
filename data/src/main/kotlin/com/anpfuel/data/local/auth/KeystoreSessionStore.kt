package com.anpfuel.data.local.auth

import android.content.SharedPreferences
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.portable.PortableAuth
import java.security.KeyStore
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey

/**
 * Keystore-backed [AuthSessionStore] (P13-T05B).
 *
 * The AES/GCM session key lives in AndroidKeyStore under [alias] and
 * never leaves it; the sealed envelope rests in private
 * [SharedPreferences]. Key custody is the only Android boundary, kept
 * behind [SessionKeyProvider] so JVM tests inject an in-memory key
 * while production uses [AndroidKeystoreKeys]. A key rotation (or a
 * foreign blob) simply fails to open: [load] returns null and the
 * flow rehydrates to logged-out.
 */
class KeystoreSessionStore(
    private val prefs: SharedPreferences,
    private val keys: SessionKeyProvider,
    private val alias: String = DEFAULT_ALIAS,
    private val prefKey: String = PREF_SESSION,
) : AuthSessionStore {

    override fun save(session: PortableAuth.Session) {
        val key = keys.getOrCreateKey(alias) ?: return
        persistAuthPreference(prefs, prefKey, SessionEnvelope.seal(session, key))
    }

    override fun load(): PortableAuth.Session? {
        val blob = prefs.getString(prefKey, null) ?: return null
        val key = keys.getOrCreateKey(alias) ?: return null
        return SessionEnvelope.open(blob, key)
    }

    override fun clear() {
        prefs.edit().remove(prefKey).commit()
    }

    companion object {
        const val DEFAULT_ALIAS: String = "anpfuel_account_session_v1"
        const val PREF_SESSION: String = "portable_auth_session"
    }
}

/**
 * Android key custody boundary. Production returns the AndroidKeyStore
 * key, creating it once; tests return in-memory keys. Null means the
 * keystore is unavailable and the operation fails closed.
 */
fun interface SessionKeyProvider {
    fun getOrCreateKey(alias: String): SecretKey?
}

/**
 * AndroidKeyStore AES/GCM key source (API 26+, no user authentication:
 * device-unlock gating would strand background refresh; the key
 * material itself never leaves hardware-backed storage when present).
 *
 * The ~30 lines of KeyStore boilerplate below cannot run on JVM unit
 * tests (no Android runtime): they are statically reviewed and
 * lint-guarded, while [SessionEnvelope] and [KeystoreSessionStore]
 * carry the behavioral suites.
 */
class AndroidKeystoreKeys : SessionKeyProvider {

    override fun getOrCreateKey(alias: String): SecretKey? {
        return try {
            val store = KeyStore.getInstance(ANDROID_KEYSTORE)
            store.load(null)
            val existing = store.getKey(alias, null) as? SecretKey
            if (existing != null) return existing
            val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, ANDROID_KEYSTORE)
            generator.init(
                KeyGenParameterSpec.Builder(
                    alias,
                    KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT,
                )
                    .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                    .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                    .setUserAuthenticationRequired(false)
                    .setRandomizedEncryptionRequired(true)
                    .build(),
            )
            generator.generateKey()
        } catch (_: Exception) {
            null
        }
    }

    companion object {
        const val ANDROID_KEYSTORE: String = "AndroidKeyStore"
    }
}

/** A read-back of apply() only proves memory, not a durable credential write. */
internal fun persistAuthPreference(prefs: SharedPreferences, name: String, envelope: String) {
    val previous = prefs.getString(name, null)
    if (!prefs.edit().putString(name, envelope).commit()) {
        // commit() updates memory even on disk failure. Restore the prior
        // envelope so a retry cannot mistake that memory state for success.
        prefs.edit().putString(name, previous).commit()
        error("Secure storage write failed")
    }
}
