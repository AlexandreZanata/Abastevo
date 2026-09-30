package com.anpfuel.data.local.media

import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.security.SecureRandom
import java.util.Base64
import javax.crypto.Cipher
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import org.json.JSONObject

/**
 * AES/GCM transient-photo envelope for sealed cache files (P15-T02B).
 *
 * Pure JVM crypto over an explicitly passed [SecretKey]: the sealed
 * plaintext is 8 big-endian bytes of capture millis followed by the
 * wire JPEG bytes, wrapped as base64url envelope
 * `{"v":1,"iv":"..","ct":".."}`. Key custody stays with the caller
 * (AndroidKeyStore on device, in-memory fakes in tests), so this file
 * is fully unit-testable. Tampered, wrong-key, malformed, versioned
 * or oversized blobs decode to null — never throw — so reads fail
 * closed and the sweep purges them.
 */
object PhotoBlobSeal {

    /** Envelope version; bump on any format change (old blobs refuse). */
    const val VERSION: Int = 1

    /** GCM tag length, bits. */
    const val TAG_BITS: Int = 128

    /** GCM nonce length, bytes. */
    const val IV_BYTES: Int = 12

    /** Capture-stamp prefix length, bytes (big-endian millis). */
    const val STAMP_BYTES: Int = 8

    /**
     * Stored blobs beyond this size refuse (DoS cap): 256 KiB of photo
     * plus stamp, GCM tag and base64/JSON framing headroom.
     */
    const val MAX_ENVELOPE_CHARS: Int = 400000

    /** Sealed photo entry: capture instant plus wire bytes. */
    data class Entry(val capturedAtMillis: Long, val bytes: ByteArray) {
        override fun equals(other: Any?): Boolean {
            if (this === other) return true
            if (other !is Entry) return false
            return capturedAtMillis == other.capturedAtMillis && bytes.contentEquals(other.bytes)
        }

        override fun hashCode(): Int =
            31 * capturedAtMillis.hashCode() + bytes.contentHashCode()
    }

    /**
     * Seals [entry] under [key]. Empty photo bytes refuse to null (no
     * empty cache entries). Randomness comes from [random] (a
     * [SecureRandom] on device) so tests stay deterministic.
     */
    fun seal(
        entry: Entry,
        key: SecretKey,
        random: SecureRandom = SecureRandom(),
    ): String? {
        if (entry.bytes.isEmpty()) return null
        val plain = ByteBuffer
            .allocate(STAMP_BYTES + entry.bytes.size)
            .order(ByteOrder.BIG_ENDIAN)
            .putLong(entry.capturedAtMillis)
            .put(entry.bytes)
            .array()
        val iv = ByteArray(IV_BYTES)
        random.nextBytes(iv)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, key, GCMParameterSpec(TAG_BITS, iv))
        val ct = cipher.doFinal(plain)
        val envelope = JSONObject()
            .put("v", VERSION)
            .put("iv", Base64.getUrlEncoder().withoutPadding().encodeToString(iv))
            .put("ct", Base64.getUrlEncoder().withoutPadding().encodeToString(ct))
            .toString()
        if (envelope.length > MAX_ENVELOPE_CHARS) return null
        return envelope
    }

    /** Opens [envelope] under [key], or null when it must not be trusted. */
    fun open(envelope: String, key: SecretKey): Entry? {
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
        if (plain.size <= STAMP_BYTES) return null
        val stamp = ByteBuffer.wrap(plain, 0, STAMP_BYTES).order(ByteOrder.BIG_ENDIAN).long
        val bytes = plain.copyOfRange(STAMP_BYTES, plain.size)
        if (bytes.isEmpty()) return null
        return Entry(stamp, bytes)
    }
}
