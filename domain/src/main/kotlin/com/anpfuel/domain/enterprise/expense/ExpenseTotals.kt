package com.anpfuel.domain.enterprise.expense

import java.math.BigInteger
import java.time.LocalDate
import java.time.format.DateTimeFormatter
import java.time.format.DateTimeParseException

/** How a review state enters cost totals (BR-REV-009, BR-EXP-002). */
enum class CostCounting { APPROVED, PENDING, EXCLUDED }

/** Closed input: BR-REV states plus VOIDED; the lifecycle that produces it is owned elsewhere. */
enum class ReviewState(val counting: CostCounting) {
    DRAFT(CostCounting.EXCLUDED),
    SUBMITTED(CostCounting.PENDING),
    IN_REVIEW(CostCounting.PENDING),
    APPROVED(CostCounting.APPROVED),
    RETURNED(CostCounting.EXCLUDED),
    REJECTED(CostCounting.EXCLUDED),
    CANCELLED(CostCounting.EXCLUDED),
    VOIDED(CostCounting.EXCLUDED);

    companion object {
        fun parse(raw: String): ReviewState = entries.firstOrNull { it.name == raw } ?: throw invalid()
    }
}

/** APPROVED counts approved cost only; INCLUDES_PENDING adds the pending part and says so. */
enum class TotalsBasis {
    APPROVED, INCLUDES_PENDING;

    companion object {
        fun parse(raw: String): TotalsBasis = entries.firstOrNull { it.name == raw } ?: throw invalid()
    }
}

/** Half-open [start, end) range of YYYY-MM-DD reporting days. */
class ReportingPeriod private constructor(val start: String, val end: String) {
    /** start <= day < end; a malformed day is never contained. */
    fun contains(day: String): Boolean = validDay(day) && day >= start && day < end
    override fun equals(other: Any?): Boolean = other is ReportingPeriod && start == other.start && end == other.end
    override fun hashCode(): Int = 31 * start.hashCode() + end.hashCode()
    override fun toString(): String = "ReportingPeriod[$start, $end)"

    companion object {
        private val dayPattern = Regex("^[0-9]{4}-[0-9]{2}-[0-9]{2}$")

        /** A real calendar day in years 0001..9999; fixed width makes lexical order calendar order. */
        private fun validDay(raw: String): Boolean {
            if (!dayPattern.matches(raw) || raw.startsWith("0000")) return false
            return try { LocalDate.parse(raw, DateTimeFormatter.ISO_LOCAL_DATE); true } catch (_: DateTimeParseException) { false }
        }

        fun of(start: String, end: String): ReportingPeriod {
            if (!validDay(start) || !validDay(end) || start >= end) throw invalid()
            return ReportingPeriod(start, end)
        }
    }
}

/** An expense with its current review state. */
data class ExpenseEntry(val expense: Expense, val state: ReviewState)

/** Exact review-basis total for one organization and period. */
class CostTotals internal constructor(
    val organizationId: String,
    val period: ReportingPeriod,
    val basis: TotalsBasis,
    /** Approved cost: APPROVED company amounts only. */
    val approvedCentavos: BigInteger,
    val approvedCount: Int,
    /** The separately identified SUBMITTED/IN_REVIEW part. */
    val pendingCentavos: BigInteger,
    val pendingCount: Int,
    val excludedCount: Int,
) {
    /** The "includes pending" label; it follows the declared basis. */
    val includesPending: Boolean get() = basis == TotalsBasis.INCLUDES_PENDING

    /** Approved cost, plus the pending part only under INCLUDES_PENDING. */
    val figureCentavos: BigInteger get() = if (includesPending) approvedCentavos + pendingCentavos else approvedCentavos
    override fun toString(): String = "EnterpriseCostTotals(redacted)"
}

object ExpenseTotals {
    /**
     * Sums company amounts on the declared basis. Each entry, in input order, must belong to the scope
     * (SCOPE) and period (OUT_OF_PERIOD) and carry an identity and origin key not seen before
     * (DUPLICATE_COST); the first failure returns no total. Sums are BigInteger, so no selection overflows.
     */
    fun compute(scope: String, period: ReportingPeriod, basis: TotalsBasis, entries: List<ExpenseEntry>): CostTotals {
        requireScope(scope)
        val identities = HashSet<String>()
        val keys = HashSet<OriginKey>()
        var approved = BigInteger.ZERO
        var pending = BigInteger.ZERO
        var approvedCount = 0
        var pendingCount = 0
        var excludedCount = 0
        for ((expense, state) in entries.toList()) {
            if (expense.reference.organizationId != scope) throw foreignScope()
            if (!period.contains(expense.reporting.day)) throw ExpenseDomainException(ExpenseFailure.OUT_OF_PERIOD)
            val key = expense.originKey
            if (expense.reference.id in identities || key in keys) throw ExpenseDomainException(ExpenseFailure.DUPLICATE_COST)
            identities += expense.reference.id
            keys += key
            val company = BigInteger.valueOf(expense.amounts.companyCentavos)
            when (state.counting) {
                CostCounting.APPROVED -> { approved += company; approvedCount++ }
                CostCounting.PENDING -> { pending += company; pendingCount++ }
                CostCounting.EXCLUDED -> excludedCount++
            }
        }
        return CostTotals(scope, period, basis, approved, approvedCount, pending, pendingCount, excludedCount)
    }
}
