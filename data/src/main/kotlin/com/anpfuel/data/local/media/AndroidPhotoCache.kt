package com.anpfuel.data.local.media

import com.anpfuel.application.portable.PhotoCache
import com.anpfuel.domain.portable.PortablePhoto
import java.io.File

/**
 * Sealed transient photo cache over private app files (P15-T02B).
 *
 * Implements portable [PhotoCache]: sealed envelopes in
 * `<cacheDir>/anpfuel_photos/photo_<id>`, private to the app (never
 * the Gallery, never a backup, never logs). Reads fail closed
 * (missing/expired/corrupt/tampered all answer null) and delete
 * expired entries on read; [sweepExpired] purges the rest at
 * launch/resume. Writes are atomic (temp file + rename), so an
 * interrupted capture never leaves a partial entry. Ids outside
 * `[A-Za-z0-9_-]+` refuse the write: file names never escape the
 * directory. A missing keystore fails every write closed (nothing
 * lands unsealed) while reads still answer null.
 */
class AndroidPhotoCache(
    private val dir: File,
    private val keys: PhotoKeyProvider,
    private val alias: String = DEFAULT_ALIAS,
    private val nowMillis: () -> Long = { System.currentTimeMillis() },
) : PhotoCache {

    override fun put(id: String, bytes: ByteArray, capturedAtMillis: Long) {
        if (!ID_PATTERN.matches(id) || bytes.isEmpty()) return
        val key = keys.getOrCreateKey(alias) ?: return
        val envelope = PhotoBlobSeal.seal(PhotoBlobSeal.Entry(capturedAtMillis, bytes), key) ?: return
        val target = File(dir, FILE_PREFIX + id)
        val tmp = File(dir, FILE_PREFIX + id + TMP_SUFFIX)
        try {
            dir.mkdirs()
            tmp.writeText(envelope, Charsets.UTF_8)
            if (!tmp.renameTo(target)) {
                tmp.delete()
            }
        } catch (_: Exception) {
            tmp.delete()
        }
    }

    override fun get(id: String): ByteArray? {
        if (!ID_PATTERN.matches(id)) return null
        val key = keys.getOrCreateKey(alias) ?: return null
        val file = File(dir, FILE_PREFIX + id)
        val envelope = try {
            if (!file.isFile) return null
            file.readText(Charsets.UTF_8)
        } catch (_: Exception) {
            return null
        }
        val entry = PhotoBlobSeal.open(envelope, key) ?: run {
            file.delete()
            return null
        }
        if (PortablePhoto.isTransientExpired(entry.capturedAtMillis, nowMillis())) {
            file.delete()
            return null
        }
        return entry.bytes
    }

    override fun delete(id: String) {
        if (!ID_PATTERN.matches(id)) return
        try {
            File(dir, FILE_PREFIX + id).delete()
        } catch (_: Exception) {
            // Fail closed: the entry stays until the next sweep.
        }
    }

    override fun sweepExpired(now: Long): Int {
        var purged = 0
        val files = try {
            dir.listFiles { f -> f.isFile && f.name.startsWith(FILE_PREFIX) && !f.name.endsWith(TMP_SUFFIX) }
                ?: return 0
        } catch (_: Exception) {
            return 0
        }
        val key = keys.getOrCreateKey(alias)
        for (file in files) {
            val expired = if (key == null) {
                true
            } else {
                val envelope = try {
                    file.readText(Charsets.UTF_8)
                } catch (_: Exception) {
                    null
                }
                val entry = envelope?.let { PhotoBlobSeal.open(it, key) }
                entry == null || PortablePhoto.isTransientExpired(entry.capturedAtMillis, now)
            }
            if (expired && file.delete()) purged++
        }
        return purged
    }

    companion object {
        const val DEFAULT_ALIAS: String = "anpfuel_photo_cache_v1"
        const val SUBDIR: String = "anpfuel_photos"
        const val FILE_PREFIX: String = "photo_"
        const val TMP_SUFFIX: String = ".tmp"
        val ID_PATTERN: Regex = Regex("[A-Za-z0-9_-]+")
    }
}
