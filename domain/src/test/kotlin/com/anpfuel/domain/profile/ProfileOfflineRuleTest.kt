package com.anpfuel.domain.profile

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

/**
 * P32-T04 — Offline and restart recovery RED→GREEN.
 *
 * Locks: offline pending with a valid grant requeues after process
 * restart; expired declarations drop with notice instead of submitting;
 * revoked grants block without replay; UIs without cached authority never
 * imply privilege after restart.
 */
class ProfileOfflineRuleTest {

    @Test
    fun `valid pending requeues after restart`() {
        assertEquals(
            OfflineRecovery.Requeue,
            ProfileOfflineRule.recover(
                hasPending = true,
                grantRevoked = false,
                declarationExpired = false,
            ),
        )
    }

    @Test
    fun `expired pending drops with notice`() {
        assertEquals(
            OfflineRecovery.DropExpired,
            ProfileOfflineRule.recover(
                hasPending = true,
                grantRevoked = false,
                declarationExpired = true,
            ),
        )
    }

    @Test
    fun `revoked grant blocks without replay`() {
        assertEquals(
            OfflineRecovery.BlockedRevoked,
            ProfileOfflineRule.recover(
                hasPending = true,
                grantRevoked = true,
                declarationExpired = false,
            ),
        )
    }

    @Test
    fun `nothing pending stays idle`() {
        assertEquals(
            OfflineRecovery.Idle,
            ProfileOfflineRule.recover(
                hasPending = false,
                grantRevoked = false,
                declarationExpired = false,
            ),
        )
    }
}
