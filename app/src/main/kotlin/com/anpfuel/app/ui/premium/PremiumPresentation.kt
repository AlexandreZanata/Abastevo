package com.anpfuel.app.ui.premium

import com.anpfuel.app.R
import com.anpfuel.domain.premium.PremiumState

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
}
