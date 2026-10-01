package com.anpfuel.application.port

/**
 * P10-T05 contribution-outbox flag provider (data owns storage).
 */
interface ContributionOutboxFlagProvider {
    fun isEnabled(): Boolean
}
