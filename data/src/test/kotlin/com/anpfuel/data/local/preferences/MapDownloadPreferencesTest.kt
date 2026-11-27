package com.anpfuel.data.local.preferences

import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.io.File

class MapDownloadPreferencesTest {
    @TempDir lateinit var directory: File

    @Test fun `map mobile data is opt in and independent of price sync`() = runTest {
        val store = UserPreferencesDataStore(
            PreferenceDataStoreFactory.create(scope = backgroundScope) {
                File(directory, "maps.preferences_pb")
            },
        )
        val defaults = store.read()
        assertFalse(defaults.allowMapDownloadsOnMobileData)
        assertTrue(defaults.autoSyncOnWifi)

        store.write(defaults.copy(allowMapDownloadsOnMobileData = true))
        assertTrue(store.read().allowMapDownloadsOnMobileData)
        assertTrue(store.read().autoSyncOnWifi)

        store.write(store.read().copy(autoSyncOnWifi = false))
        assertTrue(store.read().allowMapDownloadsOnMobileData)
        assertFalse(store.read().autoSyncOnWifi)

        store.write(store.read().copy(allowMapDownloadsOnMobileData = false))
        assertFalse(store.read().allowMapDownloadsOnMobileData)
        assertFalse(store.read().autoSyncOnWifi)
    }
}
