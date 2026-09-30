package com.anpfuel.application.portable

/**
 * Explicit platform ports for portable application logic (P12-T03).
 *
 * Pure Kotlin with zero `java.*`/Android imports so this file moves unchanged
 * into a future `commonMain` source set. Native adapters implement these
 * behind the boundary: Android via Room/WorkManager/Hilt, iOS via its
 * persistence/background execution. Tests inject deterministic fakes; no
 * real clock, randomness or I/O ever lives in portable use cases.
 */
interface PortableClock {

    /** Milliseconds on a monotonic per-device timeline (no wall-clock math). */
    fun tickMillis(): Long
}

fun interface PortableNonceSource {

    /** Fresh opaque single-use value per dispatch attempt (never reused). */
    fun nextNonce(): String
}
