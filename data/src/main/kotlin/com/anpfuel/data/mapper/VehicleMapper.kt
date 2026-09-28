package com.anpfuel.data.mapper

import com.anpfuel.data.local.entity.VehicleEntity
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.valueobject.Cnpj
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource
import com.anpfuel.domain.valueobject.VehiclePriceSourceMode

object VehicleMapper {

    fun toDomain(entity: VehicleEntity): Vehicle {
        val fuelProduct = runCatching { FuelProduct.valueOf(entity.fuelProduct) }
            .getOrElse { throw DomainException("Unknown FuelProduct stored for vehicle ${entity.id}") }
        val priceSourceMode = runCatching { VehiclePriceSourceMode.valueOf(entity.priceSourceMode) }
            .getOrElse { throw DomainException("Unknown VehiclePriceSourceMode stored for vehicle ${entity.id}") }
        val priceSource = when (priceSourceMode) {
            VehiclePriceSourceMode.CHEAPEST_STATION -> VehiclePriceSource.cheapest()
            VehiclePriceSourceMode.SPECIFIC_STATION -> {
                val cnpjRaw = entity.specificStationCnpj
                    ?: throw DomainException("SPECIFIC_STATION vehicle ${entity.id} is missing CNPJ")
                VehiclePriceSource.specific(Cnpj.parse(cnpjRaw))
            }
        }

        return Vehicle.restore(
            id = DomainId.from(entity.id),
            displayName = entity.displayName,
            tankCapacity = TankCapacity.of(entity.tankCapacityLiters),
            fuelProduct = fuelProduct,
            priceSource = priceSource,
            priceDropAlertEnabled = entity.priceDropAlertEnabled,
            sortOrder = entity.sortOrder,
        )
    }

    fun toEntity(vehicle: Vehicle): VehicleEntity = VehicleEntity(
        id = vehicle.id.value,
        displayName = vehicle.displayName,
        tankCapacityLiters = vehicle.tankCapacity.liters.toDouble(),
        fuelProduct = vehicle.fuelProduct.name,
        priceSourceMode = vehicle.priceSource.mode.name,
        specificStationCnpj = vehicle.priceSource.specificStationCnpj?.digits,
        priceDropAlertEnabled = vehicle.priceDropAlertEnabled,
        sortOrder = vehicle.sortOrder,
    )
}
