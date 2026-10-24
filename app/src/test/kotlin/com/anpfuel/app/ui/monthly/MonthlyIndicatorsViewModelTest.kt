package com.anpfuel.app.ui.monthly

import com.anpfuel.application.usecase.ledger.GetMonthlyIndicatorsUseCase
import com.anpfuel.application.usecase.ledger.MonthlyIndicatorsCsv
import com.anpfuel.application.usecase.ledger.MonthlyVehicleIndicators
import com.anpfuel.application.usecase.ledger.MonthlyWindowNote
import com.anpfuel.application.usecase.vehicle.GetVehicleUseCase
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.premium.PremiumConsumptionResult
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource
import io.mockk.coEvery
import io.mockk.every
import io.mockk.mockk
import java.time.ZoneId
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class MonthlyIndicatorsViewModelTest {

    private val dispatcher = StandardTestDispatcher()

    private val indicators = mockk<GetMonthlyIndicatorsUseCase>()
    private val vehicle = mockk<GetVehicleUseCase>()

    private lateinit var viewModel: MonthlyIndicatorsViewModel

    private val vehicleId = DomainId.generate()

    private fun row(month: Int = 3, purchase: Long = 24800, unknown: String = "") = MonthlyVehicleIndicators(
        vehicleId = vehicleId, vehicleName = "Gol", year = 2026, month = month, timezone = "UTC",
        purchaseCentavos = purchase, expenseCentavos = 1000, chargeCostCentavos = 0, chargeEnergyWh = 0,
        chargeFeesCentavos = 0, byCategory = emptyMap(), recordCount = 3,
        measured = PremiumConsumptionResult(eligibleQuantityMilli = 20000, eligibleCostCentavos = 12400, distanceMetres = 100000, kmPerUnit = "5", centavosPerKm = "124", unknown = unknown),
        costPerKm = "124", costPerKmMethod = "fuel-purchase-interval",
        combinedCentavosPerKm = "124", combinedMethod = "combined-purchase-interval", combinedUnknown = unknown,
        estimatedKmPerL = null, comparisonChangePercent = "1/4", comparisonUnknown = "",
        windowNote = MonthlyWindowNote(recordCount = 3, anchorFills = 2, hasMissingOdometer = false, distanceMetres = 100000, hasMixedCarrier = false),
    )

    private fun sampleVehicle() = Vehicle.create(
        displayName = "Gol", tankCapacity = TankCapacity.of(50.0),
        fuelProduct = FuelProduct.GASOLINE_REGULAR,
        priceSource = VehiclePriceSource.cheapest(), id = vehicleId,
    )

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        coEvery { vehicle.invoke(vehicleId) } returns sampleVehicle()
        coEvery { indicators.invoke(any(), any(), any(), any()) } returns listOf(row())
        viewModel = MonthlyIndicatorsViewModel(indicators, vehicle)
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test fun `load shows month rows and navigates months`() = runTest(dispatcher) {
        viewModel.load(vehicleId, 2026, 3, ZoneId.of("UTC"))
        advanceUntilIdle()
        assertEquals("Gol", viewModel.uiState.value.vehicleName)
        assertEquals("5", viewModel.uiState.value.rows.single().measured.kmPerUnit)
        assertEquals("1/4", viewModel.uiState.value.rows.single().comparisonChangePercent)
        viewModel.nextMonth()
        advanceUntilIdle()
        assertEquals(4, viewModel.uiState.value.month)
        viewModel.previousMonth()
        advanceUntilIdle()
        viewModel.previousMonth()
        advanceUntilIdle()
        assertEquals(2, viewModel.uiState.value.month)
    }

    @Test fun `stale correction rebuilds and export shares csv`() = runTest(dispatcher) {
        viewModel.load(vehicleId, 2026, 3, ZoneId.of("UTC"))
        advanceUntilIdle()
        coEvery { indicators.invoke(any(), any(), any(), any()) } returns listOf(row().copy(purchaseCentavos = 30000))
        viewModel.reload()
        advanceUntilIdle()
        assertEquals(30000L, viewModel.uiState.value.rows.single().purchaseCentavos)
        viewModel.prepareExport()
        advanceUntilIdle()
        val csv = viewModel.uiState.value.exportCsv
        assertTrue(csv!!.startsWith("vehicle,metric,value\n"))
        assertTrue(csv.contains("purchase_centavos,30000"))
        viewModel.onExportConsumed()
        assertNull(viewModel.uiState.value.exportCsv)
    }

    @Test fun `timezone switch changes month placement`() = runTest(dispatcher) {
        viewModel.load(vehicleId, 2026, 3, ZoneId.of("UTC"))
        advanceUntilIdle()
        coEvery { indicators.invoke(any(), any(), any(), any()) } returns emptyList()
        viewModel.load(vehicleId, 2026, 3, ZoneId.of("America/Sao_Paulo"))
        advanceUntilIdle()
        assertTrue(viewModel.uiState.value.rows.isEmpty())
    }
}
