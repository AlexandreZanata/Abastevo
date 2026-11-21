package com.anpfuel.domain.enterprise.maintenance

import java.math.BigInteger
import java.time.Instant
import java.time.LocalDate
import java.time.temporal.ChronoUnit

/**
 * EMP-MAINT-001..012: Multi-dimension preventive due/lead calculator, SCHEDULED
 * vs ACTUAL rebase, work-order line reconciliation, hold window merging, and
 * document/licence expiry evaluation.
 */
enum class DueState(val severity: Int) {
    UPCOMING(1),
    DUE(2),
    OVERDUE(3),
}

enum class TriggerKind { NONE, DISTANCE, HOURS, TIME }

enum class RebaseRule { ACTUAL, SCHEDULED }

enum class WorkOrderLineKind { PART, SERVICE, LABOR }

enum class ComplianceState { VALID, DUE_SOON, EXPIRED, CONTROL_DISABLED }

data class PreventivePlanSpec(
    val distanceIntervalM: Long? = null,
    val distanceLeadM: Long? = null,
    val baselineOdometerM: Long? = null,
    val hourIntervalS: Long? = null,
    val hourLeadS: Long? = null,
    val baselineHourS: Long? = null,
    val timeIntervalDays: Int? = null,
    val timeIntervalMonths: Int? = null,
    val timeLeadDays: Int? = null,
    val baselineDay: LocalDate,
) {
    fun validate(): Boolean {
        val hasTrigger = distanceIntervalM != null || hourIntervalS != null ||
            timeIntervalDays != null || timeIntervalMonths != null
        if (!hasTrigger) return false
        if (distanceIntervalM != null) {
            if (distanceLeadM == null || distanceIntervalM <= 0L || distanceLeadM <= 0L ||
                distanceLeadM > distanceIntervalM || baselineOdometerM == null
            ) return false
        }
        if (hourIntervalS != null) {
            if (hourLeadS == null || hourIntervalS <= 0L || hourLeadS <= 0L ||
                hourLeadS > hourIntervalS || baselineHourS == null
            ) return false
        }
        if ((timeIntervalDays != null || timeIntervalMonths != null) &&
            (timeLeadDays == null || timeLeadDays <= 0)
        ) return false
        return true
    }
}

data class PlanEvalResult(
    val state: DueState,
    val decidingTrigger: TriggerKind,
    val usedM: Long? = null,
    val remainingM: Long? = null,
    val usedS: Long? = null,
    val remainingS: Long? = null,
    val dueDay: LocalDate? = null,
    val daysToDue: Int? = null,
    val projectedDueDay: LocalDate? = null,
)

data class WorkOrderLine(
    val kind: WorkOrderLineKind,
    val quantityMilli: Long,
    val unitPriceMilli: Long,
)

data class WorkOrderReconciliation(
    val partsCentavos: Long,
    val serviceCentavos: Long,
    val sumCentavos: Long,
    val overrunCeiling: Long,
    val arithmeticMismatch: Boolean,
    val overrunExceeded: Boolean,
)

object EnterpriseMaintenanceCalculator {
    fun addIntervalDay(base: LocalDate, months: Int?, days: Int?): LocalDate {
        var d = base
        if (months != null && months > 0) d = d.plusMonths(months.toLong())
        if (days != null && days > 0) d = d.plusDays(days.toLong())
        return d
    }

    fun evaluatePlan(
        spec: PreventivePlanSpec,
        evalDay: LocalDate,
        currentOdoM: Long? = null,
        currentHourS: Long? = null,
        windowStartDay: LocalDate? = null,
        windowStartOdoM: Long? = null,
    ): PlanEvalResult {
        require(spec.validate()) { "Invalid preventive maintenance plan specification" }
        var state = DueState.UPCOMING
        var trigger = TriggerKind.NONE
        fun advance(st: DueState, trig: TriggerKind) {
            if (st.severity > state.severity) {
                state = st
                trigger = if (st == DueState.UPCOMING) TriggerKind.NONE else trig
            }
        }

        var usedM: Long? = null
        var remainingM: Long? = null
        var projectedDay: LocalDate? = null
        if (spec.distanceIntervalM != null && currentOdoM != null && spec.baselineOdometerM != null) {
            val used = currentOdoM - spec.baselineOdometerM
            val rem = spec.distanceIntervalM - used
            usedM = used
            remainingM = rem
            val st = when {
                used > spec.distanceIntervalM -> DueState.OVERDUE
                rem <= spec.distanceLeadM!! -> DueState.DUE
                else -> DueState.UPCOMING
            }
            advance(st, TriggerKind.DISTANCE)
            if (rem > 0L && windowStartDay != null && windowStartOdoM != null) {
                val elapsedDays = ChronoUnit.DAYS.between(windowStartDay, evalDay)
                val winUsage = currentOdoM - windowStartOdoM
                if (elapsedDays > 0L && winUsage > 0L) {
                    val num = BigInteger.valueOf(rem).multiply(BigInteger.valueOf(elapsedDays))
                    val den = BigInteger.valueOf(winUsage)
                    val ceilDays = num.add(den.subtract(BigInteger.ONE)).divide(den).longValueExact()
                    projectedDay = evalDay.plusDays(ceilDays)
                }
            }
        }

        var usedS: Long? = null
        var remainingS: Long? = null
        if (spec.hourIntervalS != null && currentHourS != null && spec.baselineHourS != null) {
            val used = currentHourS - spec.baselineHourS
            val rem = spec.hourIntervalS - used
            usedS = used
            remainingS = rem
            val st = when {
                used > spec.hourIntervalS -> DueState.OVERDUE
                rem <= spec.hourLeadS!! -> DueState.DUE
                else -> DueState.UPCOMING
            }
            advance(st, TriggerKind.HOURS)
        }

        var dueDay: LocalDate? = null
        var daysToDue: Int? = null
        if (spec.timeIntervalDays != null || spec.timeIntervalMonths != null) {
            val due = addIntervalDay(spec.baselineDay, spec.timeIntervalMonths, spec.timeIntervalDays)
            val dtd = ChronoUnit.DAYS.between(evalDay, due).toInt()
            dueDay = due
            daysToDue = dtd
            val st = when {
                dtd < 0 -> DueState.OVERDUE
                dtd <= spec.timeLeadDays!! -> DueState.DUE
                else -> DueState.UPCOMING
            }
            advance(st, TriggerKind.TIME)
        }

        return PlanEvalResult(
            state = state,
            decidingTrigger = trigger,
            usedM = usedM,
            remainingM = remainingM,
            usedS = usedS,
            remainingS = remainingS,
            dueDay = dueDay,
            daysToDue = daysToDue,
            projectedDueDay = projectedDay,
        )
    }

