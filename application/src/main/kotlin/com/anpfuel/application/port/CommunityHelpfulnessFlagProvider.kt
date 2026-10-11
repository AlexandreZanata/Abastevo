package com.anpfuel.application.port

/**
 * CS08A helpfulness flag (default OFF).
 *
 * Flag OFF hides helpful/unhelpful actions while recorded votes stay
 * preserved server-side for review (rollback is flag OFF).
 */
interface CommunityHelpfulnessFlagProvider {
    fun isEnabled(): Boolean
}
