package com.anpfuel.domain.rule

import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.time.LocalDate

class SurveyWeekFreshnessRuleTest {

    @Test
    fun givenWeekEnded7DaysAgo_whenCheckingFreshness_thenNotStale() {
        val endDate = LocalDate.parse("2026-06-10")
        val today = endDate.plusDays(7)

        assertFalse(SurveyWeekFreshnessRule.isStale(endDate, today))
    }

    @Test
    fun givenWeekEnded8DaysAgo_whenCheckingFreshness_thenStale() {
        val endDate = LocalDate.parse("2026-06-10")
        val today = endDate.plusDays(8)

        assertTrue(SurveyWeekFreshnessRule.isStale(endDate, today))
    }
}
