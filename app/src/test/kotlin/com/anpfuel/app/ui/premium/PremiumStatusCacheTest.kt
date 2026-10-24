package com.anpfuel.app.ui.premium

import com.anpfuel.domain.premium.PremiumState
import com.anpfuel.domain.premium.PremiumStatus
import com.anpfuel.domain.premium.PremiumStatusSnapshot
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class PremiumStatusCacheTest {

    private val active = PremiumStatus(
        policyVersion = 1,
        state = PremiumState.ACTIVE,
        reason = "verified",
        capabilities = setOf("cloud.sync"),
        validUntil = "2026-11-30T23:59:59Z",
        retainedDataAccess = true,
        paymentsConfigured = false,
        managementUrl = PremiumStatus.MANAGEMENT_URL,
    )

    @Test
    fun `keeps the verified status for the same owner inside the ttl`() {
        val cache = PremiumStatusCache()
        cache.put(PremiumStatusSnapshot("acct", active), nowMillis = 1_000L)

        assertEquals(active, cache.get("acct", nowMillis = 1_000L + PremiumStatusCache.TTL_MILLIS))
    }

    @Test
    fun `expires the status after the ttl`() {
        val cache = PremiumStatusCache()
        cache.put(PremiumStatusSnapshot("acct", active), nowMillis = 1_000L)

        assertNull(cache.get("acct", nowMillis = 1_001L + PremiumStatusCache.TTL_MILLIS))
    }

    @Test
    fun `never reuses another account status`() {
        val cache = PremiumStatusCache()
        cache.put(PremiumStatusSnapshot("acct", active), nowMillis = 1_000L)

        assertNull(cache.get("other", nowMillis = 1_500L))
    }

    @Test
    fun `clear drops the cached status`() {
        val cache = PremiumStatusCache()
        cache.put(PremiumStatusSnapshot("acct", active), nowMillis = 1_000L)
        cache.clear()

        assertNull(cache.get("acct", nowMillis = 1_100L))
    }
}
