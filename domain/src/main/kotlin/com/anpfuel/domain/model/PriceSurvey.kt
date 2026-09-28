package com.anpfuel.domain.model

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.PriceTableType
import com.anpfuel.domain.valueobject.SurveyWeek
import java.time.Instant

/**
 * Aggregate root for a weekly ANP fuel price collection.
 */
class PriceSurvey private constructor(
    val id: DomainId,
    val surveyWeek: SurveyWeek,
    summaryImportedAt: Instant?,
    stationImportedAt: Instant?,
) {
    var summaryImportedAt: Instant? = summaryImportedAt
        private set

    var stationImportedAt: Instant? = stationImportedAt
        private set

    val hasSummaryData: Boolean
        get() = summaryImportedAt != null

    val hasStationData: Boolean
        get() = stationImportedAt != null

    fun markSummaryImported(importedAt: Instant) {
        if (importedAt.isBefore(summaryImportedAt ?: Instant.EPOCH)) {
            throw DomainException("Summary import timestamp must not move backwards")
        }
        summaryImportedAt = importedAt
    }

    fun markStationImported(importedAt: Instant) {
        if (summaryImportedAt == null) {
            throw DomainException("Summary data must be imported before station detail")
        }
        if (importedAt.isBefore(stationImportedAt ?: Instant.EPOCH)) {
            throw DomainException("Station import timestamp must not move backwards")
        }
        stationImportedAt = importedAt
    }

    fun isReadyFor(tableType: PriceTableType): Boolean = when (tableType) {
        PriceTableType.WEEKLY_SUMMARY -> hasSummaryData
        PriceTableType.STATION_DETAIL -> hasStationData
    }

    companion object {
        fun create(
            surveyWeek: SurveyWeek,
            id: DomainId = DomainId.forSurveyWeek(surveyWeek),
        ): PriceSurvey = PriceSurvey(
            id = id,
            surveyWeek = surveyWeek,
            summaryImportedAt = null,
            stationImportedAt = null,
        )

        fun restore(
            id: DomainId,
            surveyWeek: SurveyWeek,
            summaryImportedAt: Instant?,
            stationImportedAt: Instant?,
        ): PriceSurvey = PriceSurvey(
            id = id,
            surveyWeek = surveyWeek,
            summaryImportedAt = summaryImportedAt,
            stationImportedAt = stationImportedAt,
        )
    }
}
