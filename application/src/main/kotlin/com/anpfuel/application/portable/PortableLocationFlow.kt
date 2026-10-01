package com.anpfuel.application.portable

import com.anpfuel.domain.portable.PortableLocation

/**
 * Portable location risk flow shared by Android and iPhone (P16-T02,
 * B-BR-L01…L04).
 *
 * Pure Kotlin with zero `java.*`/Android imports so this file moves unchanged
 * into a future `commonMain` source set. The flow reads the OS signal once,
 * folds app-known debug injection into the simulated verdict on release
 * builds, and classifies through the frozen contract: only a VERIFIED fix
 * authorizes a location-dependent claim, while denied/unknown/manual
 * paths stay permitted for browsing, manual lookup and offline use. No
 * app blacklists, no busy polling, no developer-option blanket denial:
 * unknown sources refuse by default instead of being enumerated.
 * The server always revalidates (P16-T03); the client only gates.
 */
class LocationFlow(private val ports: LocationPorts) {

    /** Claim gate outcome: the stable code travels to UI/audit. */
    sealed interface ClaimAuth {
        data object Allowed : ClaimAuth
        data class Blocked(val code: String) : ClaimAuth
    }

    /**
     * Assesses one location interaction. Manual entry needs no device
     * signal at all: the source is not consulted, so no permission
     * prompt or provider touch can leak into a manual pick.
     */
    fun assess(manual: Boolean): PortableLocation.FixRisk {
        if (manual) {
            return PortableLocation.classify(manualInput())
        }
        val signal = ports.source.read() ?: return PortableLocation.classify(noFixInput())
        // App-known debug injection is simulation on release builds
        // (no injection hook ships there); debuggable builds admit it
        // for isolated tests only.
        val effectiveSimulated = signal.simulated ||
            (signal.testInjected && !ports.environment.allowTestInjection)
        return PortableLocation.classify(
            PortableLocation.FixInput(
                permissionGranted = signal.permissionGranted,
                hasFix = true,
                sourceInfoPresent = signal.sourceInfoAvailable,
                simulated = effectiveSimulated,
                accuracyMeters = signal.accuracyMeters.takeIf { signal.hasAccuracy },
                fixAgeSeconds = signal.fixAgeSeconds,
                clockSkewSeconds = signal.clockSkewSeconds,
                manual = false,
            ),
        )
    }

    /**
     * Authorizes a location-dependent claim: ALLOWED only for a VERIFIED
     * fix, BLOCKED with the stable reason code otherwise. Known
     * simulation blocks on both platforms by construction.
     */
    fun authorizeClaim(manual: Boolean): ClaimAuth {
        val risk = assess(manual)
        return if (risk.allowsClaim) ClaimAuth.Allowed else ClaimAuth.Blocked(risk.reason)
    }

    /**
     * Stable disclosure code for UI/audit: the frozen reason when
     * blocked, `fix-accepted` when the fix passed platform checks.
     * Acceptance is not a truth claim about the user's position.
     */
    fun disclosureCode(risk: PortableLocation.FixRisk): String =
        if (risk.allowsClaim) DISCLOSURE_FIX_ACCEPTED else risk.reason

    private fun manualInput() = PortableLocation.FixInput(
        permissionGranted = false,
        hasFix = false,
        sourceInfoPresent = false,
        simulated = false,
        accuracyMeters = null,
        fixAgeSeconds = null,
        clockSkewSeconds = null,
        manual = true,
    )

    private fun noFixInput() = PortableLocation.FixInput(
        permissionGranted = true,
        hasFix = false,
        sourceInfoPresent = false,
        simulated = false,
        accuracyMeters = null,
        fixAgeSeconds = null,
        clockSkewSeconds = null,
        manual = false,
    )

    companion object {
        /** Fix passed platform checks; never a claim about true position. */
        const val DISCLOSURE_FIX_ACCEPTED: String = "fix-accepted"
    }
}
