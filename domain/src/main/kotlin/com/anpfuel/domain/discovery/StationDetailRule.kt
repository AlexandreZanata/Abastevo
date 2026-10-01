package com.anpfuel.domain.discovery

import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.rule.SurveyWeekFreshnessRule
import com.anpfuel.domain.valueobject.SurveyWeek
import java.time.LocalDate

/**
 * P20-T03 — Pure station-detail state resolver.
 *
 * The detail prioritizes community cards, but community stays UNKNOWN
 * until P04, so a supported detail today is an explicitly labelled dated
 * ANP reference plus an honest no-coverage community slot. Missing
 * coverage is [StationDetailState.NoCoverage], never an error or an
 * invented price. A missing collection date is [StationRowFreshness.UNKNOWN],
 * never presented as fresh. Only the backend can report a dispute, carried
 * by [communityDisputed] without inventing one locally.
 */
enum class StationRowFreshness {
    FRESH,
    STALE,
    UNKNOWN,
}

sealed interface StationDetailState {
    data object NoCoverage : StationDetailState

    data class AnpReference(
        val rows: List<StationDetailRow>,
        val weekStale: Boolean,
        val communityDisputed: Boolean,
    ) : StationDetailState
}

data class StationDetailRow(
    val price: StationPrice,
    val freshness: StationRowFreshness,
)

object StationDetailRule {

    fun resolve(
        rows: List<StationPrice>,
        surveyWeek: SurveyWeek,
        today: LocalDate,
        staleCache: Boolean,
        communityDisputed: Boolean = false,
    ): StationDetailState {
        if (rows.isEmpty()) return StationDetailState.NoCoverage
        val weekStale = staleCache ||
            SurveyWeekFreshnessRule.isStale(surveyWeek.endDate, today)
        return StationDetailState.AnpReference(
            rows = rows.map { row ->
                StationDetailRow(
                    price = row,
                    freshness = rowFreshness(row.collectedAt, weekStale),
                )
            },
            weekStale = weekStale,
            communityDisputed = communityDisputed,
        )
    }

    private fun rowFreshness(collectedAt: LocalDate?, weekStale: Boolean): StationRowFreshness =
        when {
            collectedAt == null -> StationRowFreshness.UNKNOWN
            weekStale -> StationRowFreshness.STALE
            else -> StationRowFreshness.FRESH
        }
}
