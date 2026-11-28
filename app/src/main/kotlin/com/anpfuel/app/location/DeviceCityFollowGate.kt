package com.anpfuel.app.location

import javax.inject.Inject
import javax.inject.Singleton

/**
 * B-BR-LOC-03 pacing, process-wide and in memory only: Explorar asks for
 * location once per app process and follows the device city at most once per
 * [FOLLOW_INTERVAL_MILLIS]. Undo or a manual city choice stops following
 * until the process ends, so a deliberate choice is never overwritten.
 */
@Singleton
class DeviceCityFollowGate @Inject constructor() {

    private var permissionPromptClaimed = false
    private var lastFollowAtMillis: Long? = null

    @Volatile
    var isSuppressed: Boolean = false
        private set

    /** True only for the first caller in this process. */
    @Synchronized
    fun claimPermissionPrompt(): Boolean {
        if (permissionPromptClaimed) return false
        permissionPromptClaimed = true
        return true
    }

    /** True when a follow may start now; the attempt itself starts the next interval. */
    @Synchronized
    fun tryBeginFollow(nowMillis: Long): Boolean {
        if (isSuppressed) return false
        val last = lastFollowAtMillis
        if (last != null && nowMillis - last in 0 until FOLLOW_INTERVAL_MILLIS) return false
        lastFollowAtMillis = nowMillis
        return true
    }

    fun suppressForSession() {
        isSuppressed = true
    }

    companion object {
        const val FOLLOW_INTERVAL_MILLIS: Long = 15 * 60 * 1000L
    }
}
