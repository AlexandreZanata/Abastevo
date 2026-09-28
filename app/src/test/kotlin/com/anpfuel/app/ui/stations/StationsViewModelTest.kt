package com.anpfuel.app.ui.stations

import androidx.lifecycle.SavedStateHandle
import com.anpfuel.app.location.LocationPermissionHandler
import com.anpfuel.application.usecase.location.SelectLocationUseCase
import com.anpfuel.application.usecase.network.ObserveNetworkConnectivityUseCase
import com.anpfuel.application.usecase.price.GetStationPricesUseCase
import com.anpfuel.application.usecase.station.BuildStationNavigationQueryUseCase
import com.anpfuel.application.usecase.station.FindNearestBestPriceStationUseCase
import com.anpfuel.application.usecase.station.FindNearestStationOutcome
import com.anpfuel.application.usecase.sync.DownloadStationDetailUseCase
import com.anpfuel.domain.model.RetailStation
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.Cnpj
import com.anpfuel.domain.valueobject.DeviceLocation
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.PriceAmount
import io.mockk.coEvery
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class StationsViewModelTest {

    private val dispatcher = StandardTestDispatcher()

    private val getStationPricesUseCase = mockk<GetStationPricesUseCase>()
    private val buildStationNavigationQueryUseCase = mockk<BuildStationNavigationQueryUseCase>()
    private val downloadStationDetailUseCase = mockk<DownloadStationDetailUseCase>()
    private val findNearestBestPriceStationUseCase = mockk<FindNearestBestPriceStationUseCase>()
    private val locationPermissionHandler = mockk<LocationPermissionHandler>()
    private val selectLocationUseCase = mockk<SelectLocationUseCase>()
    private val observeNetworkConnectivityUseCase = mockk<ObserveNetworkConnectivityUseCase>()

    private lateinit var viewModel: StationsViewModel

    private val deviceLocation = DeviceLocation.of(-25.4284, -49.2733)
    private val station = RetailStation.create(
        cnpj = Cnpj.parse("61602199002409"),
        legalName = "POSTO CENTRO",
        tradeName = "POSTO CENTRO",
        address = "RUA XV DE NOVEMBRO, 1000",
        municipality = "Curitiba",
        state = BrazilianState.PARANA,
        brand = "BR",
    )

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        every { observeNetworkConnectivityUseCase.invoke() } returns flowOf(true)
        every { locationPermissionHandler.hasLocationPermission() } returns false
        coEvery { locationPermissionHandler.getCurrentLocation(any()) } returns null

        viewModel = StationsViewModel(
            getStationPricesUseCase = getStationPricesUseCase,
            buildStationNavigationQueryUseCase = buildStationNavigationQueryUseCase,
            downloadStationDetailUseCase = downloadStationDetailUseCase,
            findNearestBestPriceStationUseCase = findNearestBestPriceStationUseCase,
            locationPermissionHandler = locationPermissionHandler,
            selectLocationUseCase = selectLocationUseCase,
            observeNetworkConnectivityUseCase = observeNetworkConnectivityUseCase,
            savedStateHandle = SavedStateHandle(mapOf("fuelProduct" to FuelProduct.ETHANOL.name)),
        )
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun requestsLocationPermissionWhenItWasNeverGranted() = runTest(dispatcher) {
        val permissionRequests = mutableListOf<Unit>()
        backgroundScope.launch(UnconfinedTestDispatcher(testScheduler)) {
            viewModel.locationPermissionRequest.collect { permissionRequests.add(it) }
        }

        viewModel.onFindNearestStation()
        advanceUntilIdle()

        assertEquals(1, permissionRequests.size)
    }

    @Test
    fun emitsNeedsLocationWhenPermissionIsDenied() = runTest(dispatcher) {
        val messages = mutableListOf<StationsMessage>()
        backgroundScope.launch(UnconfinedTestDispatcher(testScheduler)) {
            viewModel.messages.collect { messages.add(it) }
        }

        viewModel.onLocationPermissionDenied()
        advanceUntilIdle()

        assertEquals(listOf(StationsMessage.NearestNeedsLocation), messages)
    }

    @Test
    fun emitsNoFixMessageWhenPermissionGrantedButNoLocationFix() = runTest(dispatcher) {
        every { locationPermissionHandler.hasLocationPermission() } returns true
        val messages = mutableListOf<StationsMessage>()
        backgroundScope.launch(UnconfinedTestDispatcher(testScheduler)) {
            viewModel.messages.collect { messages.add(it) }
        }

        viewModel.onFindNearestStation()
        advanceUntilIdle()

        assertEquals(listOf(StationsMessage.NearestNoFix), messages)
    }

    @Test
    fun opensMapsForTheRecommendedStation() = runTest(dispatcher) {
        every { locationPermissionHandler.hasLocationPermission() } returns true
        coEvery { locationPermissionHandler.getCurrentLocation(any()) } returns deviceLocation
        coEvery {
            findNearestBestPriceStationUseCase.invoke(
                fuelProduct = FuelProduct.ETHANOL,
                deviceLocation = deviceLocation.coordinates,
            )
        } returns FindNearestStationOutcome.Success(
            station = station,
            price = PriceAmount.of("5.89"),
            distanceMeters = 111.0,
            navigationQuery = "POSTO CENTRO, RUA XV DE NOVEMBRO, 1000, Curitiba - PR, Brazil",
            stationName = "POSTO CENTRO",
        )
        val effects = mutableListOf<StationsNavigationEffect>()
        backgroundScope.launch(UnconfinedTestDispatcher(testScheduler)) {
            viewModel.navigationEffects.collect { effects.add(it) }
        }

        viewModel.onFindNearestStation()
        advanceUntilIdle()

        assertEquals(
            listOf(
                StationsNavigationEffect.LaunchMaps(
                    "POSTO CENTRO, RUA XV DE NOVEMBRO, 1000, Curitiba - PR, Brazil",
                ),
            ),
            effects,
        )
        assertFalse(viewModel.uiState.value.isFindingNearest)
    }

    @Test
    fun emitsFailedMessageWhenGeocodingFails() = runTest(dispatcher) {
        every { locationPermissionHandler.hasLocationPermission() } returns true
        coEvery { locationPermissionHandler.getCurrentLocation(any()) } returns deviceLocation
        coEvery {
            findNearestBestPriceStationUseCase.invoke(any(), any())
        } returns FindNearestStationOutcome.GeocodingFailed
        val messages = mutableListOf<StationsMessage>()
        backgroundScope.launch(UnconfinedTestDispatcher(testScheduler)) {
            viewModel.messages.collect { messages.add(it) }
        }

        viewModel.onFindNearestStation()
        advanceUntilIdle()

        assertEquals(listOf(StationsMessage.NearestUnavailable), messages)
        assertFalse(viewModel.uiState.value.isFindingNearest)
    }
}
