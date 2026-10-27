package com.anpfuel.domain.driver

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

/** Adversarial contract cases, not generated getters/copy methods for coverage. */
class DriverBoundaryTest {
    private val w = DriverWindow("owner", "car", "BRL", 1, 1000)
    private val clock = DriverClock("boot", 0, 100000)
    private val policy = DriverClockPolicy(60000, 1000)
    private fun refuses(work: () -> Unit) {
        val failure = assertThrows(RuntimeException::class.java) { work() }
        assertTrue(failure is IllegalArgumentException || failure is IllegalStateException || failure is java.time.DateTimeException || failure is ArithmeticException,
            "Expected a declared validation/transition failure, not an implementation crash: ${failure.javaClass.name}")
    }
    @Test fun `money boundaries unknown denominator and incompatible sources cannot certify a result`() {
        refuses { DriverEarnings("DETAILED", gross = DRIVER_MAX_AMOUNT, fees = 0, tips = DRIVER_MAX_AMOUNT, bonus = 0, refund = 0).earned() }
        refuses { DriverEarnings("DETAILED", gross = 1, fees = -1, tips = 0, bonus = 0, refund = 0).earned() }
        refuses { DriverEarnings("INCLUSIVE_NET", net = -1).earned() }
        refuses { DriverMoney.views(listOf(-DRIVER_MAX_AMOUNT - 1), emptyList()) }
        refuses { DriverMoney.views(emptyList(), listOf(DriverCashMovement("a", "OTHER", "BANK", 1))) }
        refuses { DriverMoney.views(emptyList(), listOf(DriverCashMovement("a", "PAYOUT", "BANK", DRIVER_MAX_AMOUNT), DriverCashMovement("b", "PAYOUT", "BANK", 1))) }
        assertEquals("model-assumptions-missing", DriverMoney.result(1, w, w, emptyList(), true, null, null, null).unknown)
        refuses { DriverMoney.result(1, w, w, listOf(DriverCost("x", 1, w)), true, listOf(DriverCost("x", 1, w)), 1, 1) }
        refuses { DriverMoney.result(1, w, w, listOf(DriverCost("x", 1, w.copy(vehicle = "foreign"))), true, null, 1, 1) }
        refuses { DriverMoney.ratios(1, DRIVER_MAX_AMOUNT + 1, 1) }
        refuses { DriverMoney.weightedRatios(emptyList(), emptyList(), emptyList()) }
        refuses { DriverMoney.weightedRatios(listOf(1), listOf(1, 2), listOf(1)) }
        refuses { DriverAdjustment("a", "b", "OTHER", 1, -1) }
        refuses { DriverAdjustment("a", "b", "CORRECTION", 1, 0) }
        refuses { DriverWindow("owner", "car", "USD", 1, 2) }
    }
    @Test fun `distance absence is not zero and partition totals cannot exceed measured travel`() {
        assertEquals("distance-source-ambiguous-or-missing", DriverDistance.select(w, DriverDistanceBasis.ODOMETER, emptyList()).unknown)
        val missing = DriverDistanceObservation("m", w, "s", DriverDistanceBasis.GPS_ESTIMATE, null, false)
        assertNull(DriverDistance.select(w, missing.basis, listOf(missing)).metres)
        refuses { DriverDistanceObservation("a", w, "s", DriverDistanceBasis.ODOMETER, -1, true) }
        refuses { DriverDistance.odometer(-1, 2, "a", "a") }
        refuses { DriverDistance.odometer(1, 2, "", "a") }
        assertNull(DriverDistance.partition(100, null, null, null, false).emptyMetres)
        assertEquals("work-distance-nonpositive", DriverDistance.partition(0, 0, 0, 0, true).unknown)
        refuses { DriverDistance.partition(100, 101, 10, 0, true) }
        refuses { DriverDistance.partition(100, 80, 40, -1, true) }
        refuses { DriverDistanceScope("owner", "origin", "car", "") }
    }
    @Test fun `invalid clock policies and invalid initial shifts fail before transitions`() {
        refuses { DriverClock("", 1, 100) }; refuses { DriverClock("b", -1, 100) }
        refuses { DriverClockPolicy(0, 1000) }; refuses { DriverClockPolicy(1, -1) }
        refuses { DriverShiftCommand("", DriverShiftAction.START, 0, clock) }
        refuses { DriverShiftCommand("a", DriverShiftAction.START, -1, clock) }
        refuses { DriverShiftCommand("a", DriverShiftAction.REVIEW_GAP, 0, clock, reviewedWorkMillis = -1) }
        refuses { DriverShift("a", "owner", "origin", "car", "UTC", activeMillis = -1) }
        refuses { DriverShift("a", "owner", "origin", "car", "UTC", state = DriverShiftState.RUNNING) }
        refuses { DriverShift("a", "owner", "origin", "car", "UTC", last = clock) }
        refuses { DriverShift("a", "owner", "origin", "car", "UTC", receipts = List(51) { DriverShiftReceipt("$it", "hash", 1) }) }
        refuses { DriverTimeSpan(2, 1) }
        assertEquals(0L, DriverOnlineTime.unionSeconds(emptyList()))
    }
    @Test fun `gap review cannot target invented gaps or auto resume location`() {
        val ready = DriverShift("s", "owner", "origin", "car", "UTC")
        val start = DriverShiftCommand("start", DriverShiftAction.START, 0, clock)
        val running = ready.apply(start, policy).shift
        refuses { running.apply(DriverShiftCommand("bad", DriverShiftAction.START, 1, clock), policy) }
        val recovered = running.apply(DriverShiftCommand("lost", DriverShiftAction.RECOVER, 1, clock), policy).shift
        refuses { recovered.apply(DriverShiftCommand("a", DriverShiftAction.REVIEW_GAP, 2, clock, gapId = "invented", reviewedWorkMillis = 1), policy) }
        val paused = recovered.apply(DriverShiftCommand("review", DriverShiftAction.REVIEW_GAP, 2, clock, gapId = "lost"), policy).shift
        val reviewed = paused.apply(DriverShiftCommand("explicit", DriverShiftAction.REVIEW_GAP, 3, clock, gapId = "lost", reviewedWorkMillis = 0), policy).shift
        assertEquals(DriverShiftState.PAUSED, reviewed.state)
        assertEquals(0L, reviewed.completeWorkSeconds)
        refuses { reviewed.apply(DriverShiftCommand("again", DriverShiftAction.REVIEW_GAP, 4, clock, gapId = "lost", reviewedWorkMillis = 1), policy) }
        val fullGaps = running.copy(gaps = List(50) { DriverShiftGap("gap-$it", "unknown", clock, clock) })
        refuses { fullGaps.apply(DriverShiftCommand("overflow", DriverShiftAction.RECOVER, 1, clock), policy) }
        val pausedRecovered = paused.apply(DriverShiftCommand("recover-paused", DriverShiftAction.RECOVER, 3, clock), policy).shift
        assertEquals(DriverShiftState.PAUSED, pausedRecovered.state)
    }
    @Test fun `GPS quality rejects inconsistent boot evidence and invalid policy without raw log data`() {
        refuses { DriverGpsPolicy(0, 1.0, 1, 1, 1.0, 0.0) }
        refuses { DriverGpsPolicy(1, Double.NaN, 1, 1, 1.0, 0.0) }
        refuses { DriverGpsPolicy(1, 1.0, -1, 1, 1.0, 0.0) }
        refuses { DriverGpsPolicy(1, 1.0, 1, 0, 1.0, 0.0) }
        refuses { DriverGpsPolicy(1, 1.0, 1, 1, 0.0, 0.0) }
        refuses { DriverGpsPolicy(1, 1.0, 1, 1, 1.0, -1.0) }
        val p = DriverGpsPolicy(1, 30.0, 1000, 1000, 100.0, 5.0)
        val track = DriverGpsQuality.empty(DriverDistanceScope("a", "origin", "car", "session"), p)
        val old = DriverGpsFix(0.0, 0.0, 5.0, clock)
        assertEquals(1L, track.append(old, clock.copy(bootId = "new")).estimate().rejected["fix-boot-mismatch"])
        for (bad in listOf(DriverGpsFix(91.0, 0.0, 1.0, clock), DriverGpsFix(0.0, 181.0, 1.0, clock), DriverGpsFix(0.0, 0.0, -1.0, clock))) {
            assertEquals(0L, track.append(bad, clock).estimate().distanceMetres)
        }
        assertTrue(track.append(old, clock).estimate().complete)
    }
    @Test fun `profile provenance temporal validity and replacement bounds remain explicit`() {
        val reserve = DriverProfileCost("r", "RESERVE", "MODELED", 100, "2026-01-01")
        val profile = DriverVehicleCostProfile("car", "OWNED", "UTC", "2026-01-01", "2026-02-01", 1, listOf(reserve.copy(until = "2026-02-01")))
        assertEquals("profile-not-effective", profile.at("2026-02-01").unknown)
        refuses { profile.copy(costs = listOf(reserve)) }
        refuses { profile.copy(costs = listOf(reserve.copy(provenance = "ACTUAL", until = "2026-02-01"))) }
        refuses { profile.copy(costs = listOf(reserve.copy(from = "2025-01-01", until = "2026-02-01"))) }
        refuses { profile.copy(costs = listOf(reserve.copy(kind = "OTHER", until = "2026-02-01"))) }
        refuses { DriverAllocationRules.days(1, "2026-01-01", "2026-01-02", "Local", emptyList()) }
        refuses { DriverAllocationRules.prorate(1, DriverTimeSpan(0, 2), listOf(DriverAllocationSpan("_unallocated", DriverTimeSpan(0, 1)))) }
        refuses { DriverReserveRules.reconcile(w, listOf(DriverReserve("x", 1, w)), listOf(DriverCost("x", 1, w)), emptyList()) }
        refuses { DriverReserveRules.reconcile(w, emptyList(), listOf(DriverCost("a", 1, w.copy(owner = "foreign"))), emptyList()) }
        refuses { DriverReserveRules.reconcile(w, listOf(DriverReserve("r", 1, w)), listOf(DriverCost("s", 1, w)), listOf(DriverReserveReplacement("missing", "s"))) }
    }
}
