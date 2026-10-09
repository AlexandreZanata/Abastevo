package com.anpfuel.data.local.auth

import com.anpfuel.domain.portable.PortableAuth
import java.security.SecureRandom
import javax.crypto.KeyGenerator
import javax.crypto.Cipher
import javax.crypto.spec.GCMParameterSpec
import org.json.JSONObject
import java.util.Base64
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Test

class SessionEnvelopeTest {

    private fun session() = PortableAuth.Session(
        familyId = "family-1", accountId = "account-1",
        accessToken = "access-1", refreshToken = "refresh-1",
        accessExpiresAt = 1_000_900L, absoluteExpiresAt = 4_259_200L,
    )

    private fun key(): javax.crypto.SecretKey {
        val gen = KeyGenerator.getInstance("AES")
        gen.init(256, SecureRandom())
        return gen.generateKey()
    }

    @Test
    fun opensExistingVersionOneEnvelopeWithCallerGeneratedIv() {
        val k = key()
        val iv = ByteArray(SessionEnvelope.IV_BYTES) { it.toByte() }
        val body = JSONObject()
            .put("family_id", "family-1").put("account_id", "account-1")
            .put("access_token", "access-1").put("refresh_token", "refresh-1")
            .put("access_expires_at", 1_000_900L).put("absolute_expires_at", 4_259_200L)
            .toString().toByteArray(Charsets.UTF_8)
        val legacyCipher = Cipher.getInstance("AES/GCM/NoPadding")
        legacyCipher.init(Cipher.ENCRYPT_MODE, k, GCMParameterSpec(SessionEnvelope.TAG_BITS, iv))
        val encoder = Base64.getUrlEncoder().withoutPadding()
        val envelope = JSONObject().put("v", 1)
            .put("iv", encoder.encodeToString(iv))
            .put("ct", encoder.encodeToString(legacyCipher.doFinal(body))).toString()
        assertEquals(session(), SessionEnvelope.open(envelope, k))
    }

    @Test
    fun roundTripsSession() {
        val k = key()
        val blob = SessionEnvelope.seal(session(), k)
        assertEquals(session(), SessionEnvelope.open(blob, k))
    }

    @Test
    fun ivUniquenessAcrossSeals() {
        val k = key()
        val first = SessionEnvelope.seal(session(), k)
        val second = SessionEnvelope.seal(session(), k)
        assertNotEquals(first, second)
        assertEquals(session(), SessionEnvelope.open(second, k))
    }

    @Test
    fun tamperedBlobRefuses() {
        val k = key()
        val blob = SessionEnvelope.seal(session(), k).toMutableList()
        val flipAt = blob.size / 2
        blob[flipAt] = if (blob[flipAt] == 'A') 'B' else 'A'
        assertNull(SessionEnvelope.open(blob.joinToString(""), k))
    }

    @Test
    fun wrongKeyRefuses() {
        val blob = SessionEnvelope.seal(session(), key())
        assertNull(SessionEnvelope.open(blob, key()))
    }

    @Test
    fun malformedBlobsRefuse() {
        val k = key()
        assertNull(SessionEnvelope.open("", k))
        assertNull(SessionEnvelope.open("not json", k))
        assertNull(SessionEnvelope.open("""{"v":2,"iv":"AA","ct":"AA"}""", k))
        assertNull(SessionEnvelope.open("""{"v":1}""", k))
        assertNull(SessionEnvelope.open("x".repeat(SessionEnvelope.MAX_ENVELOPE_CHARS + 1), k))
        val short = SessionEnvelope.seal(session(), k)
        assertNotNull(SessionEnvelope.open(short, k))
    }
}
