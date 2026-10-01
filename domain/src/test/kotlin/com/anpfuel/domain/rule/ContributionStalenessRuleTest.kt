package com.anpfuel.domain.rule

import com.anpfuel.domain.portable.PortablePhoto
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ContributionStalenessRuleTest {

    @Test
    fun `fresh capture stays fresh`() {
        val now = 1_000_000_000L
        assertFalse(ContributionStalenessRule.isHistorical(now - 3_600_000L, now))
        assertEquals("fresh", ContributionStalenessRule.labelFor(now - 3_600_000L, now))
    }

    @Test
    fun `capture older than 24h is historical and never fresh`() {
        val now = 1_000_000_000L
        val old = now - PortablePhoto.TRANSIENT_TTL_MILLIS - 1L
        assertTrue(ContributionStalenessRule.isHistorical(old, now))
        assertEquals("historical", ContributionStalenessRule.labelFor(old, now))
    }

    @Test
    fun `future capture beyond skew is historical`() {
        val now = 1_000_000_000L
        val future = now + ContributionStalenessRule.FUTURE_SKEW_MILLIS + 1L
        assertTrue(ContributionStalenessRule.isHistorical(future, now))
    }
}
