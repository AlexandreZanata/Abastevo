package com.anpfuel.app.ui.stations

import androidx.lifecycle.SavedStateHandle
import com.anpfuel.app.location.LocationPermissionHandler
import com.anpfuel.application.usecase.directory.GetServerStationDetailUseCase
import com.anpfuel.application.usecase.directory.GetServerStationsUseCase
import com.anpfuel.application.usecase.directory.ServerStationDetailOutcome
import com.anpfuel.application.usecase.directory.ServerStationsOutcome
import com.anpfuel.application.usecase.location.SelectLocationUseCase
import com.anpfuel.application.usecase.network.ObserveNetworkConnectivityUseCase
import com.anpfuel.application.usecase.price.GetStationPricesUseCase
import com.anpfuel.application.usecase.station.BuildStationNavigationQueryUseCase
import com.anpfuel.application.usecase.station.FindNearestBestPriceStationUseCase
import com.anpfuel.application.usecase.sync.DownloadStationDetailUseCase
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.ServerStationPage
import com.anpfuel.domain.discovery.StationLocationQuality
import com.anpfuel.domain.valueobject.FuelProduct
import io.mockk.coEvery
import io.mockk.every
import io.mockk.mockk
import java.io.IOException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class StationsServerDiscoveryTest {

    private val dispatcher = StandardTestDispatcher()

    private val getStationPricesUseCase = mockk<GetStationPricesUseCase>(relaxed = true)
    private val buildStationNavigationQueryUseCase = mockk<BuildStationNavigationQueryUseCase>(relaxed = true)
    private val downloadStationDetailUseCase = mockk<DownloadStationDetailUseCase>(relaxed = true)
    private val findNearestBestPriceStationUseCase = mockk<FindNearestBestPriceStationUseCase>(relaxed = true)
    private val locationPermissionHandler = mockk<LocationPermissionHandler>(relaxed = true)
    private val selectLocationUseCase = mockk<SelectLocationUseCase>(relaxed = true)
    private val observeNetworkConnectivityUseCase = mockk<ObserveNetworkConnectivityUseCase>()
    private val getServerStationsUseCase = mockk<GetServerStationsUseCase>()
    private val getServerStationDetailUseCase = mockk<GetServerStationDetailUseCase>()

    private val stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26"
    private val station = ServerStation.create(
        stationId = stationId,
        displayName = "Posto Central",
        locationQuality = StationLocationQuality.REVIEWED,
        latitude = -23.55,
        longitude = -46.63,
        cnpjNormalized = "04218406000104",
        municipalityCode = "3550308",
        state = "SP",
        currentRevisionId = null,
    )

    private lateinit var viewModel: StationsViewModel

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        every { observeNetworkConnectivityUseCase.invoke() } returns flowOf(true)
        viewModel = StationsViewModel(
            getStationPricesUseCase = getStationPricesUseCase,
            buildStationNavigationQueryUseCase = buildStationNavigationQueryUseCase,
            downloadStationDetailUseCase = downloadStationDetailUseCase,
            findNearestBestPriceStationUseCase = findNearestBestPriceStationUseCase,
            locationPermissionHandler = locationPermissionHandler,
            selectLocationUseCase = selectLocationUseCase,
            observeNetworkConnectivityUseCase = observeNetworkConnectivityUseCase,
            savedStateHandle = SavedStateHandle(),
            getServerStationsUseCase = getServerStationsUseCase,
            getServerStationDetailUseCase = getServerStationDetailUseCase,
        )
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun `fresh server page populates list without error`() = runTest(dispatcher) {
        coEvery { getServerStationsUseCase(20, null) } returns
            ServerStationsOutcome.Fresh(ServerStationPage(listOf(station), null))

        viewModel.loadServerStations()
        advanceUntilIdle()

        assertEquals(1, viewModel.uiState.value.serverStations.size)
        assertFalse(viewModel.uiState.value.serverFromCache)
        assertNull(viewModel.uiState.value.serverError)
    }

    @Test
    fun `stale cache is labelled and first failure stays honest`() = runTest(dispatcher) {
        coEvery { getServerStationsUseCase(20, null) } returns
            ServerStationsOutcome.StaleCache(ServerStationPage(listOf(station), null), IOException("down"))

        viewModel.loadServerStations()
        advanceUntilIdle()

        assertEquals(1, viewModel.uiState.value.serverStations.size)
        assertTrue(viewModel.uiState.value.serverFromCache)

        coEvery { getServerStationsUseCase(20, null) } returns
            ServerStationsOutcome.Unavailable(IOException("down"))
        val freshViewModel = StationsViewModel(
            getStationPricesUseCase = getStationPricesUseCase,
            buildStationNavigationQueryUseCase = buildStationNavigationQueryUseCase,
            downloadStationDetailUseCase = downloadStationDetailUseCase,
            findNearestBestPriceStationUseCase = findNearestBestPriceStationUseCase,
            locationPermissionHandler = locationPermissionHandler,
            selectLocationUseCase = selectLocationUseCase,
            observeNetworkConnectivityUseCase = observeNetworkConnectivityUseCase,
            savedStateHandle = SavedStateHandle(),
            getServerStationsUseCase = getServerStationsUseCase,
            getServerStationDetailUseCase = getServerStationDetailUseCase,
        )
        freshViewModel.loadServerStations()
        advanceUntilIdle()

        assertTrue(freshViewModel.uiState.value.serverStations.isEmpty())
        assertNotNull(freshViewModel.uiState.value.serverError)
    }

    @Test
    fun `selecting server station opens detail and dismiss clears it`() = runTest(dispatcher) {
        coEvery { getServerStationDetailUseCase(stationId) } returns
            ServerStationDetailOutcome.Fresh(station)

        viewModel.onServerStationSelected(stationId)
        advanceUntilIdle()

        assertEquals(stationId, viewModel.uiState.value.selectedServerStation?.stationId)
        assertFalse(viewModel.uiState.value.serverDetailFromCache)

        viewModel.onServerDetailDismissed()
        assertNull(viewModel.uiState.value.selectedServerStation)
    }

    @Test
    fun `unknown server station yields no detail`() = runTest(dispatcher) {
        coEvery { getServerStationDetailUseCase(stationId) } returns
            ServerStationDetailOutcome.Unavailable(IOException("404"))

        viewModel.onServerStationSelected(stationId)
        advanceUntilIdle()

        assertNull(viewModel.uiState.value.selectedServerStation)
        assertNotNull(viewModel.uiState.value.serverError)
    }

    @Test
    fun `navigating to unknown-location station emits nothing`() = runTest(dispatcher) {
        val unknown = ServerStation.create(
            stationId = "e7d85d34-74ec-5d35-b3f6-519dc34ce370",
            displayName = "[P34-TEST] Posto Gama",
            locationQuality = StationLocationQuality.UNKNOWN,
            latitude = null,
            longitude = null,
            cnpjNormalized = "12ABC34501DE35",
            municipalityCode = "3550308",
            state = "SP",
            currentRevisionId = null,
        )
        val effects = mutableListOf<StationsNavigationEffect>()
        val job = launch { viewModel.navigationEffects.collect { effects += it } }

        viewModel.onServerStationNavigate(unknown)
        advanceUntilIdle()
        job.cancel()

        assertTrue(effects.isEmpty())
    }
}
