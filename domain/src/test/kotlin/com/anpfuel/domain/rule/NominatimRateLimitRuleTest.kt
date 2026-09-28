package com.anpfuel.domain.rule

import com.anpfuel.domain.exception.DomainException
import java.time.Instant
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

class NominatimRateLimitRuleTest {

    @Test
    fun allowsFirstRequestWhenNoPreviousTimestamp() {
        assertTrue(NominatimRateLimitRule.canRequest(Instant.parse("2026-06-21T12:00:00Z"), null))
    }

    @Test
    fun blocksRequestWithinOneSecond() {
        val first = Instant.parse("2026-06-21T12:00:00Z")
        val second = first.plusMillis(500)

        assertFalse(NominatimRateLimitRule.canRequest(second, first))
    }

    @Test
    fun allowsRequestAfterOneSecond() {
        val first = Instant.parse("2026-06-21T12:00:00Z")
        val second = first.plusMillis(NominatimRateLimitRule.MIN_INTERVAL_MILLIS)

        assertTrue(NominatimRateLimitRule.canRequest(second, first))
    }

    @Test
    fun requireCanRequestThrowsWhenThrottled() {
        val first = Instant.parse("2026-06-21T12:00:00Z")
        val second = first.plusMillis(250)

        assertThrows<DomainException> {
            NominatimRateLimitRule.requireCanRequest(second, first)
        }
    }
}
