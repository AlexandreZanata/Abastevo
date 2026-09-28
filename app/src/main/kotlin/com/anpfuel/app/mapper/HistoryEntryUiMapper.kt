package com.anpfuel.app.mapper

import com.anpfuel.app.ui.model.HistoryEntryUiModel
import com.anpfuel.domain.model.AveragePrice
import java.util.Locale

object HistoryEntryUiMapper {

    fun toUiModels(
        entries: List<AveragePrice>,
        locale: Locale,
    ): List<HistoryEntryUiModel> =
        entries.map { entry -> toUiModel(entry, locale) }

    fun toUiModel(entry: AveragePrice, locale: Locale): HistoryEntryUiModel =
        HistoryEntryUiModel(
            surveyWeek = entry.surveyWeek,
            averageFormatted = PriceFormatter.formatAmount(entry.average, locale),
            averageValue = entry.average?.value,
        )
}
