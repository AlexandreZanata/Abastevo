package com.anpfuel.domain.enterprise.fueling

import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncProjection
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncStatus
import java.math.BigInteger
import java.time.Instant

/*
 * What the fueling screen may state (EMP-FUEL-011, B-BR-EMP-458, EDR-295): facts from the immutable operation, the
 * sync state from its own retained receipt, and only the review state that receipt proves.
 */

/** The review line (BR-REV-002): awaiting approval only after the fueling's own ACCEPTED receipt. */
enum class FuelingReviewLine { NOT_SENT, AWAITING_APPROVAL, NEEDS_REVIEW, REFUSED }

class FuelingView(val fact: FuelingFact, val sync: EnterpriseSyncStatus, val code: String?, val receivedAt: Instant?, val review: FuelingReviewLine) {
    override fun toString(): String = "EnterpriseFuelingView(redacted)"
}

object FuelingPresentation {
    fun view(entry: EnterpriseQueueEntry): FuelingView {
        val fact = FuelingOperations.read(entry.operation)
        val sync = EnterpriseSyncProjection.status(EnterpriseSyncProjection.item(entry))
        val review = when (sync) {
            EnterpriseSyncStatus.ACCEPTED -> FuelingReviewLine.AWAITING_APPROVAL
            EnterpriseSyncStatus.NEEDS_REVIEW -> FuelingReviewLine.NEEDS_REVIEW
            EnterpriseSyncStatus.REJECTED -> FuelingReviewLine.REFUSED
            else -> FuelingReviewLine.NOT_SENT
        }
        val receipt = entry.acknowledgement
        return FuelingView(fact, sync, receipt?.code, receipt?.receivedAt, review)
    }

    /** Only `fueling.submit` entries; newest declared first, equal instants by fueling identifier. */
    fun views(entries: List<EnterpriseQueueEntry>): List<FuelingView> = entries.filter { it.operation.type == FuelingOperations.TYPE }
        .map(::view).sortedWith(compareByDescending<FuelingView> { it.fact.declared.occurredAt }.thenBy { it.fact.fuelingId })
}

/**
 * Exact conversion of what the driver typed (EDR-296): digits with at most one decimal comma or point and no grouping;
 * quantities and unit prices keep three decimals, money two. Null when the text does not fit; bounds stay the
 * builder's (the FUEL-001 profile), so no value is ever rounded or guessed.
 */
object FuelingEntry {
    private fun scaled(raw: String, decimals: Int): String? {
        val text = raw.trim()
        if (text.isEmpty() || text.length > 24) return null
        val parts = text.replace(',', '.').split('.')
        if (parts.size > 2) return null
        val whole = parts[0]
        val fraction = parts.getOrElse(1) { "" }
        if (whole.isEmpty() && fraction.isEmpty() || parts.size == 2 && fraction.isEmpty()) return null
        if (fraction.length > decimals || (whole + fraction).any { it !in '0'..'9' }) return null
        val value = BigInteger((whole.ifEmpty { "0" }) + fraction.padEnd(decimals, '0'))
        return value.toString()
    }
    /** Litres, cubic metres or kilowatt-hours with up to three decimals → milli-units. */
    fun quantity(raw: String): String? = scaled(raw, 3)
    /** Reais with up to two decimals → centavos. */
    fun money(raw: String): String? = scaled(raw, 2)
    /** Reais per unit with up to three decimals → milli-BRL. */
    fun unitPrice(raw: String): String? = scaled(raw, 3)
}
