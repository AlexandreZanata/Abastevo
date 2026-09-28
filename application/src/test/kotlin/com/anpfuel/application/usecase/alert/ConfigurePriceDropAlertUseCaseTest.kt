package com.anpfuel.application.usecase.alert

import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.repository.PriceDropNotificationRepository
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class ConfigurePriceDropAlertUseCaseTest {

    private val priceDropNotificationRepository = mockk<PriceDropNotificationRepository>(relaxed = true)
    private lateinit var useCase: ConfigurePriceDropAlertUseCase

    @BeforeEach
    fun setUp() {
        useCase = ConfigurePriceDropAlertUseCase(priceDropNotificationRepository)
    }

    @Test
    fun cancelsNotificationWhenAlertDisabled() = runTest {
        val vehicle = sampleVehicle(priceDropAlertEnabled = false)
        val previous = sampleVehicle(priceDropAlertEnabled = true)

        useCase(updated = vehicle, previous = previous)

        coVerify(exactly = 1) { priceDropNotificationRepository.cancelForVehicle(vehicle.id) }
    }

    @Test
    fun doesNotCancelWhenAlertStillEnabled() = runTest {
        val vehicle = sampleVehicle(priceDropAlertEnabled = true)
        val previous = sampleVehicle(priceDropAlertEnabled = true)

        useCase(updated = vehicle, previous = previous)

        coVerify(exactly = 0) { priceDropNotificationRepository.cancelForVehicle(any()) }
    }

    private fun sampleVehicle(priceDropAlertEnabled: Boolean): Vehicle =
        Vehicle.create(
            displayName = "Gol",
            tankCapacity = TankCapacity.of(50.0),
            fuelProduct = FuelProduct.GASOLINE_REGULAR,
            priceSource = VehiclePriceSource.cheapest(),
            priceDropAlertEnabled = priceDropAlertEnabled,
        )
}
