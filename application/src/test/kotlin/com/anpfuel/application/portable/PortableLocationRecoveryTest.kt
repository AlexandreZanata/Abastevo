package com.anpfuel.application.portable

import com.anpfuel.domain.portable.PortableLocation
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class PortableLocationRecoveryTest {

    private fun risk(
        verdict: String,
        reason: String,
        allowsClaim: Boolean,
    ) = PortableLocation.FixRisk(
        verdict = verdict,
        reason = reason,
        freshness = PortableLocation.FRESHNESS_UNKNOWN,
        accuracy = PortableLocation.ACCURACY_UNKNOWN,
    ).let {
        // FixRisk derives allowsClaim from verdict; assert the fixture agrees.
        assertEquals(allowsClaim, it.allowsClaim, "fixture verdict/claim mismatch for $verdict")
        it
    }

    @Test
    fun verifiedAllowsClaimWithNoRecovery() {
        val verified = PortableLocation.FixRisk(
            verdict = PortableLocation.VERIFIED,
            reason = "",
            freshness = PortableLocation.FRESHNESS_FRESH,
            accuracy = PortableLocation.ACCURACY_ACCURATE,
        )
        val recovery = PortableLocationRecovery.recover(verified)
        assertTrue(recovery.allowsClaim)
        assertTrue(recovery.preservesFreeUse)
        assertEquals(LocationFlow.DISCLOSURE_FIX_ACCEPTED, recovery.disclosureCode)
        assertEquals(PortableLocationRecovery.RECOVERY_NONE, recovery.recoveryCode)
    }

    @Test
    fun deniedBlocksClaimButPreservesFreeUse() {
        val recovery = PortableLocationRecovery.recover(
            risk(PortableLocation.DENIED, PortableLocation.REASON_PERMISSION_DENIED, false),
        )
        assertFalse(recovery.allowsClaim)
        assertTrue(recovery.preservesFreeUse)
        assertEquals(PortableLocation.REASON_PERMISSION_DENIED, recovery.disclosureCode)
        assertEquals(PortableLocationRecovery.RECOVERY_OPEN_SETTINGS, recovery.recoveryCode)
    }

    @Test
    fun simulatedBlocksClaimWithDisableSimulationRecovery() {
        val recovery = PortableLocationRecovery.recover(
            risk(PortableLocation.SIMULATED, PortableLocation.REASON_SIMULATED_SOURCE, false),
        )
        assertFalse(recovery.allowsClaim)
        assertTrue(recovery.preservesFreeUse)
        assertEquals(PortableLocation.REASON_SIMULATED_SOURCE, recovery.disclosureCode)
        assertEquals(PortableLocationRecovery.RECOVERY_DISABLE_SIMULATION, recovery.recoveryCode)
    }

    @Test
    fun degradedBlocksClaimButKeepsBrowseOnly() {
        val recovery = PortableLocationRecovery.recover(
            risk(PortableLocation.DEGRADED, PortableLocation.REASON_COARSE_ACCURACY, false),
        )
        assertFalse(recovery.allowsClaim)
        assertTrue(recovery.preservesFreeUse)
        assertEquals(PortableLocation.REASON_COARSE_ACCURACY, recovery.disclosureCode)
        assertEquals(PortableLocationRecovery.RECOVERY_ENABLE_PRECISE, recovery.recoveryCode)
    }

    @Test
    fun manualBlocksClaimButPreservesFreeUse() {
        val recovery = PortableLocationRecovery.recover(
            risk(PortableLocation.MANUAL, PortableLocation.REASON_MANUAL_ENTRY, false),
        )
        assertFalse(recovery.allowsClaim)
        assertTrue(recovery.preservesFreeUse)
        assertEquals(PortableLocation.REASON_MANUAL_ENTRY, recovery.disclosureCode)
        assertEquals(PortableLocationRecovery.RECOVERY_BROWSE_ONLY, recovery.recoveryCode)
    }

    @Test
    fun unknownMatrixAlwaysRetriesWithFreeUse() {
        val reasons = listOf(
            PortableLocation.REASON_NO_FIX,
            PortableLocation.REASON_SOURCE_MISSING,
            PortableLocation.REASON_STALE_FIX,
            PortableLocation.REASON_CLOCK_ANOMALY,
        )
        for (reason in reasons) {
            val recovery = PortableLocationRecovery.recover(
                risk(PortableLocation.UNKNOWN, reason, false),
            )
            assertFalse(recovery.allowsClaim, "UNKNOWN/$reason must not authorize")
            assertTrue(recovery.preservesFreeUse, "UNKNOWN/$reason must preserve free use")
            assertEquals(reason, recovery.disclosureCode, "disclosure stays honest for $reason")
            assertEquals(
                PortableLocationRecovery.RECOVERY_RETRY_FIX,
                recovery.recoveryCode,
                "UNKNOWN/$reason recovers by retry/manual",
            )
        }
    }

    @Test
    fun revocationCoarseAndOfflineResumeStayHonest() {
        // Permission revocation: a later DENIED read refuses even after a
        // VERIFIED history (no cached fix reuse on this path).
        val revoked = PortableLocationRecovery.recover(
            risk(PortableLocation.DENIED, PortableLocation.REASON_PERMISSION_DENIED, false),
        )
        assertFalse(revoked.allowsClaim)
        assertTrue(revoked.preservesFreeUse)

        // Coarse downgrade: DEGRADED never promotes to VERIFIED proximity.
        val coarse = PortableLocationRecovery.recover(
            risk(PortableLocation.DEGRADED, PortableLocation.REASON_COARSE_ACCURACY, false),
        )
        assertFalse(coarse.allowsClaim)

        // Offline resume: UNKNOWN (no fix) still permits manual/browse/offline;
        // the manual verdict itself never authorizes a proximity claim.
        val offline = PortableLocationRecovery.recover(
            risk(PortableLocation.UNKNOWN, PortableLocation.REASON_NO_FIX, false),
        )
        assertTrue(offline.preservesFreeUse)
        val manual = PortableLocationRecovery.recover(
            risk(PortableLocation.MANUAL, PortableLocation.REASON_MANUAL_ENTRY, false),
        )
        assertFalse(manual.allowsClaim)
        assertTrue(manual.preservesFreeUse)
    }

    @Test
    fun recoveryIsPureWithNoSourceTouch() {
        var reads = 0
        val source = object : LocationSignalSource {
            override fun read(): LocationSignal? {
                reads++
                return null
            }
        }
        val flow = LocationFlow(
            LocationPorts(source, object : LocationEnvironment {
                override val allowTestInjection: Boolean = false
            }),
        )
        val risk = flow.assess(manual = true)
        val recovery = PortableLocationRecovery.recover(risk)
        assertEquals(PortableLocation.MANUAL, recovery.verdict)
        assertEquals(0, reads, "recovery mapping reads no source; manual already skips it")
    }
}
