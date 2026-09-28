package com.anpfuel.app.ui.weekpicker

import com.anpfuel.domain.model.SurveyWeekCatalogEntry

data class SurveyWeekCatalogSection(
    val year: Int,
    val entries: List<SurveyWeekCatalogEntry>,
)

/**
 * Groups ANP catalog entries by survey year (gov.br `arquivos-lpc/{YEAR}/` parity).
 * Preserves newest-first order within each year section.
 */
internal fun groupSurveyWeekCatalogByYear(
    catalog: List<SurveyWeekCatalogEntry>,
): List<SurveyWeekCatalogSection> =
    catalog
        .groupBy { it.surveyWeek.endDate.year }
        .toSortedMap(compareByDescending { it })
        .map { (year, entries) ->
            SurveyWeekCatalogSection(year = year, entries = entries)
        }
