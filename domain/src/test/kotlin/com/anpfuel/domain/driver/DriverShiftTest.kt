package com.anpfuel.domain.driver

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class DriverShiftTest {
    @Test fun `six plus five hours with four hours overlap is seven hours`() {
        assertEquals(25200L, DriverOnlineTime.unionSeconds(listOf(DriverTimeSpan(0,21600), DriverTimeSpan(7200,25200))))
    }
    private val policy = DriverClockPolicy(60000, 1000)
    private fun ready() = DriverShift("shift", "owner", "origin", "vehicle", "America/Cuiaba")
    private fun clock(ms: Long, boot: String = "boot") = DriverClock(boot, ms, 100000 + ms)
    private fun command(id: String, action: DriverShiftAction, v: Long, ms: Long) = DriverShiftCommand(id, action, v, clock(ms))
    @Test fun `pause resume preserves subsecond work and exact retries do not double it`() {
        val start = command("start", DriverShiftAction.START, 0, 0)
        val active = ready().apply(start, policy).shift
        assertEquals(active, active.apply(start, policy).shift)
        val paused = active.apply(command("pause", DriverShiftAction.PAUSE, 1, 1500), policy).shift
        val resumed = paused.apply(command("resume", DriverShiftAction.RESUME, 2, 10000), policy).shift
        val ended = resumed.apply(command("end", DriverShiftAction.END, 3, 11500), policy).shift
        assertEquals(3000L, ended.activeMillis)
        assertEquals(3L, ended.completeWorkSeconds)
        assertThrows(IllegalStateException::class.java) { ended.apply(command("end-again", DriverShiftAction.END, 4, 12000), policy) }
        assertThrows(IllegalStateException::class.java) { active.apply(start.copy(clock = clock(1)), policy) }
        assertThrows(IllegalStateException::class.java) { active.apply(command("stale", DriverShiftAction.PAUSE, 0, 10), policy) }
    }
    @Test fun `reboot rollback excessive gap and process recovery never invent worked time`() {
        val active = ready().apply(command("start", DriverShiftAction.START, 0, 0), policy).shift
        val failures = listOf(
            command("reboot", DriverShiftAction.CHECKPOINT, 1, 1000).copy(clock = clock(1000, "next-boot")),
            command("rollback", DriverShiftAction.CHECKPOINT, 1, 1000).copy(clock = DriverClock("boot", 1000, 99999)),
            command("gap", DriverShiftAction.CHECKPOINT, 1, 60001),
            command("recovery", DriverShiftAction.RECOVER, 1, 1000),
        )
        for (c in failures) {
            val recovered = active.apply(c, policy).shift
            assertEquals(DriverShiftState.RECOVERY_REQUIRED, recovered.state)
            assertEquals(0L, recovered.activeMillis)
            assertNull(recovered.completeWorkSeconds)
            val ack = DriverShiftCommand("ack", DriverShiftAction.REVIEW_GAP, 2, c.clock, gapId = c.id, reviewedWorkMillis = 2000)
            val reviewed = recovered.apply(ack, policy).shift
            assertEquals(DriverShiftState.PAUSED, reviewed.state)
            assertEquals(2L, reviewed.completeWorkSeconds)
            assertEquals(2000L, reviewed.manuallyReviewedMillis)
            assertTrue(reviewed.apply(ack, policy).replayed)
        }
    }
    @Test fun `resume with unknown gap keeps incomplete coverage and bounded receipts`() {
        var shift = ready().apply(command("start", DriverShiftAction.START, 0, 0), policy).shift
        shift = shift.apply(command("lost", DriverShiftAction.RECOVER, 1, 1000), policy).shift
        shift = shift.apply(DriverShiftCommand("ack", DriverShiftAction.REVIEW_GAP, 2, clock(1000), gapId = "lost"), policy).shift
        assertNull(shift.completeWorkSeconds)
        shift = shift.apply(command("resume", DriverShiftAction.RESUME, 3, 1000), policy).shift
        repeat(60) { shift = shift.apply(command("checkpoint-$it", DriverShiftAction.CHECKPOINT, shift.version, 1001L + it), policy).shift }
        assertEquals(50, shift.receipts.size)
        assertNull(shift.completeWorkSeconds)
        assertThrows(IllegalStateException::class.java) { shift.apply(command("start", DriverShiftAction.START, 0, 0), policy) }
    }
    @Test fun `all three shared online time examples run the actual union`() {
        val rows = java.io.File("../contracts/testdata/driver-suite/driver-manual-reference-v1.psv").readLines().drop(1).map { it.split('|') }.filter { it[1] == "UNION" }
        assertEquals(3, rows.size)
        rows.forEach { (_, _, raw, expected) ->
            val spans = raw.substringAfter("intervals=").split(',').map { val (a,b) = it.split(':'); DriverTimeSpan(a.toLong(), b.toLong()) }
            val want = expected.substringAfter("union=").substringBefore(';').toLong()
            assertEquals(want, DriverOnlineTime.unionSeconds(spans))
        }
    }

}
