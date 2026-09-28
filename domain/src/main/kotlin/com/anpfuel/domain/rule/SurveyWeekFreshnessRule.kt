package com.anpfuel.domain.rule

import java.time.LocalDate
import java.time.temporal.ChronoUnit

/**
 * Data freshness threshold — latest [SurveyWeek] end date older than 8 days marks data as stale.
 */
object SurveyWeekFreshnessRule {

    const val STALE_THRESHOLD_DAYS = 8L

    fun isStale(
        surveyWeekEndDate: LocalDate,
        today: LocalDate,
    ): Boolean = ChronoUnit.DAYS.between(surveyWeekEndDate, today) >= STALE_THRESHOLD_DAYS
}
