package com.anpfuel.domain.games

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class FuelFlightTest {
    @Test fun gravityImpulseAndGroundAreExact() {
        val game = FuelFlight(7)
        game.step()
        assertEquals(802, game.y)
        assertEquals(2, game.velocity)
        game.step(GameAction.IMPULSE)
        assertEquals(782, game.y)
        assertEquals(-20, game.velocity)
        repeat(100) { game.step() }
        assertEquals(RunEnd.COLLISION, game.end)
        val terminal = Triple(game.tick, game.score, game.y)
        game.step(GameAction.IMPULSE)
        assertEquals(terminal, Triple(game.tick, game.score, game.y))
    }

    @Test fun constantImpulsesHitCeilingWithoutPoints() {
        val game = FuelFlight(1)
        repeat(80) { game.step(GameAction.IMPULSE) }
        assertEquals(RunEnd.COLLISION, game.end)
        assertEquals(0, game.score)
        assertTrue(game.tick < 80)
    }

    @Test fun sameSeedAndActionsReproduceObstaclesAndScore() {
        val a = FuelFlight(1234)
        val b = FuelFlight(1234)
        repeat(500) {
            val action = if (a.y > (a.gates.firstOrNull { it.x + 130 >= FuelFlight.PLAYER_X - FuelFlight.RADIUS }?.center ?: 800)) GameAction.IMPULSE else GameAction.NONE
            a.step(action); b.step(action)
            assertEquals(listOf(a.tick, a.score, a.y, a.velocity, a.end), listOf(b.tick, b.score, b.y, b.velocity, b.end))
            assertEquals(a.gates.map { it.x to it.center }, b.gates.map { it.x to it.center })
            assertTrue(a.gates.size <= ArcadeRules.MAX_ENTITIES)
        }
        // Independently computed unsigned/integer reference vector (500 ticks, seed 1234).
        assertEquals(listOf(500, 17, 666, -14), listOf(a.tick, a.score, a.y, a.velocity))
        assertNull(a.end)
    }

    @Test fun collisionEdgesAreInclusiveAndGapIsSafe() {
        assertTrue(FlightCollision.hitsGate(602, 262, 800))
        assertTrue(FlightCollision.hitsGate(998, 262, 800))
        assertFalse(FlightCollision.hitsGate(603, 262, 800))
        assertFalse(FlightCollision.hitsGate(602, 263, 800))
        assertFalse(FlightCollision.hitsGate(602, 47, 800))
    }

    @Test fun threeMinuteLimitStopsAllFurtherSimulation() {
        val game = FuelFlight(1234)
        repeat(ArcadeRules.MAX_TICKS + 50) {
            val target = game.gates.firstOrNull { it.x + 130 >= 178 }?.center ?: 800
            game.step(if (game.y > target) GameAction.IMPULSE else GameAction.NONE)
            assertTrue(game.gates.size <= ArcadeRules.MAX_ENTITIES)
        }
        assertEquals(RunEnd.TIME_LIMIT, game.end)
        assertEquals(listOf(10_800, 576, 908, -12), listOf(game.tick, game.score, game.y, game.velocity))
    }

    @Test fun unrelatedActionsDoNotImpulse() {
        val a = FuelFlight(2); val b = FuelFlight(2)
        a.step(GameAction.LEFT); b.step()
        assertEquals(a.y, b.y)
    }

    @Test fun randomSequencePinsUnsignedRulesAndZeroSeed() {
        val random = ArcadeRandom(1)
        assertEquals(listOf(369, 689, 461, 695, 233), List(5) { random.next(1000) })
        val zero = ArcadeRandom(0); val one = ArcadeRandom(1)
        repeat(30) { assertEquals(zero.next(641), one.next(641)) }
        assertThrows(IllegalArgumentException::class.java) { random.next(0) }
    }
}
