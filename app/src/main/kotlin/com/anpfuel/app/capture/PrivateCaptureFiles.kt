package com.anpfuel.app.capture

import android.content.Context
import android.net.Uri
import androidx.core.content.FileProvider
import java.io.File

/** Private raw review material; never a gallery entry or an outbox payload. */
class PrivateCaptureFiles(private val context: Context) {
    private val directory get() = File(context.cacheDir, "capture").apply { mkdirs() }

    fun create(capturedAtMillis: Long): Uri {
        sweep()
        val file = File.createTempFile("price_", ".jpg", directory)
        file.setLastModified(capturedAtMillis)
        return FileProvider.getUriForFile(context, context.packageName + ".fileprovider", file)
    }

    fun write(bytes: ByteArray, capturedAtMillis: Long): Uri {
        require(bytes.size in 1..MAX_BYTES && isFresh(capturedAtMillis))
        val uri = create(capturedAtMillis)
        context.contentResolver.openOutputStream(uri)?.use { it.write(bytes) } ?: error("photo.write-failed")
        resolve(uri)?.setLastModified(capturedAtMillis)
        return uri
    }

    fun read(uri: Uri, capturedAtMillis: Long): ByteArray? {
        if (!isFresh(capturedAtMillis)) { delete(uri); return null }
        val file = resolve(uri) ?: return null
        if (file.length() !in 1..MAX_BYTES.toLong()) return null
        return file.inputStream().use { stream ->
            val output = java.io.ByteArrayOutputStream()
            val buffer = ByteArray(8192)
            var count = 0
            while (true) {
                val read = stream.read(buffer)
                if (read < 0) break
                count += read
                if (count > MAX_BYTES) return null
                output.write(buffer, 0, read)
            }
            file.setLastModified(capturedAtMillis)
            output.toByteArray().takeIf { it.isNotEmpty() }
        }
    }

    fun readReview(uri: Uri): ByteArray? = resolve(uri)?.let { read(uri, it.lastModified()) }

    fun delete(uri: Uri) { resolve(uri)?.delete() }
    fun exists(uri: Uri, capturedAtMillis: Long): Boolean = isFresh(capturedAtMillis) && resolve(uri)?.isFile == true
    fun sweep(now: Long = System.currentTimeMillis()) {
        directory.listFiles()?.filter { it.isFile && (it.lastModified() > now || now - it.lastModified() >= TTL) }?.forEach { it.delete() }
    }

    private fun resolve(uri: Uri): File? {
        if (uri.scheme != "content" || uri.authority != context.packageName + ".fileprovider") return null
        val segments = uri.pathSegments
        if (segments.size != 2 || segments[0] != "capture" || !segments[1].matches(Regex("price_[A-Za-z0-9_]+\\.jpg"))) return null
        val candidate = File(directory, segments[1]).canonicalFile
        return candidate.takeIf { it.parentFile == directory.canonicalFile }
    }
    private fun isFresh(capturedAtMillis: Long): Boolean {
        val now = System.currentTimeMillis()
        return capturedAtMillis > 0 && capturedAtMillis <= now && now - capturedAtMillis < TTL
    }
    companion object { const val MAX_BYTES = 32 * 1024 * 1024; private const val TTL = 86_400_000L }
}
