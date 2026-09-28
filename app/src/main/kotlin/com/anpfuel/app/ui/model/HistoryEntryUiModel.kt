package com.anpfuel.app.ui.model

import com.anpfuel.domain.valueobject.SurveyWeek
import java.math.BigDecimal

data class HistoryEntryUiModel(
    val surveyWeek: SurveyWeek,
    val averageFormatted: String?,
    val averageValue: BigDecimal?,
)
