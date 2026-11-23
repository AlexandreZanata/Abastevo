package com.anpfuel.app.ui.premium

import com.anpfuel.app.R
import com.anpfuel.domain.premium.PremiumState
import com.anpfuel.domain.premium.StoreBillingPeriod

/**
 * Presentation-only mapping for the verified Premium status.
 *
 * Technical capability identifiers and provider reason strings never reach
 * the UI. Unknown capabilities are ignored instead of being exposed verbatim.
 */
private val capabilityLabelPairs = listOf(
    "vehicles.multiple" to R.string.premium_capability_vehicles,
    "cloud.sync" to R.string.premium_capability_cloud_sync,
    "cloud.backup" to R.string.premium_capability_cloud_backup,
    "cloud.restore" to R.string.premium_capability_cloud_restore,
    "alerts.advanced" to R.string.premium_capability_advanced_alerts,
    "professional.write" to R.string.premium_capability_professional_write,
    "professional.report" to R.string.premium_capability_professional_report,
)

fun premiumStateLabelRes(state: PremiumState?): Int = when (state) {
    PremiumState.NONE -> R.string.premium_status_none
    PremiumState.PENDING -> R.string.premium_status_pending
    PremiumState.ACTIVE -> R.string.premium_status_active
    PremiumState.CANCELED -> R.string.premium_status_canceled
    PremiumState.GRACE -> R.string.premium_status_grace
    PremiumState.HOLD -> R.string.premium_status_hold
    PremiumState.PAUSED -> R.string.premium_status_paused
    PremiumState.EXPIRED -> R.string.premium_status_expired
    PremiumState.REVOKED -> R.string.premium_status_revoked
    PremiumState.UNKNOWN, null -> R.string.premium_status_unknown
}

/**
 * The paid-through date is only labelled as access while the verified state
 * actually keeps paid access. HOLD, PAUSED, PENDING, EXPIRED, REVOKED and
 * UNKNOWN must not imply a currently usable date.
 */
fun premiumShowsAccessUntil(state: PremiumState?, validUntil: String?): Boolean =
    !validUntil.isNullOrBlank() && state in setOf(
        PremiumState.ACTIVE,
        PremiumState.CANCELED,
        PremiumState.GRACE,
    )

fun premiumCapabilityLabelRes(capability: String): Int? =
    capabilityLabelPairs.firstOrNull { it.first == capability }?.second

fun premiumCapabilityLabels(capabilities: Set<String>): List<Int> =
    capabilityLabelPairs.filter { it.first in capabilities }.map { it.second }

fun premiumNoticeMessageRes(notice: PremiumNotice): Int = when (notice) {
    PremiumNotice.OFFLINE -> R.string.premium_offline
    PremiumNotice.DENIED -> R.string.premium_denied
    PremiumNotice.ERROR -> R.string.premium_error
    PremiumNotice.RESTORE_UNAVAILABLE -> R.string.premium_restore_unavailable
    PremiumNotice.STORE_FAILED -> R.string.premium_store_failed
    PremiumNotice.PURCHASE_PENDING -> R.string.premium_purchase_pending
    PremiumNotice.PURCHASE_CANCELLED -> R.string.premium_purchase_cancelled
    PremiumNotice.PURCHASE_VERIFIED -> R.string.premium_purchase_verified
    PremiumNotice.PURCHASE_DENIED -> R.string.premium_purchase_denied
    PremiumNotice.PURCHASE_RETRY -> R.string.premium_purchase_retry
    PremiumNotice.PURCHASE_STORE_ERROR -> R.string.premium_purchase_store_error
    PremiumNotice.PURCHASE_ALREADY_OWNED -> R.string.premium_purchase_already_owned
    PremiumNotice.PURCHASE_OWNER_CHANGED -> R.string.premium_purchase_owner_changed
    PremiumNotice.RESTORE_DONE -> R.string.premium_restore_done
    PremiumNotice.RESTORE_NONE -> R.string.premium_restore_none
    PremiumNotice.RESTORE_OTHER_ACCOUNT -> R.string.premium_restore_other_account
}

/** Notices that report a failure and use the error color. */
fun premiumNoticeIsError(notice: PremiumNotice): Boolean =
    notice == PremiumNotice.ERROR || notice == PremiumNotice.PURCHASE_DENIED

fun premiumPurchasePhaseRes(phase: PremiumPurchasePhase): Int? = when (phase) {
    PremiumPurchasePhase.IDLE -> null
    PremiumPurchasePhase.LAUNCHING -> R.string.premium_purchase_launching
    PremiumPurchasePhase.VERIFYING -> R.string.premium_purchase_verifying
    PremiumPurchasePhase.RESTORING -> R.string.premium_purchase_restoring
}

/** A string resource with an optional count argument after the price. */
data class PremiumPlanLabel(val res: Int, val count: Int? = null)

/** Plan name from the store billing period; the server's base plan ID is never shown. */
fun premiumPlanTitle(period: StoreBillingPeriod?): PremiumPlanLabel = when {
    period == null -> PremiumPlanLabel(R.string.premium_plan_title_other)
    period.count == 1 && period.unit == StoreBillingPeriod.PeriodUnit.WEEK -> PremiumPlanLabel(R.string.premium_plan_title_week)
    period.count == 1 && period.unit == StoreBillingPeriod.PeriodUnit.MONTH -> PremiumPlanLabel(R.string.premium_plan_title_month)
    period.count == 1 && period.unit == StoreBillingPeriod.PeriodUnit.YEAR -> PremiumPlanLabel(R.string.premium_plan_title_year)
    period.unit == StoreBillingPeriod.PeriodUnit.MONTH -> PremiumPlanLabel(R.string.premium_plan_title_months, period.count)
    else -> PremiumPlanLabel(R.string.premium_plan_title_other)
}

/** Price line: the store-formatted price plus its billing period. */
fun premiumPlanPrice(period: StoreBillingPeriod?): PremiumPlanLabel = when {
    period == null -> PremiumPlanLabel(R.string.premium_plan_price_other)
    period.count == 1 && period.unit == StoreBillingPeriod.PeriodUnit.WEEK -> PremiumPlanLabel(R.string.premium_plan_price_week)
    period.count == 1 && period.unit == StoreBillingPeriod.PeriodUnit.MONTH -> PremiumPlanLabel(R.string.premium_plan_price_month)
    period.count == 1 && period.unit == StoreBillingPeriod.PeriodUnit.YEAR -> PremiumPlanLabel(R.string.premium_plan_price_year)
    period.unit == StoreBillingPeriod.PeriodUnit.MONTH -> PremiumPlanLabel(R.string.premium_plan_price_every_months, period.count)
    else -> PremiumPlanLabel(R.string.premium_plan_price_other)
}
