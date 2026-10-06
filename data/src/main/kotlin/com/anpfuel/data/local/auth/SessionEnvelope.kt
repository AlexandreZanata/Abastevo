package com.anpfuel.data.local.auth

import com.anpfuel.domain.portable.PortableAuth
import java.util.Base64
import javax.crypto.Cipher
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import org.json.JSONObject

/**
 * AES/GCM session envelope for secure storage (P13-T05B).
 *
 * Pure JVM crypto over an explicitly passed [SecretKey]: IV (12 random
 * bytes) prefixed to the ciphertext, base64url envelope
 * `{"v":1,"iv":"..","ct":".."}`. Key custody stays with the caller
 * (AndroidKeyStore on device, in-memory fakes in tests), so this file
 * is fully unit-testable and portable-safe except `java.*`, which is
 * why it lives in `data`, not `portable/`. Tampered, wrong-key,
 * malformed or oversized blobs decode to null — never throw — so the
 * flow rehydrates to logged-out and the owner simply logs in again.
 */
object SessionEnvelope {

    /** Envelope version; bump on any format change (old blobs refuse). */
    const val VERSION: Int = 1

    /** GCM tag length, bits. */
    const val TAG_BITS: Int = 128

    /** GCM nonce length, bytes. */
    const val IV_BYTES: Int = 12

    /** Stored blobs beyond this size refuse (DoS cap). */
    const val MAX_ENVELOPE_CHARS: Int = 65536

    /**
     * Seals [session] under [key]. The encryption provider generates the IV; AndroidKeyStore rejects
     * caller-supplied IVs when randomized encryption is required.
     */
    fun seal(
        session: PortableAuth.Session,
        key: SecretKey,
    ): String {
        val plain = JSONObject()
            .put("family_id", session.familyId)
            .put("account_id", session.accountId)
            .put("access_token", session.accessToken)
            .put("refresh_token", session.refreshToken)
            .put("access_expires_at", session.accessExpiresAt)
            .put("absolute_expires_at", session.absoluteExpiresAt)
            .toString()
            .toByteArray(Charsets.UTF_8)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, key)
        val iv = cipher.iv
        check(iv != null && iv.size == IV_BYTES) { "Invalid encryption IV" }
        val ct = cipher.doFinal(plain)
        return JSONObject()
            .put("v", VERSION)
            .put("iv", Base64.getUrlEncoder().withoutPadding().encodeToString(iv))
            .put("ct", Base64.getUrlEncoder().withoutPadding().encodeToString(ct))
            .toString()
    }

    /** Opens [envelope] under [key], or null when it must not be trusted. */
    fun open(envelope: String, key: SecretKey): PortableAuth.Session? {
        if (envelope.isEmpty() || envelope.length > MAX_ENVELOPE_CHARS) return null
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
        if (iv.size != IV_BYTES || ct.isEmpty()) return null
        val plain = try {
            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(TAG_BITS, iv))
            cipher.doFinal(ct)
        } catch (_: Exception) {
            return null
        }
        val body = try {
            JSONObject(String(plain, Charsets.UTF_8))
        } catch (_: Exception) {
            return null
        }
        val familyId = body.optString("family_id", "")
        val accountId = body.optString("account_id", "")
        val accessToken = body.optString("access_token", "")
        val refreshToken = body.optString("refresh_token", "")
        if (familyId.isEmpty() || accountId.isEmpty() ||
            accessToken.isEmpty() || refreshToken.isEmpty()
        ) {
            return null
        }
        return PortableAuth.Session(
            familyId = familyId,
            accountId = accountId,
            accessToken = accessToken,
            refreshToken = refreshToken,
            accessExpiresAt = body.optLong("access_expires_at", 0L),
            absoluteExpiresAt = body.optLong("absolute_expires_at", 0L),
        )
    }
}
