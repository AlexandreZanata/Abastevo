package com.anpfuel.application.usecase.sync

import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.valueobject.SurveyWeek

/**
 * Returns the survey weeks already downloaded locally (summary data imported),
 * so the week picker can badge them without triggering any sync.
 */
class GetDownloadedSurveyWeeksUseCase(
    private val priceTableRepository: PriceTableRepository,
) {

    suspend operator fun invoke(): Set<SurveyWeek> =
        priceTableRepository.getImportedPriceSurveys()
            .filter { it.hasSummaryData }
            .map { it.surveyWeek }
            .toSet()
}
