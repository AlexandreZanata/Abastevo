package com.anpfuel.data.local.preferences

import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.valueobject.BrazilianState
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.io.File

class PublicCatalogPreferencesTest {
    @TempDir lateinit var directory: File

    @Test fun `old completion cannot overwrite opt out or current unrelated preferences`() = runTest {
        val store = UserPreferencesDataStore(PreferenceDataStoreFactory.create(scope = backgroundScope,
            produceFile = { File(directory, "actual.preferences_pb") }))
        store.write(UserPreferences(syncPublicCityCatalog = true, preferredState = BrazilianState.SAO_PAULO, preferredMunicipality = "Synthetic"))
        val admitted = store.read()
        store.write(admitted.copy(syncPublicCityCatalog = false, localeTag = "pt-BR", syncStationDetail = false))
        assertFalse(store.markPublicCityCatalogSynced(admitted, 1000L))
        val current = store.read()
        assertFalse(current.syncPublicCityCatalog)
        assertFalse(current.syncStationDetail)
        assertEquals("pt-BR", current.localeTag)
        assertNull(current.publicCityCatalogLastSyncedAtMillis)
    }

    @Test fun `same city off and on increments policy revision and rejects old completion`() = runTest {
        val store = UserPreferencesDataStore(PreferenceDataStoreFactory.create(scope = backgroundScope,
            produceFile = { File(directory, "revision.preferences_pb") }))
        store.write(UserPreferences(syncPublicCityCatalog = true))
        val admitted = store.read()
        store.write(admitted.copy(syncPublicCityCatalog = false))
        store.write(store.read().copy(syncPublicCityCatalog = true))
        assertFalse(store.markPublicCityCatalogSynced(admitted, 1000L))
        assertTrue(store.markPublicCityCatalogSynced(store.read(), 2000L))
        assertEquals(2000L, store.read().publicCityCatalogLastSyncedAtMillis)
    }

    @Test fun `changing city resets old completion and clear fences active work`() = runTest {
        val store = UserPreferencesDataStore(PreferenceDataStoreFactory.create(scope = backgroundScope,
            produceFile = { File(directory, "city.preferences_pb") }))
        store.write(UserPreferences(syncPublicCityCatalog = true, preferredMunicipality = "A"))
        assertTrue(store.markPublicCityCatalogSynced(store.read(), 1000L))
        store.write(store.read().copy(preferredMunicipality = "B"))
        assertNull(store.read().publicCityCatalogLastSyncedAtMillis)
        val admitted = store.read()
        store.clearPublicCityCatalogSyncMarker()
        assertFalse(store.markPublicCityCatalogSynced(admitted, 3000L))
    }
}
