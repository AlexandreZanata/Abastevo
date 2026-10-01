package com.anpfuel.domain.feature

/**
 * P10-T04 capture/OCR feature flag.
 *
 * Disabled by default so existing browsing stays unchanged. Enabling only
 * unlocks the local capture + OCR candidate path (system camera intent,
 * [com.anpfuel.domain.portable.PortablePhoto] budgets, human-confirmed
 * price); it never uploads, never auto-picks a price and never migrates
 * existing local data. Rollback is flag OFF + metadata-only contribution.
 */
data class CaptureOcrConfig(val enabled: Boolean) {
    companion object {
        val DISABLED = CaptureOcrConfig(enabled = false)
        val ENABLED = CaptureOcrConfig(enabled = true)
    }
}
