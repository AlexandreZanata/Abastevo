package com.anpfuel.domain.enterprise.expense

import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.EnterpriseTimeException
import com.anpfuel.domain.enterprise.EnterpriseValueException
import com.anpfuel.domain.enterprise.EnterpriseValues
import com.anpfuel.domain.enterprise.ReportingAttribution
import com.anpfuel.domain.enterprise.ReportingZones
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.fueling.FuelingRef

/** Closed failure vocabulary shared with the Go package through expense-v1. */
enum class ExpenseFailure { INVALID, SCOPE, NOT_COST_ORIGIN, COMPANY_EXCEEDS_PAID, DUPLICATE_COST, OUT_OF_PERIOD }

class ExpenseDomainException(val failure: ExpenseFailure = ExpenseFailure.INVALID) :
    IllegalArgumentException("expense: ${failure.name.lowercase()}")

internal fun invalid(): ExpenseDomainException = ExpenseDomainException()
internal fun foreignScope(): ExpenseDomainException = ExpenseDomainException(ExpenseFailure.SCOPE)

/** A malformed organization is a scope failure; a malformed identity is invalid. */
internal fun requireScope(raw: String): String =
    try { EnterprisePartition.requireId(raw) } catch (_: IllegalArgumentException) { throw foreignScope() }
private fun requireIdentity(raw: String): String =
    try { EnterprisePartition.requireId(raw) } catch (_: IllegalArgumentException) { throw invalid() }

/** Closed economic cost origins (EDR-131); settlement kinds are refused as NOT_COST_ORIGIN. */
enum class OriginKind {
    FUELING, WORK_ORDER, DIRECT;

    companion object {
        /** Settlement, evidence and stock-movement kinds never recognize a (second) cost (EDR-013). */
        private val notCostOrigins = setOf(
            "PAYMENT", "REIMBURSEMENT", "CARD_STATEMENT", "ADVANCE", "PAYABLE", "DOCUMENT", "STOCK_PURCHASE", "STOCK_TRANSFER",
        )

        fun parse(raw: String): OriginKind = entries.firstOrNull { it.name == raw }
            ?: throw if (raw in notCostOrigins) ExpenseDomainException(ExpenseFailure.NOT_COST_ORIGIN) else invalid()
    }
}

/** Supplied scoped identities never prove existence, acceptance or authority. */
sealed class ScopedExpenseRef(val organizationId: String, val id: String) {
    init {
        requireScope(organizationId)
        requireIdentity(id)
    }
    final override fun equals(other: Any?): Boolean =
        other is ScopedExpenseRef && javaClass == other.javaClass && organizationId == other.organizationId && id == other.id
    final override fun hashCode(): Int = 31 * (31 * javaClass.hashCode() + organizationId.hashCode()) + id.hashCode()
    final override fun toString(): String = "EnterpriseExpenseRef(redacted)"
}
class ExpenseRef(organizationId: String, id: String) : ScopedExpenseRef(organizationId, id)
class SourceRef(organizationId: String, id: String) : ScopedExpenseRef(organizationId, id)

/** One cost origin. DIRECT carries no source: the expense is its own source. */
class ExpenseOrigin private constructor(val kind: OriginKind, val source: SourceRef?) {
    override fun toString(): String = "EnterpriseExpenseOrigin(redacted)"

    companion object {
        /** FUELING and WORK_ORDER need a same-organization source; DIRECT refuses one. */
        fun create(scope: String, kind: OriginKind, source: SourceRef?): ExpenseOrigin {
            requireScope(scope)
            if (kind == OriginKind.DIRECT) {
                if (source != null) throw invalid()
                return ExpenseOrigin(kind, null)
            }
            if (source == null) throw invalid()
            if (source.organizationId != scope) throw foreignScope()
            return ExpenseOrigin(kind, source)
        }

        /** References an EMP-FUEL-001 fueling without re-modelling it. */
        fun fueling(scope: String, fueling: FuelingRef): ExpenseOrigin =
            create(scope, OriginKind.FUELING, SourceRef(fueling.organizationId, fueling.id))
    }
}

