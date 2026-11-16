package com.anpfuel.domain.enterprise.expense

import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncProjection
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncStatus
import java.time.Instant

/*
 * What the expense screen may state (EMP-EXP-004, B-BR-EMP-518/521, EDR-336): facts from the immutable
 * `expense.submit` operation, the sync state from its own retained receipt, and only the review state that receipt
 * proves. No approval, return or decision is ever derived on the device.
 */

/** The review line (BR-REV-002): awaiting approval only after the expense's own ACCEPTED receipt. */
enum class ExpenseReviewLine { NOT_SENT, AWAITING_APPROVAL, NEEDS_REVIEW, REFUSED }

class ExpenseView(val fact: ExpenseSubmitFact, val sync: EnterpriseSyncStatus, val code: String?, val receivedAt: Instant?,
    val review: ExpenseReviewLine) {
    override fun toString(): String = "EnterpriseExpenseView(redacted)"
}

object ExpensePresentation {
    fun view(entry: EnterpriseQueueEntry): ExpenseView {
        val fact = ExpenseOperations.read(entry.operation)
        val sync = EnterpriseSyncProjection.status(EnterpriseSyncProjection.item(entry))
        val review = when (sync) {
            EnterpriseSyncStatus.ACCEPTED -> ExpenseReviewLine.AWAITING_APPROVAL
            EnterpriseSyncStatus.NEEDS_REVIEW -> ExpenseReviewLine.NEEDS_REVIEW
            EnterpriseSyncStatus.REJECTED -> ExpenseReviewLine.REFUSED
            else -> ExpenseReviewLine.NOT_SENT
        }
        val receipt = entry.acknowledgement
        return ExpenseView(fact, sync, receipt?.code, receipt?.receivedAt, review)
    }

    /** Only `expense.submit` entries; newest declared first, equal instants by expense identifier. */
    fun views(entries: List<EnterpriseQueueEntry>): List<ExpenseView> = entries.filter { it.operation.type == ExpenseOperations.SUBMIT }
        .map(::view).sortedWith(compareByDescending<ExpenseView> { it.fact.declared.occurredAt }.thenBy { it.fact.expenseId })
}
