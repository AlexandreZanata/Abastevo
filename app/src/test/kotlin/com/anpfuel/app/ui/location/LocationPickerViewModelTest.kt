package com.anpfuel.app.ui.location

import com.anpfuel.app.location.LocationPermissionHandler
import com.anpfuel.application.usecase.location.CatalogMunicipalityItem
import com.anpfuel.application.usecase.location.LocationMunicipalitiesResult
import com.anpfuel.application.usecase.location.LocationStatesResult
import com.anpfuel.application.usecase.location.ResolveDeviceLocationUseCase
import com.anpfuel.application.usecase.location.SelectLocationUseCase
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DataAvailability
import com.anpfuel.domain.valueobject.SurveyWeek
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.delay
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

/**
 * MEX-06 — Fast picker UI: generation fencing for async loads, immediate
 * bounded results, retained data on failure, no-week discovery.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class LocationPickerViewModelTest {

    private val dispatcher = StandardTestDispatcher()

    private val selectLocationUseCase = mockk<SelectLocationUseCase>()
    private val resolveDeviceLocationUseCase = mockk<ResolveDeviceLocationUseCase>()
    private val locationPermissionHandler = mockk<LocationPermissionHandler>()

    private lateinit var viewModel: LocationPickerViewModel

    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val saoPaulo = BrazilianState.SAO_PAULO
    private val rio = BrazilianState.RIO_DE_JANEIRO

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        coEvery { selectLocationUseCase.getPreferredLocation() } returns null
        coEvery { selectLocationUseCase.getStatesWithData() } returns LocationStatesResult(
            surveyWeek = surveyWeek,
            states = listOf(saoPaulo, rio),
        )
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    private fun municipalitiesResult(state: BrazilianState, vararg names: String) =
        LocationMunicipalitiesResult(
            surveyWeek = surveyWeek,
            state = state,
            municipalities = names.map {
                CatalogMunicipalityItem(it, DataAvailability.NEVER_IN_ANP)
            },
        )

    @Test
    fun rapidStateTapsKeepOnlyLatestMunicipalities() = runTest {
        coEvery { selectLocationUseCase.getMunicipalities(saoPaulo) } coAnswers {
            delay(1_000)
            municipalitiesResult(saoPaulo, "São Paulo")
        }
        coEvery { selectLocationUseCase.getMunicipalities(rio) } returns
            municipalitiesResult(rio, "Rio de Janeiro")
        viewModel = LocationPickerViewModel(
            selectLocationUseCase,
            resolveDeviceLocationUseCase,
            locationPermissionHandler,
        )
        advanceUntilIdle()

        viewModel.onStateSelected(saoPaulo)
        viewModel.onStateSelected(rio)
        advanceUntilIdle()

        val municipalities = viewModel.uiState.value.municipalities.map { it.municipality }
        assertEquals(listOf("Rio de Janeiro"), municipalities)
        coVerify(exactly = 1) { selectLocationUseCase.getMunicipalities(rio) }
    }

    @Test
    fun staleStatesLoadCannotOverwriteFreshLoad() = runTest {
        coEvery { selectLocationUseCase.getStatesWithData() } coAnswers {
            delay(1_000)
            LocationStatesResult(surveyWeek, listOf(saoPaulo))
        }
        viewModel = LocationPickerViewModel(
            selectLocationUseCase,
            resolveDeviceLocationUseCase,
            locationPermissionHandler,
        )
        runCurrent()

        coEvery { selectLocationUseCase.getStatesWithData() } returns
            LocationStatesResult(surveyWeek, listOf(saoPaulo, rio))
        viewModel.loadStates()
        advanceUntilIdle()

        assertEquals(listOf(rio, saoPaulo), viewModel.uiState.value.states)
        assertFalse(viewModel.uiState.value.isLoading)
    }

    @Test
    fun offlineFailureRetainsLoadedStates() = runTest {
        viewModel = LocationPickerViewModel(
            selectLocationUseCase,
            resolveDeviceLocationUseCase,
            locationPermissionHandler,
        )
        advanceUntilIdle()
        assertEquals(listOf(rio, saoPaulo), viewModel.uiState.value.states)

        coEvery { selectLocationUseCase.getStatesWithData() } throws
            DomainException("offline")
        viewModel.loadStates()
        advanceUntilIdle()

        // Retained useful data is never cleared on refresh failure.
        assertEquals(listOf(rio, saoPaulo), viewModel.uiState.value.states)
        assertEquals("offline", viewModel.uiState.value.errorMessage)
        assertFalse(viewModel.uiState.value.isLoading)
    }

    @Test
    fun listsStatesWithoutImportedWeek() = runTest {
        coEvery { selectLocationUseCase.getStatesWithData() } returns LocationStatesResult(
            surveyWeek = null,
            states = listOf(saoPaulo),
        )
        viewModel = LocationPickerViewModel(
            selectLocationUseCase,
            resolveDeviceLocationUseCase,
            locationPermissionHandler,
        )
        advanceUntilIdle()

        assertEquals(listOf(saoPaulo), viewModel.uiState.value.states)
        assertNull(viewModel.uiState.value.errorMessage)
        assertFalse(viewModel.uiState.value.isLoading)
    }

    @Test
    fun selectionFailureResetsSavingAndSurfacesError() = runTest {
        coEvery { selectLocationUseCase.getMunicipalities(saoPaulo) } returns
            municipalitiesResult(saoPaulo, "São Paulo")
        coEvery {
            selectLocationUseCase.selectLocation(saoPaulo, "São Paulo")
        } throws DomainException("BR-006: No successfully imported SurveyWeek is available")
        viewModel = LocationPickerViewModel(
            selectLocationUseCase,
            resolveDeviceLocationUseCase,
            locationPermissionHandler,
        )
        advanceUntilIdle()

        viewModel.onStateSelected(saoPaulo)
        advanceUntilIdle()
        viewModel.onMunicipalitySelected("São Paulo")
        advanceUntilIdle()

        assertTrue(viewModel.uiState.value.errorMessage?.contains("BR-006") == true)
        assertFalse(viewModel.uiState.value.isSaving)
    }
}
