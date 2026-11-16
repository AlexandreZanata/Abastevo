package com.anpfuel.data.enterprise.attachment

import android.content.Context
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.net.Uri
import androidx.core.content.FileProvider
import com.anpfuel.application.enterprise.attachment.ReceiptOriginals
import com.anpfuel.domain.enterprise.attachment.LumaSample
import com.anpfuel.domain.enterprise.attachment.ReceiptIntake
import com.anpfuel.domain.enterprise.attachment.ReceiptLegibility
import java.io.ByteArrayOutputStream
import java.io.File
import java.io.FileNotFoundException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/**
 * EMP-DOC-004 platform side (EDR-291): the system camera writes into one private cache file served by the app's
 * FileProvider (`enterprise-receipts/`), every selection is read once and bounded, and a decoded luminance sample feeds
 * the legibility hint (EDR-292). Nothing here persists a photo: only the EMP-OFFL-005 sealed store does.
 */
class AndroidReceiptOriginals(private val context: Context) : ReceiptOriginals {
    private val directory: File get() = File(context.cacheDir, DIRECTORY).apply { mkdirs() }
    private val authority: String get() = context.packageName + ".fileprovider"

    override suspend fun cameraTarget(): String = withContext(Dispatchers.IO) {
        sweep()
        val file = File.createTempFile(PREFIX, SUFFIX, directory)
        FileProvider.getUriForFile(context, authority, file).toString()
    }

    override suspend fun read(reference: String): ByteArray? = withContext(Dispatchers.IO) {
        val uri = Uri.parse(reference)
        if (uri.scheme != "content") return@withContext null
        val stream = try { context.contentResolver.openInputStream(uri) } catch (_: FileNotFoundException) { null } ?: return@withContext null
        stream.use { input ->
            val output = ByteArrayOutputStream()
            val buffer = ByteArray(BUFFER)
            var total = 0L
            // One byte beyond the budget is enough to recognise a file that is too large (B-BR-EMP-451).
            while (total < ReceiptIntake.READ_LIMIT) {
                val count = input.read(buffer, 0, minOf(BUFFER.toLong(), ReceiptIntake.READ_LIMIT - total).toInt())
                if (count < 0) break
                output.write(buffer, 0, count)
                total += count
            }
            output.toByteArray().takeIf { it.isNotEmpty() }
        }
    }

    override suspend fun discard(reference: String) {
        withContext(Dispatchers.IO) { own(Uri.parse(reference))?.delete() }
    }

    override suspend fun sample(bytes: ByteArray): LumaSample? = withContext(Dispatchers.Default) {
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        BitmapFactory.decodeByteArray(bytes, 0, bytes.size, bounds)
        val width = bounds.outWidth
        val height = bounds.outHeight
        if (width < LumaSample.MIN_SIDE || height < LumaSample.MIN_SIDE) return@withContext null
        var scale = 1
        while (maxOf(width, height) / (scale * 2) >= ReceiptLegibility.MAX_SAMPLE_SIDE) scale *= 2
        val decoded = BitmapFactory.decodeByteArray(bytes, 0, bytes.size, BitmapFactory.Options().apply {
            inSampleSize = scale
            inPreferredConfig = Bitmap.Config.ARGB_8888
        }) ?: return@withContext null
        try {
            val longest = maxOf(decoded.width, decoded.height)
            val target = if (longest <= ReceiptLegibility.MAX_SAMPLE_SIDE) decoded else Bitmap.createScaledBitmap(decoded,
                maxOf(LumaSample.MIN_SIDE, decoded.width * ReceiptLegibility.MAX_SAMPLE_SIDE / longest),
                maxOf(LumaSample.MIN_SIDE, decoded.height * ReceiptLegibility.MAX_SAMPLE_SIDE / longest), true)
            try {
                if (target.width < LumaSample.MIN_SIDE || target.height < LumaSample.MIN_SIDE) return@withContext null
                val pixels = IntArray(target.width * target.height)
                target.getPixels(pixels, 0, target.width, 0, 0, target.width, target.height)
                LumaSample.fromArgb(target.width, target.height, width, height, pixels)
            } finally { if (target !== decoded) target.recycle() }
        } catch (_: IllegalArgumentException) { null } finally { decoded.recycle() }
    }

    /** Deletes every leftover camera file; only this folder of the app's own cache is ever touched. */
    private fun sweep() {
        directory.listFiles()?.filter { it.isFile }?.forEach { it.delete() }
    }

    /** The app's own camera file behind [uri], or null for any other reference (the picker's files are never deleted). */
    private fun own(uri: Uri): File? {
        if (uri.scheme != "content" || uri.authority != authority) return null
        val segments = uri.pathSegments
        if (segments.size != 2 || segments[0] != PATH_NAME || !segments[1].matches(NAME)) return null
        val candidate = File(directory, segments[1]).canonicalFile
        return candidate.takeIf { it.parentFile == directory.canonicalFile }
    }

    override fun toString(): String = "AndroidReceiptOriginals(redacted)"

    companion object {
        /** The cache folder named by `res/xml/file_paths.xml` (`enterprise_receipts`). */
        const val DIRECTORY = "enterprise-receipts"
        const val PATH_NAME = "enterprise_receipts"
        private const val PREFIX = "receipt_"
        private const val SUFFIX = ".jpg"
        private const val BUFFER = 8192
        private val NAME = Regex("receipt_[A-Za-z0-9_-]+\\.jpg")
    }
}
