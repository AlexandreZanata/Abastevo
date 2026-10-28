package com.anpfuel.app.ui.driver.work

import com.anpfuel.data.local.entity.DriverShiftEntity
import com.anpfuel.domain.driver.DriverClock
import com.anpfuel.domain.driver.DriverShiftAction
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class DriverWorkShiftTransitionTest {
    private fun row() = DriverShiftEntity(
        ownerAccount = "owner", origin = "android", shiftId = "s", vehicleId = "car", timezone = "America/Cuiaba",
        state = "RUNNING", version = 1, lastBootId = "boot-1", lastElapsedMillis = 1_000,
        lastUtcMillis = 10_000, activeMillis = 2_000, manuallyReviewedMillis = 0,
        createdAtMillis = 10_000, updatedAtMillis = 10_000,
    )
    @Test fun `pause resume and end exclude the pause`() {
        val (paused, gaps) = DriverWorkShiftTransition.apply(row(), emptyList(), DriverShiftAction.PAUSE, DriverClock("boot-1", 2_500, 11_500), "pause")
        val (resumed, gaps2) = DriverWorkShiftTransition.apply(paused, gaps, DriverShiftAction.RESUME, DriverClock("boot-1", 12_500, 21_500), "resume")
        val (closed, _) = DriverWorkShiftTransition.apply(resumed, gaps2, DriverShiftAction.END, DriverClock("boot-1", 14_000, 23_000), "end")
        assertEquals(5_000L, closed.activeMillis)
        assertEquals("CLOSED", closed.state)
    }
    @Test fun `reboot clock rollback overdue checkpoint and process recovery require review without inventing time`() {
        val cases = listOf(
            DriverShiftAction.PAUSE to DriverClock("boot-2", 40_000, 50_000),
            DriverShiftAction.PAUSE to DriverClock("boot-1", 2_000, 9_000),
            DriverShiftAction.CHECKPOINT to DriverClock("boot-1", 61_001, 70_001),
            DriverShiftAction.RECOVER to DriverClock("boot-1", 2_000, 11_000),
        )
        cases.forEachIndexed { index, (action, clock) ->
            val (recovered, gaps) = DriverWorkShiftTransition.apply(row(), emptyList(), action, clock, "op-$index")
            assertEquals("RECOVERY_REQUIRED", recovered.state)
            assertEquals(2_000L, recovered.activeMillis)
            assertEquals(1, gaps.size)
            assertFalse(gaps.single().reviewed)
            val (reviewed, after) = DriverWorkShiftTransition.apply(recovered, gaps, DriverShiftAction.REVIEW_GAP, clock, "review-$index", gaps.single().gapId)
            assertEquals("PAUSED", reviewed.state)
            assertEquals(2_000L, reviewed.activeMillis)
            assertFalse(after.single().reviewed)
            assertNull(after.single().reviewedWorkMillis)
            val (confirmed, confirmedGaps) = DriverWorkShiftTransition.apply(reviewed, after, DriverShiftAction.REVIEW_GAP, clock, "confirm-$index", gaps.single().gapId, 0)
            assertEquals(2_000L, confirmed.activeMillis)
            assertTrue(confirmedGaps.single().reviewed)
            assertEquals(0L, confirmedGaps.single().reviewedWorkMillis)
        }
    }
    @Test fun `legacy shift without a complete clock is rejected without changing its row`() {
        val legacy = row().copy(lastBootId = null, lastElapsedMillis = null, lastUtcMillis = null)
        assertThrows(IllegalArgumentException::class.java) {
            DriverWorkShiftTransition.apply(legacy, emptyList(), DriverShiftAction.RECOVER, DriverClock("boot-1", 2_000, 11_000), "recover")
        }
        assertEquals("RUNNING", legacy.state)
        assertEquals(2_000L, legacy.activeMillis)
    }
    @Test fun `invalid state transition is refused`() {
        assertThrows(IllegalStateException::class.java) {
            DriverWorkShiftTransition.apply(row(), emptyList(), DriverShiftAction.RESUME, DriverClock("boot-1", 2_000, 11_000), "invalid")
        }
    }
}
