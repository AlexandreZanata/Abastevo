package com.anpfuel.domain.games

enum class GamePhase { READY, RUNNING, PAUSED, FINISHED }

/** Fractional ticks in nanoseconds × 60, avoiding truncated tick durations. */
class FixedTickClock {
    private var previous: Long? = null
    private var accumulated = 0L
    val interpolation: Float get() = accumulated / 1_000_000_000f

    fun reset() {
        previous = null
        accumulated = 0
    }

    fun frame(nowNanos: Long, step: () -> Unit) {
        require(nowNanos >= 0)
        val last = previous
        previous = nowNanos
        if (last == null || nowNanos < last) {
            accumulated = 0
            return
        }
        val elapsed = (nowNanos - last).coerceAtMost(100_000_000L)
        accumulated = (accumulated + elapsed * ArcadeRules.TICKS_PER_SECOND).coerceAtMost(5_000_000_000L)
        while (accumulated >= 1_000_000_000L) {
            accumulated -= 1_000_000_000L
            step()
        }
    }
}

class ArcadeSession(val gameId: GameId, private val seed: Int) {
    var simulation: ArcadeSimulation = createSimulation()
        private set
    var phase = GamePhase.READY
        private set
    private val clock = FixedTickClock()
    private var pending = GameAction.NONE
    val interpolation: Float get() = clock.interpolation

    fun start() {
        if (phase == GamePhase.READY) {
            clock.reset()
            phase = GamePhase.RUNNING
        }
    }

    fun pause() {
        if (phase == GamePhase.RUNNING) {
            phase = GamePhase.PAUSED
            pending = GameAction.NONE
            clock.reset()
        }
    }

    fun resume() {
        if (phase == GamePhase.PAUSED) {
            clock.reset()
            phase = GamePhase.RUNNING
        }
    }

    fun restart() {
        simulation = createSimulation()
        pending = GameAction.NONE
        clock.reset()
        phase = GamePhase.RUNNING
    }

    fun enqueue(action: GameAction) {
        if (phase == GamePhase.RUNNING && pending == GameAction.NONE) pending = action
    }

    fun frame(nowNanos: Long) {
        if (phase != GamePhase.RUNNING) return
        clock.frame(nowNanos) {
            simulation.step(pending)
            pending = GameAction.NONE
            if (simulation.end != null) phase = GamePhase.FINISHED
        }
    }

    private fun createSimulation(): ArcadeSimulation = when (gameId) {
        GameId.FLIGHT -> FuelFlight(seed)
        GameId.RUN -> FuelRun(seed)
    }
}

object LocalBestPolicy {
    fun sanitize(value: Int): Int = if (value in 0..ArcadeRules.MAX_SCORE) value else 0
    fun update(existing: Int, phase: GamePhase, score: Int): Int =
        if (phase == GamePhase.FINISHED && score in 0..ArcadeRules.MAX_SCORE) {
            maxOf(sanitize(existing), score)
        } else sanitize(existing)
}
