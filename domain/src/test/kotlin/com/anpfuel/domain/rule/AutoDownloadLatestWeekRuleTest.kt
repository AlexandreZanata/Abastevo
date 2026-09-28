package com.anpfuel.domain.rule

import com.anpfuel.domain.valueobject.SurveyWeek
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class AutoDownloadLatestWeekRuleTest {

    private val latestWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")

    @Test
    fun br020SkipsManualWeekSelectionWhenAutoDownloadEnabled() {
        assertFalse(
            AutoDownloadLatestWeekRule.requiresManualWeekSelection(
                activeSurveyWeek = null,
                autoDownloadLatestWeek = true,
            ),
        )
    }

    @Test
    fun br020RequiresManualWeekSelectionWhenAutoDownloadDisabled() {
        assertTrue(
            AutoDownloadLatestWeekRule.requiresManualWeekSelection(
                activeSurveyWeek = null,
                autoDownloadLatestWeek = false,
            ),
        )
    }

    @Test
    fun br020ResolvesSyncTargetFromCatalogLatestWhenEnabledAndNoExplicitTarget() {
        assertTrue(
            AutoDownloadLatestWeekRule.shouldResolveSyncTargetFromCatalogLatest(
                autoDownloadLatestWeek = true,
                explicitTargetSurveyWeek = null,
            ),
        )
        assertFalse(
            AutoDownloadLatestWeekRule.shouldResolveSyncTargetFromCatalogLatest(
                autoDownloadLatestWeek = true,
                explicitTargetSurveyWeek = latestWeek,
            ),
        )
    }

    @Test
    fun br020SkipsWeekPickerOnColdStartWhenEnabled() {
        assertTrue(AutoDownloadLatestWeekRule.skipsWeekPickerOnColdStart(autoDownloadLatestWeek = true))
        assertFalse(AutoDownloadLatestWeekRule.skipsWeekPickerOnColdStart(autoDownloadLatestWeek = false))
    }
}
