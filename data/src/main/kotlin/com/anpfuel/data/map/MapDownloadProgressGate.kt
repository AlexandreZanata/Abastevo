package com.anpfuel.data.map

/**
 * B-BR-MAP-DL-PROGRESS-01 — how often a streaming package stores its
 * downloaded bytes. The downloads screen polls the store, so a transfer that
 * only stores its start and end shows 0% until it completes. The gate stores
 * at least every [stepBytes] or every [intervalMillis], always the final byte,
 * never a value that goes backwards or past the package size. Pure.
 */
class MapDownloadProgressGate(
    private val totalBytes: Long,
    startBytes: Long,
    startMillis: Long,
    private val stepBytes: Long = DEFAULT_STEP_BYTES,
    private val intervalMillis: Long = DEFAULT_INTERVAL_MILLIS,
) {
    private var storedBytes = startBytes
    private var storedAtMillis = startMillis

    /** True when [downloaded] should be stored now; it then becomes the last stored value. */
    fun shouldStore(downloaded: Long, nowMillis: Long): Boolean {
        if (downloaded <= storedBytes || downloaded > totalBytes) return false
        val due = downloaded == totalBytes ||
            downloaded - storedBytes >= stepBytes ||
            nowMillis - storedAtMillis >= intervalMillis
        if (due) {
            storedBytes = downloaded
            storedAtMillis = nowMillis
        }
        return due
    }

    companion object {
        /** About 1% of the smallest real package; small enough for a smooth bar. */
        const val DEFAULT_STEP_BYTES: Long = 512L * 1024
        /** Matches the downloads screen poll period. */
        const val DEFAULT_INTERVAL_MILLIS: Long = 500L
    }
}
