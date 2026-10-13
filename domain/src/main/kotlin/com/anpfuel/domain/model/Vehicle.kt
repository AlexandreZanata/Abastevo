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
    /**
     * Local-only photo file name (private app storage). Cloud sync is a
     * future premium feature; the photo never leaves the device today.
     */
    val photoPath: String? = null,
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

    fun withPhotoPath(photoPath: String?): Vehicle = copy(photoPath = photoPath)

    private fun copy(
        displayName: String = this.displayName,
        tankCapacity: TankCapacity = this.tankCapacity,
        fuelProduct: FuelProduct = this.fuelProduct,
        priceSource: VehiclePriceSource = this.priceSource,
        priceDropAlertEnabled: Boolean = this.priceDropAlertEnabled,
        sortOrder: Int = this.sortOrder,
        photoPath: String? = this.photoPath,
    ): Vehicle = Vehicle(
        id = id,
        displayName = displayName,
        tankCapacity = tankCapacity,
        fuelProduct = fuelProduct,
        priceSource = priceSource,
        priceDropAlertEnabled = priceDropAlertEnabled,
        sortOrder = sortOrder,
        photoPath = photoPath,
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
            photoPath: String? = null,
        ): Vehicle = Vehicle(
            id = id,
            displayName = displayName.trim(),
            tankCapacity = tankCapacity,
            fuelProduct = fuelProduct,
            priceSource = priceSource,
            priceDropAlertEnabled = priceDropAlertEnabled,
            sortOrder = sortOrder,
            photoPath = photoPath,
        )

        fun restore(
            id: DomainId,
            displayName: String,
            tankCapacity: TankCapacity,
            fuelProduct: FuelProduct,
            priceSource: VehiclePriceSource,
            priceDropAlertEnabled: Boolean,
            sortOrder: Int,
            photoPath: String? = null,
        ): Vehicle = Vehicle(
            id = id,
            displayName = displayName,
            tankCapacity = tankCapacity,
            fuelProduct = fuelProduct,
            priceSource = priceSource,
            priceDropAlertEnabled = priceDropAlertEnabled,
            sortOrder = sortOrder,
            photoPath = photoPath,
        )
    }
}
