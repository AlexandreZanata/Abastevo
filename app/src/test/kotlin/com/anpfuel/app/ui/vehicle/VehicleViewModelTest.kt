package com.anpfuel.app.ui.vehicle

import com.anpfuel.app.notification.NotificationPermissionHandler
import com.anpfuel.application.usecase.alert.ConfigurePriceDropAlertUseCase
import com.anpfuel.application.usecase.price.GetStationPricesUseCase
import com.anpfuel.application.usecase.vehicle.DeleteVehicleUseCase
import com.anpfuel.application.usecase.vehicle.ListVehiclesUseCase
import com.anpfuel.application.usecase.vehicle.SaveVehicleUseCase
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.rule.MaxRegisteredVehiclesRule
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import java.util.Locale
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
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class VehicleViewModelTest {

    private val dispatcher = StandardTestDispatcher()

    private val listVehiclesUseCase = mockk<ListVehiclesUseCase>()
    private val saveVehicleUseCase = mockk<SaveVehicleUseCase>()
    private val deleteVehicleUseCase = mockk<DeleteVehicleUseCase>()
    private val getStationPricesUseCase = mockk<GetStationPricesUseCase>()
    private val configurePriceDropAlertUseCase = mockk<ConfigurePriceDropAlertUseCase>(relaxed = true)
    private val notificationPermissionHandler = mockk<NotificationPermissionHandler>()

    private lateinit var viewModel: VehicleViewModel

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        coEvery { notificationPermissionHandler.hasPostNotificationsPermission() } returns true
        viewModel = VehicleViewModel(
            listVehiclesUseCase = listVehiclesUseCase,
            saveVehicleUseCase = saveVehicleUseCase,
            deleteVehicleUseCase = deleteVehicleUseCase,
            getStationPricesUseCase = getStationPricesUseCase,
            configurePriceDropAlertUseCase = configurePriceDropAlertUseCase,
            notificationPermissionHandler = notificationPermissionHandler,
        )
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun loadPopulatesVehicleList() = runTest(dispatcher) {
        val vehicles = listOf(sampleVehicle("Gol"))
        coEvery { listVehiclesUseCase.invoke() } returns vehicles

        viewModel.load()
        advanceUntilIdle()

        assertEquals(vehicles, viewModel.uiState.value.vehicles)
        assertFalse(viewModel.uiState.value.isLoading)
        assertTrue(viewModel.uiState.value.canAddVehicle)
    }

    @Test
    fun loadSetsCanAddVehicleFalseWhenMaxReached() = runTest(dispatcher) {
        val vehicles = List(MaxRegisteredVehiclesRule.MAX_VEHICLES) { index ->
            sampleVehicle("Vehicle $index")
        }
        coEvery { listVehiclesUseCase.invoke() } returns vehicles

        viewModel.load()
        advanceUntilIdle()

        assertFalse(viewModel.uiState.value.canAddVehicle)
    }

    @Test
    fun startAddVehicleDoesNothingWhenMaxReached() = runTest(dispatcher) {
        val vehicles = List(MaxRegisteredVehiclesRule.MAX_VEHICLES) { index ->
            sampleVehicle("Vehicle $index")
        }
        coEvery { listVehiclesUseCase.invoke() } returns vehicles

        viewModel.load()
        advanceUntilIdle()
        viewModel.startAddVehicle()

        assertFalse(viewModel.uiState.value.showForm)
    }

    @Test
    fun saveVehicleCallsUseCaseAndReloads() = runTest(dispatcher) {
        coEvery { listVehiclesUseCase.invoke() } returnsMany listOf(
            emptyList(),
            listOf(sampleVehicle("Gol")),
        )
        coEvery { saveVehicleUseCase.invoke(any()) } answers {
            com.anpfuel.application.usecase.vehicle.SaveVehicleResult(
                vehicle = firstArg(),
                events = emptyList(),
            )
        }

        viewModel.load()
        advanceUntilIdle()
        viewModel.startAddVehicle()
        viewModel.onDisplayNameChanged("Gol")
        viewModel.onTankCapacityChanged("50")
        viewModel.saveVehicle(Locale.US)
        advanceUntilIdle()

        coVerify(exactly = 1) { saveVehicleUseCase.invoke(any()) }
        coVerify(exactly = 1) { configurePriceDropAlertUseCase.invoke(updated = any(), previous = null) }
        coVerify(exactly = 2) { listVehiclesUseCase.invoke() }
        assertFalse(viewModel.uiState.value.showForm)
        assertEquals(1, viewModel.uiState.value.vehicles.size)
    }

    @Test
    fun confirmDeleteVehicleCallsUseCase() = runTest(dispatcher) {
        val vehicle = sampleVehicle("Gol")
        coEvery { listVehiclesUseCase.invoke() } returnsMany listOf(
            listOf(vehicle),
            emptyList(),
        )
        coEvery { deleteVehicleUseCase.invoke(vehicle.id) } returns
            com.anpfuel.application.usecase.vehicle.DeleteVehicleResult(
                vehicleId = vehicle.id,
                event = mockk(relaxed = true),
            )

        viewModel.load()
        advanceUntilIdle()
        viewModel.requestDeleteVehicle(vehicle.id)
        viewModel.confirmDeleteVehicle()
        advanceUntilIdle()

        coVerify(exactly = 1) { deleteVehicleUseCase.invoke(vehicle.id) }
        assertTrue(viewModel.uiState.value.vehicles.isEmpty())
    }

    @Test
    fun saveVehicleSetsValidationErrorWhenNameBlank() = runTest(dispatcher) {
        coEvery { listVehiclesUseCase.invoke() } returns emptyList()

        viewModel.load()
        advanceUntilIdle()
        viewModel.startAddVehicle()
        viewModel.saveVehicle(Locale.US)
        advanceUntilIdle()

        assertEquals(
            VehicleFormValidationError.NAME_REQUIRED,
            viewModel.uiState.value.form.validationErrorKey,
        )
        coVerify(exactly = 0) { saveVehicleUseCase.invoke(any()) }
    }

    private fun sampleVehicle(name: String): Vehicle =
        Vehicle.create(
            displayName = name,
            tankCapacity = TankCapacity.of(50.0),
            fuelProduct = FuelProduct.GASOLINE_REGULAR,
            priceSource = VehiclePriceSource.cheapest(),
        )
}
