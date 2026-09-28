package com.anpfuel.domain.rule

import com.anpfuel.domain.model.AveragePrice

/**
 * UC-006 / BR-003 — Present immutable average price history ordered by survey week start date,
 * newest first (descending chronological order for UI lists).
 */
object PriceHistoryOrderingRule {

    fun orderBySurveyWeekStartDate(prices: List<AveragePrice>): List<AveragePrice> =
        prices.sortedByDescending { it.surveyWeek.startDate }
}
