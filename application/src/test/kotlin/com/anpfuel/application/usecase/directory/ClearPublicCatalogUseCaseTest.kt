package com.anpfuel.application.usecase.directory

import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.ServerStationCache
import com.anpfuel.domain.repository.UserPreferencesRepository
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class ClearPublicCatalogUseCaseTest {
    private val cache: ServerStationCache = mockk(relaxed = true)
    private var current = UserPreferences(syncPublicCityCatalog = true, publicCityCatalogLastSyncedAtMillis = 123L)
    private val preferences = object : UserPreferencesRepository {
        override suspend fun getPreferences() = current
        override suspend fun savePreferences(preferences: UserPreferences) { current = preferences }
    }
    @Test fun `clearing resets the marker and retires the admitted revision without disabling opt in`() = runTest {
        val prior = current
        ClearPublicCatalogUseCase(cache, preferences)()
        coVerify(exactly = 1) { cache.clearPublicPages() }
        assertNull(current.publicCityCatalogLastSyncedAtMillis)
        assertTrue(current.syncPublicCityCatalog)
        assertEquals(1, current.publicCityCatalogGeneration)
        assertFalse(preferences.markPublicCityCatalogSynced(prior, 456L))
    }
    @Test fun `clearing even an empty cache fences a pending download`() = runTest {
        current = UserPreferences()
        ClearPublicCatalogUseCase(cache, preferences)()
        assertEquals(1, current.publicCityCatalogGeneration)
        assertNull(current.publicCityCatalogLastSyncedAtMillis)
    }
}
