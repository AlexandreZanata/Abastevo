package com.anpfuel.domain.driver

import java.io.File
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestFactory

class DriverMoneyTest {
    private val w = DriverWindow("owner", "vehicle", "BRL", 1, 2)
    @Test fun `matched results keep cash recorded and modeled amounts distinct`() {
        val costs = listOf(DriverCost("fuel", 13000, w), DriverCost("toll", 2000, w), DriverCost("lease", 5000, w))
        val r = DriverMoney.result(45000, w, w, costs, true, listOf(DriverCost("reserve", 4000, w), DriverCost("depreciation", 6000, w)), 200000, 28800)
        assertEquals(25000L, r.recorded); assertEquals(15000L, r.modeled)
        assertEquals("125", r.recordedRatios.perKm); assertEquals("1875", r.modeledRatios.perHour)
        assertEquals(DriverMoneyViews(45000, mapOf("BANK" to 30000)), DriverMoney.views(listOf(45000), listOf(DriverCashMovement("p", "PAYOUT", "BANK", 30000))))
        assertEquals(DriverRatios("70", "70000/3"), DriverMoney.weightedRatios(listOf(10000, 60000), listOf(100000, 900000), listOf(3600, 7200)))
        assertNull(DriverMoney.result(45000, w, w.copy(vehicle = "foreign"), costs, true, null, null, null).recorded)
        assertNull(DriverMoney.result(45000, w, w, costs, false, null, null, null).recorded)
        assertThrows(IllegalArgumentException::class.java) { DriverMoney.result(45000, w, w, costs + costs.first(), true, null, null, null) }
        assertThrows(IllegalArgumentException::class.java) { DriverMoney.weightedRatios(listOf(DRIVER_MAX_AMOUNT, 1), listOf(1, 1), listOf(1, 1)) }
    }
    @Test fun `malformed bases bounds and adjustment types refuse without normalization`() {
        for (e in listOf(DriverEarnings("INCLUSIVE_NET", net = 40000, tips = 3000), DriverEarnings("DETAILED", gross = 1), DriverEarnings("INCLUSIVE_NET", net = Long.MAX_VALUE), DriverEarnings("OTHER"))) {
            assertThrows(IllegalArgumentException::class.java) { e.earned() }
        }
        DriverAdjustment("a", "target", "REVERSAL", 1, -100)
        DriverAdjustment("a", "target", "CORRECTION", 1, 100)
        assertThrows(IllegalArgumentException::class.java) { DriverAdjustment("a", "target", "REVERSAL", 1, 100) }
        assertThrows(IllegalArgumentException::class.java) { DriverAdjustment("a", "target", "CORRECTION", 0, 100) }
    }
    @TestFactory fun actualSharedManualExamples(): List<DynamicTest> {
        fun fields(s: String) = s.split(';').associate { it.substringBefore('=') to it.substringAfter('=') }
        fun ratios(r: DriverRatios) = mapOf("per_km" to (r.perKm ?: "null"), "per_hour" to (r.perHour ?: "null"), "unknown" to r.unknown)
        val rows = File("../contracts/testdata/driver-suite/driver-manual-reference-v1.psv").readLines().drop(1).map { it.split('|') }.filter { it[1] != "UNION" }
        assertEquals(12, rows.size)
        return rows.map { (id, operation, raw, expected) -> DynamicTest.dynamicTest(id) {
            val a = fields(raw)
            fun n(key: String) = a.getValue(key).toLong()
            fun optional(key: String) = a.getValue(key).takeUnless { it == "null" }?.toLong()
            val got = when (operation) {
                "DETAILED" -> mapOf("earned" to DriverEarnings("DETAILED", n("gross"), n("fees"), n("tips"), n("bonus"), n("refund")).earned().toString())
                "NET" -> mapOf("earned" to DriverEarnings("INCLUSIVE_NET", net = n("inclusive")).earned().toString(), "gross" to "null", "fees" to "null", "unknown" to "gross-and-fee-components-unknown")
                "PAYOUT" -> DriverMoney.views(listOf(n("earned")), listOf(DriverCashMovement("p", "PAYOUT", "BANK", n("payout")))).let { mapOf("earned" to it.earned.toString(), "received" to it.received.getValue("BANK").toString()) }
                "RATIOS" -> ratios(DriverMoney.ratios(n("amount"), optional("distance"), optional("seconds")))
                "RESULT", "MODEL" -> {
                    val key = if (operation == "MODEL") "estimates" else "costs"
                    val items = if (a[key] == "null") emptyList() else a.getValue(key).split(',').mapIndexed { i, v -> DriverCost("$i", v.toLong(), w) }
                    val r = DriverMoney.result(if (operation == "MODEL") n("recorded") else n("earned"), w, w,
                        if (operation == "MODEL") emptyList() else items, operation == "MODEL" || a["complete"] == "1",
                        if (operation == "MODEL") items else emptyList(), optional("distance"), optional("seconds"))
                    when {
                        operation == "MODEL" -> ratios(r.modeledRatios) + mapOf("recorded" to r.recorded.toString(), "modeled" to r.modeled.toString())
                        r.recorded == null -> mapOf("result" to "null", "per_km" to "null", "per_hour" to "null", "unknown" to r.unknown)
                        else -> ratios(r.recordedRatios) + ("result" to r.recorded.toString())
                    }
                }
                else -> error("unknown vector $operation")
            }
            assertEquals(fields(expected), got, id)
        } }
    }
}
