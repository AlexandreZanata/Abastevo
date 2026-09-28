package com.anpfuel.domain.rule

import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.state.DataReadinessState
import com.anpfuel.domain.state.SyncJobState
import com.anpfuel.domain.valueobject.SurveyWeek
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test
import java.time.Instant
import java.time.LocalDate

class DataReadinessRuleTest {

    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val today = LocalDate.parse("2026-06-15")

    @Test
    fun givenNoImportedWeeks_whenResolving_thenEmpty() {
        assertEquals(
            DataReadinessState.EMPTY,
            DataReadinessRule.resolve(
                importedWeekCount = 0,
                syncJobState = SyncJobState.IDLE,
                latestSurvey = null,
                today = today,
            ),
        )
    }

    @Test
    fun givenActiveSync_whenResolving_thenSyncing() {
        assertEquals(
            DataReadinessState.SYNCING,
            DataReadinessRule.resolve(
                importedWeekCount = 1,
                syncJobState = SyncJobState.DOWNLOADING,
                latestSurvey = fullSurvey(),
                today = today,
            ),
        )
    }

    @Test
    fun givenFailedSyncWithCache_whenResolving_thenError() {
        assertEquals(
            DataReadinessState.ERROR,
            DataReadinessRule.resolve(
                importedWeekCount = 1,
                syncJobState = SyncJobState.FAILED,
                latestSurvey = fullSurvey(),
                today = today,
            ),
        )
    }

    @Test
    fun givenSummaryOnly_whenResolving_thenPartial() {
        val summaryOnly = PriceSurvey.restore(
            id = com.anpfuel.domain.valueobject.DomainId.forSurveyWeek(surveyWeek),
            surveyWeek = surveyWeek,
            summaryImportedAt = Instant.parse("2026-06-14T12:00:00Z"),
            stationImportedAt = null,
        )

        assertEquals(
            DataReadinessState.PARTIAL,
            DataReadinessRule.resolve(
                importedWeekCount = 1,
                syncJobState = SyncJobState.IDLE,
                latestSurvey = summaryOnly,
                today = today,
            ),
        )
    }

    @Test
    fun givenOldSurveyWeek_whenResolving_thenStale() {
        val oldWeek = SurveyWeek.fromIsoDates("2026-05-31", "2026-06-06")

        assertEquals(
            DataReadinessState.STALE,
            DataReadinessRule.resolve(
                importedWeekCount = 1,
                syncJobState = SyncJobState.IDLE,
                latestSurvey = fullSurvey(oldWeek),
                today = today,
            ),
        )
    }

    @Test
    fun givenFreshFullData_whenResolving_thenReady() {
        assertEquals(
            DataReadinessState.READY,
            DataReadinessRule.resolve(
                importedWeekCount = 1,
                syncJobState = SyncJobState.IDLE,
                latestSurvey = fullSurvey(),
                today = today,
            ),
        )
    }

    private fun fullSurvey(week: SurveyWeek = surveyWeek): PriceSurvey = PriceSurvey.restore(
        id = com.anpfuel.domain.valueobject.DomainId.forSurveyWeek(week),
        surveyWeek = week,
        summaryImportedAt = Instant.parse("2026-06-14T12:00:00Z"),
        stationImportedAt = Instant.parse("2026-06-14T12:05:00Z"),
    )
}
