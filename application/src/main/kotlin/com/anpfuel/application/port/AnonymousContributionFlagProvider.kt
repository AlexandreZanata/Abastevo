package com.anpfuel.application.port

/**
 * P10-T03 anonymous-contribution flag provider (data owns storage).
 */
interface AnonymousContributionFlagProvider {
    fun isEnabled(): Boolean
}
