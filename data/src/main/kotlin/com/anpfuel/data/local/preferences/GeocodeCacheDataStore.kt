package com.anpfuel.data.local.preferences

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import com.anpfuel.domain.model.ReverseGeocodeResult
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import javax.inject.Inject
import javax.inject.Singleton

private val Context.geocodeCacheDataStore: DataStore<Preferences> by preferencesDataStore(
    name = "geocode_cache",
)

@Singleton
class GeocodeCacheDataStore @Inject constructor(
    @ApplicationContext context: Context,
) : GeocodeCacheStore {

    private val dataStore = context.geocodeCacheDataStore

    override suspend fun get(cacheKey: String): ReverseGeocodeResult? =
        dataStore.data.map { preferences ->
            preferences[stringPreferencesKey(cacheKey)]?.let(GeocodeCacheCodec::decode)
        }.first()

    override suspend fun put(cacheKey: String, result: ReverseGeocodeResult) {
        dataStore.edit { preferences ->
            preferences[stringPreferencesKey(cacheKey)] = GeocodeCacheCodec.encode(result)
        }
    }
}
