package com.anpfuel.application.portable

import com.anpfuel.domain.portable.PortablePhoto

/**
 * Portable lightweight-photo pipeline shared by Android and iPhone
 * (P15-T02A, B-BR-M01…M03).
 *
 * Pure Kotlin with zero `java.*`/Android imports so this file moves unchanged
 * into a future `commonMain` source set. The flow plans sample-decode from
 * probed headers (never full-res allocation), bounds encoding to 3 attempts
 * inside the frozen KiB budgets and publishes to the transient cache only on
 * success — an interrupted capture leaves no partial entry (write-then-
 * publish). The transient cache holds private app files only (never the
 * Gallery); expiry is checked on launch/resume/read. The server always
 * revalidates; the client only hints. Native codecs, sealed storage and
 * device measurement arrive behind [PhotoPorts]; device evidence stays
 * deferred to the release horizon per owner decision.
 */
class PhotoFlow(private val ports: PhotoPorts) {

    /** Probed frame dimensions from real headers (never client claims). */
    data class Dims(val width: Int, val height: Int)

    /** One bounded encode attempt input. */
    data class EncodeRequest(
        val sampleSize: Int,
        val attempt: Int,
    )

    /** Prepared photo outcome: sealed ready entry or a stable hint code. */
    sealed interface PhotoResult {
        data class Ready(
            val id: String,
            val bytes: Long,
            val attempts: Int,
        ) : PhotoResult

        data class Refused(val code: String) : PhotoResult
    }

    /**
     * Prepares one capture: supported intent → header probe → sample plan →
     * bounded encode (≤3) inside the 256 KiB cap → transient publish.
     * Corrupt/oversize/unsupported inputs refuse with stable codes;
     * failures never leave partial cache entries.
     */
    fun prepare(intentBytes: ByteArray, mime: String): PhotoResult =
        prepareAt(intentBytes, mime, ports.clock.nowMillis())

    /** Crops/retries retain original age; expired evidence never gets a fresh TTL. */
    fun prepareAt(intentBytes: ByteArray, mime: String, capturedAtMillis: Long): PhotoResult {
        val now = ports.clock.nowMillis()
        if (capturedAtMillis <= 0 || capturedAtMillis > now || now - capturedAtMillis >= 86_400_000) {
            return PhotoResult.Refused("photo.expired")
        }
        if (intentBytes.isEmpty() || intentBytes.size > 32 * 1024 * 1024) return PhotoResult.Refused(PortablePhoto.UNDECODABLE_INPUT)
        if (!PortablePhoto.isSupportedIntentMime(mime)) {
            return PhotoResult.Refused(PortablePhoto.UNSUPPORTED_FORMAT)
        }
        val dims = ports.decoder.probeDims(intentBytes)
            ?: return PhotoResult.Refused(PortablePhoto.UNDECODABLE_INPUT)
        if (dims.width <= 0 || dims.height <= 0 || dims.width.toLong() * dims.height > 100_000_000) {
            return PhotoResult.Refused(PortablePhoto.UNDECODABLE_INPUT)
        }
        val sampleSize = PortablePhoto.sampleSizeForBounds(dims.width, dims.height)
        var sawBytes = false
        for (attempt in 1..PortablePhoto.MAX_ATTEMPTS) {
            val encoded = ports.encoder.encode(intentBytes, EncodeRequest(sampleSize, attempt))
            if (encoded == null) continue
            if (!PortablePhoto.fitsWireCap(encoded.size.toLong())) {
                sawBytes = true
                continue
            }
            val id = ports.ids.nextId()
            ports.cache.put(id, encoded, capturedAtMillis)
            return PhotoResult.Ready(id, encoded.size.toLong(), attempt)
        }
        // Attempts produced only oversize bytes → over budget; attempts
        // produced nothing → the encoder failed (corrupt/alpha/rotated
        // inputs surface here when the probe passed headers).
        return if (sawBytes) {
            PhotoResult.Refused(PortablePhoto.OVER_BUDGET)
        } else {
            PhotoResult.Refused(PortablePhoto.ENCODE_FAILED)
        }
    }

    /** Removes one transient entry (post-upload delete, rights erasure). */
    fun discard(id: String) {
        ports.cache.delete(id)
    }

    /**
     * Sweeps expired transient entries (launch/resume/read hook, M05).
     * Returns the purged count; failures isolate per entry behind the port.
     */
    fun sweepExpired(): Int =
        ports.cache.sweepExpired(ports.clock.nowMillis())
}