    fun rebaseNextMeter(scheduledDue: Long, completedMeter: Long, interval: Long, rule: RebaseRule): Long =
        if (rule == RebaseRule.SCHEDULED) scheduledDue + interval else completedMeter + interval

    private fun roundHalfUp(num: Long, den: Long): Long {
        val bNum = BigInteger.valueOf(num)
        val bDen = BigInteger.valueOf(den)
        return bNum.add(bDen.shiftRight(1)).divide(bDen).longValueExact()
    }

    fun evaluateWorkOrder(
        lines: List<WorkOrderLine>,
        approvedCents: Long,
        tolerancePct: Int,
        discountCents: Long,
        paidCents: Long,
    ): WorkOrderReconciliation {
        require(approvedCents >= 0L && tolerancePct in 0..100 && discountCents >= 0L && paidCents >= 0L)
        var partsCents = 0L
        var serviceCents = 0L
        for (line in lines) {
            require(line.quantityMilli > 0L && line.unitPriceMilli >= 0L)
            when (line.kind) {
                WorkOrderLineKind.PART -> partsCents += roundHalfUp(line.quantityMilli * line.unitPriceMilli, 10_000L)
                WorkOrderLineKind.SERVICE -> serviceCents += roundHalfUp(line.quantityMilli * line.unitPriceMilli, 10_000L)
                WorkOrderLineKind.LABOR -> serviceCents += roundHalfUp(line.quantityMilli * line.unitPriceMilli, 36_000L)
            }
        }
        val sumCents = partsCents + serviceCents
        val ceiling = approvedCents + (approvedCents * tolerancePct.toLong()) / 100L
        return WorkOrderReconciliation(
            partsCentavos = partsCents,
            serviceCentavos = serviceCents,
            sumCentavos = sumCents,
            overrunCeiling = ceiling,
            arithmeticMismatch = paidCents + discountCents != sumCents,
            overrunExceeded = paidCents > ceiling,
        )
    }

    fun unavailableSeconds(
        windows: List<Pair<Instant, Instant>>,
        periodStart: Instant,
        periodEnd: Instant,
    ): Long {
        if (!periodEnd.isAfter(periodStart) || windows.isEmpty()) return 0L
        val clipped = windows.mapNotNull { (rawStart, rawEnd) ->
            val s = if (rawStart.isBefore(periodStart)) periodStart else rawStart
            val e = if (rawEnd.isAfter(periodEnd)) periodEnd else rawEnd
            if (e.isAfter(s)) s to e else null
        }.sortedBy { it.first }
        if (clipped.isEmpty()) return 0L
        var total = 0L
        var curStart = clipped.first().first
        var curEnd = clipped.first().second
        for (i in 1 until clipped.size) {
            val (ns, ne) = clipped[i]
            if (!ns.isAfter(curEnd)) {
                if (ne.isAfter(curEnd)) curEnd = ne
            } else {
                total += ChronoUnit.SECONDS.between(curStart, curEnd)
                curStart = ns
                curEnd = ne
            }
        }
        total += ChronoUnit.SECONDS.between(curStart, curEnd)
        return total
    }

    fun evaluateCompliance(
        expiryDay: LocalDate,
        evalDay: LocalDate,
        leadDays: Int,
        controlEnabled: Boolean = true,
    ): ComplianceState {
        if (!controlEnabled) return ComplianceState.CONTROL_DISABLED
        val daysLeft = ChronoUnit.DAYS.between(evalDay, expiryDay).toInt()
        return when {
            daysLeft < 0 -> ComplianceState.EXPIRED
            daysLeft <= leadDays -> ComplianceState.DUE_SOON
            else -> ComplianceState.VALID
        }
    }
}
