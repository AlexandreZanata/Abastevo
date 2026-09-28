package com.anpfuel.domain.rule

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.valueobject.FuelProduct

/**
 * BR-022 — Each vehicle binds exactly one [FuelProduct].
 */
object VehicleFuelProductRule {

    fun requireVehicleFuelProduct(vehicle: Vehicle) {
        requireFuelProduct(vehicle.fuelProduct)
    }

    fun requireMatchingFuel(vehicle: Vehicle, fuelProduct: FuelProduct) {
        if (vehicle.fuelProduct != fuelProduct) {
            throw DomainException(
                "Vehicle ${vehicle.id.value} is bound to ${vehicle.fuelProduct}, not $fuelProduct",
            )
        }
    }

    fun requireFuelProduct(fuelProduct: FuelProduct?) {
        if (fuelProduct == null) {
            throw DomainException("Vehicle must have exactly one FuelProduct")
        }
    }
}
