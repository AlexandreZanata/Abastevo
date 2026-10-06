package com.anpfuel.data.local.auth

import android.content.SharedPreferences
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthKeyStore
import java.security.SecureRandom
import java.util.Base64
import javax.crypto.Cipher
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import org.json.JSONObject

/**
 * Keystore-sealed account-key backup (key-account login).
 *
 * Same AES/GCM envelope discipline as [SessionEnvelope] under a
 * separate preference key and envelope version, so session and key
 * blobs never alias: the key backup survives session rotation and
 * clears on logout/deletion with it. Plaintext holds only the username
 * and the account key; oversized, tampered or foreign blobs refuse to
 * null instead of throwing.
 */
class KeystoreAccountKey(
    private val prefs: SharedPreferences,
    private val keys: SessionKeyProvider,
    private val alias: String = KEY_ALIAS,
    private val prefKey: String = PREF_KEY,
) : AuthKeyStore {

    override fun saveKey(backup: AuthFlow.KeyBackup) {
        val key = keys.getOrCreateKey(alias) ?: return
        seal(backup, key)?.let { prefs.edit().putString(prefKey, it).apply() }
    }

    override fun loadKey(): AuthFlow.KeyBackup? {
        val blob = prefs.getString(prefKey, null) ?: return null
        val key = keys.getOrCreateKey(alias) ?: return null
        return open(blob, key)
    }

    override fun clearKey() {
        prefs.edit().remove(prefKey).apply()
    }

    companion object {
        const val KEY_ALIAS: String = "anpfuel_account_key_v1"
        const val PREF_KEY: String = "portable_auth_account_key"
        const val VERSION: Int = 1
        const val MAX_CHARS: Int = 4096

        internal fun seal(backup: AuthFlow.KeyBackup, key: SecretKey): String? {
            if (backup.username.isEmpty() || backup.accountKey.isEmpty()) return null
            if (backup.username.length > 64 || backup.accountKey.length > 128) return null
            val plain = JSONObject()
                .put("username", backup.username)
                .put("account_key", backup.accountKey)
                .toString()
                .toByteArray(Charsets.UTF_8)
            return try {
                val iv = ByteArray(SessionEnvelope.IV_BYTES)
                SecureRandom().nextBytes(iv)
                val cipher = Cipher.getInstance("AES/GCM/NoPadding")
                cipher.init(Cipher.ENCRYPT_MODE, key, GCMParameterSpec(SessionEnvelope.TAG_BITS, iv))
                val ct = cipher.doFinal(plain)
                JSONObject()
                    .put("v", VERSION)
                    .put("iv", Base64.getUrlEncoder().withoutPadding().encodeToString(iv))
                    .put("ct", Base64.getUrlEncoder().withoutPadding().encodeToString(ct))
                    .toString()
            } catch (_: Exception) {
                null
            }
        }

        internal fun open(envelope: String, key: SecretKey): AuthFlow.KeyBackup? {
            if (envelope.isEmpty() || envelope.length > MAX_CHARS) return null
            val doc = try {
                JSONObject(envelope)
            } catch (_: Exception) {
                return null
            }
            if (doc.optInt("v", -1) != VERSION) return null
            val iv = try {
                Base64.getUrlDecoder().decode(doc.optString("iv", ""))
            } catch (_: Exception) {
                return null
            }
            val ct = try {
                Base64.getUrlDecoder().decode(doc.optString("ct", ""))
            } catch (_: Exception) {
                return null
            }
            if (iv.size != SessionEnvelope.IV_BYTES || ct.isEmpty()) return null
            val plain = try {
                val cipher = Cipher.getInstance("AES/GCM/NoPadding")
                cipher.init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(SessionEnvelope.TAG_BITS, iv))
                cipher.doFinal(ct)
            } catch (_: Exception) {
                return null
            }
            val body = try {
                JSONObject(String(plain, Charsets.UTF_8))
            } catch (_: Exception) {
                return null
            }
            val username = body.optString("username", "")
            val accountKey = body.optString("account_key", "")
            if (username.isEmpty() || accountKey.isEmpty()) return null
            return AuthFlow.KeyBackup(username, accountKey)
        }
    }
}
