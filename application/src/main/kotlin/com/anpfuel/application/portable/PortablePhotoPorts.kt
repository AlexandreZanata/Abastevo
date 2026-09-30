package com.anpfuel.application.portable

import com.anpfuel.application.portable.PhotoFlow.Dims
import com.anpfuel.application.portable.PhotoFlow.EncodeRequest

/**
 * Explicit platform ports for the portable photo pipeline (P15-T02A).
 *
 * Pure Kotlin with zero `java.*`/Android imports so this file moves unchanged
 * into a future `commonMain` source set. Native adapters implement these
 * behind the boundary: Android via BitmapFactory sample-decode plus sealed
 * private cache files, iOS via ImageIO downsampling plus its ephemeral
 * cache. Tests inject deterministic fakes; no real codec, clock,
 * randomness or I/O ever lives in [PhotoFlow].
 */
interface PhotoDecoder {

    /**
     * Header probe: real dimensions or null when the bytes are corrupt,
     * truncated, animated or otherwise undecodable. Never allocates
     * full pixels and never throws: malformed input is a null, not an
     * exception.
     */
    fun probeDims(bytes: ByteArray): Dims?
}

interface PhotoEncoder {

    /**
     * One bounded encode attempt at the planned sample size: stripped,
     * re-encoded JPEG bytes or null when this attempt fails. Attempt
     * numbering starts at 1; adapters tighten quality per attempt.
     */
    fun encode(source: ByteArray, request: EncodeRequest): ByteArray?
}

interface PhotoCache {

    /** Sealed transient publish (private app files only, never Gallery). */
    fun put(id: String, bytes: ByteArray, capturedAtMillis: Long)

    /** Reads one live entry, or null when missing/expired/corrupt. */
    fun get(id: String): ByteArray?

    /** Deletes one entry; unknown ids are a no-op. */
    fun delete(id: String)

    /** Purges entries past their 24 h lifetime; returns the purged count. */
    fun sweepExpired(nowMillis: Long): Int
}

interface PhotoClock {

    /** Wall millis for capture stamps and expiry math (device clock). */
    fun nowMillis(): Long
}

fun interface PhotoIdSource {

    /** Fresh opaque entry id per successful prepare (never reused). */
    fun nextId(): String
}

/** Wiring bundle for [PhotoFlow]; every port is required (no nulls). */
data class PhotoPorts(
    val decoder: PhotoDecoder,
    val encoder: PhotoEncoder,
    val cache: PhotoCache,
    val clock: PhotoClock,
    val ids: PhotoIdSource,
)
