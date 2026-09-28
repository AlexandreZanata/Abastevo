package com.anpfuel.app.ui.home

import com.anpfuel.application.usecase.location.PreferredLocation
import com.anpfuel.application.usecase.location.SelectLocationUseCase
import com.anpfuel.application.usecase.network.ObserveNetworkConnectivityUseCase
import com.anpfuel.application.usecase.price.GetMunicipalityPricesUseCase
import com.anpfuel.application.usecase.price.MunicipalityPricesResult
import com.anpfuel.application.usecase.readiness.DataReadinessResult
import com.anpfuel.application.usecase.readiness.GetDataReadinessUseCase
import com.anpfuel.application.usecase.sync.SyncPriceTablesUseCase
import com.anpfuel.application.usecase.vehicle.GetTankFillCostEstimatesUseCase
import com.anpfuel.application.usecase.vehicle.ListVehiclesUseCase
import com.anpfuel.application.usecase.vehicle.TankFillCostEstimatesResult
import com.anpfuel.app.mapper.PriceFormatter
import com.anpfuel.application.usecase.vehicle.VehicleTankFillCostEstimate
import com.anpfuel.domain.model.TankFillCostEstimate
import com.anpfuel.domain.model.TankFillCostUnitPriceSource
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.state.DataReadinessState
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DataAvailability
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.PriceAmount
import com.anpfuel.domain.valueobject.SurveyWeek
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource
import io.mockk.coEvery
import io.mockk.every
import io.mockk.mockk
import java.util.Locale
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class HomeViewModelTest {

    private val dispatcher = StandardTestDispatcher()

    private val getDataReadinessUseCase = mockk<GetDataReadinessUseCase>()
    private val getMunicipalityPricesUseCase = mockk<GetMunicipalityPricesUseCase>()
    private val selectLocationUseCase = mockk<SelectLocationUseCase>()
    private val syncPriceTablesUseCase = mockk<SyncPriceTablesUseCase>()
    private val listVehiclesUseCase = mockk<ListVehiclesUseCase>()
    private val getTankFillCostEstimatesUseCase = mockk<GetTankFillCostEstimatesUseCase>()
    private val observeNetworkConnectivityUseCase = mockk<ObserveNetworkConnectivityUseCase>()

    private lateinit var viewModel: HomeViewModel

    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val state = BrazilianState.PARANA
    private val municipality = "Curitiba"

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        every { observeNetworkConnectivityUseCase.invoke() } returns flowOf(true)
        viewModel = HomeViewModel(
            getDataReadinessUseCase = getDataReadinessUseCase,
            getMunicipalityPricesUseCase = getMunicipalityPricesUseCase,
            selectLocationUseCase = selectLocationUseCase,
            syncPriceTablesUseCase = syncPriceTablesUseCase,
            listVehiclesUseCase = listVehiclesUseCase,
            getTankFillCostEstimatesUseCase = getTankFillCostEstimatesUseCase,
            observeNetworkConnectivityUseCase = observeNetworkConnectivityUseCase,
        )
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun loadLeavesTankFillEstimatesEmptyWhenNoVehicles() = runTest(dispatcher) {
        stubReadyHome(vehicles = emptyList(), tankItems = emptyList())

        viewModel.load(Locale.US)
        advanceUntilIdle()

        assertTrue(viewModel.uiState.value.tankFillCostEstimates.isEmpty())
    }

    @Test
    fun loadMapsTankFillEstimatesForRegisteredVehicles() = runTest(dispatcher) {
        val vehicle = sampleVehicle("Gol")
        val estimate = TankFillCostEstimate(
            vehicleId = vehicle.id,
            displayName = vehicle.displayName,
            tankCapacity = vehicle.tankCapacity,
            fuelProduct = vehicle.fuelProduct,
            unitPrice = PriceAmount.of("5.49"),
            totalCost = PriceAmount.of("274.50"),
            unitPriceSource = TankFillCostUnitPriceSource.CHEAPEST_STATION,
            stationDisplayName = "Cheap",
        )
        stubReadyHome(
            vehicles = listOf(vehicle),
            tankItems = listOf(VehicleTankFillCostEstimate(vehicle = vehicle, estimate = estimate)),
        )

        viewModel.load(Locale.US)
        advanceUntilIdle()

        assertEquals(1, viewModel.uiState.value.tankFillCostEstimates.size)
        assertEquals("Gol", viewModel.uiState.value.tankFillCostEstimates.single().displayName)
        assertEquals(
            PriceFormatter.formatAmount(PriceAmount.of("274.50"), Locale.US),
            viewModel.uiState.value.tankFillCostEstimates.single().totalCostFormatted,
        )
    }

    @Test
    fun loadDoesNotShowLoadingIndicatorWhenContentAlreadyPresent() = runTest(dispatcher) {
        stubReadyHome(vehicles = emptyList(), tankItems = emptyList())

        viewModel.load(Locale.US)
        advanceUntilIdle()
        assertEquals(false, viewModel.uiState.value.isLoading)

        viewModel.load(Locale.US)
        advanceUntilIdle()

        assertEquals(false, viewModel.uiState.value.isLoading)
    }

    @Test
    fun loadMapsMultipleTankFillCardsInOrder() = runTest(dispatcher) {
        val first = sampleVehicle("Gol", sortOrder = 0)
        val second = sampleVehicle("Onix", sortOrder = 1)
        stubReadyHome(
            vehicles = listOf(first, second),
            tankItems = listOf(
                VehicleTankFillCostEstimate(
                    vehicle = first,
                    estimate = tankEstimate(first, "274.50"),
                ),
                VehicleTankFillCostEstimate(
                    vehicle = second,
                    estimate = tankEstimate(second, "220.00"),
                ),
            ),
        )

        viewModel.load(Locale.US)
        advanceUntilIdle()

        assertEquals(
            listOf("Gol", "Onix"),
            viewModel.uiState.value.tankFillCostEstimates.map { it.displayName },
        )
    }

    private fun stubReadyHome(
        vehicles: List<Vehicle>,
        tankItems: List<VehicleTankFillCostEstimate>,
    ) {
        coEvery { getDataReadinessUseCase.invoke() } returns DataReadinessResult(
            readiness = DataReadinessState.READY,
            hasCachedData = true,
            latestSurveyWeek = surveyWeek,
            lastSummarySyncAt = null,
        )
        coEvery { selectLocationUseCase.getPreferredLocation() } returns PreferredLocation(
            municipality = municipality,
            state = state,
        )
        coEvery { listVehiclesUseCase.invoke() } returns vehicles
        coEvery { getMunicipalityPricesUseCase.invoke() } returns MunicipalityPricesResult(
            surveyWeek = surveyWeek,
            state = state,
            municipality = municipality,
            prices = emptyList(),
            dataAvailability = DataAvailability.HAS_DATA,
        )
        coEvery {
            getTankFillCostEstimatesUseCase.invoke(
                vehicles = vehicles,
                state = state,
                municipality = municipality,
                surveyWeek = surveyWeek,
            )
        } returns TankFillCostEstimatesResult(
            surveyWeek = surveyWeek,
            state = state,
            municipality = municipality,
            items = tankItems,
        )
    }

    private fun sampleVehicle(name: String, sortOrder: Int = 0): Vehicle =
        Vehicle.create(
            displayName = name,
            tankCapacity = TankCapacity.of(50.0),
            fuelProduct = FuelProduct.GASOLINE_REGULAR,
            priceSource = VehiclePriceSource.cheapest(),
            sortOrder = sortOrder,
        )

    private fun tankEstimate(vehicle: Vehicle, total: String): TankFillCostEstimate =
        TankFillCostEstimate(
            vehicleId = vehicle.id,
            displayName = vehicle.displayName,
            tankCapacity = vehicle.tankCapacity,
            fuelProduct = vehicle.fuelProduct,
            unitPrice = PriceAmount.of("5.49"),
            totalCost = PriceAmount.of(total),
            unitPriceSource = TankFillCostUnitPriceSource.CHEAPEST_STATION,
        )
}
