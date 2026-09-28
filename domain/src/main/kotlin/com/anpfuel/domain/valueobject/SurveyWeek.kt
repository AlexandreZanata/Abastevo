package com.anpfuel.domain.valueobject

import com.anpfuel.domain.exception.DomainException
import java.time.LocalDate
import java.time.temporal.ChronoUnit

/**
 * The date range of a PriceSurvey (BR-001).
 */
data class SurveyWeek(
    val startDate: LocalDate,
    val endDate: LocalDate,
) {
    init {
        validate()
    }

    val inclusiveDayCount: Long
        get() = ChronoUnit.DAYS.between(startDate, endDate) + 1

    companion object {
        private const val MAX_INCLUSIVE_DAYS = 7L

        fun fromIsoDates(startDate: String, endDate: String): SurveyWeek =
            SurveyWeek(
                startDate = LocalDate.parse(startDate),
                endDate = LocalDate.parse(endDate),
            )
    }

    private fun validate() {
        if (startDate.isAfter(endDate)) {
            throw DomainException("SurveyWeek start date must be before or equal to end date")
        }
        if (inclusiveDayCount > MAX_INCLUSIVE_DAYS) {
            throw DomainException("SurveyWeek range must be at most $MAX_INCLUSIVE_DAYS days")
        }
    }
}
