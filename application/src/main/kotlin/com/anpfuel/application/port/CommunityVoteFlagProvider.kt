package com.anpfuel.application.port

/**
 * P10-T07 community-vote flag provider (data owns storage).
 *
 * Rollback is flag OFF: confirm/dispute actions disappear while
 * submitted records stay preserved for review.
 */
interface CommunityVoteFlagProvider {
    fun isEnabled(): Boolean
}
