package com.anpfuel.app.ui.onboarding

import app.cash.turbine.test
import com.anpfuel.app.location.LocationPermissionHandler
import com.anpfuel.application.usecase.location.ResolveDeviceLocationOutcome
import com.anpfuel.application.usecase.location.ResolveDeviceLocationUseCase
import com.anpfuel.application.usecase.location.SelectLocationResult
import com.anpfuel.application.usecase.onboarding.CompleteLocationPromptUseCase
import com.anpfuel.application.usecase.onboarding.CompleteOnboardingResult
import com.anpfuel.application.usecase.onboarding.CompleteOnboardingUseCase
import com.anpfuel.application.usecase.onboarding.OnboardingSelectWeekAndSyncResult
import com.anpfuel.application.usecase.onboarding.OnboardingSelectWeekAndSyncUseCase
import com.anpfuel.application.usecase.sync.DiscoverSurveyWeekCatalogOutcome
import com.anpfuel.application.usecase.sync.DiscoverSurveyWeekCatalogUseCase
import com.anpfuel.application.usecase.sync.SyncPriceTablesResult
import com.anpfuel.domain.event.CitySelected
import com.anpfuel.domain.event.SyncJobOutcome
import com.anpfuel.domain.model.ReverseGeocodeResult
import com.anpfuel.domain.model.SurveyWeekCatalogEntry
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DeviceLocation
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.SurveyWeek
import com.anpfuel.domain.valueobject.SurveyWeekSelectionMode
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import java.time.LocalDate
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class OnboardingViewModelTest {

    private val dispatcher = StandardTestDispatcher()

    private val discoverSurveyWeekCatalogUseCase = mockk<DiscoverSurveyWeekCatalogUseCase>()
    private val onboardingSelectWeekAndSyncUseCase = mockk<OnboardingSelectWeekAndSyncUseCase>()
    private val completeOnboardingUseCase = mockk<CompleteOnboardingUseCase>()
    private val completeLocationPromptUseCase = mockk<CompleteLocationPromptUseCase>()
    private val resolveDeviceLocationUseCase = mockk<ResolveDeviceLocationUseCase>()
    private val locationPermissionHandler = mockk<LocationPermissionHandler>()
    private val userPreferencesRepository = mockk<UserPreferencesRepository>()

    private lateinit var viewModel: OnboardingViewModel

    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val catalogEntry = SurveyWeekCatalogEntry.create(
        surveyWeek = surveyWeek,
        summaryUrl = "https://example.com/summary.xlsx",
        stationUrl = "https://example.com/station.xlsx",
        publishedAt = LocalDate.parse("2026-06-12"),
    )
    private val deviceLocation = DeviceLocation.of(-25.4284, -49.2733)

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        viewModel = OnboardingViewModel(
            discoverSurveyWeekCatalogUseCase = discoverSurveyWeekCatalogUseCase,
            onboardingSelectWeekAndSyncUseCase = onboardingSelectWeekAndSyncUseCase,
            completeOnboardingUseCase = completeOnboardingUseCase,
            completeLocationPromptUseCase = completeLocationPromptUseCase,
            resolveDeviceLocationUseCase = resolveDeviceLocationUseCase,
            locationPermissionHandler = locationPermissionHandler,
            userPreferencesRepository = userPreferencesRepository,
        )
        coEvery { completeLocationPromptUseCase.invoke() } returns Unit
        coEvery { discoverSurveyWeekCatalogUseCase() } returns DiscoverSurveyWeekCatalogOutcome.Success(
            catalog = listOf(catalogEntry),
        )
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences(
            autoDownloadLatestWeek = false,
        )
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun showsLocationPromptAfterSyncWhenNoMunicipalityAndPromptNotCompleted() = runTest {
        stubSyncCompletion()
        coEvery { userPreferencesRepository.getPreferences() } returnsMany listOf(
            UserPreferences(autoDownloadLatestWeek = false),
            UserPreferences(
                onboardingCompleted = true,
                locationPromptCompleted = false,
            ),
        )
        loadWeekPickerCatalog()

        viewModel.selectWeekAndSync(catalogEntry)
        advanceUntilIdle()

        assertEquals(OnboardingStep.LOCATION_PROMPT, viewModel.uiState.value.step)
    }

    @Test
    fun navigatesToHomeAfterSyncWhenMunicipalityAlreadySet() = runTest {
        stubSyncCompletion()
        coEvery { userPreferencesRepository.getPreferences() } returnsMany listOf(
            UserPreferences(autoDownloadLatestWeek = false),
            UserPreferences(
                onboardingCompleted = true,
                preferredState = BrazilianState.PARANA,
                preferredMunicipality = "CURITIBA",
            ),
        )
        loadWeekPickerCatalog()

        viewModel.navigation.test {
            viewModel.selectWeekAndSync(catalogEntry)
            assertEquals(OnboardingNavigation.ToHome, awaitItem())
        }
    }

    @Test
    fun navigatesToLocationWhenPromptAlreadyCompleted() = runTest {
        stubSyncCompletion()
        coEvery { userPreferencesRepository.getPreferences() } returnsMany listOf(
            UserPreferences(autoDownloadLatestWeek = false),
            UserPreferences(
                onboardingCompleted = true,
                locationPromptCompleted = true,
            ),
        )
        loadWeekPickerCatalog()

        viewModel.navigation.test {
            viewModel.selectWeekAndSync(catalogEntry)
            assertEquals(OnboardingNavigation.ToLocation, awaitItem())
        }
    }

    @Test
    fun chooseManualLocationCompletesPromptAndNavigatesToLocationPicker() = runTest {
        viewModel.navigation.test {
            viewModel.onChooseManualLocation()
            assertEquals(OnboardingNavigation.ToLocation, awaitItem())
        }

        coVerify(exactly = 1) { completeLocationPromptUseCase.invoke() }
    }

    @Test
    fun permissionDeniedCompletesPromptAndNavigatesToLocationPicker() = runTest {
        viewModel.navigation.test {
            viewModel.onLocationPermissionDenied()
            assertEquals(OnboardingNavigation.ToLocation, awaitItem())
        }

        coVerify(exactly = 1) { completeLocationPromptUseCase.invoke() }
    }

    @Test
    fun successfulGeocodeNavigatesToHome() = runTest {
        val selectLocationResult = SelectLocationResult(
            event = CitySelected.create(
                payload = CitySelected.Payload(
                    municipality = "CURITIBA",
                    state = BrazilianState.PARANA,
                    surveyWeekId = DomainId.generate(),
                ),
            ),
            preferences = UserPreferences(
                preferredState = BrazilianState.PARANA,
                preferredMunicipality = "CURITIBA",
            ),
        )
        coEvery { resolveDeviceLocationUseCase(deviceLocation) } returns ResolveDeviceLocationOutcome.Success(
            reverseGeocodeResult = ReverseGeocodeResult(
                state = BrazilianState.PARANA,
                municipality = "CURITIBA",
            ),
            selectLocationResult = selectLocationResult,
        )

        viewModel.navigation.test {
            viewModel.onResolveDeviceLocation(deviceLocation)
            assertEquals(OnboardingNavigation.ToHome, awaitItem())
        }

        coVerify(exactly = 1) { completeLocationPromptUseCase.invoke() }
    }

    @Test
    fun geocodeFailureNavigatesToLocationPicker() = runTest {
        coEvery { resolveDeviceLocationUseCase(deviceLocation) } returns
            ResolveDeviceLocationOutcome.NetworkError

        viewModel.navigation.test {
            viewModel.onResolveDeviceLocation(deviceLocation)
            assertEquals(OnboardingNavigation.ToLocation, awaitItem())
        }

        coVerify(exactly = 1) { completeLocationPromptUseCase.invoke() }
    }

    @Test
    fun useDeviceLocationWithPermissionResolvesLocation() = runTest {
        every { locationPermissionHandler.hasLocationPermission() } returns true
        every { locationPermissionHandler.getLastKnownLocation() } returns deviceLocation
        coEvery { resolveDeviceLocationUseCase(deviceLocation) } returns
            ResolveDeviceLocationOutcome.NetworkError

        viewModel.navigation.test {
            viewModel.onUseDeviceLocationClick()
            assertEquals(OnboardingNavigation.ToLocation, awaitItem())
        }

        coVerify(exactly = 1) { resolveDeviceLocationUseCase(deviceLocation) }
    }

    private fun stubSyncCompletion() {
        coEvery {
            onboardingSelectWeekAndSyncUseCase(
                catalogEntry = catalogEntry,
                selectionMode = SurveyWeekSelectionMode.LATEST,
            )
        } returns OnboardingSelectWeekAndSyncResult.Completed(
            syncResult = SyncPriceTablesResult(outcome = SyncJobOutcome.SUCCESS),
            onboardingResult = CompleteOnboardingResult.Completed,
        )
    }

    private fun TestScope.loadWeekPickerCatalog() {
        viewModel.proceedToWeekPicker()
        advanceUntilIdle()
    }
}
