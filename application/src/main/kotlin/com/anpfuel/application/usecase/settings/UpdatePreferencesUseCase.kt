package com.anpfuel.application.usecase.settings

import com.anpfuel.domain.event.DomainEvent
import com.anpfuel.domain.event.PreferencesUpdated
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.DomainEventPublisher
import com.anpfuel.domain.repository.UserPreferencesRepository

data class UpdatePreferencesResult(
    val preferences: UserPreferences,
    val event: PreferencesUpdated,
)

class UpdatePreferencesUseCase(
    private val userPreferencesRepository: UserPreferencesRepository,
    private val eventPublisher: DomainEventPublisher,
) {

    suspend operator fun invoke(preferences: UserPreferences): UpdatePreferencesResult {
        val current = userPreferencesRepository.getPreferences()
        val changedKeys = detectChangedKeys(current, preferences)

        userPreferencesRepository.savePreferences(preferences)

        val event = PreferencesUpdated.create(
            payload = PreferencesUpdated.Payload(changedKeys = changedKeys),
        )
        eventPublisher.publish(event)

        return UpdatePreferencesResult(
            preferences = preferences,
            event = event,
        )
    }

    internal fun detectChangedKeys(
        current: UserPreferences,
        updated: UserPreferences,
    ): Set<String> = buildSet {
        if (current.preferredState != updated.preferredState) add(KEY_PREFERRED_STATE)
        if (current.preferredMunicipality != updated.preferredMunicipality) add(KEY_PREFERRED_MUNICIPALITY)
        if (current.preferredFuelProduct != updated.preferredFuelProduct) add(KEY_PREFERRED_FUEL_PRODUCT)
        if (current.localeTag != updated.localeTag) add(KEY_LOCALE_TAG)
        if (current.syncStationDetail != updated.syncStationDetail) add(KEY_SYNC_STATION_DETAIL)
        if (current.autoDownloadLatestWeek != updated.autoDownloadLatestWeek) add(KEY_AUTO_DOWNLOAD_LATEST_WEEK)
        if (current.stationDetailRetentionWeeks != updated.stationDetailRetentionWeeks) {
            add(KEY_STATION_DETAIL_RETENTION_WEEKS)
        }
        if (current.nearestStationRadiusKm != updated.nearestStationRadiusKm) {
            add(KEY_NEAREST_STATION_RADIUS_KM)
        }
        if (current.autoSyncOnWifi != updated.autoSyncOnWifi) add(KEY_AUTO_SYNC_ON_WIFI)
        if (current.showPriceHistory != updated.showPriceHistory) add(KEY_SHOW_PRICE_HISTORY)
        if (current.onboardingCompleted != updated.onboardingCompleted) add(KEY_ONBOARDING_COMPLETED)
        if (current.locationPromptCompleted != updated.locationPromptCompleted) {
            add(KEY_LOCATION_PROMPT_COMPLETED)
        }
        if (current.activeSurveyWeek != updated.activeSurveyWeek) add(KEY_ACTIVE_SURVEY_WEEK)
    }

    companion object {
        const val KEY_PREFERRED_STATE = "preferredState"
        const val KEY_PREFERRED_MUNICIPALITY = "preferredMunicipality"
        const val KEY_PREFERRED_FUEL_PRODUCT = "preferredFuelProduct"
        const val KEY_LOCALE_TAG = "localeTag"
        const val KEY_SYNC_STATION_DETAIL = "syncStationDetail"
        const val KEY_AUTO_DOWNLOAD_LATEST_WEEK = "autoDownloadLatestWeek"
        const val KEY_STATION_DETAIL_RETENTION_WEEKS = "stationDetailRetentionWeeks"
        const val KEY_NEAREST_STATION_RADIUS_KM = "nearestStationRadiusKm"
        const val KEY_AUTO_SYNC_ON_WIFI = "autoSyncOnWifi"
        const val KEY_SHOW_PRICE_HISTORY = "showPriceHistory"
        const val KEY_ONBOARDING_COMPLETED = "onboardingCompleted"
        const val KEY_LOCATION_PROMPT_COMPLETED = "locationPromptCompleted"
        const val KEY_ACTIVE_SURVEY_WEEK = "activeSurveyWeek"
    }
}
