package com.anpfuel.domain.enterprise.maintenance

import java.io.File
import java.time.Instant
import java.time.LocalDate
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class MaintenanceDueVectorsTest {
    private fun loadVectors(): List<String> {
        val candidates = listOf(
            File("../contracts/testdata/enterprise/maintenance/maintenance-due-v1.psv"),
            File("contracts/testdata/enterprise/maintenance/maintenance-due-v1.psv"),
        )
        val file = candidates.firstOrNull(File::isFile)
            ?: error("Missing maintenance-due-v1.psv fixture")
        return file.readLines().drop(1).filter { it.isNotBlank() }
    }

    private fun parseMap(raw: String): Map<String, String> =
        raw.split(';').filter { it.contains('=') }.associate { part ->
            val (k, v) = part.split('=', limit = 2)
            k to v
        }

    @Test
    fun `matches canonical maintenance and workorder golden vectors`() {
        val rows = loadVectors()
        assertEquals(12, rows.size)

        for (line in rows) {
            val parts = line.split('|')
            val id = parts[0]
            val op = parts[1]
            val args = parseMap(parts[2])
            val exp = parseMap(parts[3])

            when (op) {
                "EVAL_PLAN", "PROJECT_DUE" -> {
                    val spec = PreventivePlanSpec(
                        distanceIntervalM = args.getValue("int_m").toLong(),
                        distanceLeadM = args.getValue("lead_m").toLong(),
                        baselineOdometerM = args.getValue("base_m").toLong(),
                        timeIntervalMonths = args.getValue("int_months").toInt(),
                        timeLeadDays = args.getValue("lead_days").toInt(),
                        baselineDay = LocalDate.parse(args.getValue("base_day")),
                    )
                    val res = EnterpriseMaintenanceCalculator.evaluatePlan(
                        spec = spec,
                        evalDay = LocalDate.parse(args.getValue("eval_day")),
                        currentOdoM = args.getValue("eval_m").toLong(),
                        windowStartDay = args["win_start_day"]?.let(LocalDate::parse),
                        windowStartOdoM = args["win_start_m"]?.toLong(),
                    )
                    assertEquals(exp.getValue("used_m").toLong(), res.usedM, id)
                    assertEquals(exp.getValue("remaining_m").toLong(), res.remainingM, id)
                    assertEquals(DueState.valueOf(exp.getValue("state")), res.state, id)
                    exp["days_to_due"]?.toInt()?.let { assertEquals(it, res.daysToDue, id) }
                    exp["trigger"]?.let { assertEquals(TriggerKind.valueOf(it), res.decidingTrigger, id) }
                    exp["projected_day"]?.let { assertEquals(LocalDate.parse(it), res.projectedDueDay, id) }
                }
                "EVAL_HOURS" -> {
                    val spec = PreventivePlanSpec(
                        hourIntervalS = args.getValue("int_s").toLong(),
                        hourLeadS = args.getValue("lead_s").toLong(),
                        baselineHourS = args.getValue("base_s").toLong(),
                        baselineDay = LocalDate.of(2026, 1, 1),
                    )
                    val res = EnterpriseMaintenanceCalculator.evaluatePlan(
                        spec = spec,
                        evalDay = LocalDate.of(2026, 2, 1),
                        currentHourS = args.getValue("eval_s").toLong(),
                    )
                    assertEquals(exp.getValue("used_s").toLong(), res.usedS, id)
                    assertEquals(exp.getValue("remaining_s").toLong(), res.remainingS, id)
                    assertEquals(DueState.valueOf(exp.getValue("state")), res.state, id)
                    assertEquals(TriggerKind.valueOf(exp.getValue("trigger")), res.decidingTrigger, id)
                }
                "REBASE_HOURS" -> {
                    val due = args.getValue("due_s").toLong()
                    val done = args.getValue("done_s").toLong()
                    val intS = args.getValue("int_s").toLong()
                    assertEquals(exp.getValue("next_actual").toLong(), EnterpriseMaintenanceCalculator.rebaseNextMeter(due, done, intS, RebaseRule.ACTUAL), id)
                    assertEquals(exp.getValue("next_scheduled").toLong(), EnterpriseMaintenanceCalculator.rebaseNextMeter(due, done, intS, RebaseRule.SCHEDULED), id)
                }
                "EVAL_WO" -> {
                    val lines = listOf(
                        WorkOrderLine(WorkOrderLineKind.PART, args.getValue("oil_ml").toLong(), args.getValue("oil_price").toLong()),
                        WorkOrderLine(WorkOrderLineKind.PART, args.getValue("filter_u").toLong(), args.getValue("filter_price").toLong()),
                        WorkOrderLine(WorkOrderLineKind.LABOR, args.getValue("labor_s").toLong(), args.getValue("labor_rate").toLong()),
                    )
                    val rec = EnterpriseMaintenanceCalculator.evaluateWorkOrder(
                        lines = lines,
                        approvedCents = args.getValue("approved").toLong(),
                        tolerancePct = args.getValue("tol_pct").toInt(),
                        discountCents = args.getValue("discount").toLong(),
                        paidCents = args.getValue("paid").toLong(),
                    )
                    assertEquals(exp.getValue("overrun_ceiling").toLong(), rec.overrunCeiling, id)
                    assertEquals(exp.getValue("parts_cents").toLong(), rec.partsCentavos, id)
                    assertEquals(exp.getValue("service_cents").toLong(), rec.serviceCentavos, id)
                    assertEquals(exp.getValue("sum_cents").toLong(), rec.sumCentavos, id)
                    assertEquals(exp.getValue("mismatch").toBooleanStrict(), rec.arithmeticMismatch, id)
                    assertEquals(exp.getValue("overrun").toBooleanStrict(), rec.overrunExceeded, id)
                }
                else -> error("Unknown operation $op")
            }
        }
    }

    @Test
    fun `covers hold interval merging compliance evaluation and validation branches`() {
        val base = Instant.parse("2026-11-01T00:00:00Z")
        val merged = EnterpriseMaintenanceCalculator.unavailableSeconds(
            windows = listOf(
                base to base.plusSeconds(3600),
                base.plusSeconds(1800) to base.plusSeconds(5400),
                base.plusSeconds(7200) to base.plusSeconds(9000),
            ),
            periodStart = base,
            periodEnd = base.plusSeconds(10_000),
        )
        assertEquals(7200L, merged)
        assertEquals(0L, EnterpriseMaintenanceCalculator.unavailableSeconds(emptyList(), base, base))

        val day = LocalDate.of(2026, 11, 18)
        assertEquals(ComplianceState.CONTROL_DISABLED, EnterpriseMaintenanceCalculator.evaluateCompliance(day.minusDays(1), day, 30, controlEnabled = false))
        assertEquals(ComplianceState.EXPIRED, EnterpriseMaintenanceCalculator.evaluateCompliance(day.minusDays(1), day, 30))
        assertEquals(ComplianceState.DUE_SOON, EnterpriseMaintenanceCalculator.evaluateCompliance(day.plusDays(15), day, 30))
        assertEquals(ComplianceState.VALID, EnterpriseMaintenanceCalculator.evaluateCompliance(day.plusDays(45), day, 30))

        assertFalse(PreventivePlanSpec(baselineDay = day).validate())
        assertFalse(PreventivePlanSpec(distanceIntervalM = 100L, baselineDay = day).validate())
        assertFalse(PreventivePlanSpec(hourIntervalS = 100L, baselineDay = day).validate())
        assertFalse(PreventivePlanSpec(timeIntervalDays = 10, baselineDay = day).validate())
        assertTrue(PreventivePlanSpec(timeIntervalDays = 10, timeLeadDays = 2, baselineDay = day).validate())
        assertEquals(LocalDate.of(2026, 11, 28), EnterpriseMaintenanceCalculator.addIntervalDay(day, null, 10))
        assertThrows(IllegalArgumentException::class.java) {
            EnterpriseMaintenanceCalculator.evaluatePlan(PreventivePlanSpec(baselineDay = day), day)
        }
        val woService = EnterpriseMaintenanceCalculator.evaluateWorkOrder(
            listOf(WorkOrderLine(WorkOrderLineKind.SERVICE, 1000L, 10000L)),
            approvedCents = 500L, tolerancePct = 10, discountCents = 0L, paidCents = 1000L,
        )
        assertEquals(1000L, woService.serviceCentavos)
        assertTrue(woService.overrunExceeded)
        val timedPlan = EnterpriseMaintenanceCalculator.evaluatePlan(
            PreventivePlanSpec(timeIntervalDays = 10, timeLeadDays = 2, baselineDay = day),
            evalDay = day.plusDays(9),
        )
        assertEquals(DueState.DUE, timedPlan.state)
        assertEquals(day.plusDays(10), timedPlan.dueDay)
        assertEquals(1, timedPlan.daysToDue)
    }
}
