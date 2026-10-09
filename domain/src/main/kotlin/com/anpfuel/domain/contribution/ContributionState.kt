package com.anpfuel.domain.contribution

import com.anpfuel.domain.portable.PortablePhoto
import com.anpfuel.domain.repository.ContributionRemoteStatus

/**
 * P21-T01 — Frozen photo-led contribution state contract.
 *
 * Five public/owner states (queued/pending/accepted/disputed/rejected).
 * The client never derives review outcomes: [ContributionReview] mirrors
 * the server, and server timestamps plus consensus stay authoritative.
 * Photo evidence is optional (metadata-only drafts validate with weaker
 * confidence); a future mandatory-photo decision is documented separately
 * and changes nothing here. A cancelled draft never left the device, so
 * it resolves to no status.
 */
enum class ContributionLocalPhase {
    NOT_SENT,
    IN_FLIGHT,
    ACKNOWLEDGED,
    TRANSPORT_FAILED,
    CANCELLED,
}

enum class ContributionReview {
    NONE,
    ACCEPTED,
    DISPUTED,
    REJECTED,
}

sealed interface ContributionState {
    data class Queued(val retryable: Boolean) : ContributionState
    data object Pending : ContributionState
    data object Accepted : ContributionState
    data object Disputed : ContributionState
    data object Rejected : ContributionState
}

object ContributionStateRule {

    fun resolve(
        local: ContributionLocalPhase,
        remote: ContributionRemoteStatus?,
        review: ContributionReview,
    ): ContributionState? {
        if (local == ContributionLocalPhase.CANCELLED) return null
        when (review) {
            ContributionReview.REJECTED -> return ContributionState.Rejected
            ContributionReview.DISPUTED -> return ContributionState.Disputed
            ContributionReview.ACCEPTED -> return ContributionState.Accepted
            ContributionReview.NONE -> Unit
        }
        return when (remote) {
            ContributionRemoteStatus.VALIDATED -> ContributionState.Accepted
            ContributionRemoteStatus.REJECTED, ContributionRemoteStatus.EXPIRED -> ContributionState.Rejected
            ContributionRemoteStatus.RECEIVED, ContributionRemoteStatus.VALIDATING -> ContributionState.Pending
            ContributionRemoteStatus.QUEUED, null -> ContributionState.Queued(
                retryable = local == ContributionLocalPhase.TRANSPORT_FAILED,
            )
        }
    }
}

/**
 * P21-T01 — Private-evidence retention boundary.
 *
 * Photo copies are transient ([PortablePhoto.TRANSIENT_TTL_MILLIS], 24 h,
 * server-enforced); status records are metadata and persist. Expiry never
 * blocks fact validation: metadata-only drafts stay valid with weaker
 * confidence. Full object/cache/temp/restore proof belongs to P21-T04.
 */
object ContributionRetention {

    fun isEvidenceLive(capturedAtMillis: Long, nowMillis: Long): Boolean =
        !PortablePhoto.isTransientExpired(capturedAtMillis, nowMillis)
}
