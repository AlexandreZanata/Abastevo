package com.anpfuel.data.local.media

import java.security.SecureRandom
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class PhotoBlobSealTest {

    private fun key(): SecretKey {
        val gen = KeyGenerator.getInstance("AES")
        gen.init(256, SecureRandom())
        return gen.generateKey()
    }

    private fun entry() = PhotoBlobSeal.Entry(1_700_000_000_000L, ByteArray(100_000) { (it % 251).toByte() })

    @Test
    fun roundTripsEntry() {
        val k = key()
        val blob = PhotoBlobSeal.seal(entry(), k)
        assertNotNull(blob)
        assertEquals(entry(), PhotoBlobSeal.open(blob!!, k))
    }

    @Test
    fun ivUniquenessAcrossSeals() {
        val k = key()
        val first = PhotoBlobSeal.seal(entry(), k)!!
        val second = PhotoBlobSeal.seal(entry(), k)!!
        assertNotEquals(first, second)
        assertEquals(entry(), PhotoBlobSeal.open(second, k))
    }

    @Test
    fun tamperedBlobRefuses() {
        val k = key()
        val blob = PhotoBlobSeal.seal(entry(), k)!!.toMutableList()
        val flipAt = blob.size / 2
        blob[flipAt] = if (blob[flipAt] == 'A') 'B' else 'A'
        assertNull(PhotoBlobSeal.open(blob.joinToString(""), k))
    }

    @Test
    fun wrongKeyRefuses() {
        val blob = PhotoBlobSeal.seal(entry(), key())!!
        assertNull(PhotoBlobSeal.open(blob, key()))
    }

    @Test
    fun malformedBlobsRefuse() {
        val k = key()
        assertNull(PhotoBlobSeal.open("", k))
        assertNull(PhotoBlobSeal.open("not-json", k))
        assertNull(PhotoBlobSeal.open("{\"v\":2,\"iv\":\"AA\",\"ct\":\"AA\"}", k))
        assertNull(PhotoBlobSeal.open("{\"v\":1,\"iv\":\"!!!\",\"ct\":\"AA\"}", k))
        assertNull(PhotoBlobSeal.open("{\"v\":1,\"iv\":\"AA\",\"ct\":\"\"}", k))
    }

    @Test
    fun emptyBytesRefuseToSeal() {
        assertNull(PhotoBlobSeal.seal(PhotoBlobSeal.Entry(1L, ByteArray(0)), key()))
    }

    @Test
    fun oversizeEnvelopeRefuses() {
        val k = key()
        assertNull(
            PhotoBlobSeal.seal(
                PhotoBlobSeal.Entry(1L, ByteArray(PhotoBlobSeal.MAX_ENVELOPE_CHARS)),
                k,
            ),
        )
    }
}
