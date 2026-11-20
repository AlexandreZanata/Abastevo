package com.anpfuel.domain.premium

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

class PremiumStatusTest {
    private val status = PremiumStatus(1, PremiumState.ACTIVE, "verified", setOf("history.export"), null,
        retainedDataAccess = true, paymentsConfigured = true, managementUrl = PremiumStatus.MANAGEMENT_URL)

    @Test
    fun `paid capability requires an eligible server state and preserves free exits`() {
        PremiumState.entries.forEach { state ->
            assertEquals(state, PremiumState.parse(state.name))
            val candidate = status.copy(state = state)
            assertEquals(state in setOf(PremiumState.ACTIVE, PremiumState.CANCELED, PremiumState.GRACE), candidate.canUsePaidCapabilities)
            assertTrue(candidate.retainedDataAccess)
        }
        assertEquals(PremiumState.UNKNOWN, PremiumState.parse("active"))
        assertTrue(status.hasCapability("history.export"))
        assertFalse(status.hasCapability("other"))
        assertFalse(status.copy(capabilities = emptySet()).canUsePaidCapabilities)
    }

    @Test
    fun `status accepts only reviewed policy and bounded capability names`() {
        assertThrows<IllegalArgumentException> { status.copy(policyVersion = 2) }
        assertThrows<IllegalArgumentException> { status.copy(reason = "x".repeat(65)) }
        assertThrows<IllegalArgumentException> { status.copy(capabilities = (0..32).map(Int::toString).toSet()) }
        assertThrows<IllegalArgumentException> { status.copy(capabilities = setOf("")) }
        assertThrows<IllegalArgumentException> { status.copy(capabilities = setOf("x".repeat(65))) }
        assertThrows<IllegalArgumentException> { status.copy(retainedDataAccess = false) }
    }
}
