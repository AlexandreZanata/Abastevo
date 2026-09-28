package com.anpfuel.domain.model

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.valueobject.SurveyWeek
import java.time.LocalDate

/**
 * Metadata for one ANP listing week block (summary + station URLs).
 * Populated by listing scraper enrichment (Phase 12.2).
 */
data class SurveyWeekCatalogEntry(
    val surveyWeek: SurveyWeek,
    val summaryUrl: String,
    val stationUrl: String,
    val publishedAt: LocalDate? = null,
    val operationalNote: String? = null,
) {
    init {
        require(summaryUrl.isNotBlank()) { "summaryUrl must not be blank" }
        require(stationUrl.isNotBlank()) { "stationUrl must not be blank" }
        operationalNote?.let { note ->
            if (note.isBlank()) {
                throw DomainException("operationalNote must not be blank when provided")
            }
        }
    }

    companion object {
        fun create(
            surveyWeek: SurveyWeek,
            summaryUrl: String,
            stationUrl: String,
            publishedAt: LocalDate? = null,
            operationalNote: String? = null,
        ): SurveyWeekCatalogEntry = SurveyWeekCatalogEntry(
            surveyWeek = surveyWeek,
            summaryUrl = summaryUrl,
            stationUrl = stationUrl,
            publishedAt = publishedAt,
            operationalNote = operationalNote,
        )
    }
}
