package com.anpfuel.data.driver

import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.domain.driver.*
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

/** Real Android runtime proof, including minSdk 26 arithmetic/backport behavior. */
@RunWith(AndroidJUnit4::class)
class DriverModelRuntimeInstrumentedTest {
    @Test fun exactMoneyAndRemainderRunOnAndroid() {
        val w = DriverWindow("synthetic-owner", "synthetic-car", "BRL", 1, 1000)
        val earned = DriverEarnings("DETAILED", 50000, 10000, 2000, 3000, 0).earned()
        val result = DriverMoney.result(earned, w, w, listOf(DriverCost("fuel", 13000, w), DriverCost("toll", 2000, w), DriverCost("rent", 5000, w)), true, emptyList(), 200000, 28800)
        assertEquals(25000L, requireNotNull(result.recorded))
        assertEquals("125", result.recordedRatios.perKm)
        val allocation = DriverAllocationRules.allocate(100, listOf(DriverWeight("b", 1), DriverWeight("a", 1), DriverWeight("c", 1)))
        assertEquals(34L, requireNotNull(allocation.amounts["a"]))
        assertEquals(100L, allocation.amounts.values.sum())
        try { DriverMoney.weightedRatios(listOf(DRIVER_MAX_AMOUNT, 1), listOf(1, 1), listOf(1, 1)); fail("overflow was accepted") }
        catch (_: IllegalArgumentException) { }
    }
    @Test fun monotonicShiftAndOnlineUnionRunOnAndroid() {
        val initial = DriverShift("synthetic-shift", "synthetic-owner", "synthetic-origin", "synthetic-car", "UTC")
        val policy = DriverClockPolicy(60000, 1000)
        val started = initial.apply(DriverShiftCommand("start", DriverShiftAction.START, 0, DriverClock("boot", 0, 100000)), policy).shift
        val stopped = started.apply(DriverShiftCommand("stop", DriverShiftAction.END, 1, DriverClock("boot", 1500, 101500)), policy).shift
        assertEquals(1500L, stopped.activeMillis)
        assertEquals(DriverShiftState.CLOSED, stopped.state)
        assertEquals(25200L, DriverOnlineTime.unionSeconds(listOf(DriverTimeSpan(0, 21600), DriverTimeSpan(7200, 25200))))
    }
}
