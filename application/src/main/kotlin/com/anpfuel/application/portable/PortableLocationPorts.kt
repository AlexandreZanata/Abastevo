package com.anpfuel.application.portable

/**
 * Explicit platform ports for the portable location flow (P16-T02).
 *
 * Pure Kotlin with zero `java.*`/Android imports so this file moves unchanged
 * into a future `commonMain` source set. Native adapters implement these
 * behind the boundary: Android via one-shot last-known reads plus the
 * framework mock flag, iOS via Core Location source information. Tests
 * inject deterministic fakes; no real provider, clock or I/O lives here.
 */
interface LocationSignalSource {

    /**
     * One-shot read of the current device signal, or null when no fix is
     * available right now. Never polls, never requests location updates
     * and never enables background tracking: staleness is enforced by
     * the frozen contract, not by repeated reads.
     */
    fun read(): LocationSignal?
}

/**
 * One device location signal reduced to non-identifying values.
 * Coordinates never cross this boundary: adapters reduce the fix to
 * accuracy and age first, and only bands persist downstream.
 *
 * [sourceInfoAvailable] is OS-provided truth (the mock/simulated flag
 * existed on this fix); [simulated] is meaningful only then.
 * [testInjected] marks app-known debug injection; release builds
 * refuse it as simulation, debug builds admit it for isolated tests.
 */
data class LocationSignal(
    val permissionGranted: Boolean,
    val sourceInfoAvailable: Boolean,
    val simulated: Boolean,
    val hasAccuracy: Boolean,
    val accuracyMeters: Double,
    val fixAgeSeconds: Long?,
    val clockSkewSeconds: Long?,
    val testInjected: Boolean = false,
) {
    companion object {
        /** Permission missing: adapters return this without touching providers. */
        fun denied() = LocationSignal(
            permissionGranted = false,
            sourceInfoAvailable = false,
            simulated = false,
            hasAccuracy = false,
            accuracyMeters = 0.0,
            fixAgeSeconds = null,
            clockSkewSeconds = null,
        )

        /** Permission granted but no fix available right now. */
        fun absent() = LocationSignal(
            permissionGranted = true,
            sourceInfoAvailable = false,
            simulated = false,
            hasAccuracy = false,
            accuracyMeters = 0.0,
            fixAgeSeconds = null,
            clockSkewSeconds = null,
        )
    }
}

/**
 * Build environment for the location flow. Production wires the
 * release flag (no debug injection exists there); tests inject either.
 */
interface LocationEnvironment {

    /** True only in debuggable builds and isolated tests. */
    val allowTestInjection: Boolean
}

/** Wiring bundle for [LocationFlow]; every port is required (no nulls). */
data class LocationPorts(
    val source: LocationSignalSource,
    val environment: LocationEnvironment,
)
