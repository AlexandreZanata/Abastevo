package com.anpfuel.app.ui.driver.work

import com.anpfuel.data.local.entity.DriverGpsObservationEntity
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/** Calls, including callbacks, must be confined to the service's main thread. */
internal class DriverGpsLifecycle(
    private val currentOwner: () -> String,
    private val eligible: suspend (String, String) -> Boolean,
    private val persist: suspend (DriverGpsObservationEntity) -> Unit,
    private val attach: () -> Unit,
    private val detach: () -> Unit,
) {
    private data class Recording(val owner: String, val shift: String, val id: String, val started: Long, var metres: Double? = null)
    private val lock = Mutex()
    private var recording: Recording? = null
    val isActive: Boolean get() = recording != null

    suspend fun start(owner: String, shift: String, id: String, now: Long): Boolean = lock.withLock {
        if (recording != null || owner != currentOwner() || shift.isBlank() || id.isBlank() || !eligible(owner, shift)) return false
        val next = Recording(owner, shift, id, now)
        persist(observation(next, now))
        if (owner != currentOwner()) return false
        recording = next
        try {
            attach()
        } catch (failure: Exception) {
            recording = null
            detach()
            throw failure
        }
        true
    }

    /** False tells the caller to stop immediately after a detected owner change. */
    fun addMetres(step: Double): Boolean {
        val current = recording ?: return false
        if (current.owner != currentOwner()) return false
        if (step.isFinite() && step >= 0 && step <= Long.MAX_VALUE.toDouble()) {
            val total = (current.metres ?: 0.0) + step
            if (total.isFinite() && total < Long.MAX_VALUE.toDouble()) current.metres = total
        }
        return true
    }

    suspend fun stop(now: Long) = lock.withLock {
        val current = recording ?: return
        recording = null
        detach()
        persist(observation(current, maxOf(now, current.started)))
    }

    fun destroy() {
        recording = null
        detach()
    }

    private fun observation(value: Recording, now: Long) = DriverGpsObservationEntity(
        value.owner, "android", value.id, value.shift, value.started, now,
        value.metres?.toLong(), false, true, value.started,
    )
}
