package com.anpfuel.app.ui.settings

import com.anpfuel.application.usecase.directory.ClearPublicCatalogUseCase
import com.anpfuel.application.usecase.settings.ClearCacheUseCase
import com.anpfuel.application.usecase.settings.GetSettingsUseCase
import com.anpfuel.application.usecase.settings.GetStorageUsageUseCase
import com.anpfuel.application.usecase.settings.UpdatePreferencesResult
import com.anpfuel.application.usecase.settings.UpdatePreferencesUseCase
import com.anpfuel.application.usecase.sync.AutoDownloadLatestWeekUseCase
import com.anpfuel.application.usecase.sync.SyncPriceTablesUseCase
import com.anpfuel.application.usecase.vehicle.ListVehiclesUseCase
import com.anpfuel.app.notification.NotificationPermissionHandler
import com.anpfuel.data.worker.SyncWorkScheduler
import com.anpfuel.domain.event.PreferencesUpdated
import com.anpfuel.domain.model.StorageUsage
import com.anpfuel.domain.model.UserPreferences
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class SettingsPublicCatalogTest {

    private val dispatcher = StandardTestDispatcher()

    private val getSettingsUseCase = mockk<GetSettingsUseCase>()
    private val getStorageUsageUseCase = mockk<GetStorageUsageUseCase>()
    private val updatePreferencesUseCase = mockk<UpdatePreferencesUseCase>()
    private val clearCacheUseCase = mockk<ClearCacheUseCase>(relaxed = true)
    private val clearPublicCatalogUseCase = mockk<ClearPublicCatalogUseCase>(relaxed = true)
    private val syncPriceTablesUseCase = mockk<SyncPriceTablesUseCase>(relaxed = true)
    private val autoDownloadLatestWeekUseCase = mockk<AutoDownloadLatestWeekUseCase>(relaxed = true)
    private val syncWorkScheduler = mockk<SyncWorkScheduler>(relaxed = true)
    private val listVehiclesUseCase = mockk<ListVehiclesUseCase>()
    private val notificationPermissionHandler = mockk<NotificationPermissionHandler>()

    private lateinit var viewModel: SettingsViewModel

    private val usage = StorageUsage(summaryRowCount = 1, stationRowCount = 2, importedWeekCount = 1)

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        coEvery { getSettingsUseCase() } returns UserPreferences()
        coEvery { getStorageUsageUseCase() } returns usage
        coEvery { listVehiclesUseCase() } returns emptyList()
        every { notificationPermissionHandler.hasPostNotificationsPermission() } returns true
        viewModel = SettingsViewModel(
            getSettingsUseCase = getSettingsUseCase,
            getStorageUsageUseCase = getStorageUsageUseCase,
            updatePreferencesUseCase = updatePreferencesUseCase,
            clearCacheUseCase = clearCacheUseCase,
            clearPublicCatalogUseCase = clearPublicCatalogUseCase,
            syncPriceTablesUseCase = syncPriceTablesUseCase,
            autoDownloadLatestWeekUseCase = autoDownloadLatestWeekUseCase,
            syncWorkScheduler = syncWorkScheduler,
            listVehiclesUseCase = listVehiclesUseCase,
            notificationPermissionHandler = notificationPermissionHandler,
        )
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun `public catalog toggle persists without touching anp flags`() = runTest(dispatcher) {
        val enabled = UserPreferences(syncPublicCityCatalog = true)
        coEvery { updatePreferencesUseCase(any()) } returns UpdatePreferencesResult(
            preferences = enabled,
            event = PreferencesUpdated.create(
                PreferencesUpdated.Payload(
                    changedKeys = setOf(UpdatePreferencesUseCase.KEY_SYNC_PUBLIC_CITY_CATALOG),
                ),
            ),
        )

        viewModel.onSyncPublicCityCatalogChanged(true)
        advanceUntilIdle()

        assertTrue(viewModel.uiState.value.preferences.syncPublicCityCatalog)
        coVerify(exactly = 1) { updatePreferencesUseCase(match { it.syncPublicCityCatalog }) }
    }

    @Test
    fun `clearing the public catalog refreshes state and confirms`() = runTest(dispatcher) {
        coEvery { getSettingsUseCase() } returns UserPreferences(syncPublicCityCatalog = true)

        viewModel.clearPublicCatalog()
        advanceUntilIdle()

        coVerify(exactly = 1) { clearPublicCatalogUseCase() }
        assertFalse(viewModel.uiState.value.isClearingPublicCatalog)
        assertEquals(
            SettingsViewModel.PUBLIC_CATALOG_CLEARED_MESSAGE,
            viewModel.uiState.value.syncMessage,
        )
        assertNull(viewModel.uiState.value.error)
    }
}
