package com.anpfuel.data.local.media

import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.graphics.BitmapRegionDecoder
import android.graphics.Matrix
import android.graphics.Rect
import android.os.Build
import androidx.exifinterface.media.ExifInterface
import java.io.ByteArrayInputStream

/** Shared upright pixel decoder: headers/bytes/dimensions are bounded before allocation. */
object BoundedPhotoBitmap {
    fun decode(bytes: ByteArray, maxSide: Int = 2048, minimumSample: Int = 1): Bitmap {
        require(bytes.isNotEmpty() && bytes.size <= 32 * 1024 * 1024 && maxSide in 120..2048 && minimumSample in 1..1024) { "photo.input-size" }
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        BitmapFactory.decodeByteArray(bytes, 0, bytes.size, bounds)
        require(bounds.outWidth > 0 && bounds.outHeight > 0 && bounds.outWidth.toLong() * bounds.outHeight <= 100_000_000) { "photo.invalid-image" }
        var sample = minimumSample
        while (maxOf(bounds.outWidth, bounds.outHeight) / sample > maxSide) sample *= 2
        val raw = BitmapFactory.decodeByteArray(bytes, 0, bytes.size, BitmapFactory.Options().apply { inSampleSize = sample })
            ?: error("photo.invalid-image")
        return upright(raw, orientation(bytes))
    }

    /** Longest side of the upright photo in its own pixels, before any downsampling. */
    fun fullLongSide(bytes: ByteArray): Int {
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        BitmapFactory.decodeByteArray(bytes, 0, bytes.size, bounds)
        return maxOf(bounds.outWidth, bounds.outHeight)
    }

    /**
     * Upright pixels of one region of the photo, given as fractions of the upright photo, at the
     * finest sampling whose longest side stays within [maxSide]. A far board keeps the camera's
     * own detail instead of being enlarged from the downsampled whole photo.
     */
    fun decodeRegion(bytes: ByteArray, left: Float, top: Float, right: Float, bottom: Float, maxSide: Int = 2048): Bitmap {
        require(bytes.isNotEmpty() && bytes.size <= 32 * 1024 * 1024 && maxSide in 120..2048) { "photo.input-size" }
        require(left >= 0f && top >= 0f && right <= 1f && bottom <= 1f && left < right && top < bottom) { "photo.region" }
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        BitmapFactory.decodeByteArray(bytes, 0, bytes.size, bounds)
        require(bounds.outWidth > 0 && bounds.outHeight > 0 && bounds.outWidth.toLong() * bounds.outHeight <= 100_000_000) { "photo.invalid-image" }
        val orientation = orientation(bytes)
        // The same region in the stored (not yet upright) pixel grid.
        val stored = when (orientation) {
            ExifInterface.ORIENTATION_FLIP_HORIZONTAL -> floatArrayOf(1 - right, top, 1 - left, bottom)
            ExifInterface.ORIENTATION_ROTATE_180 -> floatArrayOf(1 - right, 1 - bottom, 1 - left, 1 - top)
            ExifInterface.ORIENTATION_FLIP_VERTICAL -> floatArrayOf(left, 1 - bottom, right, 1 - top)
            ExifInterface.ORIENTATION_TRANSPOSE -> floatArrayOf(top, left, bottom, right)
            ExifInterface.ORIENTATION_ROTATE_90 -> floatArrayOf(top, 1 - right, bottom, 1 - left)
            ExifInterface.ORIENTATION_TRANSVERSE -> floatArrayOf(1 - bottom, 1 - right, 1 - top, 1 - left)
            ExifInterface.ORIENTATION_ROTATE_270 -> floatArrayOf(1 - bottom, left, 1 - top, right)
            else -> floatArrayOf(left, top, right, bottom)
        }
        val rect = Rect(
            (stored[0] * bounds.outWidth).toInt().coerceIn(0, bounds.outWidth - 1),
            (stored[1] * bounds.outHeight).toInt().coerceIn(0, bounds.outHeight - 1),
            Math.ceil((stored[2] * bounds.outWidth).toDouble()).toInt().coerceIn(1, bounds.outWidth),
            Math.ceil((stored[3] * bounds.outHeight).toDouble()).toInt().coerceIn(1, bounds.outHeight))
        require(rect.width() > 0 && rect.height() > 0) { "photo.region" }
        var sample = 1
        while (maxOf(rect.width(), rect.height()) / sample > maxSide) sample *= 2
        @Suppress("DEPRECATION")
        val decoder = (if (Build.VERSION.SDK_INT >= 31) BitmapRegionDecoder.newInstance(bytes, 0, bytes.size)
            else BitmapRegionDecoder.newInstance(bytes, 0, bytes.size, false)) ?: error("photo.invalid-image")
        val raw = try {
            decoder.decodeRegion(rect, BitmapFactory.Options().apply { inSampleSize = sample }) ?: error("photo.invalid-image")
        } finally { decoder.recycle() }
        return upright(raw, orientation)
    }

    private fun orientation(bytes: ByteArray): Int =
        runCatching { ExifInterface(ByteArrayInputStream(bytes)).getAttributeInt(ExifInterface.TAG_ORIENTATION, ExifInterface.ORIENTATION_NORMAL) }
            .getOrDefault(ExifInterface.ORIENTATION_NORMAL)

    private fun upright(raw: Bitmap, orientation: Int): Bitmap {
        val matrix = Matrix().apply {
            when (orientation) {
                ExifInterface.ORIENTATION_FLIP_HORIZONTAL -> setScale(-1f, 1f)
                ExifInterface.ORIENTATION_ROTATE_180 -> setRotate(180f)
                ExifInterface.ORIENTATION_FLIP_VERTICAL -> setScale(1f, -1f)
                ExifInterface.ORIENTATION_TRANSPOSE -> { setRotate(90f); postScale(-1f, 1f) }
                ExifInterface.ORIENTATION_ROTATE_90 -> setRotate(90f)
                ExifInterface.ORIENTATION_TRANSVERSE -> { setRotate(270f); postScale(-1f, 1f) }
                ExifInterface.ORIENTATION_ROTATE_270 -> setRotate(270f)
            }
        }
        if (matrix.isIdentity) return raw
        return try {
            Bitmap.createBitmap(raw, 0, 0, raw.width, raw.height, matrix, true).also { if (it !== raw) raw.recycle() }
        } catch (error: Exception) { raw.recycle(); throw error }
    }
}
