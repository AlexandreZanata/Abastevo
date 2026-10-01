package com.anpfuel.domain.feature

/**
 * P10-T03 anonymous-contribution feature flag.
 *
 * Disabled by default so existing browsing stays unchanged. Enabling only
 * unlocks the device-key proof path (Keystore P-256 + frozen profile);
 * it never forces login nor migrates existing local data.
 */
data class AnonymousContributionConfig(val enabled: Boolean) {
    companion object {
        val DISABLED = AnonymousContributionConfig(enabled = false)
        val ENABLED = AnonymousContributionConfig(enabled = true)
    }
}
