package com.anpfuel.application.usecase.directory

import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.ServerStationCache
import com.anpfuel.domain.repository.UserPreferencesRepository
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class ClearPublicCatalogUseCaseTest {

    private val cache: ServerStationCache = mockk(relaxed = true)
    private val preferencesRepository: UserPreferencesRepository = mockk()
    private val useCase = ClearPublicCatalogUseCase(cache, preferencesRepository)

    @Test
    fun `clearing drops pages and resets last sync`() = runTest {
        val saved = slot<UserPreferences>()
        coEvery { preferencesRepository.getPreferences() } returns
            UserPreferences(syncPublicCityCatalog = true, publicCityCatalogLastSyncedAtMillis = 123L)
        coEvery { preferencesRepository.savePreferences(capture(saved)) } returns Unit

        useCase()

        coVerify(exactly = 1) { cache.clearPublicPages() }
        assertNull(saved.captured.publicCityCatalogLastSyncedAtMillis)
    }

    @Test
    fun `clearing without a marker skips the rewrite`() = runTest {
        coEvery { preferencesRepository.getPreferences() } returns UserPreferences()

        useCase()

        coVerify(exactly = 1) { cache.clearPublicPages() }
        coVerify(exactly = 0) { preferencesRepository.savePreferences(any()) }
    }
}
