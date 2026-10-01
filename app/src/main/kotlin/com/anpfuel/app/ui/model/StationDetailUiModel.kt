package com.anpfuel.app.ui.model

/**
 * P20-T03 — Station detail presentation.
 *
 * Community stays UNKNOWN until P04, so the supported detail is an
 * explicitly labelled dated ANP reference plus an honest no-coverage
 * community slot. [isStale] marks an old survey/cache; [dateUnknown]
 * marks a missing collection date (never shown as fresh).
 */
data class StationDetailUiModel(
    val station: StationPriceUiModel,
    val surveyWeekLabel: String?,
    val isStale: Boolean,
    val dateUnknown: Boolean,
    val communityDisputed: Boolean,
)
