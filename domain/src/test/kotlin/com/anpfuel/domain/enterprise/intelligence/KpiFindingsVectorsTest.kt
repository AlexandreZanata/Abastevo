package com.anpfuel.domain.enterprise.intelligence

import java.io.File
import java.math.BigInteger
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class KpiFindingsVectorsTest {
    private fun loadVectors(): List<String> {
        val candidates = listOf(
            File("../contracts/testdata/enterprise/intelligence/kpi-findings-v1.psv"),
            File("contracts/testdata/enterprise/intelligence/kpi-findings-v1.psv"),
        )
        val file = candidates.firstOrNull(File::isFile)
            ?: error("Missing kpi-findings-v1.psv fixture")
        return file.readLines().drop(1).filter { it.isNotBlank() }
    }

    private fun parseMap(raw: String): Map<String, String> =
        raw.split(';').filter { it.contains('=') }.associate { part ->
            val (k, v) = part.split('=', limit = 2)
            k to v
        }

    @Test
    fun `matches canonical section 10 KPI and findings golden vectors`() {
        val rows = loadVectors()
        assertEquals(12, rows.size)

        for (line in rows) {
            val parts = line.split('|')
            val id = parts[0]
            val op = parts[1]
            val args = parseMap(parts[2])
            val exp = parseMap(parts[3])

            when (op) {
                "RATIO_GROUP" -> {
                    val intervals = listOf(
                        EnterpriseKpiCalculator.RatioInterval("DIESEL_S10", args.getValue("i1_m").toLong(), 0L, args.getValue("i1_ml").toLong(), args.getValue("i1_c").toLong()),
                        EnterpriseKpiCalculator.RatioInterval("DIESEL_S10", args.getValue("i2_m").toLong(), 0L, args.getValue("i2_ml").toLong(), args.getValue("i2_c").toLong()),
                        EnterpriseKpiCalculator.RatioInterval("DIESEL_S10", args.getValue("i3_m").toLong(), 0L, args.getValue("i3_ml").toLong(), args.getValue("i3_c").toLong()),
                    )
                    val s = EnterpriseKpiCalculator.computeDistanceConsumption(intervals)
                    assertEquals(exp.getValue("sum_m").toLong(), s.distanceM, id)
                    assertEquals(exp.getValue("sum_ml").toLong(), s.quantityMilli, id)
                    assertEquals(exp.getValue("sum_c").toLong(), s.costCentavos, id)
                    assertEquals(exp.getValue("km_per_l_rat"), s.kmPerUnitRat, id)
                    assertEquals(exp.getValue("km_per_l_2dp"), s.kmPerUnit2Dp, id)
                    assertEquals(exp.getValue("l_per_100_2dp"), s.litresPer100Km2Dp, id)
                    assertEquals(exp.getValue("c_per_km_2dp").toLong(), s.centsPerKmRounded, id)
                }
                "RATIO_HOURS" -> {
                    val s = EnterpriseKpiCalculator.computeHourConsumption(
                        listOf(EnterpriseKpiCalculator.RatioInterval("DIESEL_S10", 0L, args.getValue("seconds").toLong(), args.getValue("ml").toLong(), args.getValue("cents").toLong())),
                    )
                    assertEquals(exp.getValue("l_per_h_rat"), s.litresPerHourRat, id)
                    assertEquals(exp.getValue("l_per_h_2dp"), s.litresPerHour2Dp, id)
                    assertEquals(exp.getValue("c_per_h").toLong(), s.centsPerHourRounded, id)
                    assertTrue(s.smallSample, id)
                }
                "OP_COST" -> {
                    val total = args.getValue("fuel").toLong() + args.getValue("maint").toLong() +
                        args.getValue("tolls").toLong() + args.getValue("parking").toLong()
                    assertEquals(exp.getValue("total_cents").toLong(), total, id)
                    val cpk = EnterpriseKpiCalculator.roundHalfUp(
                        BigInteger.valueOf(total).multiply(BigInteger.valueOf(1000L)),
                        BigInteger.valueOf(args.getValue("dist_m").toLong()),
                    )
                    assertEquals(exp.getValue("c_per_km_2dp").toLong(), cpk, id)
                }
                "AVAIL" -> {
                    val a = EnterpriseKpiCalculator.computeAvailability(args.getValue("active_s").toLong(), args.getValue("unavail_s").toLong())
                    assertEquals(exp.getValue("avail_s").toLong(), a.availableSeconds, id)
                    assertEquals(exp.getValue("avail_rat"), a.ratio, id)
                    assertEquals(exp.getValue("avail_pct_1dp"), a.percent1Dp, id)
                }
                "PRICE_CMP" -> {
                    val res = EnterpriseKpiCalculator.evaluatePriceComparison(
                        EnterpriseKpiCalculator.PriceComparisonInput(
                            fuelingProduct = "DIESEL_S10",
                            fuelingUnit = "L",
                            placeKnown = true,
                            placeRule = "SAME_MUNICIPALITY",
                            fuelingCondition = args.getValue("fuel_cond"),
                            quantityMilli = args.getValue("qty_ml").toLong(),
                            paidCents = args.getValue("paid_cents").toLong(),
                            hasReference = true,
                            referenceSource = "OFFICIAL",
                            referenceProduct = "DIESEL_S10",
                            referenceUnit = "L",
                            referenceCondition = args.getValue("ref_cond"),
                            referencePriceMilli = args.getValue("ref_milli").toLong(),
                            referenceAgeDays = args.getValue("age_days").toInt(),
                            maxAgeDays = args.getValue("max_age").toInt(),
                            coverageStations = args.getValue("cov").toInt(),
                            minCoverageStations = args.getValue("min_cov").toInt(),
                        ),
                    )
                    assertEquals(exp.getValue("status"), res.status, id)
                    exp["reason"]?.let { assertEquals(it, res.reason?.name, id) }
                    exp["ref_cents"]?.toLong()?.let { assertEquals(it, res.referenceCents, id) }
                    exp["diff_cents"]?.toLong()?.let { assertEquals(it, res.differenceCents, id) }
                    exp["diff_per_unit_milli"]?.toLong()?.let { assertEquals(it, res.differencePerUnitMilli, id) }
                }
                "FIND_FT001" -> {
                    val s = EnterpriseKpiCalculator.evaluateFT001Duplicate(args.getValue("delta_min").toInt(), args.getValue("window_min").toInt(), args.getValue("same_key").toBooleanStrict())
                    assertEquals(exp.getValue("raised").toBooleanStrict(), s.raised, id)
                    assertEquals(exp.getValue("severity"), s.severity?.name, id)
                }
                "FIND_FT002" -> {
                    val s = EnterpriseKpiCalculator.evaluateFT002IncoherentMeter(false, args.getValue("delta_m").toLong(), args.getValue("delta_s").toLong(), args.getValue("max_kmh").toInt(), false)
                    assertEquals(exp.getValue("raised").toBooleanStrict(), s.raised, id)
                    assertEquals(exp.getValue("severity"), s.severity?.name, id)
                }
                "FIND_FT003" -> {
                    val s = EnterpriseKpiCalculator.evaluateFT003TankCapacity(args.getValue("qty_ml").toLong(), args.getValue("cap_ml").toLong(), args.getValue("tol_pct").toInt(), false)
                    assertEquals(exp.getValue("raised").toBooleanStrict(), s.raised, id)
                    assertEquals(exp.getValue("severity"), s.severity?.name, id)
                }
                "FIND_FT004" -> {
                    val s = EnterpriseKpiCalculator.evaluateFT004Outlier(args.getValue("base_m").toLong(), args.getValue("base_ml").toLong(), args.getValue("test_m").toLong(), args.getValue("test_ml").toLong(), args.getValue("thresh_pct").toInt())
                    assertEquals(exp.getValue("raised").toBooleanStrict(), s.raised, id)
                    assertEquals(exp.getValue("severity"), s.severity?.name, id)
                }
                "FIND_FT005" -> {
                    val s = EnterpriseKpiCalculator.evaluateFT005DocDivergence(args.getValue("rec_cents").toLong(), args.getValue("doc_cents").toLong(), false)
                    assertEquals(exp.getValue("raised").toBooleanStrict(), s.raised, id)
                    assertEquals(exp.getValue("severity"), s.severity?.name, id)
                }
                "FIND_FT006" -> {
                    val s = EnterpriseKpiCalculator.evaluateFT006PriceAboveRef(args.getValue("unit_milli").toLong(), args.getValue("ref_milli").toLong(), args.getValue("diff_cents").toLong(), args.getValue("pct").toInt(), args.getValue("min_cents").toLong())
                    assertEquals(exp.getValue("raised").toBooleanStrict(), s.raised, id)
                    assertEquals(exp.getValue("severity"), s.severity?.name, id)
                }
                else -> error("Unknown op $op")
            }
        }
    }

    @Test
    fun `covers insufficiency reasons finding severity edges and decision guard`() {
        assertEquals(EnterpriseKpiCalculator.InsufficiencyReason.NO_CLOSED_INTERVAL, EnterpriseKpiCalculator.computeDistanceConsumption(emptyList()).reason)
        assertEquals(EnterpriseKpiCalculator.InsufficiencyReason.METHOD_UNSUPPORTED, EnterpriseKpiCalculator.computeDistanceConsumption(listOf(EnterpriseKpiCalculator.RatioInterval(product = "CNG", distanceM = 100L, quantityMilli = 10L))).reason)
        assertEquals(EnterpriseKpiCalculator.InsufficiencyReason.METHOD_UNSUPPORTED, EnterpriseKpiCalculator.computeDistanceConsumption(listOf(EnterpriseKpiCalculator.RatioInterval("GAS", 100L, 0L, 10L), EnterpriseKpiCalculator.RatioInterval("DIESEL_S10", 100L, 0L, 10L))).reason)
        assertEquals(EnterpriseKpiCalculator.InsufficiencyReason.NO_HOUR_BASIS, EnterpriseKpiCalculator.computeHourConsumption(emptyList()).reason)
        assertEquals(EnterpriseKpiCalculator.InsufficiencyReason.NO_AVAILABILITY_HISTORY, EnterpriseKpiCalculator.computeAvailability(0L, 0L).reason)
        assertEquals("0", EnterpriseKpiCalculator.formatScaledDecimal(BigInteger.ZERO, BigInteger.ONE, 0))
        assertEquals("-1.5", EnterpriseKpiCalculator.formatScaledDecimal(BigInteger.valueOf(-3L), BigInteger.valueOf(2L), 1))

        val basePrice = EnterpriseKpiCalculator.PriceComparisonInput(
            "DIESEL_S10", "L", true, "SAME_MUNICIPALITY", "CASH", 100_000L, 60_000L,
            true, "OFFICIAL", "DIESEL_S10", "L", "CASH", 6000L, 1, 14, 5, 3,
        )
        assertEquals(EnterpriseKpiCalculator.InsufficiencyReason.NO_REFERENCE, EnterpriseKpiCalculator.evaluatePriceComparison(basePrice.copy(hasReference = false)).reason)
        assertEquals(EnterpriseKpiCalculator.InsufficiencyReason.PLACE_UNKNOWN, EnterpriseKpiCalculator.evaluatePriceComparison(basePrice.copy(placeKnown = false)).reason)
        assertEquals(EnterpriseKpiCalculator.InsufficiencyReason.REFERENCE_STALE, EnterpriseKpiCalculator.evaluatePriceComparison(basePrice.copy(referenceAgeDays = 20)).reason)
        assertEquals(EnterpriseKpiCalculator.InsufficiencyReason.LOW_COVERAGE, EnterpriseKpiCalculator.evaluatePriceComparison(basePrice.copy(coverageStations = 1)).reason)
        assertEquals(EnterpriseKpiCalculator.InsufficiencyReason.CONDITION_UNKNOWN, EnterpriseKpiCalculator.evaluatePriceComparison(basePrice.copy(fuelingCondition = "UNKNOWN")).reason)

        assertFalse(EnterpriseKpiCalculator.evaluateFT001Duplicate(45, 30, false).raised)
        assertTrue(EnterpriseKpiCalculator.evaluateFT001Duplicate(45, 30, true).raised)
        assertFalse(EnterpriseKpiCalculator.evaluateFT002IncoherentMeter(false, 100_000L, 3600L, 150, false).raised)
        assertTrue(EnterpriseKpiCalculator.evaluateFT002IncoherentMeter(true, 0L, 0L, 150, false).raised)
        assertFalse(EnterpriseKpiCalculator.evaluateFT003TankCapacity(200_000L, 275_000L, 5, false).raised)
        assertEquals(EnterpriseKpiCalculator.FindingSeverity.HIGH, EnterpriseKpiCalculator.evaluateFT003TankCapacity(350_000L, 275_000L, 5, false).severity)
        assertTrue(EnterpriseKpiCalculator.evaluateFT003TankCapacity(1L, 0L, 5, true).raised)
        assertFalse(EnterpriseKpiCalculator.evaluateFT004Outlier(100L, 100L, 100L, 100L, 20).raised)
        assertEquals(EnterpriseKpiCalculator.FindingSeverity.HIGH, EnterpriseKpiCalculator.evaluateFT004Outlier(100L, 100L, 100L, 200L, 20).severity)
        assertFalse(EnterpriseKpiCalculator.evaluateFT005DocDivergence(1000L, 1001L, false).raised)
        assertEquals(EnterpriseKpiCalculator.FindingSeverity.HIGH, EnterpriseKpiCalculator.evaluateFT005DocDivergence(1000L, 2000L, true).severity)
        assertFalse(EnterpriseKpiCalculator.evaluateFT006PriceAboveRef(6100L, 6090L, 10L, 5, 1000L).raised)
        assertEquals(EnterpriseKpiCalculator.FindingSeverity.HIGH, EnterpriseKpiCalculator.evaluateFT006PriceAboveRef(7500L, 6000L, 5000L, 5, 1000L).severity)
        assertEquals(EnterpriseKpiCalculator.FindingSeverity.MEDIUM, EnterpriseKpiCalculator.evaluateFT006PriceAboveRef(6700L, 6000L, 5000L, 5, 1000L).severity)

        assertEquals(EnterpriseKpiCalculator.DecisionError.FINDING_CLOSED, EnterpriseKpiCalculator.validateFindingDecision(EnterpriseKpiCalculator.FindingState.CONFIRMED, "a", "b", "ok"))
        assertEquals(EnterpriseKpiCalculator.DecisionError.SELF_APPROVAL, EnterpriseKpiCalculator.validateFindingDecision(EnterpriseKpiCalculator.FindingState.OPEN, "a", "a", "ok"))
        assertEquals(EnterpriseKpiCalculator.DecisionError.JUSTIFICATION_REQUIRED, EnterpriseKpiCalculator.validateFindingDecision(EnterpriseKpiCalculator.FindingState.IN_REVIEW, "a", "b", "  "))
        assertNull(EnterpriseKpiCalculator.validateFindingDecision(EnterpriseKpiCalculator.FindingState.OPEN, "a", "b", "Confirmed with supervisor"))
    }
}
