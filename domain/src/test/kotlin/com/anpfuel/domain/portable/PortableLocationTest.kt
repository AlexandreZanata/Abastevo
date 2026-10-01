package com.anpfuel.domain.portable

import com.anpfuel.domain.portable.PortableLocation.FixInput
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class PortableLocationTest {

    private fun input(
        permissionGranted: Boolean = true,
        hasFix: Boolean = true,
        sourceInfoPresent: Boolean = true,
        simulated: Boolean = false,
        accuracyMeters: Double? = 25.0,
        fixAgeSeconds: Long? = 30L,
        clockSkewSeconds: Long? = 10L,
        manual: Boolean = false,
    ) = FixInput(
        permissionGranted = permissionGranted,
        hasFix = hasFix,
        sourceInfoPresent = sourceInfoPresent,
        simulated = simulated,
        accuracyMeters = accuracyMeters,
        fixAgeSeconds = fixAgeSeconds,
        clockSkewSeconds = clockSkewSeconds,
        manual = manual,
    )

    @Test
    fun budgetsMirrorFrozenContract() {
        assertEquals("location-v1", PortableLocation.POLICY_VERSION)
        assertEquals(120L, PortableLocation.MAX_FIX_AGE_SECONDS)
        assertEquals(300L, PortableLocation.MAX_CLOCK_SKEW_SECONDS)
        assertEquals(100.0, PortableLocation.MAX_CLAIM_ACCURACY_M)
    }

    @Test
    fun verifiedOnlyOnFreshAccurateNonSimulatedSource() {
        val risk = PortableLocation.classify(input())
        assertEquals(PortableLocation.VERIFIED, risk.verdict)
        assertEquals("", risk.reason)
        assertEquals(PortableLocation.FRESHNESS_FRESH, risk.freshness)
        assertEquals(PortableLocation.ACCURACY_ACCURATE, risk.accuracy)
        assertTrue(risk.allowsClaim)
    }

    @Test
    fun boundariesStayInclusive() {
        assertTrue(PortableLocation.classify(input(fixAgeSeconds = 120L)).allowsClaim)
        assertTrue(PortableLocation.classify(input(accuracyMeters = 100.0)).allowsClaim)
        assertTrue(PortableLocation.classify(input(clockSkewSeconds = 300L)).allowsClaim)
        assertTrue(PortableLocation.classify(input(clockSkewSeconds = -300L)).allowsClaim)
    }

    @Test
    fun simulatedInputIsBlocked() {
        val risk = PortableLocation.classify(input(simulated = true))
        assertEquals(PortableLocation.SIMULATED, risk.verdict)
        assertEquals(PortableLocation.REASON_SIMULATED_SOURCE, risk.reason)
        assertFalse(risk.allowsClaim)
    }

    @Test
    fun missingSourceInfoBlocksForgedClientFlag() {
        val risk = PortableLocation.classify(input(sourceInfoPresent = false, simulated = false))
        assertEquals(PortableLocation.UNKNOWN, risk.verdict)
        assertEquals(PortableLocation.REASON_SOURCE_MISSING, risk.reason)
        assertFalse(risk.allowsClaim)
        // Even a forged simulated=true cannot pass without OS source info.
        val forged = PortableLocation.classify(input(sourceInfoPresent = false, simulated = true))
        assertEquals(PortableLocation.REASON_SOURCE_MISSING, forged.reason)
        assertFalse(forged.allowsClaim)
    }

    @Test
    fun deniedAndManualPathsArePermittedWithoutClaims() {
        val denied = PortableLocation.classify(
            input(permissionGranted = false, hasFix = false, accuracyMeters = null, fixAgeSeconds = null),
        )
        assertEquals(PortableLocation.DENIED, denied.verdict)
        assertEquals(PortableLocation.REASON_PERMISSION_DENIED, denied.reason)
        assertFalse(denied.allowsClaim)

        val manual = PortableLocation.classify(input(manual = true))
        assertEquals(PortableLocation.MANUAL, manual.verdict)
        assertEquals(PortableLocation.REASON_MANUAL_ENTRY, manual.reason)
        assertFalse(manual.allowsClaim)

        val manualOffline = PortableLocation.classify(
            input(
                permissionGranted = false, hasFix = false, sourceInfoPresent = false,
                accuracyMeters = null, fixAgeSeconds = null, clockSkewSeconds = null, manual = true,
            ),
        )
        assertEquals(PortableLocation.MANUAL, manualOffline.verdict)
        assertFalse(manualOffline.allowsClaim)
    }

    @Test
    fun unknownCannotBecomeVerifiedProximity() {
        val cases = listOf(
            input(hasFix = false, accuracyMeters = null, fixAgeSeconds = null) to PortableLocation.REASON_NO_FIX,
            input(accuracyMeters = null) to PortableLocation.REASON_NO_FIX,
            input(fixAgeSeconds = null) to PortableLocation.REASON_NO_FIX,
            input(fixAgeSeconds = 121L) to PortableLocation.REASON_STALE_FIX,
            input(fixAgeSeconds = -5L) to PortableLocation.REASON_CLOCK_ANOMALY,
            input(clockSkewSeconds = 301L) to PortableLocation.REASON_CLOCK_ANOMALY,
        )
        for ((inputCase, reason) in cases) {
            val risk = PortableLocation.classify(inputCase)
            assertEquals(PortableLocation.UNKNOWN, risk.verdict, "verdict for $reason")
            assertEquals(reason, risk.reason)
            assertFalse(risk.allowsClaim, "claim for $reason")
        }
    }

    @Test
    fun coarseAccuracyIsDegradedNeverVerified() {
        val risk = PortableLocation.classify(input(accuracyMeters = 250.0))
        assertEquals(PortableLocation.DEGRADED, risk.verdict)
        assertEquals(PortableLocation.REASON_COARSE_ACCURACY, risk.reason)
        assertEquals(PortableLocation.ACCURACY_COARSE, risk.accuracy)
        assertFalse(risk.allowsClaim)
    }

    @Test
    fun precedenceNeverPromotesWeakerSignals() {
        // Simulated dominates coarse accuracy.
        val simCoarse = PortableLocation.classify(input(simulated = true, accuracyMeters = 500.0))
        assertEquals(PortableLocation.SIMULATED, simCoarse.verdict)
        assertEquals(PortableLocation.ACCURACY_COARSE, simCoarse.accuracy)
        // Denied dominates stale.
        val deniedStale = PortableLocation.classify(input(permissionGranted = false, fixAgeSeconds = 600L))
        assertEquals(PortableLocation.DENIED, deniedStale.verdict)
        assertEquals(PortableLocation.FRESHNESS_STALE, deniedStale.freshness)
    }
}
