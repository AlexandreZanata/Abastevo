package com.anpfuel.domain.rule

import com.anpfuel.domain.model.AveragePrice

/**
 * UC-006 — Price history requires at least two imported survey weeks for the same scope.
 */
object PriceHistoryAvailabilityRule {

    const val MIN_WEEKS = 2

    fun hasSufficientHistory(prices: List<AveragePrice>): Boolean =
        distinctSurveyWeekCount(prices) >= MIN_WEEKS

    fun distinctSurveyWeekCount(prices: List<AveragePrice>): Int =
        prices
            .asSequence()
            .map { it.surveyWeek }
            .distinctBy { week -> week.startDate to week.endDate }
            .count()
}
