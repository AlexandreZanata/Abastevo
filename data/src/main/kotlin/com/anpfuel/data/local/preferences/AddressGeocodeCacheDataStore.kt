package com.anpfuel.data.local.preferences

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import com.anpfuel.domain.valueobject.GeoCoordinates
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map

private val Context.addressGeocodeCacheDataStore: DataStore<Preferences> by preferencesDataStore(
    name = "address_geocode_cache",
)

@Singleton
class AddressGeocodeCacheDataStore @Inject constructor(
    @ApplicationContext context: Context,
) : AddressGeocodeCacheStore {

    private val dataStore = context.addressGeocodeCacheDataStore

    override suspend fun get(cacheKey: String): GeoCoordinates? =
        dataStore.data.map { preferences ->
            preferences[stringPreferencesKey(cacheKey)]?.let(AddressGeocodeCacheCodec::decode)
        }.first()

    override suspend fun put(cacheKey: String, coordinates: GeoCoordinates) {
        dataStore.edit { preferences ->
            preferences[stringPreferencesKey(cacheKey)] = AddressGeocodeCacheCodec.encode(coordinates)
        }
    }
}
