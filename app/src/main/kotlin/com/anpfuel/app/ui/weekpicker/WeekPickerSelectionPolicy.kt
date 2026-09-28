package com.anpfuel.app.ui.weekpicker

import com.anpfuel.domain.model.SurveyWeekCatalogEntry

internal object WeekPickerSelectionPolicy {

    fun isLatestCatalogEntry(
        catalog: List<SurveyWeekCatalogEntry>,
        entry: SurveyWeekCatalogEntry,
    ): Boolean = catalog.firstOrNull()?.surveyWeek == entry.surveyWeek

    fun requiresConfirmation(
        catalog: List<SurveyWeekCatalogEntry>,
        entry: SurveyWeekCatalogEntry,
    ): Boolean = !isLatestCatalogEntry(catalog, entry)
}
