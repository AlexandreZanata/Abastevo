package com.anpfuel.application.port

/**
 * P10-T02 community-reads flag provider (data owns storage).
 */
interface CommunityReadsFlagProvider {
    fun isEnabled(): Boolean
}