/** One-origin-one-cost identity: organization, kind and source, or the expense identity for DIRECT. */
class OriginKey(val organizationId: String, val kind: OriginKind, val source: String) {
    override fun equals(other: Any?): Boolean =
        other is OriginKey && organizationId == other.organizationId && kind == other.kind && source == other.source
    override fun hashCode(): Int = 31 * (31 * organizationId.hashCode() + kind.hashCode()) + source.hashCode()
    override fun toString(): String = "EnterpriseOriginKey(redacted)"
}

/** Authoritative paid amount and company amount, exact centavos with 0 <= company <= paid. */
class ExpenseAmounts private constructor(val paidCentavos: Long, val companyCentavos: Long) {
    override fun toString(): String = "EnterpriseExpenseAmounts(redacted)"

    companion object {
        /** Null is absence, never zero. Bounds and canonical form are the exact-value kernel's. */
        fun parse(paid: String?, company: String?): ExpenseAmounts {
            if (paid == null || company == null) throw invalid()
            val (p, c) = try {
                EnterpriseValues.parseCentavos(paid) to EnterpriseValues.parseCentavos(company)
            } catch (_: EnterpriseValueException) { throw invalid() }
            if (c > p) throw ExpenseDomainException(ExpenseFailure.COMPANY_EXCEEDS_PAID)
            return ExpenseAmounts(p, c)
        }
    }
}

/** Structurally valid category code; the configured catalogue belongs to EMP-EXP-003. */
class ExpenseCategory private constructor(val code: String) {
    override fun equals(other: Any?): Boolean = other is ExpenseCategory && code == other.code
    override fun hashCode(): Int = code.hashCode()
    override fun toString(): String = "EnterpriseExpenseCategory(redacted)"

    companion object {
        /** The repository's closed ASCII key profile. */
        private val pattern = Regex("^[A-Z][A-Z0-9_]{0,63}$")

        fun parse(raw: String): ExpenseCategory = if (pattern.matches(raw)) ExpenseCategory(raw) else throw invalid()
    }
}

/** Required expense facts. Zones must come from the resolved operating context. */
data class ExpenseInput(
    val reference: ExpenseRef? = null,
    val origin: ExpenseOrigin? = null,
    val category: ExpenseCategory? = null,
    val amounts: ExpenseAmounts? = null,
    val competence: TimeDeclaration? = null,
    val zones: ReportingZones? = null,
) {
    override fun toString(): String = "EnterpriseExpenseInput(redacted)"
}

/** Immutable validated cost value; its review state is an external input. */
class Expense private constructor(
    val reference: ExpenseRef,
    val origin: ExpenseOrigin,
    val category: ExpenseCategory,
    val amounts: ExpenseAmounts,
    val competence: TimeDeclaration,
    val reporting: ReportingAttribution,
) {
    /** The one-origin-one-cost identity of this expense. */
    val originKey: OriginKey
        get() = OriginKey(reference.organizationId, origin.kind, origin.source?.id ?: reference.id)

    override fun toString(): String = "EnterpriseExpense(redacted)"

    companion object {
        /**
         * Validates, in order, scope and identity, origin, category, amounts, competence and reporting
         * zones. Reads no clock and invents no receipt; the reporting day comes from the supplied context.
         */
        fun create(scope: String, input: ExpenseInput): Expense {
            requireScope(scope)
            val reference = input.reference ?: throw invalid()
            if (reference.organizationId != scope) throw foreignScope()
            val supplied = input.origin ?: throw invalid()
            val origin = ExpenseOrigin.create(scope, supplied.kind, supplied.source)
            val category = input.category ?: throw invalid()
            val amounts = input.amounts ?: throw invalid()
            val declared = input.competence ?: throw invalid()
            val zones = input.zones ?: throw invalid()
            return try {
                val competence = EnterpriseTime.validateDeclaration(declared)
                Expense(reference, origin, category, amounts, competence, EnterpriseTime.attributeReportingDay(competence.occurredAt, zones))
            } catch (_: EnterpriseTimeException) { throw invalid() }
        }
    }
}
