package com.anpfuel.data.mapper

import com.anpfuel.data.local.entity.VehicleEntity
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.valueobject.Cnpj
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource
import com.anpfuel.domain.valueobject.VehiclePriceSourceMode
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class VehicleMapperTest {

    @Test
    fun roundTripPreservesVehicleFields() {
        val vehicle = Vehicle.create(
            id = DomainId.from("vehicle-1"),
            displayName = "Gol 1.0",
            tankCapacity = TankCapacity.of(50.0),
            fuelProduct = FuelProduct.GASOLINE_REGULAR,
            priceSource = VehiclePriceSource.specific(Cnpj.parse("12345678000195")),
            priceDropAlertEnabled = true,
            sortOrder = 2,
        )

        val entity = VehicleMapper.toEntity(vehicle)
        val restored = VehicleMapper.toDomain(entity)

        assertEquals(
            VehicleEntity(
                id = "vehicle-1",
                displayName = "Gol 1.0",
                tankCapacityLiters = 50.0,
                fuelProduct = FuelProduct.GASOLINE_REGULAR.name,
                priceSourceMode = VehiclePriceSourceMode.SPECIFIC_STATION.name,
                specificStationCnpj = "12345678000195",
                priceDropAlertEnabled = true,
                sortOrder = 2,
            ),
            entity,
        )
        assertEquals(vehicle.displayName, restored.displayName)
        assertEquals(vehicle.tankCapacity, restored.tankCapacity)
        assertEquals(vehicle.fuelProduct, restored.fuelProduct)
        assertEquals(vehicle.priceSource, restored.priceSource)
        assertEquals(vehicle.priceDropAlertEnabled, restored.priceDropAlertEnabled)
        assertEquals(vehicle.sortOrder, restored.sortOrder)
    }

    @Test
    fun cheapestPriceSourceMapsWithoutCnpj() {
        val vehicle = Vehicle.create(
            displayName = "Onix",
            tankCapacity = TankCapacity.of(44.0),
            fuelProduct = FuelProduct.ETHANOL,
            priceSource = VehiclePriceSource.cheapest(),
        )

        val entity = VehicleMapper.toEntity(vehicle)

        assertEquals(VehiclePriceSourceMode.CHEAPEST_STATION.name, entity.priceSourceMode)
        assertEquals(null, entity.specificStationCnpj)

        val restored = VehicleMapper.toDomain(entity)
        assertEquals(vehicle.displayName, restored.displayName)
        assertEquals(vehicle.fuelProduct, restored.fuelProduct)
        assertEquals(vehicle.priceSource.mode, restored.priceSource.mode)
    }
}
