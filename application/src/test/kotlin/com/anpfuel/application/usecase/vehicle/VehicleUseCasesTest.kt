package com.anpfuel.application.usecase.vehicle

import com.anpfuel.domain.event.PriceDropAlertConfigured
import com.anpfuel.domain.event.VehicleRegistered
import com.anpfuel.domain.event.VehicleRemoved
import com.anpfuel.domain.event.VehicleUpdated
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.repository.DomainEventPublisher
import com.anpfuel.domain.repository.VehicleRepository
import com.anpfuel.domain.rule.MaxRegisteredVehiclesRule
import com.anpfuel.domain.valueobject.Cnpj
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class VehicleUseCasesTest {

    private val vehicleRepository = mockk<VehicleRepository>()
    private val eventPublisher = mockk<DomainEventPublisher>()

    private lateinit var listVehiclesUseCase: ListVehiclesUseCase
    private lateinit var getVehicleUseCase: GetVehicleUseCase
    private lateinit var saveVehicleUseCase: SaveVehicleUseCase
    private lateinit var deleteVehicleUseCase: DeleteVehicleUseCase

    @BeforeEach
    fun setUp() {
        listVehiclesUseCase = ListVehiclesUseCase(vehicleRepository)
        getVehicleUseCase = GetVehicleUseCase(vehicleRepository)
        saveVehicleUseCase = SaveVehicleUseCase(
            vehicleRepository = vehicleRepository,
            eventPublisher = eventPublisher,
        )
        deleteVehicleUseCase = DeleteVehicleUseCase(
            vehicleRepository = vehicleRepository,
            eventPublisher = eventPublisher,
        )

        coEvery { eventPublisher.publish(any()) } returns Unit
        coEvery { vehicleRepository.save(any()) } returns Unit
        coEvery { vehicleRepository.delete(any()) } returns Unit
    }

    @Test
    fun listVehiclesReturnsPersistedProfiles() = runTest {
        val vehicles = listOf(sampleVehicle(displayName = "Gol"))
        coEvery { vehicleRepository.listAll() } returns vehicles

        assertEquals(vehicles, listVehiclesUseCase.invoke())
    }

    @Test
    fun getVehicleReturnsNullWhenMissing() = runTest {
        val id = DomainId.generate()
        coEvery { vehicleRepository.findById(id) } returns null

        assertNull(getVehicleUseCase.invoke(id))
    }

    @Test
    fun saveNewVehiclePublishesVehicleRegistered() = runTest {
        val vehicle = sampleVehicle()
        coEvery { vehicleRepository.findById(vehicle.id) } returns null
        coEvery { vehicleRepository.count() } returns 0

        val result = saveVehicleUseCase.invoke(vehicle)

        assertEquals(vehicle, result.vehicle)
        assertTrue(result.events.any { it is VehicleRegistered })
        coVerify(exactly = 1) { vehicleRepository.save(vehicle) }
    }

    @Test
    fun saveUpdatedVehiclePublishesVehicleUpdated() = runTest {
        val existing = sampleVehicle(displayName = "Gol")
        val updated = existing.withDisplayName("Gol 1.0")
        coEvery { vehicleRepository.findById(updated.id) } returns existing

        val result = saveVehicleUseCase.invoke(updated)

        assertTrue(result.events.any { it is VehicleUpdated })
        coVerify(exactly = 1) { vehicleRepository.save(updated) }
    }

    @Test
    fun saveVehicleWithAlertEnabledPublishesPriceDropAlertConfigured() = runTest {
        val vehicle = sampleVehicle(priceDropAlertEnabled = true)
        coEvery { vehicleRepository.findById(vehicle.id) } returns null
        coEvery { vehicleRepository.count() } returns 0

        val result = saveVehicleUseCase.invoke(vehicle)

        assertTrue(result.events.any { it is PriceDropAlertConfigured })
    }

    @Test
    fun saveVehicleRejectsFourthRegistration() = runTest {
        val vehicle = sampleVehicle()
        coEvery { vehicleRepository.findById(vehicle.id) } returns null
        coEvery { vehicleRepository.count() } returns MaxRegisteredVehiclesRule.MAX_VEHICLES

        assertThrows(DomainException::class.java) {
            runBlocking {
                saveVehicleUseCase.invoke(vehicle)
            }
        }
    }

    @Test
    fun deleteVehiclePublishesVehicleRemoved() = runTest {
        val id = DomainId.generate()
        val published = slot<com.anpfuel.domain.event.DomainEvent>()

        val result = deleteVehicleUseCase.invoke(id)

        assertEquals(id, result.vehicleId)
        assertTrue(result.event is VehicleRemoved)
        coVerify(exactly = 1) { vehicleRepository.delete(id) }
        coVerify(exactly = 1) { eventPublisher.publish(capture(published)) }
        assertTrue(published.captured is VehicleRemoved)
    }

    private fun sampleVehicle(
        displayName: String = "Civic",
        priceDropAlertEnabled: Boolean = false,
    ): Vehicle = Vehicle.create(
        displayName = displayName,
        tankCapacity = TankCapacity.of(50.0),
        fuelProduct = FuelProduct.GASOLINE_REGULAR,
        priceSource = VehiclePriceSource.specific(Cnpj.parse("12345678000195")),
        priceDropAlertEnabled = priceDropAlertEnabled,
    )
}
