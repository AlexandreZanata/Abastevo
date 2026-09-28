package com.anpfuel.application.usecase.settings

import com.anpfuel.domain.event.CacheClearScope
import com.anpfuel.domain.event.CacheCleared
import com.anpfuel.domain.event.PreferencesUpdated
import com.anpfuel.domain.model.StorageUsage
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.CacheRepository
import com.anpfuel.domain.repository.DomainEventPublisher
import com.anpfuel.domain.repository.StationPriceRepository
import com.anpfuel.domain.repository.StorageStatsRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class SettingsUseCasesTest {

    private val userPreferencesRepository = mockk<UserPreferencesRepository>()
    private val eventPublisher = mockk<DomainEventPublisher>()
    private val cacheRepository = mockk<CacheRepository>()
    private val stationPriceRepository = mockk<StationPriceRepository>()
    private val storageStatsRepository = mockk<StorageStatsRepository>()

    private lateinit var getSettingsUseCase: GetSettingsUseCase
    private lateinit var getStorageUsageUseCase: GetStorageUsageUseCase
    private lateinit var updatePreferencesUseCase: UpdatePreferencesUseCase
    private lateinit var clearCacheUseCase: ClearCacheUseCase
    private lateinit var applyStationDetailRetentionUseCase: ApplyStationDetailRetentionUseCase

    private val defaultPreferences = UserPreferences(
        preferredState = BrazilianState.SAO_PAULO,
        preferredMunicipality = "São Paulo",
        preferredFuelProduct = FuelProduct.ETHANOL,
        localeTag = "en",
        syncStationDetail = false,
        stationDetailRetentionWeeks = 12,
        autoSyncOnWifi = true,
        showPriceHistory = true,
        onboardingCompleted = true,
    )

    @BeforeEach
    fun setUp() {
        getSettingsUseCase = GetSettingsUseCase(userPreferencesRepository)
        getStorageUsageUseCase = GetStorageUsageUseCase(storageStatsRepository)
        updatePreferencesUseCase = UpdatePreferencesUseCase(
            userPreferencesRepository = userPreferencesRepository,
            eventPublisher = eventPublisher,
        )
        clearCacheUseCase = ClearCacheUseCase(
            cacheRepository = cacheRepository,
            stationPriceRepository = stationPriceRepository,
            userPreferencesRepository = userPreferencesRepository,
            eventPublisher = eventPublisher,
        )
        applyStationDetailRetentionUseCase = ApplyStationDetailRetentionUseCase(
            userPreferencesRepository = userPreferencesRepository,
            stationPriceRepository = stationPriceRepository,
        )

        coEvery { eventPublisher.publish(any()) } returns Unit
    }

    @Test
    fun getSettingsReturnsPersistedPreferences() = runTest {
        coEvery { userPreferencesRepository.getPreferences() } returns defaultPreferences

        val settings = getSettingsUseCase.invoke()

        assertEquals(defaultPreferences, settings)
    }

    @Test
    fun getStorageUsageReturnsRepositoryCounts() = runTest {
        val usage = StorageUsage(
            summaryRowCount = 2344,
            stationRowCount = 19676,
            importedWeekCount = 2,
        )
        coEvery { storageStatsRepository.getStorageUsage() } returns usage

        val result = getStorageUsageUseCase.invoke()

        assertEquals(usage, result)
    }

    @Test
    fun updatePreferencesPersistsChangesAndEmitsEvent() = runTest {
        val updated = defaultPreferences.copy(
            syncStationDetail = true,
            localeTag = "pt-BR",
        )
        coEvery { userPreferencesRepository.getPreferences() } returns defaultPreferences
        coEvery { userPreferencesRepository.savePreferences(updated) } returns Unit

        val result = updatePreferencesUseCase.invoke(updated)

        assertEquals(updated, result.preferences)
        assertTrue(result.event.payload.changedKeys.contains(UpdatePreferencesUseCase.KEY_SYNC_STATION_DETAIL))
        assertTrue(result.event.payload.changedKeys.contains(UpdatePreferencesUseCase.KEY_LOCALE_TAG))
        coVerify(exactly = 1) { eventPublisher.publish(any<PreferencesUpdated>()) }
    }

    @Test
    fun clearAllImportedDataAndResetsOnboarding() = runTest {
        val savedPreferences = slot<UserPreferences>()
        coEvery { cacheRepository.clearAllImportedData() } returns Unit
        coEvery { userPreferencesRepository.getPreferences() } returns defaultPreferences
        coEvery { userPreferencesRepository.savePreferences(capture(savedPreferences)) } returns Unit

        val result = clearCacheUseCase.invoke(CacheClearScope.ALL)

        assertEquals(CacheClearScope.ALL, result.scope)
        assertFalse(savedPreferences.captured.onboardingCompleted)
        assertFalse(savedPreferences.captured.locationPromptCompleted)
        assertEquals(null, savedPreferences.captured.preferredState)
        assertEquals(null, savedPreferences.captured.preferredMunicipality)
        coVerify(exactly = 1) { cacheRepository.clearAllImportedData() }
        coVerify(exactly = 1) { eventPublisher.publish(any<CacheCleared>()) }
    }

    @Test
    fun clearStationDetailOnlyAppliesRetentionPolicy() = runTest {
        coEvery { userPreferencesRepository.getPreferences() } returns defaultPreferences
        coEvery { stationPriceRepository.deleteStationPricesOlderThanRetention(12) } returns Unit

        val result = clearCacheUseCase.invoke(CacheClearScope.STATION_DETAIL_ONLY)

        assertEquals(CacheClearScope.STATION_DETAIL_ONLY, result.scope)
        coVerify(exactly = 1) { stationPriceRepository.deleteStationPricesOlderThanRetention(12) }
        coVerify(exactly = 0) { cacheRepository.clearAllImportedData() }
    }

    @Test
    fun br013ApplyRetentionUsesConfiguredWeeks() = runTest {
        coEvery { userPreferencesRepository.getPreferences() } returns defaultPreferences.copy(
            stationDetailRetentionWeeks = 8,
        )
        coEvery { stationPriceRepository.deleteStationPricesOlderThanRetention(8) } returns Unit

        applyStationDetailRetentionUseCase.invoke()

        coVerify(exactly = 1) { stationPriceRepository.deleteStationPricesOlderThanRetention(8) }
    }

    @Test
    fun updatePreferencesDetectsChangedKeys() {
        val changed = updatePreferencesUseCase.detectChangedKeys(
            current = defaultPreferences,
            updated = defaultPreferences.copy(showPriceHistory = false),
        )

        assertEquals(setOf(UpdatePreferencesUseCase.KEY_SHOW_PRICE_HISTORY), changed)
    }
}
