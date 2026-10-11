package com.anpfuel.application.port

/**
 * CS11A — Local training-consent state (default OFF).
 *
 * The switch preference, the per-device monotonic revision the
 * server orders, and the queued offline revocation. Synchronous
 * reads so the use case never blocks; the server stays
 * authoritative and reloads on every foreground.
 */
interface TrainingConsentLocalStore {

    fun isToggleOn(): Boolean

    fun setToggleOn(enabled: Boolean)

    /** Next per-device revision, persisted before returning. */
    fun nextRevision(): Long

    fun hasPendingRevocation(): Boolean

    fun setPendingRevocation(pending: Boolean)
}
