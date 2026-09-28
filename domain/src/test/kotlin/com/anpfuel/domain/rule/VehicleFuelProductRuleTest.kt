package com.anpfuel.domain.rule

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class VehicleFuelProductRuleTest {

    private val vehicle = Vehicle.create(
        displayName = "Gol",
        tankCapacity = TankCapacity.of(50.0),
        fuelProduct = FuelProduct.GASOLINE_REGULAR,
        priceSource = VehiclePriceSource.cheapest(),
    )

    @Test
    fun requireMatchingFuelAcceptsBoundProduct() {
        VehicleFuelProductRule.requireMatchingFuel(vehicle, FuelProduct.GASOLINE_REGULAR)
    }

    @Test
    fun requireMatchingFuelThrowsWhenFuelDiffers() {
        assertThrows(DomainException::class.java) {
            VehicleFuelProductRule.requireMatchingFuel(vehicle, FuelProduct.ETHANOL)
        }
    }

    @Test
    fun requireFuelProductThrowsWhenNull() {
        assertThrows(DomainException::class.java) {
            VehicleFuelProductRule.requireFuelProduct(null)
        }
    }
}
