package com.anpfuel.app.games

import android.content.SharedPreferences
import io.mockk.every
import io.mockk.mockk
import com.anpfuel.domain.games.GameId
import com.anpfuel.domain.games.GamePhase
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class GamesRecordsTest {
    private class Disk : GameValues {
        val values = mutableMapOf<String, Int>()
        var writable = true
        override fun readInt(key: String): Int = values[key] ?: 0
        override fun writeInt(key: String, value: Int): Boolean {
            if (!writable) return false
            values[key] = value
            return true
        }
    }
    @Test fun recordsSurviveRecreationAndStaySeparatePerGameAndRules() {
        val disk = Disk(); val records = GamesRecords(disk)
        records.record(GameId.FLIGHT, GamePhase.FINISHED, 8)
        records.record(GameId.RUN, GamePhase.FINISHED, 41)
        records.record(GameId.FLIGHT, GamePhase.FINISHED, 2)
        val recreated = GamesRecords(disk)
        assertEquals(8, recreated.best(GameId.FLIGHT))
        assertEquals(41, recreated.best(GameId.RUN))
        assertEquals(setOf("FLIGHT_mobile-v1_best", "RUN_mobile-v1_best"), disk.values.keys)
        disk.values["FLIGHT_old-rules_best"] = 900
        assertEquals(8, recreated.best(GameId.FLIGHT))
    }
    @Test fun unfinishedGamesNeverWriteARecord() {
        val disk = Disk(); val records = GamesRecords(disk)
        for (phase in listOf(GamePhase.READY, GamePhase.RUNNING, GamePhase.PAUSED)) {
            records.record(GameId.RUN, phase, 150)
        }
        assertTrue(disk.values.isEmpty())
    }
    @Test fun failedDiskWriteDoesNotClaimSavedAndCanRetry() {
        val disk = Disk(); val records = GamesRecords(disk)
        disk.writable = false
        assertEquals(BestSave(0, false), records.record(GameId.RUN, GamePhase.FINISHED, 9))
        disk.writable = true
        assertEquals(BestSave(9, true), records.record(GameId.RUN, GamePhase.FINISHED, 9))
    }
    @Test fun androidFailedCommitCacheIsRestoredSoRetryReallyWrites() {
        val preferences = mockk<SharedPreferences>()
        val editor = mockk<SharedPreferences.Editor>()
        val memory = mutableMapOf("RUN_mobile-v1_best" to 3)
        var pendingKey = ""
        var pendingValue = 0
        val results = ArrayDeque(listOf(false, true))
        every { preferences.getInt(any(), any()) } answers { memory[firstArg()] ?: 0 }
        every { preferences.edit() } returns editor
        every { editor.putInt(any(), any()) } answers {
            pendingKey = firstArg(); pendingValue = secondArg(); editor
        }
        every { editor.commit() } answers {
            memory[pendingKey] = pendingValue // Real Android updates memory even on false.
            results.removeFirst()
        }
        every { editor.apply() } answers { memory[pendingKey] = pendingValue; Unit }
        val records = GamesRecords(PreferenceGameValues(preferences))
        assertEquals(BestSave(3, false), records.record(GameId.RUN, GamePhase.FINISHED, 9))
        assertEquals(3, records.best(GameId.RUN))
        assertEquals(BestSave(9, true), records.record(GameId.RUN, GamePhase.FINISHED, 9))
        assertTrue(results.isEmpty())
    }

    @Test fun corruptValuesAreNotDisplayedAndAreReplacedByValidCompletedScore() {
        val disk = Disk(); disk.values["RUN_mobile-v1_best"] = Int.MAX_VALUE
        val records = GamesRecords(disk)
        assertEquals(0, records.best(GameId.RUN))
        records.record(GameId.RUN, GamePhase.FINISHED, 10)
        assertEquals(10, records.best(GameId.RUN))
    }
}
