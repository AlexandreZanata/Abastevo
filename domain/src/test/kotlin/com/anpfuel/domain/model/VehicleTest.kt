package com.anpfuel.domain.model

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.valueobject.Cnpj
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class VehicleTest {

    @Test
    fun createStoresSingleFuelProductAndPriceSource() {
        val vehicle = Vehicle.create(
            displayName = "Gol 1.0",
            tankCapacity = TankCapacity.of(50.0),
            fuelProduct = FuelProduct.GASOLINE_REGULAR,
            priceSource = VehiclePriceSource.cheapest(),
            priceDropAlertEnabled = true,
            sortOrder = 1,
        )

        assertEquals("Gol 1.0", vehicle.displayName)
        assertEquals(FuelProduct.GASOLINE_REGULAR, vehicle.fuelProduct)
        assertEquals(VehiclePriceSource.cheapest(), vehicle.priceSource)
        assertTrue(vehicle.priceDropAlertEnabled)
        assertEquals(1, vehicle.sortOrder)
    }

    @Test
    fun specificStationPriceSourceRequiresCnpj() {
        val vehicle = Vehicle.create(
            displayName = "Onix",
            tankCapacity = TankCapacity.of(44.0),
            fuelProduct = FuelProduct.ETHANOL,
            priceSource = VehiclePriceSource.specific(Cnpj.parse("12345678000195")),
        )

        assertEquals(Cnpj.parse("12345678000195"), vehicle.priceSource.specificStationCnpj)
    }

    @Test
    fun withMethodsReturnUpdatedVehicle() {
        val original = Vehicle.create(
            displayName = "Civic",
            tankCapacity = TankCapacity.of(47.0),
            fuelProduct = FuelProduct.GASOLINE_PREMIUM,
            priceSource = VehiclePriceSource.cheapest(),
        )

        val updated = original
            .withDisplayName("Civic Touring")
            .withTankCapacity(TankCapacity.of(48.0))
            .withFuelProduct(FuelProduct.GASOLINE_REGULAR)
            .withPriceSource(VehiclePriceSource.specific(Cnpj.parse("12345678000195")))
            .withPriceDropAlertEnabled(true)
            .withSortOrder(2)

        assertEquals(original.id, updated.id)
        assertEquals("Civic Touring", updated.displayName)
        assertEquals(FuelProduct.GASOLINE_REGULAR, updated.fuelProduct)
        assertTrue(updated.priceDropAlertEnabled)
        assertEquals(2, updated.sortOrder)
    }

    @Test
    fun blankDisplayNameThrowsDomainException() {
        assertThrows(DomainException::class.java) {
            Vehicle.create(
                displayName = " ",
                tankCapacity = TankCapacity.of(50.0),
                fuelProduct = FuelProduct.GASOLINE_REGULAR,
                priceSource = VehiclePriceSource.cheapest(),
            )
        }
    }

    @Test
    fun negativeSortOrderThrowsDomainException() {
        assertThrows(DomainException::class.java) {
            Vehicle.create(
                displayName = "Gol",
                tankCapacity = TankCapacity.of(50.0),
                fuelProduct = FuelProduct.GASOLINE_REGULAR,
                priceSource = VehiclePriceSource.cheapest(),
                sortOrder = -1,
            )
        }
    }

    @Test
    fun restorePreservesId() {
        val id = DomainId.generate()
        val restored = Vehicle.restore(
            id = id,
            displayName = "HB20",
            tankCapacity = TankCapacity.of(50.0),
            fuelProduct = FuelProduct.ETHANOL,
            priceSource = VehiclePriceSource.cheapest(),
            priceDropAlertEnabled = false,
            sortOrder = 0,
        )

        assertEquals(id, restored.id)
        assertFalse(restored.priceDropAlertEnabled)
    }
}
