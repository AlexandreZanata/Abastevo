package com.anpfuel.data.local.media

import android.graphics.Bitmap
import android.graphics.BitmapFactory
import com.anpfuel.application.portable.PhotoDecoder
import com.anpfuel.application.portable.PhotoEncoder
import com.anpfuel.application.portable.PhotoFlow
import com.anpfuel.domain.portable.PortablePhoto
import java.io.ByteArrayOutputStream

/**
 * Bounded JPEG quality schedule per encode attempt (P15-T02B).
 *
 * Pure values, JVM-tested: attempt 1 aims near the 150 KiB target,
 * later attempts tighten toward the 256 KiB hard cap. Out-of-range
 * attempts clamp to the last step; the flow never exceeds 3.
 */
fun qualityForAttempt(attempt: Int): Int =
    when {
        attempt <= 1 -> 85
        attempt == 2 -> 70
        else -> 55
    }

/**
 * Native Android photo codec: sample-decode probe plus bounded JPEG
 * re-encode (P15-T02B, M02).
 *
 * [probeDims] reads headers only (`inJustDecodeBounds`, zero pixel
 * allocation) and answers null on corrupt/truncated/animated input.
 * [encode] decodes at the portable sample plan (never full camera
 * pixels), strips metadata by re-encoding (EXIF/GPS/orientation never
 * survive: only pixels cross into the new JPEG) and compresses at the
 * attempt quality; any failure is a null, never a throw. Work stays
 * off the UI thread at the call site (single image at a time, M02).
 *
 * The ~40 BitmapFactory lines below cannot run on JVM unit tests (no
 * Android runtime): they are statically reviewed and lint-guarded,
 * while [qualityForAttempt], the portable sample plan and the
 * seal/cache lanes carry the behavioral suites.
 */
class AndroidPhotoCodec : PhotoDecoder, PhotoEncoder {

    override fun probeDims(bytes: ByteArray): PhotoFlow.Dims? {
        if (bytes.isEmpty() || bytes.size > 32 * 1024 * 1024) return null
        return try {
            val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
            BitmapFactory.decodeByteArray(bytes, 0, bytes.size, bounds)
            if (bounds.outWidth <= 0 || bounds.outHeight <= 0 || bounds.outWidth.toLong() * bounds.outHeight > 100_000_000) {
                null
            } else {
                PhotoFlow.Dims(bounds.outWidth, bounds.outHeight)
            }
        } catch (_: Exception) {
            null
        }
    }

    override fun encode(source: ByteArray, request: PhotoFlow.EncodeRequest): ByteArray? {
        if (source.isEmpty() || request.sampleSize < 1) return null
        return try {
            val bitmap = BoundedPhotoBitmap.decode(source, PortablePhoto.MAX_EDGE_PIXELS, request.sampleSize)
            val out = ByteArrayOutputStream()
            try {
                if (!bitmap.compress(Bitmap.CompressFormat.JPEG, qualityForAttempt(request.attempt), out)) {
                    return null
                }
                out.toByteArray()
            } finally {
                if (!bitmap.isRecycled) bitmap.recycle()
                out.close()
            }
        } catch (_: Exception) {
            null
        }
    }
}
