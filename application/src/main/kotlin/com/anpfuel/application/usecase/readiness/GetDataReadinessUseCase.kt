package com.anpfuel.application.usecase.readiness

import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.SyncJobRepository
import com.anpfuel.domain.rule.DataReadinessRule
import com.anpfuel.domain.rule.DefaultSurveyWeekRule
import com.anpfuel.domain.state.DataReadinessState
import com.anpfuel.domain.valueobject.SurveyWeek
import java.time.Instant
import java.time.LocalDate

data class DataReadinessResult(
    val readiness: DataReadinessState,
    val latestSurveyWeek: SurveyWeek?,
    val lastSummarySyncAt: Instant?,
    val hasCachedData: Boolean,
)

class GetDataReadinessUseCase(
    private val priceTableRepository: PriceTableRepository,
    private val syncJobRepository: SyncJobRepository,
    private val todayProvider: () -> LocalDate = { LocalDate.now() },
) {

    suspend operator fun invoke(): DataReadinessResult {
        val importedWeekCount = priceTableRepository.countImportedSurveyWeeks()
        val syncJobState = syncJobRepository.getCurrentState()
        val importedSurveys = if (importedWeekCount > 0) {
            priceTableRepository.getImportedPriceSurveys()
        } else {
            emptyList()
        }
        val latestSurveyWeek = DefaultSurveyWeekRule.selectDefault(importedSurveys)
        val latestSurvey = latestSurveyWeek?.let { week ->
            importedSurveys.firstOrNull { it.surveyWeek == week }
        }

        val readiness = DataReadinessRule.resolve(
            importedWeekCount = importedWeekCount,
            syncJobState = syncJobState,
            latestSurvey = latestSurvey,
            today = todayProvider(),
        )

        return DataReadinessResult(
            readiness = readiness,
            latestSurveyWeek = latestSurveyWeek,
            lastSummarySyncAt = latestSurvey?.summaryImportedAt,
            hasCachedData = importedWeekCount > 0 && latestSurvey?.hasSummaryData == true,
        )
    }
}
