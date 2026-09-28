package com.anpfuel.domain.model

import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.SurveyWeek

/**
 * Local user preferences stored on device (UC-008).
 */
data class UserPreferences(
    val preferredState: BrazilianState? = null,
    val preferredMunicipality: String? = null,
    val preferredFuelProduct: FuelProduct? = null,
    val localeTag: String = "",
    val localeUserSelected: Boolean = false,
    val syncStationDetail: Boolean = true,
    val autoDownloadLatestWeek: Boolean = true,
    val stationDetailRetentionWeeks: Int = DEFAULT_RETENTION_WEEKS,
    val nearestStationRadiusKm: Int = DEFAULT_NEAREST_STATION_RADIUS_KM,
    val autoSyncOnWifi: Boolean = true,
    val showPriceHistory: Boolean = true,
    val onboardingCompleted: Boolean = false,
    val locationPromptCompleted: Boolean = false,
    val activeSurveyWeek: SurveyWeek? = null,
) {
    /** UC-015 — search radius in meters for the nearest best-price station (BR-028). */
    fun getNearestStationRadiusMeters(): Double =
        nearestStationRadiusKm.coerceIn(MIN_NEAREST_STATION_RADIUS_KM, MAX_NEAREST_STATION_RADIUS_KM) * 1000.0

    companion object {
        const val DEFAULT_RETENTION_WEEKS = 12

        /** UC-015 / BR-028 — selectable search radii for the nearest best-price station. */
        val NEAREST_STATION_RADIUS_KM_OPTIONS = listOf(3, 5, 10, 15)

        const val DEFAULT_NEAREST_STATION_RADIUS_KM = 3

        const val MIN_NEAREST_STATION_RADIUS_KM = 1

        const val MAX_NEAREST_STATION_RADIUS_KM = 100
    }
}
