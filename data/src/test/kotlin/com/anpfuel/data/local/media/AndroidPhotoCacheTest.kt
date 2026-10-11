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

    private fun sharedCache(dir: File, now: Long, k: javax.crypto.SecretKey): AndroidPhotoCache =
        AndroidPhotoCache(
            dir = File(dir, AndroidPhotoCache.SUBDIR),
            keys = PhotoKeyProvider { k },
            nowMillis = { now },
        )

    // SEC05-C: abandoned writes purge by file-age grace, never live
    // content; fresh tmp siblings survive the sweep.
    @Test
    fun orphanTmpPurgesAfterGraceButFreshTmpSurvives(@TempDir dir: File) {
        val now = 1_700_000_000_000L
        val cache = cache(dir, now = now)
        val sub = File(dir, AndroidPhotoCache.SUBDIR).also { it.mkdirs() }
        val stale = File(sub, "photo_orphan-1.tmp").also { it.writeText("partial") }
        stale.setLastModified(now - 2 * AndroidPhotoCache.TMP_GRACE_MILLIS)
        val fresh = File(sub, "photo_livewrite-1.tmp").also { it.writeText("partial") }
        fresh.setLastModified(now)
        cache.put("live-1", ByteArray(100) { 9 }, now)
        assertEquals(1, cache.sweepExpired(now))
        assertTrue(!stale.exists())
        assertTrue(fresh.exists())
        assertTrue(File(sub, "photo_live-1").exists())
    }

    // SEC05-C: rollback reads live inside the skew window (documented
    // limitation — deletion resumes after the clock restores) while
    // far-future captures refuse as tamper.
    @Test
    fun rollbackPolicyToleratesSkewButRefusesFarFuture(@TempDir dir: File) {
        val now = 1_700_000_000_000L
        val k = key()
        sharedCache(dir, now = now, k).put("skew-1", ByteArray(100) { 5 }, now)
        assertEquals(100, sharedCache(dir, now = now - 30 * 60_000L, k).get("skew-1")?.size)
        assertEquals(null, sharedCache(dir, now = now - 2 * 3_600_000L, k).get("skew-1"))
        sharedCache(dir, now = now, k).put("future-1", ByteArray(100) { 6 }, now + 2 * AndroidPhotoCache.FUTURE_TOLERANCE_MILLIS)
        assertEquals(null, sharedCache(dir, now = now, k).get("future-1"))
        assertTrue(!File(File(dir, AndroidPhotoCache.SUBDIR), "photo_future-1").exists())
        assertTrue(!AndroidPhotoCache.isCacheExpired(now + 30 * 60_000L, now))
        assertTrue(AndroidPhotoCache.isCacheExpired(now + 2 * AndroidPhotoCache.FUTURE_TOLERANCE_MILLIS, now))
    }

    // SEC05-C: discard covers the temporary sibling; lifetime survives
    // instance restart via the sealed envelope.
    @Test
    fun deleteDropsTmpSiblingAndLifetimeSurvivesRestart(@TempDir dir: File) {
        val now = 1_700_000_000_000L
        val k = key()
        sharedCache(dir, now = now, k).put("r-1", ByteArray(100) { 3 }, now)
        val sub = File(dir, AndroidPhotoCache.SUBDIR)
        File(sub, "photo_r-1.tmp").writeText("partial")
        sharedCache(dir, now = now, k).delete("r-1")
        assertTrue(!File(sub, "photo_r-1").exists())
        assertTrue(!File(sub, "photo_r-1.tmp").exists())
        sharedCache(dir, now = now, k).put("r-2", ByteArray(100) { 4 }, now)
        assertEquals(100, sharedCache(dir, now = now + 1_000L, k).get("r-2")?.size)
        assertEquals(null, sharedCache(dir, now = now + 86_400_000L + 1, k).get("r-2"))
    }

    // SEC04-D support: sealed copies purge by id for owner erasure
    // (unknown ids stay a no-op).
    @Test
    fun ownerPurgeDeletesExactIds(@TempDir dir: File) {
        val now = 1_700_000_000_000L
        val cache = cache(dir, now = now)
        cache.put("owner-a-1", ByteArray(50) { 1 }, now)
        cache.put("owner-b-1", ByteArray(50) { 2 }, now)
        cache.delete("owner-a-1")
        assertEquals(null, cache.get("owner-a-1"))
        assertEquals(50, cache.get("owner-b-1")?.size)
    }
}
