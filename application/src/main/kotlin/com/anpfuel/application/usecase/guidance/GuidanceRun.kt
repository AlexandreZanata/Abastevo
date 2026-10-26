package com.anpfuel.application.usecase.guidance

import com.anpfuel.domain.routing.NavigationFix
import com.anpfuel.domain.routing.NavigationFixPolicy

interface GuidanceRuntime {
    fun startNative(): Boolean
    fun stopNative()
    fun startLocation()
    fun stopLocation()
    fun startVoice()
    fun pauseVoice()
    fun stopVoice()
}

/** One main-thread run. Resource ownership is independent of Android lifecycle. */
class GuidanceRun(private val owner: String, private val currentOwner: () -> String, private val runtime: GuidanceRuntime) {
    var active: Boolean = false
        private set
    private var cleanupNeeded = false

    fun start(): Boolean {
        if (!ownerMatches()) return false
        if (active) return true
        cleanupNeeded = true
        active = true
        return try {
            if (!runtime.startNative()) { stop(); false } else {
                // Voice absence never prevents textual native guidance.
                try { runtime.startVoice() } catch (_: Exception) { }
                runtime.startLocation()
                true
            }
        } catch (_: Exception) { stop(); false }
    }

    fun accept(fix: NavigationFix, nowNanos: Long): Boolean {
        if (!checkOwner()) return false
        if (NavigationFixPolicy.accepts(fix, nowNanos)) return true
        try { runtime.pauseVoice() } catch (_: Exception) { }
        return false
    }

    fun checkOwner(): Boolean {
        if (!active) return false
        if (ownerMatches()) return true
        stop()
        return false
    }

    fun stop() {
        active = false
        if (!cleanupNeeded) return
        cleanupNeeded = false
        // Always attempt the remaining cleanup if one platform resource fails.
        listOf(runtime::stopLocation, runtime::stopVoice, runtime::stopNative).forEach { release ->
            try { release() } catch (_: Exception) { }
        }
    }

    private fun ownerMatches(): Boolean = try { currentOwner() == owner } catch (_: Exception) { false }
}
