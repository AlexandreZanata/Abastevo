package com.anpfuel.domain.games

/** Mobile-only practice rules. These scores are never a verified national rank. */
object ArcadeRules {
    const val VERSION = "mobile-v1"
    const val TICKS_PER_SECOND = 60
    const val MAX_TICKS = 10_800
    const val MAX_ENTITIES = 8
    const val MAX_SCORE = 20_000
}

enum class GameId { FLIGHT, RUN }
enum class GameAction { NONE, IMPULSE, LEFT, RIGHT, JUMP, SLIDE }
enum class RunEnd { COLLISION, TIME_LIMIT }

interface ArcadeSimulation {
    val tick: Int
    val score: Int
    val end: RunEnd?
    fun step(action: GameAction = GameAction.NONE)
}

/** xorshift32: wrapping 32-bit operations and logical right shift, zero maps to 1. */
class ArcadeRandom(seed: Int) {
    private var state = if (seed == 0) 1 else seed
    fun next(bound: Int): Int {
        require(bound > 0)
        var x = state
        x = x xor (x shl 13)
        x = x xor (x ushr 17)
        x = x xor (x shl 5)
        state = x
        return ((x.toLong() and 0xffff_ffffL) % bound).toInt()
    }
}
