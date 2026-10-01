package com.anpfuel.domain.feature

/**
 * P10-T02 community-reads feature flag.
 *
 * Disabled by default so local ANP screens work unchanged. Enabling only
 * adds backend reads with explicit Room fallback; it never removes ANP
 * offline paths nor forces network login.
 */
data class CommunityReadsConfig(val enabled: Boolean) {
    companion object {
        val DISABLED = CommunityReadsConfig(enabled = false)
        val ENABLED = CommunityReadsConfig(enabled = true)
    }
}
