package com.anpfuel.domain.premium

import java.time.Instant
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

/**
 * MEX-08 admission contract: the 23 `vehicle-registration-v2.json`
 * vectors execute case-for-case (IDs retained). Free-first is open;
 * extra local creation needs a valid `vehicles.multiple` lease or fresh
 * server proof; hosted creation never accepts a local lease; corrections
 * and replays of held IDs are always free; over-ceiling legacy data is
 * retained.
 */
class VehicleAdmissionTest {

    private data class Vector(
        val id: String,
        val scope: AdmissionScope,
        val authority: AdmissionAuthority,
        val before: Int,
        val newIds: Int,
        val outcome: AdmissionOutcome,
    )

    private val vectors = listOf(
        Vector("free-first", AdmissionScope.LOCAL, AdmissionAuthority.NONE, 0, 1, AdmissionOutcome.ACCEPT),
        Vector("free-second", AdmissionScope.LOCAL, AdmissionAuthority.NONE, 1, 1, AdmissionOutcome.PREMIUM_REQUIRED),
        Vector("legacy-two-correct", AdmissionScope.LOCAL, AdmissionAuthority.NONE, 2, 0, AdmissionOutcome.ACCEPT),
        Vector("legacy-three-read", AdmissionScope.LOCAL, AdmissionAuthority.NONE, 3, 0, AdmissionOutcome.ACCEPT),
        Vector("expired-three-new", AdmissionScope.LOCAL, AdmissionAuthority.EXPIRED, 3, 1, AdmissionOutcome.CAPACITY_EXCEEDED),
        Vector("expired-two-correct", AdmissionScope.LOCAL, AdmissionAuthority.EXPIRED, 2, 0, AdmissionOutcome.ACCEPT),
        Vector("expired-first-new-second", AdmissionScope.LOCAL, AdmissionAuthority.EXPIRED, 1, 1, AdmissionOutcome.PREMIUM_REQUIRED),
        Vector("premium-third", AdmissionScope.LOCAL, AdmissionAuthority.SERVER_VERIFIED, 2, 1, AdmissionOutcome.ACCEPT),
        Vector("premium-fourth", AdmissionScope.LOCAL, AdmissionAuthority.SERVER_VERIFIED, 3, 1, AdmissionOutcome.CAPACITY_EXCEEDED),
        Vector("concurrent-free-first", AdmissionScope.LOCAL, AdmissionAuthority.NONE, 0, 1, AdmissionOutcome.ACCEPT),
        Vector("concurrent-free-second", AdmissionScope.LOCAL, AdmissionAuthority.NONE, 1, 1, AdmissionOutcome.PREMIUM_REQUIRED),
        Vector("import-two-atomic-refusal", AdmissionScope.LOCAL, AdmissionAuthority.NONE, 0, 2, AdmissionOutcome.PREMIUM_REQUIRED),
        Vector("restore-same-ids", AdmissionScope.LOCAL, AdmissionAuthority.EXPIRED, 3, 0, AdmissionOutcome.ACCEPT),
        Vector("replay-same-id", AdmissionScope.LOCAL, AdmissionAuthority.NONE, 1, 0, AdmissionOutcome.ACCEPT),
        Vector("offline-valid", AdmissionScope.LOCAL, AdmissionAuthority.LOCAL_VALID_LEASE, 1, 1, AdmissionOutcome.ACCEPT),
        Vector("offline-expired", AdmissionScope.LOCAL, AdmissionAuthority.EXPIRED, 1, 1, AdmissionOutcome.PREMIUM_REQUIRED),
        Vector("clock-rollback", AdmissionScope.LOCAL, AdmissionAuthority.CLOCK_ROLLBACK, 1, 1, AdmissionOutcome.PREMIUM_REQUIRED),
        Vector("offline-hosted-refused", AdmissionScope.HOSTED, AdmissionAuthority.LOCAL_VALID_LEASE, 1, 1, AdmissionOutcome.HOSTED_AUTHORITY_REQUIRED),
        Vector("free-hosted-first", AdmissionScope.HOSTED, AdmissionAuthority.NONE, 0, 1, AdmissionOutcome.HOSTED_AUTHORITY_REQUIRED),
        Vector("premium-hosted-first", AdmissionScope.HOSTED, AdmissionAuthority.SERVER_VERIFIED, 0, 1, AdmissionOutcome.ACCEPT),
        Vector("stale-proof-second", AdmissionScope.LOCAL, AdmissionAuthority.PROOF_STALE, 1, 1, AdmissionOutcome.PREMIUM_REQUIRED),
        Vector("unknown-policy-second", AdmissionScope.LOCAL, AdmissionAuthority.UNKNOWN_POLICY, 1, 1, AdmissionOutcome.PREMIUM_REQUIRED),
        Vector("legacy-over-ceiling-correct", AdmissionScope.LOCAL, AdmissionAuthority.NONE, 4, 0, AdmissionOutcome.ACCEPT),
    )

