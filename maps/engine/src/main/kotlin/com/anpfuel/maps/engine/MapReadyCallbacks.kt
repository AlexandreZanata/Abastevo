package com.anpfuel.maps.engine

/** Readiness callbacks belong to one host generation, including after dispatch. */
internal class MapReadyCallbacks {
    private var generation = 0L
    private val pending = mutableListOf<Pair<Long, () -> Unit>>()

    @Synchronized
    fun add(callback: () -> Unit) {
        pending += generation to callback
    }

    @Synchronized
    fun take(): List<() -> Unit> {
        val current = pending.toList()
        pending.clear()
        return current.map { (owner, callback) ->
            { deliver(owner, callback) }
        }
    }

    @Synchronized
    fun invalidate() {
        generation++
        pending.clear()
    }

    @Synchronized
    private fun deliver(owner: Long, callback: () -> Unit) {
        if (owner == generation) callback()
    }
}
