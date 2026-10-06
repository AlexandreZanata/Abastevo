package com.anpfuel.domain.profile

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P32-T01 — Public profile badge RED→GREEN.
 *
 * Locks: blank/unknown server state never invents privilege; pending review
 * never presents as verified; stale/revoked verification never implies
 * active representation; badge labels never conflate fuel/price quality;
 * only [PUBLIC_BUSINESS_KEYS] survive the public projection.
 */
class StationProfileBadgeRuleTest {

    @Test
    fun `blank station id resolves to unclaimed without privilege`() {
        val state = ProfileBadgeRule.resolve(
            input = ProfileBadgeInput(
                stationId = "   ",
                serverState = "verified",
                operatorSource = "registry",
                verified = true,
                staleOrRevoked = false,
            ),
        )
        assertEquals(ProfileBadge.Unclaimed, state)
    }

    @Test
    fun `unknown server state resolves to unclaimed`() {
        val state = ProfileBadgeRule.resolve(
            input = ProfileBadgeInput(
                stationId = "9f6d8a2e-3b4c-4d5e-8f90-1234567890ab",
                serverState = "mystery-future-state",
                operatorSource = "registry",
                verified = true,
                staleOrRevoked = false,
            ),
        )
        assertEquals(ProfileBadge.Unclaimed, state)
    }

    @Test
    fun `pending review never presents as verified`() {
        val state = ProfileBadgeRule.resolve(
            input = ProfileBadgeInput(
                stationId = "9f6d8a2e-3b4c-4d5e-8f90-1234567890ab",
                serverState = "pending",
                operatorSource = "review",
                verified = false,
                staleOrRevoked = false,
            ),
        )
        assertEquals(ProfileBadge.PendingReview, state)
    }

    @Test
    fun `fresh verified registry source yields verified badge`() {
        val state = ProfileBadgeRule.resolve(
            input = ProfileBadgeInput(
                stationId = "9f6d8a2e-3b4c-4d5e-8f90-1234567890ab",
                serverState = "verified",
                operatorSource = "registry",
                verified = true,
                staleOrRevoked = false,
            ),
        )
        assertEquals(ProfileBadge.Verified(source = "registry"), state)
    }

    @Test
    fun `stale or revoked verification cannot imply privilege`() {
        val stale = ProfileBadgeRule.resolve(
            input = ProfileBadgeInput(
                stationId = "9f6d8a2e-3b4c-4d5e-8f90-1234567890ab",
                serverState = "verified",
                operatorSource = "registry",
                verified = true,
                staleOrRevoked = true,
            ),
        )
        assertEquals(ProfileBadge.StaleUnverified, stale)
    }

    @Test
    fun `unverified permitted source stays unclaimed`() {
        val state = ProfileBadgeRule.resolve(
            input = ProfileBadgeInput(
                stationId = "9f6d8a2e-3b4c-4d5e-8f90-1234567890ab",
                serverState = "verified",
                operatorSource = "suggestion",
                verified = false,
                staleOrRevoked = false,
            ),
        )
        assertEquals(ProfileBadge.Unclaimed, state)
    }

    @Test
    fun `public projection drops private keys`() {
        val projected = StationProfile.projectBusiness(
            mapOf(
                "opening_hours" to "Seg–Sáb 06:00–22:00",
                "phone" to "+55 41 3333-3333",
                "cpf" to "123.456.789-00",
                "password" to "hunter2",
                "precise_gps" to "-25.43,-49.27",
            ),
        )
        assertEquals(
            mapOf(
                "opening_hours" to "Seg–Sáb 06:00–22:00",
                "phone" to "+55 41 3333-3333",
            ),
            projected,
        )
        assertTrue(PUBLIC_BUSINESS_KEYS.containsAll(listOf("opening_hours", "services", "phone", "website", "description")))
    }
}
