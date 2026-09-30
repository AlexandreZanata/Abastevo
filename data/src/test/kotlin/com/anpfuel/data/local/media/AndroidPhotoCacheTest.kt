package com.anpfuel.data.local.media

import java.io.File
import java.nio.file.Files
import java.security.SecureRandom
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import org.junit.jupiter.api.Assertions.assertArrayEquals
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

class AndroidPhotoCacheTest {

    private fun key(): SecretKey {
        val gen = KeyGenerator.getInstance("AES")
        gen.init(256, SecureRandom())
        return gen.generateKey()
    }

    private fun cache(dir: File, now: Long = 1_700_000_000_000L): AndroidPhotoCache {
        val k = key()
        return AndroidPhotoCache(
            dir = File(dir, AndroidPhotoCache.SUBDIR),
            keys = PhotoKeyProvider { k },
            nowMillis = { now },
        )
    }

    @Test
    fun roundTripsLiveEntry(@TempDir dir: File) {
        val cache = cache(dir)
        val bytes = ByteArray(120_000) { (it % 251).toByte() }
        cache.put("abc-123_XY", bytes, 1_700_000_000_000L)
        assertArrayEquals(bytes, cache.get("abc-123_XY"))
    }

    @Test
    fun expiredEntryReadsNullAndDeletes(@TempDir dir: File) {
        val cache = cache(dir, now = 1_700_000_000_000L + 86_400_000L)
        cache.put("old-1", ByteArray(10_000) { 7 }, 1_700_000_000_000L)
        assertNull(cache.get("old-1"))
        assertTrue(File(File(dir, AndroidPhotoCache.SUBDIR), "photo_old-1").exists().not())
    }

    @Test
    fun tamperedFileReadsNullAndDeletes(@TempDir dir: File) {
        val cache = cache(dir)
        cache.put("t-1", ByteArray(10_000) { 7 }, 1_700_000_000_000L)
        val file = File(File(dir, AndroidPhotoCache.SUBDIR), "photo_t-1")
        file.writeText("{\"v\":1,\"iv\":\"AA\",\"ct\":\"AA\"}", Charsets.UTF_8)
        assertNull(cache.get("t-1"))
        assertTrue(!file.exists())
    }

    @Test
    fun badIdsRefuseEveryPath(@TempDir dir: File) {
        val cache = cache(dir)
        cache.put("../escape", ByteArray(10) { 1 }, 1L)
        cache.put("a/b", ByteArray(10) { 1 }, 1L)
        cache.put("", ByteArray(10) { 1 }, 1L)
        assertNull(cache.get("../escape"))
        assertNull(cache.get(""))
        cache.delete("../escape")
        val leftovers = dir.walkTopDown().filter { it.isFile }.toList()
        assertTrue(leftovers.isEmpty(), "bad ids must write nothing: $leftovers")
    }

    @Test
    fun sweepPurgesExpiredAndCorrupt(@TempDir dir: File) {
        val now = 1_700_000_000_000L
        val cache = cache(dir, now = now + 100_000L)
        cache.put("live-1", ByteArray(10_000) { 1 }, now)
        cache.put("dead-1", ByteArray(10_000) { 2 }, now - 86_400_000L)
        cache.put("dead-2", ByteArray(10_000) { 3 }, now - 86_400_001L)
        val sub = File(dir, AndroidPhotoCache.SUBDIR)
        File(sub, "photo_corrupt-1").writeText("garbage", Charsets.UTF_8)
        assertEquals(3, cache.sweepExpired(now + 100_000L))
        assertTrue(File(sub, "photo_live-1").exists())
        assertTrue(!File(sub, "photo_dead-1").exists())
    }

    @Test
    fun missingKeystoreFailsClosed(@TempDir dir: File) {
        val cache = AndroidPhotoCache(
            dir = File(dir, AndroidPhotoCache.SUBDIR),
            keys = PhotoKeyProvider { null },
        )
        cache.put("k-1", ByteArray(10) { 1 }, 1L)
        assertNull(cache.get("k-1"))
        val leftovers = dir.walkTopDown().filter { it.isFile }.toList()
        assertTrue(leftovers.isEmpty(), "nothing lands unsealed: $leftovers")
    }

    @Test
    fun deleteIsIdempotent(@TempDir dir: File) {
        val cache = cache(dir)
        cache.put("d-1", ByteArray(10) { 1 }, 1L)
        cache.delete("d-1")
        cache.delete("d-1")
        cache.delete("ghost")
        assertNull(cache.get("d-1"))
    }

    @Test
    fun qualityScheduleTightensPerAttempt() {
        assertEquals(85, qualityForAttempt(1))
        assertEquals(70, qualityForAttempt(2))
        assertEquals(55, qualityForAttempt(3))
        assertEquals(55, qualityForAttempt(9))
        assertEquals(85, qualityForAttempt(0))
    }

    @Test
    fun filesStayInsidePrivateSubdir(@TempDir dir: File) {
        val cache = cache(dir)
        cache.put("s-1", ByteArray(10) { 1 }, 1L)
        val files = dir.walkTopDown().filter { it.isFile }.toList()
        assertEquals(1, files.size)
        assertEquals("anpfuel_photos", files.single().parentFile.name)
        assertTrue(!Files.isSymbolicLink(files.single().toPath()))
    }
}
