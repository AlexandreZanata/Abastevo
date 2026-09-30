package com.anpfuel.domain.portable

/**
 * Portable lightweight-photo values (P15-T01/T02, B-BR-M01…M03).
 *
 * Pure Kotlin with zero `java.*` imports so this file moves unchanged into a
 * future `commonMain` source set. Mirrors the frozen backend forward budgets
 * (`backend/internal/modules/evidence/domain/budgets.go`): JPEG wire,
 * 150 KiB target, 256 KiB hard cap, 1600-pixel edge, 2 MP integer bound,
 * at most 3 encoding attempts, 32 MiB per-image working-memory hypothesis
 * and one 24 h transient deadline. The server always enforces; the client
 * only plans sample-decode, bounds attempts and expires its transient
 * cache. Device measurement of the memory hypothesis belongs to P15-T02
 * native evidence, never to this file.
 */
object PortablePhoto {

    /** Single frozen wire format: locally re-encoded JPEG, metadata stripped. */
    const val WIRE_MIME: String = "image/jpeg"

    /** Legibility target after bounded re-encoding: 150 KiB. */
    const val TARGET_BYTES: Long = 153600L

    /** Hard wire cap: 256 KiB. Larger results reject, never silently crush. */
    const val CAP_BYTES: Long = 262144L

    /** Longest-edge bound: 1600 pixels. */
    const val MAX_EDGE_PIXELS: Int = 1600

    /** Total-pixel bound: 2 MP as an integer (width * height <= 2_000_000). */
    const val MAX_PIXELS: Long = 2000000L

    /** Bounded encoding attempts per capture: 3. */
    const val MAX_ATTEMPTS: Int = 3

    /** Per-image working-memory hypothesis: 32 MiB (measured in P15-T02). */
    const val WORKING_MEMORY_HYPOTHESIS_BYTES: Long = 33554432L

    /** Transient app-cache lifetime from capture: 24 h in millis. */
    const val TRANSIENT_TTL_MILLIS: Long = 24L * 3600L * 1000L

    /** Local-hint refusal codes; never sent, never backend verdicts. */
    const val UNSUPPORTED_FORMAT: String = "unsupported-format"
    const val UNDECODABLE_INPUT: String = "undecodable-input"
    const val OVER_BUDGET: String = "over-budget"
    const val ENCODE_FAILED: String = "encode-failed"
    const val OK: String = "ok"

    /**
     * Intent formats the platform may decode (M02: JPEG/PNG/HEIF where the
     * platform codec handles them). Matching is exact after trim + ASCII
     * lowercase; anything else (GIF/WebP/BMP/SVG/empty) refuses before any
     * decode allocation. Animated inputs refuse at the native probe.
     */
    fun isSupportedIntentMime(mime: String): Boolean {
        when (mime.trim().lowercase()) {
            "image/jpeg", "image/png", "image/heic", "image/heif" -> return true
            else -> return false
        }
    }

    /**
     * Power-of-2 sample size so the decoded frame fits the frozen pixel
     * budgets without ever allocating full camera pixels (M02:
     * sample-decode first). Returns the smallest power of 2 with
     * width/size <= 1600, height/size <= 1600 and their product <= 2 MP.
     * Non-positive dimensions throw: callers probe real headers first.
     */
    fun sampleSizeForBounds(width: Int, height: Int): Int {
        require(width > 0 && height > 0) { "non-positive frame" }
        var size = 1
        while (width / size > MAX_EDGE_PIXELS ||
            height / size > MAX_EDGE_PIXELS ||
            (width / size).toLong() * (height / size).toLong() > MAX_PIXELS
        ) {
            size *= 2
        }
        return size
    }

    /** Decoded dimensions after sampling (integer floor, never zero). */
    fun sampledDims(width: Int, height: Int, sampleSize: Int): Pair<Int, Int> {
        require(sampleSize >= 1) { "sample size below one" }
        val w = (width / sampleSize).coerceAtLeast(1)
        val h = (height / sampleSize).coerceAtLeast(1)
        return w to h
    }

    /** True when wire bytes sit inside the frozen hard cap (nonempty). */
    fun fitsWireCap(bytes: Long): Boolean =
        bytes in 1..CAP_BYTES

    /**
     * True when a transient entry captured at [capturedAtMillis] is past
     * its 24 h lifetime at [nowMillis] (M05: expiry checked on
     * launch/resume/read; a powered-off device deletes at next run).
     */
    fun isTransientExpired(capturedAtMillis: Long, nowMillis: Long): Boolean =
        nowMillis - capturedAtMillis >= TRANSIENT_TTL_MILLIS
}
