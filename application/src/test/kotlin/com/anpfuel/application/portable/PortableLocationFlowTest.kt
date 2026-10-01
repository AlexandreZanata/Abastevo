package com.anpfuel.application.portable

import com.anpfuel.domain.portable.PortableLocation
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

private class FakeLocationSource(var signal: LocationSignal?) : LocationSignalSource {
    var reads = 0
    override fun read(): LocationSignal? {
        reads++
        return signal
    }
}

private class FakeLocationEnvironment(override val allowTestInjection: Boolean) : LocationEnvironment

private fun verifiedSignal() = LocationSignal(
    permissionGranted = true,
    sourceInfoAvailable = true,
    simulated = false,
    hasAccuracy = true,
    accuracyMeters = 25.0,
    fixAgeSeconds = 30L,
    clockSkewSeconds = 10L,
)

private fun flow(
    signal: LocationSignal?,
    allowTestInjection: Boolean = false,
): Pair<LocationFlow, FakeLocationSource> {
    val source = FakeLocationSource(signal)
    val flow = LocationFlow(
        LocationPorts(
            source = source,
            environment = FakeLocationEnvironment(allowTestInjection),
        ),
    )
    return flow to source
}

class PortableLocationFlowTest {

    @Test
    fun mockProviderBlocksClaimWithOneShotRead() {
        val (flow, source) = flow(verifiedSignal().copy(simulated = true))
        val risk = flow.assess(manual = false)
        assertEquals(PortableLocation.SIMULATED, risk.verdict)
        assertEquals(PortableLocation.REASON_SIMULATED_SOURCE, risk.reason)
        assertEquals(1, source.reads, "assess reads the source exactly once")
        val auth = flow.authorizeClaim(manual = false)
        assertEquals(
            LocationFlow.ClaimAuth.Blocked(PortableLocation.REASON_SIMULATED_SOURCE),
            auth,
        )
        assertEquals(2, source.reads, "each call reads once: one-shot, no polling loops")
    }

    @Test
    fun verifiedFixAllowsClaim() {
        val (flow, _) = flow(verifiedSignal())
        val risk = flow.assess(manual = false)
        assertTrue(risk.allowsClaim)
        assertEquals(LocationFlow.ClaimAuth.Allowed, flow.authorizeClaim(manual = false))
        assertEquals(LocationFlow.DISCLOSURE_FIX_ACCEPTED, flow.disclosureCode(risk))
    }

    @Test
    fun missingSourceInfoBlocksUnknown() {
        val (flow, _) = flow(verifiedSignal().copy(sourceInfoAvailable = false))
        val risk = flow.assess(manual = false)
        assertEquals(PortableLocation.UNKNOWN, risk.verdict)
        assertEquals(PortableLocation.REASON_SOURCE_MISSING, flow.disclosureCode(risk))
    }

    @Test
    fun deniedPermissionBlocks() {
        val (flow, _) = flow(LocationSignal.denied())
        val risk = flow.assess(manual = false)
        assertEquals(PortableLocation.DENIED, risk.verdict)
        assertEquals(
            LocationFlow.ClaimAuth.Blocked(PortableLocation.REASON_PERMISSION_DENIED),
            flow.authorizeClaim(manual = false),
        )
    }

    @Test
    fun absentFixBlocksWithoutClaim() {
        val (flow, _) = flow(LocationSignal.absent())
        assertEquals(PortableLocation.UNKNOWN, flow.assess(manual = false).verdict)
        val (nullFlow, _) = flow(null)
        assertEquals(PortableLocation.UNKNOWN, nullFlow.assess(manual = false).verdict)
    }

    @Test
    fun manualPathSkipsSourceAndNeverAuthorizes() {
        val (flow, source) = flow(verifiedSignal())
        val risk = flow.assess(manual = true)
        assertEquals(PortableLocation.MANUAL, risk.verdict)
        assertEquals(
            LocationFlow.ClaimAuth.Blocked(PortableLocation.REASON_MANUAL_ENTRY),
            flow.authorizeClaim(manual = true),
        )
        assertEquals(0, source.reads, "manual pick touches no provider")
    }

    @Test
    fun releaseRefusesTestInjectionAsSimulation() {
        val injected = verifiedSignal().copy(testInjected = true)
        val (releaseFlow, _) = flow(injected, allowTestInjection = false)
        val risk = releaseFlow.assess(manual = false)
        assertEquals(PortableLocation.SIMULATED, risk.verdict)
        assertEquals(PortableLocation.REASON_SIMULATED_SOURCE, risk.reason)
    }

    @Test
    fun debugAdmitsInjectionForIsolatedTests() {
        val injected = verifiedSignal().copy(testInjected = true)
        val (debugFlow, _) = flow(injected, allowTestInjection = true)
        assertTrue(debugFlow.assess(manual = false).allowsClaim)
    }

    @Test
    fun staleAndCoarseBlockWithStableCodes() {
        val (staleFlow, _) = flow(verifiedSignal().copy(fixAgeSeconds = 121L))
        assertEquals(
            LocationFlow.ClaimAuth.Blocked(PortableLocation.REASON_STALE_FIX),
            staleFlow.authorizeClaim(manual = false),
        )
        val (coarseFlow, _) = flow(verifiedSignal().copy(accuracyMeters = 250.0))
        assertEquals(
            LocationFlow.ClaimAuth.Blocked(PortableLocation.REASON_COARSE_ACCURACY),
            coarseFlow.authorizeClaim(manual = false),
        )
    }
}
