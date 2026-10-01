package com.anpfuel.app.community

import com.anpfuel.domain.repository.CommunityDisputeReasons
import com.anpfuel.domain.repository.CommunityVoteRejectKind

/**
 * P10-T07 — Confirm/dispute presentation (BUC-005, B-BR-006/009/011).
 *
 * Pure (no `android.*`): the confirmation summary always carries the
 * exact amount, product, unit and full condition the contributor saw,
 * so a stale or different condition is never confirmed silently. A
 * changed price points at a distinct replacement observation (recorded
 * as a new observation, never an edit). Confidence stays support, never
 * a guarantee (B-BR-009: entitlement never changes ordering). Dispute
 * detail is private copy only — labels and errors never echo it.
 */
object CommunityVoteDisplay {

    /** Backend dispute detail cap (same as the application use case). */
    const val MAX_DETAIL_CHARS = 500

    fun disputeReasonLabel(reasonWire: String): String =
        when (reasonWire) {
            CommunityDisputeReasons.PRICE_CHANGED -> "Price changed"
            CommunityDisputeReasons.WRONG_STATION -> "Wrong station"
            CommunityDisputeReasons.WRONG_PRODUCT -> "Wrong product"
            CommunityDisputeReasons.WRONG_CONDITION -> "Wrong condition"
            CommunityDisputeReasons.EVIDENCE_MISMATCH -> "Evidence mismatch"
            CommunityDisputeReasons.OTHER -> "Other"
            else -> throw IllegalArgumentException("unknown dispute reason")
        }

    fun disputeReasons(): List<String> = CommunityDisputeReasons.ALL.sorted()

    /**
     * Summary shown before either action. Exact milli money plus unit
     * and the full human condition label — never amount alone.
     */
    fun confirmSummary(
        productWire: String,
        amountMilliBrl: Long,
        unit: String,
        conditionLabel: String,
    ): String {
        require(productWire.isNotBlank()) { "product is blank" }
        require(unit.isNotBlank()) { "unit is blank" }
        require(conditionLabel.isNotBlank()) { "condition label is blank" }
        val amount = CommunityPriceDisplay.formatMilliBrl(amountMilliBrl)
        return "$productWire · $amount / $unit · $conditionLabel"
    }

    /** Only a changed price may reference a replacement observation. */
    fun needsReplacement(reasonWire: String): Boolean =
        reasonWire == CommunityDisputeReasons.PRICE_CHANGED

    fun rejectKindLabel(kind: CommunityVoteRejectKind): String =
        when (kind) {
            CommunityVoteRejectKind.SELF_CONFIRMATION -> "SELF"
            CommunityVoteRejectKind.ALREADY_RECORDED -> "ALREADY_RECORDED"
            CommunityVoteRejectKind.INELIGIBLE_TARGET -> "INELIGIBLE"
            CommunityVoteRejectKind.CONFLICT -> "CONFLICT"
            CommunityVoteRejectKind.TRANSPORT -> "TRANSPORT"
        }
}
