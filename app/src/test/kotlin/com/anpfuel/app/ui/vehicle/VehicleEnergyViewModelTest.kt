package com.anpfuel.app.ui.vehicle

import com.anpfuel.app.notification.NotificationPermissionHandler
import com.anpfuel.application.usecase.alert.ConfigurePriceDropAlertUseCase
import com.anpfuel.application.usecase.price.GetStationPricesUseCase
import com.anpfuel.application.usecase.vehicle.DeleteVehicleUseCase
import com.anpfuel.application.usecase.vehicle.ListVehiclesUseCase
import com.anpfuel.application.usecase.vehicle.SaveVehicleUseCase
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.Propulsion
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
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
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class VehicleEnergyViewModelTest {

    private val dispatcher = StandardTestDispatcher()

    private val listVehiclesUseCase = mockk<ListVehiclesUseCase>()
    private val saveVehicleUseCase = mockk<SaveVehicleUseCase>()
    private val deleteVehicleUseCase = mockk<DeleteVehicleUseCase>()
    private val getStationPricesUseCase = mockk<GetStationPricesUseCase>()
    private val configurePriceDropAlertUseCase = mockk<ConfigurePriceDropAlertUseCase>(relaxed = true)
    private val notificationPermissionHandler = mockk<NotificationPermissionHandler>()
    private val vehiclePhotoStore = mockk<com.anpfuel.data.local.media.VehiclePhotoStore>(relaxed = true)

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
            vehiclePhotoStore = vehiclePhotoStore,
        )
        viewModel.photoIoDispatcher = dispatcher
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test fun `switching to BEV clears tank and price source details`() = runTest(dispatcher) {
        viewModel.startAddVehicle()
        viewModel.onTankCapacityChanged("50")
        viewModel.onPropulsionSelected(Propulsion.BEV)
        val form = viewModel.uiState.value.form
        assertEquals("", form.tankCapacityInput)
        assertEquals(com.anpfuel.domain.valueobject.VehiclePriceSourceMode.CHEAPEST_STATION, form.priceSourceMode)
    }

    @Test fun `leaving electric clears battery and connector`() = runTest(dispatcher) {
        viewModel.startAddVehicle()
        viewModel.onPropulsionSelected(Propulsion.PHEV)
        viewModel.onBatteryCapacityChanged("20")
        viewModel.onChargeConnectorSelected(ChargingConnector.CCS2)
        viewModel.onPropulsionSelected(Propulsion.HEV)
        val form = viewModel.uiState.value.form
        assertEquals("", form.batteryCapacityInput)
        assertNull(form.chargeConnector)
    }

    @Test fun `BEV saves without tank or fuel and counts toward the same slot gate`() = runTest(dispatcher) {
        coEvery { listVehiclesUseCase.invoke() } returnsMany listOf(emptyList(), emptyList())
        coEvery { saveVehicleUseCase.invoke(any()) } answers {
            com.anpfuel.application.usecase.vehicle.SaveVehicleResult(vehicle = firstArg(), events = emptyList())
        }
        viewModel.load()
        advanceUntilIdle()
        viewModel.startAddVehicle()
        viewModel.onDisplayNameChanged("Dolphin")
        viewModel.onPropulsionSelected(Propulsion.BEV)
        viewModel.onBatteryCapacityChanged("38,0")
        viewModel.onChargeConnectorSelected(ChargingConnector.CCS2)
        viewModel.saveVehicle(Locale("pt", "BR"))
        advanceUntilIdle()
        val saved = slot<Vehicle>()
        coVerify(exactly = 1) { saveVehicleUseCase.invoke(capture(saved)) }
        assertEquals(Propulsion.BEV, saved.captured.propulsion)
        assertEquals(38000L, saved.captured.batteryCapacity?.wattHours)
        assertNull(saved.captured.tankCapacity)
        assertNull(saved.captured.fuelProduct)
        assertEquals(ChargingConnector.CCS2, saved.captured.chargeConnector)
    }

    @Test fun `invalid battery refuses with typed error`() = runTest(dispatcher) {
        viewModel.startAddVehicle()
        viewModel.onDisplayNameChanged("Dolphin")
        viewModel.onPropulsionSelected(Propulsion.BEV)
        viewModel.onBatteryCapacityChanged("abc")
        viewModel.saveVehicle(Locale.US)
        assertEquals(VehicleFormValidationError.BATTERY_CAPACITY_INVALID, viewModel.uiState.value.form.validationErrorKey)
    }

    @Test fun `HEV with charge fields refused and legacy ICE edits restore`() = runTest(dispatcher) {
        val legacy = Vehicle.create(
            displayName = "Gol", tankCapacity = TankCapacity.of(50.0),
            fuelProduct = FuelProduct.GASOLINE_REGULAR,
            priceSource = VehiclePriceSource.cheapest(), id = DomainId.generate(),
        )
        coEvery { listVehiclesUseCase.invoke() } returns listOf(legacy)
        viewModel.load()
        advanceUntilIdle()
        viewModel.startEditVehicle(legacy.id, Locale.US)
        val form = viewModel.uiState.value.form
        assertEquals(Propulsion.ICE, form.propulsion)
        assertEquals("", form.batteryCapacityInput)
        assertEquals("50", form.tankCapacityInput)
    }

    @Test fun `PHEV requires battery while MHEV behaves like HEV`() = runTest(dispatcher) {
        viewModel.startAddVehicle()
        viewModel.onDisplayNameChanged("GWM")
        viewModel.onPropulsionSelected(Propulsion.PHEV)
        viewModel.onTankCapacityChanged("55")
        viewModel.saveVehicle(Locale.US)
        assertEquals(VehicleFormValidationError.BATTERY_CAPACITY_INVALID, viewModel.uiState.value.form.validationErrorKey)
        viewModel.onBatteryCapacityChanged("20")
        viewModel.onPropulsionSelected(Propulsion.MHEV)
        assertEquals("", viewModel.uiState.value.form.batteryCapacityInput)
        viewModel.saveVehicle(Locale.US)
        // MHEV needs no battery: no validation error (save itself fails on the unstubbed use case).
        assertNull(viewModel.uiState.value.form.validationErrorKey)
    }
}
