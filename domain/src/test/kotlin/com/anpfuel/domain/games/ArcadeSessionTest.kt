package com.anpfuel.domain.games

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class ArcadeSessionTest {
    @Test fun differentDisplayRatesProduceExactly60TicksPerSecond() {
        for (rate in listOf(30, 60, 90, 120)) {
            val clock = FixedTickClock(); var count = 0
            clock.frame(0) { count++ }
            for (frame in 1..rate) clock.frame(frame * 1_000_000_000L / rate) { count++ }
            assertEquals(60, count, "display rate $rate")
            assertEquals(0f, clock.interpolation)
        }
    }
    @Test fun stallsCannotCreateUnboundedWorkOrCatchUpAfterReset() {
        val clock = FixedTickClock(); var count = 0
        clock.frame(0) { count++ }
        clock.frame(Long.MAX_VALUE) { count++ }
        assertEquals(5, count)
        clock.reset()
        clock.frame(1_000) { count++ }
        assertEquals(5, count)
        clock.frame(0) { count++ } // Regressed clock resets; never negative ticks.
        assertEquals(5, count)
    }
    @Test fun pauseResumeDropsBackgroundDebtAndInput() {
        val session = ArcadeSession(GameId.FLIGHT, 1)
        session.enqueue(GameAction.IMPULSE)
        session.frame(0)
        assertEquals(0, session.simulation.tick)
        session.start(); session.frame(0); session.frame(20_000_000)
        assertEquals(802, (session.simulation as FuelFlight).y)
        session.enqueue(GameAction.IMPULSE); session.pause()
        session.frame(Long.MAX_VALUE)
        assertEquals(1, session.simulation.tick)
        assertEquals(GamePhase.PAUSED, session.phase)
        session.resume(); session.frame(1_000_000_000); session.frame(1_020_000_000)
        assertEquals(806, (session.simulation as FuelFlight).y)
    }
    @Test fun atMostOneQueuedActionPerTick() {
        val session = ArcadeSession(GameId.RUN, 1)
        session.start(); session.frame(0)
        session.enqueue(GameAction.LEFT); session.enqueue(GameAction.RIGHT)
        session.frame(20_000_000)
        assertEquals(-1, (session.simulation as FuelRun).lane)
    }
    @Test fun finishedCannotResumeAndRestartClearsState() {
        val session = ArcadeSession(GameId.FLIGHT, 1)
        session.start(); session.frame(0)
        for (i in 1..100) session.frame(i * 20_000_000L)
        assertEquals(GamePhase.FINISHED, session.phase)
        val finalTick = session.simulation.tick
        session.resume(); session.start(); session.enqueue(GameAction.IMPULSE); session.frame(Long.MAX_VALUE)
        assertEquals(finalTick, session.simulation.tick)
        session.restart()
        assertEquals(GamePhase.RUNNING, session.phase)
        assertEquals(0, session.simulation.tick)
        assertEquals(800, (session.simulation as FuelFlight).y)
    }
    @Test fun recreationAbandonsUnfinishedRun() {
        val old = ArcadeSession(GameId.RUN, 2)
        old.start(); old.frame(0); old.frame(80_000_000); old.pause()
        val recreated = ArcadeSession(GameId.RUN, 2)
        assertEquals(GamePhase.READY, recreated.phase)
        assertEquals(0, recreated.simulation.tick)
        assertNull(recreated.simulation.end)
    }
    @Test fun bestNeverUsesPausedUnfinishedNegativeOrCorruptScores() {
        assertEquals(7, LocalBestPolicy.update(7, GamePhase.PAUSED, 100))
        assertEquals(7, LocalBestPolicy.update(7, GamePhase.RUNNING, 100))
        assertEquals(7, LocalBestPolicy.update(7, GamePhase.READY, 100))
        assertEquals(7, LocalBestPolicy.update(7, GamePhase.FINISHED, -1))
        assertEquals(7, LocalBestPolicy.update(7, GamePhase.FINISHED, Int.MAX_VALUE))
        assertEquals(7, LocalBestPolicy.update(7, GamePhase.FINISHED, 3))
        assertEquals(9, LocalBestPolicy.update(7, GamePhase.FINISHED, 9))
        assertEquals(0, LocalBestPolicy.sanitize(-1))
        assertEquals(0, LocalBestPolicy.sanitize(Int.MAX_VALUE))
        assertEquals(ArcadeRules.MAX_SCORE, LocalBestPolicy.sanitize(ArcadeRules.MAX_SCORE))
    }
    @Test fun gameplayMatchesAcross30And120Hz() {
        fun run(rate: Int): ArcadeSimulation {
            val session = ArcadeSession(GameId.RUN, 99)
            session.start(); session.frame(0)
            for (frame in 1..rate * 2) session.frame(frame * 1_000_000_000L / rate)
            return session.simulation
        }
        val slow = run(30); val fast = run(120)
        assertEquals(120, slow.tick)
        assertEquals(listOf(slow.tick, slow.score, slow.end), listOf(fast.tick, fast.score, fast.end))
    }
}
