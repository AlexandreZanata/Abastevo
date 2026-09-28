package com.anpfuel.app.ui.weekpicker

import com.anpfuel.application.error.AppError
import com.anpfuel.domain.model.SurveyWeekCatalogEntry
import com.anpfuel.domain.valueobject.SurveyWeekSelectionMode

data class WeekPickerUiState(
    val catalog: List<SurveyWeekCatalogEntry> = emptyList(),
    val isLoadingCatalog: Boolean = false,
    val isSyncing: Boolean = false,
    val catalogError: AppError? = null,
    val syncError: AppError? = null,
    val showSkipAction: Boolean = false,
    val showLatestCta: Boolean = true,
    val pendingWeekSelection: SurveyWeekCatalogEntry? = null,
    val pendingSelectionMode: SurveyWeekSelectionMode? = null,
    val pendingConfirmation: SurveyWeekCatalogEntry? = null,
)
