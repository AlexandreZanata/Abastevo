package com.anpfuel.domain.ev

import java.io.File
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestFactory

/** MAP-26: executes the independently generated vectors and adds behavioural/property checks. */
class EvEnergyModelTest {
    private fun cell(row: Map<String, String>, key: String): Long? = row.getValue(key).takeIf { it.isNotEmpty() }?.toLong()

    private fun render(result: EnergyEstimate<Long>): String = when (result) {
        is EnergyEstimate.Known -> result.value.toString()
        is EnergyEstimate.Unknown -> "unknown:" + result.reason
    }

    @TestFactory fun `shared independent vectors`(): List<DynamicTest> {
        val lines = File("../contracts/testdata/ev/trip-energy-v1.csv").readLines()
        val header = lines.first().split(',')
        assertEquals("id,operation,capacity_wh,soc_bp,reserve_bp,usable_permille,cons_milli,distance_m,margin_permille,power_w,energy_wh,sessions,expected", lines.first())
        val rows = lines.drop(1).map { line -> header.zip(line.split(',', limit = header.size)).toMap() }
        assertEquals(53, rows.size)
        assertEquals(rows.size, rows.map { it.getValue("id") }.toSet().size)
        return rows.map { r ->
            DynamicTest.dynamicTest(r.getValue("id") + " " + r.getValue("operation")) {
                fun battery() = BatteryInput(cell(r, "capacity_wh"), cell(r, "soc_bp"), cell(r, "reserve_bp"), cell(r, "usable_permille"))
                val consumption = ConsumptionInput(cell(r, "cons_milli"), cell(r, "margin_permille"))
                val got = when (r.getValue("operation")) {
                    "USABLE" -> EvEnergyModel.usableEnergyWh(battery())
                    "SEGMENT" -> EvEnergyModel.segmentEnergyWh(cell(r, "distance_m"), consumption)
                    "REACH" -> EvEnergyModel.reachableDistanceMetres(battery(), consumption)
                    "ARRIVAL" -> EvEnergyModel.arrivalSocBp(cell(r, "capacity_wh"), cell(r, "soc_bp"), cell(r, "distance_m"), consumption)
                    "CHARGE_LB" -> EvEnergyModel.minimumChargeSeconds(cell(r, "energy_wh"), cell(r, "power_w"))
                    "CALIBRATE" -> EvEnergyModel.calibrateConsumptionMilli(
                        r.getValue("sessions").split(';').filter { it.isNotEmpty() }.map { s -> s.split(':').let { CalibrationSession(it[0].toLong(), it[1].toLong()) } },
                    )
                    else -> error("operation")
                }
                assertEquals(r.getValue("expected"), render(got), r.getValue("id"))
            }
        }
    }

    private val full = BatteryInput(60_000, 8_000, 1_000)
    private fun known(e: EnergyEstimate<Long>) = (e as EnergyEstimate.Known).value

    @Test fun `reachable distance is monotonic in every input`() {
        val base = known(EvEnergyModel.reachableDistanceMetres(full, ConsumptionInput(150_000, 0)))
        assertTrue(known(EvEnergyModel.reachableDistanceMetres(full.copy(socBp = 9_000), ConsumptionInput(150_000, 0))) > base)
        assertTrue(known(EvEnergyModel.reachableDistanceMetres(full.copy(capacityWh = 70_000), ConsumptionInput(150_000, 0))) > base)
        assertTrue(known(EvEnergyModel.reachableDistanceMetres(full.copy(reserveBp = 2_000), ConsumptionInput(150_000, 0))) < base)
        assertTrue(known(EvEnergyModel.reachableDistanceMetres(full, ConsumptionInput(160_000, 0))) < base)
        assertTrue(known(EvEnergyModel.reachableDistanceMetres(full, ConsumptionInput(150_000, 200))) < base)
        var previous = Long.MAX_VALUE
        for (margin in 0..1000 step 50) {
            val now = known(EvEnergyModel.reachableDistanceMetres(full, ConsumptionInput(150_000, margin.toLong())))
            assertTrue(now <= previous, "margin $margin")
            previous = now
        }
    }

    @Test fun `energy needed is conservative against reach and never optimistic`() {
        val consumption = ConsumptionInput(173_333, 50)
        val reach = known(EvEnergyModel.reachableDistanceMetres(full, consumption))
        val usable = known(EvEnergyModel.usableEnergyWh(full))
        // Driving exactly the claimed reach never needs more than the usable energy ...
        assertTrue(known(EvEnergyModel.segmentEnergyWh(reach, consumption)) <= usable + 1)
        // ... and one more kilometre must exceed it.
        assertTrue(known(EvEnergyModel.segmentEnergyWh(reach + 1_000, consumption)) > usable)
    }

    @Test fun `segment energy is sub-additive within one watt hour and exact for whole kilometres`() {
        val c = ConsumptionInput(150_000, 0)
        for (a in listOf(1L, 999L, 12_345L, 50_001L)) for (b in listOf(1L, 777L, 30_000L)) {
            val whole = known(EvEnergyModel.segmentEnergyWh(a + b, c))
            val parts = known(EvEnergyModel.segmentEnergyWh(a, c)) + known(EvEnergyModel.segmentEnergyWh(b, c))
            assertTrue(parts >= whole && parts - whole <= 1, "a=$a b=$b")
        }
        assertEquals(15_000L, known(EvEnergyModel.segmentEnergyWh(100_000, c)))
    }

    @Test fun `phev results are labelled as the electric portion only`() {
        val phev = BatteryInput(15_000, 5_000, 0, usablePermille = 800)
        assertEquals(EnergyScope.ELECTRIC_PORTION_ONLY, EvEnergyModel.scopeOf(phev))
        assertEquals(EnergyScope.BATTERY, EvEnergyModel.scopeOf(full))
    }

    @Test fun `calibration is sum first not an average of ratios and rounds up`() {
        val got = known(EvEnergyModel.calibrateConsumptionMilli(listOf(CalibrationSession(1_000, 10_000), CalibrationSession(9_000, 40_000))))
        assertEquals(200_000L, got)
        val unweighted = (100_000L + 225_000L) / 2
        assertNotEquals(unweighted, got)
        // Calibrated consumption feeds straight into reach.
        assertEquals(210_000L, known(EvEnergyModel.reachableDistanceMetres(full, ConsumptionInput(got, 0))))
    }

    @Test fun `arrival state of charge never exceeds the exact value`() {
        // 60000 Wh pack at 80 %, 12,345 m at 150 Wh/km: exact arrival 7691.33 bp must report 7691.
        assertEquals(7_691L, known(EvEnergyModel.arrivalSocBp(60_000, 8_000, 12_345, ConsumptionInput(150_000, 0))))
    }

    @Test fun `unknown results never carry a number`() {
        val r = EvEnergyModel.usableEnergyWh(BatteryInput(null, 5_000, 0))
        assertTrue(r is EnergyEstimate.Unknown)
        assertEquals("capacity-missing", (r as EnergyEstimate.Unknown).reason)
    }
}
