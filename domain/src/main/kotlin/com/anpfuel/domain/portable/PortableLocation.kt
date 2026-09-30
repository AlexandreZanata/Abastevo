package com.anpfuel.domain.portable

/**
 * Portable location risk contract (P16-T01, B-BR-L01…L04).
 *
 * Pure Kotlin with zero `java.*` imports so this file moves unchanged into a
 * future `commonMain` source set. Mirrors the frozen backend classifier
 * (`backend/internal/modules/community/application/location.go`): OS source
 * signals (`LocationCompat.isMock` on Android /
 * `CLLocationSourceInformation.isSimulatedBySoftware` on iOS) and fix
 * metadata reduce to one verdict before any proximity claim exists.
 * Detects platform-marked simulation only; never promises universal
 * spoof-proofing, and a forged client flag cannot produce source info the
 * OS did not provide. The server always enforces; native adapters arrive
 * in P16-T02. Golden vectors:
 * `contracts/testdata/location/risk-v1.json` (replayed by Go).
 */
object PortableLocation {

    /** Frozen policy version; changes require fixtures, never silence. */
    const val POLICY_VERSION: String = "location-v1"

    /** Verdicts; only VERIFIED may authorize a location-dependent claim. */
    const val VERIFIED: String = "VERIFIED"
    const val DEGRADED: String = "DEGRADED"
    const val MANUAL: String = "MANUAL"
    const val DENIED: String = "DENIED"
    const val SIMULATED: String = "SIMULATED"
    const val UNKNOWN: String = "UNKNOWN"

    /** Stable reason codes; UI wording stays outside this layer. */
    const val REASON_SIMULATED_SOURCE: String = "simulated-source"
    const val REASON_PERMISSION_DENIED: String = "permission-denied"
    const val REASON_NO_FIX: String = "no-fix"
    const val REASON_SOURCE_MISSING: String = "source-info-missing"
    const val REASON_STALE_FIX: String = "stale-fix"
    const val REASON_CLOCK_ANOMALY: String = "clock-anomaly"
    const val REASON_COARSE_ACCURACY: String = "coarse-accuracy"
    const val REASON_MANUAL_ENTRY: String = "manual-entry"

    /** Frozen tunables (conservative hypotheses, not fraud-proof claims). */
    const val MAX_FIX_AGE_SECONDS: Long = 120L
    const val MAX_CLOCK_SKEW_SECONDS: Long = 300L
    const val MAX_CLAIM_ACCURACY_M: Double = 100.0

    /** Persisted bands; coordinates never leave this layer. */
    const val FRESHNESS_FRESH: String = "FRESH"
    const val FRESHNESS_STALE: String = "STALE"
    const val FRESHNESS_UNKNOWN: String = "UNKNOWN"
    const val ACCURACY_ACCURATE: String = "ACCURATE"
    const val ACCURACY_COARSE: String = "COARSE"
    const val ACCURACY_UNKNOWN: String = "UNKNOWN"

    /**
     * One device location signal reduced to non-identifying values.
     * [sourceInfoPresent] is OS-provided truth (the mock/simulated flag
     * existed); [simulated] is meaningful only then. Nulls mark missing
     * data, never zero defaults.
     */
    data class FixInput(
        val permissionGranted: Boolean,
        val hasFix: Boolean,
        val sourceInfoPresent: Boolean,
        val simulated: Boolean,
        val accuracyMeters: Double?,
        val fixAgeSeconds: Long?,
        val clockSkewSeconds: Long?,
        val manual: Boolean = false,
    )

    /**
     * Frozen classification: verdict plus bands and the single claim
     * gate. [allowsClaim] is true only for VERIFIED; an UNKNOWN can
     * never become verified proximity.
     */
    data class FixRisk(
        val verdict: String,
        val reason: String,
        val freshness: String,
        val accuracy: String,
        val policyVersion: String = POLICY_VERSION,
    ) {
        val allowsClaim: Boolean get() = verdict == VERIFIED
    }

    /**
     * Reduces one signal to its verdict. Precedence is deliberate:
     * manual entry beats permission denial; denial beats signal
     * quality; missing signal data beats a claimed simulated flag; OS
     * source info gates the simulated flag itself. Pure and
     * deterministic.
     */
    fun classify(input: FixInput): FixRisk {
        val freshness = freshnessBand(input.fixAgeSeconds)
        val accuracy = accuracyBand(input.accuracyMeters)
        val verdict: String
        val reason: String
        when {
            input.manual -> {
                verdict = MANUAL
                reason = REASON_MANUAL_ENTRY
            }
            !input.permissionGranted -> {
                verdict = DENIED
                reason = REASON_PERMISSION_DENIED
            }
            !input.hasFix || input.accuracyMeters == null || input.fixAgeSeconds == null -> {
                verdict = UNKNOWN
                reason = REASON_NO_FIX
            }
            !input.sourceInfoPresent -> {
                verdict = UNKNOWN
                reason = REASON_SOURCE_MISSING
            }
            input.simulated -> {
                verdict = SIMULATED
                reason = REASON_SIMULATED_SOURCE
            }
            input.fixAgeSeconds < 0 -> {
                verdict = UNKNOWN
                reason = REASON_CLOCK_ANOMALY
            }
            input.clockSkewSeconds != null &&
                (if (input.clockSkewSeconds < 0) -input.clockSkewSeconds else input.clockSkewSeconds) >
                MAX_CLOCK_SKEW_SECONDS -> {
                verdict = UNKNOWN
                reason = REASON_CLOCK_ANOMALY
            }
            input.fixAgeSeconds > MAX_FIX_AGE_SECONDS -> {
                verdict = UNKNOWN
                reason = REASON_STALE_FIX
            }
            input.accuracyMeters > MAX_CLAIM_ACCURACY_M -> {
                verdict = DEGRADED
                reason = REASON_COARSE_ACCURACY
            }
            else -> {
                verdict = VERIFIED
                reason = ""
            }
        }
        return FixRisk(verdict = verdict, reason = reason, freshness = freshness, accuracy = accuracy)
    }

    private fun freshnessBand(ageSeconds: Long?): String = when {
        ageSeconds == null || ageSeconds < 0 -> FRESHNESS_UNKNOWN
        ageSeconds > MAX_FIX_AGE_SECONDS -> FRESHNESS_STALE
        else -> FRESHNESS_FRESH
    }

    private fun accuracyBand(accuracyMeters: Double?): String = when {
        accuracyMeters == null -> ACCURACY_UNKNOWN
        accuracyMeters > MAX_CLAIM_ACCURACY_M -> ACCURACY_COARSE
        else -> ACCURACY_ACCURATE
    }
}
