package com.anpfuel.data.local.preferences

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.intPreferencesKey
import androidx.datastore.preferences.core.longPreferencesKey
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import javax.inject.Inject
import javax.inject.Singleton

private val Context.userPreferencesDataStore: DataStore<Preferences> by preferencesDataStore(
    name = "user_preferences",
)

@Singleton
class UserPreferencesDataStore internal constructor(
    private val dataStore: DataStore<Preferences>,
) {
    @Inject constructor(@ApplicationContext context: Context) : this(context.userPreferencesDataStore)

    fun observe(): kotlinx.coroutines.flow.Flow<UserPreferences> = dataStore.data.map(::toDomain)

    suspend fun read(): UserPreferences = observe().first()

    suspend fun write(preferences: UserPreferences) {
        dataStore.edit { stored ->
            val current = toDomain(stored)
            val scopeChanged = current.preferredState != preferences.preferredState ||
                current.preferredMunicipality != preferences.preferredMunicipality
            val policyChanged = scopeChanged || current.syncPublicCityCatalog != preferences.syncPublicCityCatalog ||
                current.autoSyncOnWifi != preferences.autoSyncOnWifi
            stored[Keys.PUBLIC_CITY_CATALOG_GENERATION] = current.publicCityCatalogGeneration + if (policyChanged) 1 else 0
            stored[Keys.PREFERRED_STATE] = preferences.preferredState?.abbreviation.orEmpty()
            stored[Keys.PREFERRED_MUNICIPALITY] = preferences.preferredMunicipality.orEmpty()
            stored[Keys.PREFERRED_FUEL_PRODUCT] = preferences.preferredFuelProduct?.name.orEmpty()
            stored[Keys.LOCALE_TAG] = preferences.localeTag
            stored[Keys.LOCALE_USER_SELECTED] = preferences.localeUserSelected
            stored[Keys.SYNC_STATION_DETAIL] = preferences.syncStationDetail
            stored[Keys.SYNC_PUBLIC_CITY_CATALOG] = preferences.syncPublicCityCatalog
            val lastSynced = if (policyChanged) null else preferences.publicCityCatalogLastSyncedAtMillis
            if (lastSynced != null) {
                stored[Keys.PUBLIC_CITY_CATALOG_LAST_SYNCED_AT] = lastSynced
            } else {
                stored.remove(Keys.PUBLIC_CITY_CATALOG_LAST_SYNCED_AT)
            }
            stored[Keys.AUTO_DOWNLOAD_LATEST_WEEK] = preferences.autoDownloadLatestWeek
            stored[Keys.STATION_DETAIL_RETENTION_WEEKS] = preferences.stationDetailRetentionWeeks
            stored[Keys.NEAREST_STATION_RADIUS_KM] = preferences.nearestStationRadiusKm
            stored[Keys.AUTO_SYNC_ON_WIFI] = preferences.autoSyncOnWifi
            stored[Keys.SHOW_PRICE_HISTORY] = preferences.showPriceHistory
            stored[Keys.ONBOARDING_COMPLETED] = preferences.onboardingCompleted
            stored[Keys.LOCATION_PROMPT_COMPLETED] = preferences.locationPromptCompleted
            val activeWeek = ActiveSurveyWeekCodec.encode(preferences.activeSurveyWeek)
            if (activeWeek != null) {
                stored[Keys.ACTIVE_SURVEY_WEEK_START] = activeWeek.first
                stored[Keys.ACTIVE_SURVEY_WEEK_END] = activeWeek.second
            } else {
                stored.remove(Keys.ACTIVE_SURVEY_WEEK_START)
                stored.remove(Keys.ACTIVE_SURVEY_WEEK_END)
            }
        }
    }

    suspend fun markPublicCityCatalogSynced(expected: UserPreferences, atMillis: Long): Boolean {
        var accepted = false
        dataStore.edit { stored ->
            val current = toDomain(stored)
            if (current.syncPublicCityCatalog && current.publicCityCatalogGeneration == expected.publicCityCatalogGeneration &&
                current.preferredState == expected.preferredState && current.preferredMunicipality == expected.preferredMunicipality &&
                current.autoSyncOnWifi == expected.autoSyncOnWifi) {
                stored[Keys.PUBLIC_CITY_CATALOG_LAST_SYNCED_AT] = atMillis
                accepted = true
            }
        }
        return accepted
    }

    suspend fun clearPublicCityCatalogSyncMarker() {
        dataStore.edit { stored ->
            stored.remove(Keys.PUBLIC_CITY_CATALOG_LAST_SYNCED_AT)
            stored[Keys.PUBLIC_CITY_CATALOG_GENERATION] = (stored[Keys.PUBLIC_CITY_CATALOG_GENERATION] ?: 0) + 1
        }
    }

    private fun toDomain(preferences: Preferences): UserPreferences {
        val stateAbbreviation = preferences[Keys.PREFERRED_STATE].orEmpty()
        val municipality = preferences[Keys.PREFERRED_MUNICIPALITY].orEmpty()
        val fuelProductName = preferences[Keys.PREFERRED_FUEL_PRODUCT].orEmpty()

        return UserPreferences(
            preferredState = stateAbbreviation.takeIf { it.isNotBlank() }
                ?.let(BrazilianState::fromAbbreviation),
            preferredMunicipality = municipality.takeIf { it.isNotBlank() },
            preferredFuelProduct = fuelProductName.takeIf { it.isNotBlank() }
                ?.let { runCatching { FuelProduct.valueOf(it) }.getOrNull() },
            localeTag = preferences[Keys.LOCALE_TAG].orEmpty(),
            localeUserSelected = preferences[Keys.LOCALE_USER_SELECTED] ?: false,
            syncStationDetail = preferences[Keys.SYNC_STATION_DETAIL] ?: true,
            syncPublicCityCatalog = preferences[Keys.SYNC_PUBLIC_CITY_CATALOG] ?: false,
            publicCityCatalogGeneration = preferences[Keys.PUBLIC_CITY_CATALOG_GENERATION] ?: 0,
            publicCityCatalogLastSyncedAtMillis = preferences[Keys.PUBLIC_CITY_CATALOG_LAST_SYNCED_AT],
            autoDownloadLatestWeek = preferences[Keys.AUTO_DOWNLOAD_LATEST_WEEK] ?: true,
            stationDetailRetentionWeeks = preferences[Keys.STATION_DETAIL_RETENTION_WEEKS]
                ?: UserPreferences.DEFAULT_RETENTION_WEEKS,
            nearestStationRadiusKm = preferences[Keys.NEAREST_STATION_RADIUS_KM]
                ?.takeIf { it in UserPreferences.NEAREST_STATION_RADIUS_KM_OPTIONS }
                ?: UserPreferences.DEFAULT_NEAREST_STATION_RADIUS_KM,
            autoSyncOnWifi = preferences[Keys.AUTO_SYNC_ON_WIFI] ?: true,
            showPriceHistory = preferences[Keys.SHOW_PRICE_HISTORY] ?: true,
            onboardingCompleted = preferences[Keys.ONBOARDING_COMPLETED] ?: false,
            locationPromptCompleted = preferences[Keys.LOCATION_PROMPT_COMPLETED] ?: false,
            activeSurveyWeek = ActiveSurveyWeekCodec.decode(
                startDate = preferences[Keys.ACTIVE_SURVEY_WEEK_START],
                endDate = preferences[Keys.ACTIVE_SURVEY_WEEK_END],
            ),
        )
    }

    private object Keys {
        val PREFERRED_STATE = stringPreferencesKey("preferred_state")
        val PREFERRED_MUNICIPALITY = stringPreferencesKey("preferred_municipality")
        val PREFERRED_FUEL_PRODUCT = stringPreferencesKey("preferred_fuel_product")
        val LOCALE_TAG = stringPreferencesKey("locale_tag")
        val LOCALE_USER_SELECTED = booleanPreferencesKey("locale_user_selected")
        val SYNC_STATION_DETAIL = booleanPreferencesKey("sync_station_detail")
        val SYNC_PUBLIC_CITY_CATALOG = booleanPreferencesKey("sync_public_city_catalog")
        val PUBLIC_CITY_CATALOG_GENERATION = longPreferencesKey("public_city_catalog_generation")
        val PUBLIC_CITY_CATALOG_LAST_SYNCED_AT = longPreferencesKey("public_city_catalog_last_synced_at")
        val AUTO_DOWNLOAD_LATEST_WEEK = booleanPreferencesKey("auto_download_latest_week")
        val STATION_DETAIL_RETENTION_WEEKS = intPreferencesKey("station_detail_retention_weeks")
        val NEAREST_STATION_RADIUS_KM = intPreferencesKey("nearest_station_radius_km")
        val AUTO_SYNC_ON_WIFI = booleanPreferencesKey("auto_sync_on_wifi")
        val SHOW_PRICE_HISTORY = booleanPreferencesKey("show_price_history")
        val ONBOARDING_COMPLETED = booleanPreferencesKey("onboarding_completed")
        val LOCATION_PROMPT_COMPLETED = booleanPreferencesKey("location_prompt_completed")
        val ACTIVE_SURVEY_WEEK_START = stringPreferencesKey("active_survey_week_start")
        val ACTIVE_SURVEY_WEEK_END = stringPreferencesKey("active_survey_week_end")
    }
}
