package com.anpfuel.domain.games

import kotlin.math.abs

class FuelFlight(seed: Int) : ArcadeSimulation {
    override var tick = 0
        private set
    override var score = 0
        private set
    override var end: RunEnd? = null
        private set
    var y = 800
        private set
    var velocity = 0
        private set
    private val random = ArcadeRandom(seed)
    private val activeGates = ArrayList<FlightGate>(ArcadeRules.MAX_ENTITIES)
    val gates: List<FlightGate> get() = activeGates

    override fun step(action: GameAction) {
        if (end != null) return
        tick++
        if (action == GameAction.IMPULSE) velocity = -22
        velocity = (velocity + 2).coerceAtMost(30)
        y += velocity
        if (tick >= 120 && (tick - 120) % 110 == 0 && activeGates.size < ArcadeRules.MAX_ENTITIES) {
            activeGates.add(FlightGate(1120, 480 + random.next(641)))
        }
        activeGates.forEach { it.x -= 6 }
        // Collision takes priority over every collection and pass in this tick.
        if (y - RADIUS <= 0 || y + RADIUS >= 1500 || activeGates.any {
                FlightCollision.hitsGate(y, it.x, it.center)
            }) {
            end = RunEnd.COLLISION
            return
        }
        activeGates.forEach {
            if (!it.collected && abs(it.x + 65 - PLAYER_X) <= RADIUS + 28 && abs(y - it.center) <= RADIUS + 28) {
                it.collected = true
                score += 5
            }
            if (!it.passed && it.x + 130 < PLAYER_X - RADIUS) {
                it.passed = true
                score++
            }
        }
        activeGates.removeAll { it.x < -172 }
        if (tick == ArcadeRules.MAX_TICKS) end = RunEnd.TIME_LIMIT
    }

    companion object {
        const val PLAYER_X = 220
        const val RADIUS = 42
    }
}

class FlightGate internal constructor(x: Int, val center: Int) {
    var x = x
        internal set
    var collected = false
        internal set
    var passed = false
        internal set
}

internal object FlightCollision {
    fun hitsGate(y: Int, x: Int, center: Int): Boolean =
        FuelFlight.PLAYER_X + FuelFlight.RADIUS >= x &&
            FuelFlight.PLAYER_X - FuelFlight.RADIUS <= x + 130 &&
            (y - FuelFlight.RADIUS <= center - 240 || y + FuelFlight.RADIUS >= center + 240)
}
