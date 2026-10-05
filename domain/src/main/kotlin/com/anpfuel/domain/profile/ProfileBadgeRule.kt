package com.anpfuel.domain.profile

/**
 * P32-T01 — Pure public representation-badge resolver.
 *
 * Unknown or blank identities resolve to [ProfileBadge.Unclaimed]: the
 * client never invents privilege or a synthetic station. A pending claim
 * is [ProfileBadge.PendingReview], never verified. Only a verified claim
 * from a permitted operator source (`registry`, `dou`, `review`) without a
 * stale/revoked flag yields [ProfileBadge.Verified]. Stale or revoked
 * verification falls back to [ProfileBadge.StaleUnverified] so cached
 * authority can never imply active representation. Badge semantics cover
 * representation only and never conflate fuel or price quality.
 */
sealed interface ProfileBadge {
    data object Unclaimed : ProfileBadge

    data object PendingReview : ProfileBadge

    data class Verified(val source: String) : ProfileBadge

    data object StaleUnverified : ProfileBadge
}

data class ProfileBadgeInput(
    val stationId: String,
    val serverState: String,
    val operatorSource: String,
    val verified: Boolean,
    val staleOrRevoked: Boolean,
)

object ProfileBadgeRule {

    private val permittedSources = setOf("registry", "dou", "review")

    fun resolve(input: ProfileBadgeInput): ProfileBadge {
        if (input.stationId.isBlank()) return ProfileBadge.Unclaimed
        if (input.staleOrRevoked) return ProfileBadge.StaleUnverified
        return when (input.serverState) {
            "pending", "in_review", "needs_info" -> ProfileBadge.PendingReview
            "verified" -> {
                if (input.verified && permittedSources.contains(input.operatorSource)) {
                    ProfileBadge.Verified(source = input.operatorSource)
                } else {
                    ProfileBadge.Unclaimed
                }
            }
            else -> ProfileBadge.Unclaimed
        }
    }
}
