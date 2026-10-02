package com.anpfuel.data.local.preferences

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.SurveyWeek
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map

private val Context.priceDropAlertHistoryDataStore: DataStore<Preferences> by preferencesDataStore(
    name = "price_drop_alert_history",
)

/**
 * P23-T01 — thin device-local store of shown price-drop alerts.
 *
 * One ISO week pair per vehicle key; mapping lives in the tested
 * [PriceDropAlertHistoryCodec], this wrapper only reads/writes.
 */
@Singleton
class PriceDropAlertHistoryDataStore @Inject constructor(
    @ApplicationContext context: Context,
) {
    private val dataStore = context.priceDropAlertHistoryDataStore

    suspend fun lastNotifiedWeek(vehicleId: DomainId): SurveyWeek? {
        val (startKey, endKey) = weekKeys(vehicleId)
        return dataStore.data.map { stored ->
            PriceDropAlertHistoryCodec.decode(stored[startKey], stored[endKey])
        }.first()
    }

    suspend fun recordNotified(vehicleId: DomainId, week: SurveyWeek) {
        val (start, end) = PriceDropAlertHistoryCodec.encode(week)
        val (startKey, endKey) = weekKeys(vehicleId)
        dataStore.edit { stored ->
            stored[startKey] = start
            stored[endKey] = end
        }
    }

    private fun weekKeys(vehicleId: DomainId): Pair<Preferences.Key<String>, Preferences.Key<String>> {
        val base = PriceDropAlertHistoryCodec.key(vehicleId)
        return stringPreferencesKey(base + "_start") to stringPreferencesKey(base + "_end")
    }
}
