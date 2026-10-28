package com.anpfuel.domain.driver

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class DriverCostProfileTest {
    private val w = DriverWindow("owner", "car", "BRL", 0, 100)
    @Test fun `centavo residual and permutation are exact at small and maximum source amounts`() {
        val weights = listOf(DriverWeight("b", 1), DriverWeight("c", 1), DriverWeight("a", 1))
        assertEquals(mapOf("a" to 34L, "b" to 33L, "c" to 33L), DriverAllocationRules.allocate(100, weights).amounts)
        for (amount in 0L..100L) {
            val r = DriverAllocationRules.allocate(amount, weights)
            assertEquals(amount, r.amounts.values.sum())
            assertEquals(r, DriverAllocationRules.allocate(amount, weights.reversed()))
        }
        assertEquals(DRIVER_MAX_AMOUNT / 2, DriverAllocationRules.allocate(DRIVER_MAX_AMOUNT,
            listOf(DriverWeight("a", DRIVER_MAX_AMOUNT), DriverWeight("b", DRIVER_MAX_AMOUNT))).amounts["a"])
    }
    @Test fun `missing basis stays unknown and invalid or duplicate weights refuse`() {
        assertEquals("source-amount-missing", DriverAllocationRules.allocate(null, listOf(DriverWeight("a", 1))).unknown)
        assertEquals("allocation-basis-missing", DriverAllocationRules.allocate(100, listOf(DriverWeight("a", 0))).unknown)
        for (weights in listOf(listOf(DriverWeight("a", -1)), listOf(DriverWeight("a", 1), DriverWeight("a", 2)), listOf(DriverWeight("non ascii á", 1)))) {
            assertThrows(IllegalArgumentException::class.java) { DriverAllocationRules.allocate(100, weights) }
        }
    }
    @Test fun `time and civil day proration retain unallocated coverage across DST`() {
        val time = DriverAllocationRules.prorate(100, DriverTimeSpan(0, 4), listOf(DriverAllocationSpan("work", DriverTimeSpan(0, 1))))
        assertEquals(mapOf("work" to 25L, "_unallocated" to 75L), time.amounts)
        val days = DriverAllocationRules.days(300, "2026-03-07", "2026-03-10", "America/New_York",
            listOf(DriverCalendarSpan("work", "2026-03-08", "2026-03-09")))
        assertEquals(mapOf("work" to 100L, "_unallocated" to 200L), days.amounts)
        assertThrows(IllegalArgumentException::class.java) { DriverAllocationRules.prorate(100, DriverTimeSpan(0, 4),
            listOf(DriverAllocationSpan("a", DriverTimeSpan(0, 3)), DriverAllocationSpan("b", DriverTimeSpan(2, 4)))) }
    }
    @Test fun `owned leased and financed profiles keep actual capital and modeled sources separate`() {
        fun actual(id: String, kind: String, value: Long) = DriverProfileCost(id, kind, "ACTUAL", value, "2026-01-01", ledgerSourceId = "$id-ledger", sourceRevision = 1)
        val depreciation = DriverProfileCost("depreciation", "DEPRECIATION", "MODELED", null, "2026-01-01")
        val financed = DriverVehicleCostProfile("car", "FINANCED", "UTC", "2026-01-01", null, 1,
            listOf(actual("principal", "PRINCIPAL", 10000), actual("interest", "INTEREST", 1000), depreciation))
        val buckets = financed.at("2026-02-01")
        assertEquals(listOf("principal"), buckets.capital.map { it.id })
        assertEquals(listOf("interest"), buckets.operating.map { it.id })
        assertNull(buckets.modeled.single().amount)
        assertEquals("profile-not-effective", financed.at("2025-12-31").unknown)
        val leased = DriverVehicleCostProfile("rental", "LEASED", "UTC", "2026-01-01", null, 1, listOf(actual("rent", "LEASE", 20000)))
        assertEquals(20000L, leased.at("2026-02-01").operating.single().amount)
        val owned = DriverVehicleCostProfile("owned", "OWNED", "UTC", "2026-01-01", null, 1, listOf(depreciation.copy(amount = 1000)))
        assertEquals(1000L, owned.at("2026-02-01").modeled.single().amount)
        assertThrows(IllegalArgumentException::class.java) { leased.copy(costs = leased.costs + depreciation) }
        assertThrows(IllegalArgumentException::class.java) { owned.copy(costs = listOf(actual("principal", "PRINCIPAL", 1000))) }
        assertThrows(IllegalArgumentException::class.java) { financed.copy(costs = listOf(actual("a", "FEE", 1), actual("b", "FEE", 2).copy(ledgerSourceId = "a-ledger"))) }
    }
    @Test fun `realized service replaces reserve explicitly and corrections do not subtract twice`() {
        val reserves = listOf(DriverReserve("reserve", 8000, w))
        val links = listOf(DriverReserveReplacement("reserve", "service"))
        assertEquals(DriverReserveResult(0, 2000, listOf("reserve")), DriverReserveRules.reconcile(w, reserves, listOf(DriverCost("service", 2000, w)), links))
        assertEquals(DriverReserveResult(0, 3000, listOf("reserve")), DriverReserveRules.reconcile(w, reserves, listOf(DriverCost("service", 3000, w)), links))
        assertEquals(8000L, DriverReserveRules.reconcile(w, reserves, listOf(DriverCost("service", 2000, w)), emptyList()).additionalModeled)
        assertNull(DriverReserveRules.reconcile(w, listOf(DriverReserve("missing", null, w)), emptyList(), emptyList()).additionalModeled)
        assertThrows(IllegalArgumentException::class.java) { DriverReserveRules.reconcile(w, reserves, emptyList(), links) }
        assertThrows(IllegalArgumentException::class.java) { DriverReserveRules.reconcile(w, reserves, listOf(DriverCost("service", 2000, w)), links + links) }
    }
}
