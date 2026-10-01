package com.anpfuel.application.portable

import com.anpfuel.domain.portable.PortableLocation

/**
 * Portable location denial/degraded recovery (P16-T04, BUC-L03, B-BR-L02/L04).
 *
 * Pure Kotlin with zero `java.*`/Android imports so this file moves unchanged
 * into a future `commonMain` source set. Maps one frozen [PortableLocation.FixRisk]
 * to a stable disclosure + recovery vocabulary without touching providers,
 * clocks or I/O: recovery never polls, never caches fixes and never enables
 * background tracking. Every blocked verdict preserves free use (anonymous
 * browsing, manual station lookup, offline functions); only the
 * location-dependent proximity claim stays gated. The server always
 * revalidates (P16-T03); this layer only presents and recovers.
 */
object PortableLocationRecovery {

    /** No recovery needed: the fix passed platform checks. */
    const val RECOVERY_NONE: String = "none"

    /** Permission denial: direct to system settings or manual entry. */
    const val RECOVERY_OPEN_SETTINGS: String = "open-settings"

    /** Platform-marked simulation: disable mock/simulation or go manual. */
    const val RECOVERY_DISABLE_SIMULATION: String = "disable-simulation"

    /** Coarse fix: enable precise location or continue browse-only. */
    const val RECOVERY_ENABLE_PRECISE: String = "enable-precise"

    /** Transient unknown: retry once or fall back to manual entry. */
    const val RECOVERY_RETRY_FIX: String = "retry-fix"

    /** Manual entry: no fix to retry, continue browse-only. */
    const val RECOVERY_BROWSE_ONLY: String = "browse-only"

    /**
     * One recovery decision. [allowsBrowse]/[allowsManual]/[allowsOffline]
     * are always true: denial or degradation never blocks free use.
     * [blocksBackgroundTracking] is always true by construction (no
     * background API exists on this path).
     */
    data class Recovery(
        val verdict: String,
        val reason: String,
        val disclosureCode: String,
        val recoveryCode: String,
        val allowsClaim: Boolean,
        val allowsBrowse: Boolean = true,
        val allowsManual: Boolean = true,
        val allowsOffline: Boolean = true,
    ) {
        val preservesFreeUse: Boolean get() = allowsBrowse && allowsManual && allowsOffline
    }

    /**
     * Reduces one classified risk to its recovery. Pure and deterministic:
     * no source reads, no clock, no polling. Cancellation is trivially safe
     * (no registration to clean up); revocation surfaces as DENIED on the
     * next one-shot read because fixes are never cached here.
     */
    fun recover(risk: PortableLocation.FixRisk): Recovery {
        val disclosure = if (risk.allowsClaim) {
            LocationFlow.DISCLOSURE_FIX_ACCEPTED
        } else {
            risk.reason
        }
        val recovery = when (risk.verdict) {
            PortableLocation.VERIFIED -> RECOVERY_NONE
            PortableLocation.DENIED -> RECOVERY_OPEN_SETTINGS
            PortableLocation.SIMULATED -> RECOVERY_DISABLE_SIMULATION
            PortableLocation.DEGRADED -> RECOVERY_ENABLE_PRECISE
            PortableLocation.MANUAL -> RECOVERY_BROWSE_ONLY
            else -> RECOVERY_RETRY_FIX
        }
        return Recovery(
            verdict = risk.verdict,
            reason = risk.reason,
            disclosureCode = disclosure,
            recoveryCode = recovery,
            allowsClaim = risk.allowsClaim,
        )
    }
}
