package com.anpfuel.domain.games

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class FuelRunTest {
    @Test fun lanesClampAndDoNotWrap() {
        val game = FuelRun(1)
        repeat(4) { game.step(GameAction.LEFT) }
        assertEquals(-1, game.lane)
        repeat(5) { game.step(GameAction.RIGHT) }
        assertEquals(1, game.lane)
    }
    @Test fun jumpSlideAreExclusiveAndExpireAtExactTick() {
        val game = FuelRun(1)
        game.step(GameAction.JUMP)
        assertEquals(48, game.jumpTicks)
        game.step(GameAction.SLIDE)
        assertEquals(0, game.slideTicks)
        assertEquals(47, game.jumpTicks)
        repeat(47) { game.step() }
        assertEquals(0, game.jumpTicks)
        game.step(GameAction.SLIDE)
        assertEquals(36, game.slideTicks)
        game.step(GameAction.JUMP)
        assertEquals(0, game.jumpTicks)
        repeat(35) { game.step() }
        assertEquals(0, game.slideTicks)
    }
    @Test fun unavoidedConeBeamTruckCollideAt261AndThenFreeze() {
        for (seed in 1..3) {
            val game = approach(seed)
            game.step()
            assertEquals(261, game.tick)
            assertEquals(RunEnd.COLLISION, game.end)
            assertEquals(4, game.score)
            game.step(GameAction.RIGHT)
            assertEquals(261, game.tick)
            assertEquals(-1, game.lane)
        }
    }
    @Test fun jumpAndSlideAtImpactAvoidRespectiveHazards() {
        val cone = approach(1); cone.step(GameAction.JUMP)
        assertNull(cone.end)
        val beam = approach(2); beam.step(GameAction.SLIDE)
        assertNull(beam.end)
        val truck = approach(3); truck.step(GameAction.JUMP)
        assertEquals(RunEnd.COLLISION, truck.end)
        val escaped = approach(3); escaped.step(GameAction.RIGHT)
        assertNull(escaped.end)
    }
    @Test fun expiredJumpOrSlideCannotAvoidImpact() {
        for ((seed, action, duration) in listOf(Triple(1, GameAction.JUMP, 48), Triple(2, GameAction.SLIDE, 36))) {
            val game = FuelRun(seed)
            repeat(261) { index ->
                game.step(when (index + 1) {
                    1 -> GameAction.LEFT
                    261 - duration -> action
                    else -> GameAction.NONE
                })
            }
            assertEquals(RunEnd.COLLISION, game.end)
        }
    }
    @Test fun fuelCollectsOnceAndNeverBecomesHazard() {
        val game = approach(4)
        game.step()
        assertNull(game.end)
        assertEquals(9, game.score)
        repeat(4) { game.step() }
        assertEquals(9, game.score)
    }
    @Test fun seededTraceIsReproducibleBoundedAndTerminatesAtThreeMinutes() {
        val a = FuelRun(19); val b = FuelRun(19)
        repeat(ArcadeRules.MAX_TICKS + 20) {
            val hazard = a.entities.firstOrNull { !it.crossed && it.depth <= 120 && it.kind != RunEntityKind.FUEL }
            val action = if (hazard?.lane == a.lane) {
                if (a.lane == 1) GameAction.LEFT else GameAction.RIGHT
            } else GameAction.NONE
            a.step(action); b.step(action)
            assertEquals(listOf(a.tick, a.score, a.lane, a.end), listOf(b.tick, b.score, b.lane, b.end))
            assertEquals(a.entities.map { Triple(it.lane, it.kind, it.depth) }, b.entities.map { Triple(it.lane, it.kind, it.depth) })
            assertTrue(a.entities.size <= ArcadeRules.MAX_ENTITIES)
        }
        assertEquals(ArcadeRules.MAX_TICKS, a.tick)
        assertEquals(RunEnd.TIME_LIMIT, a.end)
        assertEquals(20, a.speed)
        assertEquals(255, a.score) // Independently calculated integer reference vector.
    }
    private fun approach(seed: Int): FuelRun = FuelRun(seed).apply {
        step(GameAction.LEFT)
        repeat(259) { step() }
        assertNull(end)
        assertEquals(-1, entities.first().lane)
    }
}
