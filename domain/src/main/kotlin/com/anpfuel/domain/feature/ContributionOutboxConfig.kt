package com.anpfuel.domain.feature

/**
 * P10-T05 contribution outbox + direct media flag.
 *
 * Disabled by default so existing browsing/capture stays unchanged. Enabling
 * only unlocks durable enqueue + WorkManager dispatch behind the preview
 * base URL (never resolves until deployment config lands); it never
 * removes ANP offline paths, never relabels an old capture as fresh and
 * never deletes accepted history. Rollback is flag OFF + pause queue.
 */
data class ContributionOutboxConfig(val enabled: Boolean) {
    companion object {
        val DISABLED = ContributionOutboxConfig(enabled = false)
        val ENABLED = ContributionOutboxConfig(enabled = true)
    }
}
