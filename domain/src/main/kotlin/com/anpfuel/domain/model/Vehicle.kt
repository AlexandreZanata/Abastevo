package com.anpfuel.domain.model

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource

/**
 * User-owned vehicle profile for tank fill cost and alerts (UC-010, BR-022).
 */
class Vehicle private constructor(
    val id: DomainId,
    val displayName: String,
    val tankCapacity: TankCapacity,
    val fuelProduct: FuelProduct,
    val priceSource: VehiclePriceSource,
    val priceDropAlertEnabled: Boolean,
    val sortOrder: Int,
) {
    init {
        if (displayName.isBlank()) {
            throw DomainException("Vehicle displayName must not be blank")
        }
        if (sortOrder < 0) {
            throw DomainException("Vehicle sortOrder must be non-negative")
        }
    }

    fun withDisplayName(displayName: String): Vehicle = copy(displayName = displayName.trim())

    fun withTankCapacity(tankCapacity: TankCapacity): Vehicle = copy(tankCapacity = tankCapacity)

    fun withFuelProduct(fuelProduct: FuelProduct): Vehicle = copy(fuelProduct = fuelProduct)

    fun withPriceSource(priceSource: VehiclePriceSource): Vehicle = copy(priceSource = priceSource)

    fun withPriceDropAlertEnabled(enabled: Boolean): Vehicle = copy(priceDropAlertEnabled = enabled)

    fun withSortOrder(sortOrder: Int): Vehicle = copy(sortOrder = sortOrder)

    private fun copy(
        displayName: String = this.displayName,
        tankCapacity: TankCapacity = this.tankCapacity,
        fuelProduct: FuelProduct = this.fuelProduct,
        priceSource: VehiclePriceSource = this.priceSource,
        priceDropAlertEnabled: Boolean = this.priceDropAlertEnabled,
        sortOrder: Int = this.sortOrder,
    ): Vehicle = Vehicle(
        id = id,
        displayName = displayName,
        tankCapacity = tankCapacity,
        fuelProduct = fuelProduct,
        priceSource = priceSource,
        priceDropAlertEnabled = priceDropAlertEnabled,
        sortOrder = sortOrder,
    )

    companion object {
        fun create(
            displayName: String,
            tankCapacity: TankCapacity,
            fuelProduct: FuelProduct,
            priceSource: VehiclePriceSource,
            priceDropAlertEnabled: Boolean = false,
            sortOrder: Int = 0,
            id: DomainId = DomainId.generate(),
        ): Vehicle = Vehicle(
            id = id,
            displayName = displayName.trim(),
            tankCapacity = tankCapacity,
            fuelProduct = fuelProduct,
            priceSource = priceSource,
            priceDropAlertEnabled = priceDropAlertEnabled,
            sortOrder = sortOrder,
        )

        fun restore(
            id: DomainId,
            displayName: String,
            tankCapacity: TankCapacity,
            fuelProduct: FuelProduct,
            priceSource: VehiclePriceSource,
            priceDropAlertEnabled: Boolean,
            sortOrder: Int,
        ): Vehicle = Vehicle(
            id = id,
            displayName = displayName,
            tankCapacity = tankCapacity,
            fuelProduct = fuelProduct,
            priceSource = priceSource,
            priceDropAlertEnabled = priceDropAlertEnabled,
            sortOrder = sortOrder,
        )
    }
}
