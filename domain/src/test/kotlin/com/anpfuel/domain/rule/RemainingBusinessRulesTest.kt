package com.anpfuel.domain.rule

import com.anpfuel.domain.model.UserPreferences
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class RemainingBusinessRulesTest {

    @Test
    fun br010ZeroRowsIsEmptyStateNotError() {
        assertTrue(EmptyMunicipalityResultRule.shouldReturnEmptyList(0))
        assertFalse(EmptyMunicipalityResultRule.isError(0))
    }

    @Test
    fun br013RetentionWeeksUsesPreferenceWithMinimumOfOne() {
        val preferences = UserPreferences(stationDetailRetentionWeeks = 0)

        assertEquals(1, StationDetailRetentionRule.retentionWeeks(preferences))
        assertEquals(12, StationDetailRetentionRule.retentionWeeks(UserPreferences()))
    }

    @Test
    fun br014AutoSyncOnWifiRequiresUnmeteredNetwork() {
        val wifiOnly = UserPreferences(autoSyncOnWifi = true)
        val anyNetwork = UserPreferences(autoSyncOnWifi = false)

        assertTrue(AutoSyncOnWifiRule.requiresUnmeteredNetwork(wifiOnly))
        assertFalse(AutoSyncOnWifiRule.requiresUnmeteredNetwork(anyNetwork))
    }

    @Test
    fun br008RequiresOnDemandDownloadWhenNotOptedInAndNoLocalData() {
        assertTrue(
            StationDetailOptInRule.requiresOnDemandDownload(
                syncStationDetail = false,
                hasLocalStationData = false,
            ),
        )
        assertFalse(
            StationDetailOptInRule.requiresOnDemandDownload(
                syncStationDetail = true,
                hasLocalStationData = false,
            ),
        )
        assertFalse(
            StationDetailOptInRule.requiresOnDemandDownload(
                syncStationDetail = false,
                hasLocalStationData = true,
            ),
        )
    }
}