    @Test
    fun `all 23 registration vectors decide exactly`() {
        assertEquals(23, vectors.size)
        for (v in vectors) {
            assertEquals(
                v.outcome,
                VehicleAdmissionRule.decide(v.scope, v.authority, v.before, v.newIds),
                "vector ${v.id}",
            )
        }
    }

    @Test
    fun `lease resolution refuses forgery expiry rollback and strangers`() {
        val now = Instant.parse("2026-10-22T12:00:00Z")
        fun lease(
            version: Int = 2,
            account: String = "acct-1",
            origin: String = "android",
            verified: Instant = now.minusSeconds(60),
            paidThrough: Instant = now.plusSeconds(3600),
            revoked: Boolean = false,
        ) = VehicleCapabilityLease(version, account, origin, "vehicles.multiple", verified, paidThrough, revoked)
        assertEquals(AdmissionAuthority.LOCAL_VALID_LEASE, VehicleAdmissionRule.resolveLocalAuthority(lease(), "acct-1", "android", now))
        assertEquals(AdmissionAuthority.NONE, VehicleAdmissionRule.resolveLocalAuthority(null, "acct-1", "android", now))
        assertEquals(AdmissionAuthority.UNKNOWN_POLICY, VehicleAdmissionRule.resolveLocalAuthority(lease(version = 1), "acct-1", "android", now))
        assertEquals(AdmissionAuthority.UNKNOWN_POLICY, VehicleAdmissionRule.resolveLocalAuthority(lease(version = 3), "acct-1", "android", now))
        assertEquals(AdmissionAuthority.NONE, VehicleAdmissionRule.resolveLocalAuthority(lease(account = "acct-2"), "acct-1", "android", now))
        assertEquals(AdmissionAuthority.NONE, VehicleAdmissionRule.resolveLocalAuthority(lease(origin = "other"), "acct-1", "android", now))
        assertEquals(AdmissionAuthority.EXPIRED, VehicleAdmissionRule.resolveLocalAuthority(lease(revoked = true), "acct-1", "android", now))
        assertEquals(AdmissionAuthority.CLOCK_ROLLBACK, VehicleAdmissionRule.resolveLocalAuthority(lease(verified = now.plusSeconds(60)), "acct-1", "android", now))
        assertEquals(AdmissionAuthority.EXPIRED, VehicleAdmissionRule.resolveLocalAuthority(lease(verified = now.minusSeconds(90000)), "acct-1", "android", now))
        assertEquals(AdmissionAuthority.EXPIRED, VehicleAdmissionRule.resolveLocalAuthority(lease(paidThrough = now.minusSeconds(1)), "acct-1", "android", now))
        assertEquals(AdmissionAuthority.UNKNOWN_POLICY, VehicleAdmissionRule.resolveLocalAuthority(lease(paidThrough = now.minusSeconds(7200), verified = now.minusSeconds(3600)), "acct-1", "android", now))
    }
}
