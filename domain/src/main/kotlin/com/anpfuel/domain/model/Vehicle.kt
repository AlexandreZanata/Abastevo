package com.anpfuel.domain.model

import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.valueobject.BatteryCapacity
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.Propulsion
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource

/**
 * User-owned vehicle profile for tank fill cost and alerts (UC-010, BR-022).
 *
 * Energy variants (EE-19, backend EE-10 parity): ICE/HEV/MHEV carry tank
 * and fuel without battery or connector; PHEV carries tank, fuel,
 * battery and optional connector; BEV carries battery and optional
 * connector with no tank and no fuel. Unknown propulsion is refused by
 * [Propulsion.valueOf] at the boundary; a missing propulsion on restore
 * means legacy ICE.
 */
class Vehicle private constructor(
    val id: DomainId,
    val displayName: String,
    val tankCapacity: TankCapacity?,
    val fuelProduct: FuelProduct?,
    val priceSource: VehiclePriceSource,
    val priceDropAlertEnabled: Boolean,
    val sortOrder: Int,
    val propulsion: Propulsion,
    val batteryCapacity: BatteryCapacity?,
    val chargeConnector: ChargingConnector?,
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
        val electric = propulsion == Propulsion.BEV || propulsion == Propulsion.PHEV
        if (electric != (batteryCapacity != null)) {
            throw DomainException("Battery belongs to BEV/PHEV only")
        }
        if (!electric && chargeConnector != null) {
            throw DomainException("Charge connector belongs to BEV/PHEV only")
        }
        if (propulsion == Propulsion.BEV && (tankCapacity != null || fuelProduct != null)) {
            throw DomainException("BEV carries no tank and no fuel")
        }
        if (propulsion != Propulsion.BEV && (tankCapacity == null || fuelProduct == null)) {
            throw DomainException("Non-BEV vehicles require tank and fuel")
        }
    }

    fun withDisplayName(displayName: String): Vehicle = copy(displayName = displayName.trim())

    fun withTankCapacity(tankCapacity: TankCapacity?): Vehicle = copy(tankCapacity = tankCapacity)

    fun withFuelProduct(fuelProduct: FuelProduct?): Vehicle = copy(fuelProduct = fuelProduct)

    fun withPriceSource(priceSource: VehiclePriceSource): Vehicle = copy(priceSource = priceSource)

    fun withPriceDropAlertEnabled(enabled: Boolean): Vehicle = copy(priceDropAlertEnabled = enabled)

    fun withSortOrder(sortOrder: Int): Vehicle = copy(sortOrder = sortOrder)

    fun withPropulsion(propulsion: Propulsion): Vehicle = copy(propulsion = propulsion)

    fun withBatteryCapacity(batteryCapacity: BatteryCapacity?): Vehicle = copy(batteryCapacity = batteryCapacity)

    fun withChargeConnector(chargeConnector: ChargingConnector?): Vehicle = copy(chargeConnector = chargeConnector)

    /**
     * Atomic energy-variant transition (EE-19): chained single-field
     * copies would validate impossible intermediate states (e.g. an ICE
     * holding a null tank on the way to BEV), so variant edits apply
     * propulsion, tank, fuel, battery and connector in one step.
     */
    fun withEnergyVariant(
        propulsion: Propulsion,
        tankCapacity: TankCapacity?,
        fuelProduct: FuelProduct?,
        batteryCapacity: BatteryCapacity?,
        chargeConnector: ChargingConnector?,
    ): Vehicle = copy(
        propulsion = propulsion,
        tankCapacity = tankCapacity,
        fuelProduct = fuelProduct,
        batteryCapacity = batteryCapacity,
        chargeConnector = chargeConnector,
    )

    fun withPhotoPath(photoPath: String?): Vehicle = copy(photoPath = photoPath)

    private fun copy(
        displayName: String = this.displayName,
        tankCapacity: TankCapacity? = this.tankCapacity,
        fuelProduct: FuelProduct? = this.fuelProduct,
        priceSource: VehiclePriceSource = this.priceSource,
        priceDropAlertEnabled: Boolean = this.priceDropAlertEnabled,
        sortOrder: Int = this.sortOrder,
        propulsion: Propulsion = this.propulsion,
        batteryCapacity: BatteryCapacity? = this.batteryCapacity,
        chargeConnector: ChargingConnector? = this.chargeConnector,
        photoPath: String? = this.photoPath,
    ): Vehicle = Vehicle(
        id = id,
        displayName = displayName,
        tankCapacity = tankCapacity,
        fuelProduct = fuelProduct,
        priceSource = priceSource,
        priceDropAlertEnabled = priceDropAlertEnabled,
        sortOrder = sortOrder,
        propulsion = propulsion,
        batteryCapacity = batteryCapacity,
        chargeConnector = chargeConnector,
        photoPath = photoPath,
    )

    companion object {
        fun create(
            displayName: String,
            tankCapacity: TankCapacity?,
            fuelProduct: FuelProduct?,
            priceSource: VehiclePriceSource,
            priceDropAlertEnabled: Boolean = false,
            sortOrder: Int = 0,
            id: DomainId = DomainId.generate(),
            propulsion: Propulsion = Propulsion.ICE,
            batteryCapacity: BatteryCapacity? = null,
            chargeConnector: ChargingConnector? = null,
            photoPath: String? = null,
        ): Vehicle = Vehicle(
            id = id,
            displayName = displayName.trim(),
            tankCapacity = tankCapacity,
            fuelProduct = fuelProduct,
            priceSource = priceSource,
            priceDropAlertEnabled = priceDropAlertEnabled,
            sortOrder = sortOrder,
            propulsion = propulsion,
            batteryCapacity = batteryCapacity,
            chargeConnector = chargeConnector,
            photoPath = photoPath,
        )

        fun restore(
            id: DomainId,
            displayName: String,
            tankCapacity: TankCapacity?,
            fuelProduct: FuelProduct?,
            priceSource: VehiclePriceSource,
            priceDropAlertEnabled: Boolean,
            sortOrder: Int,
            propulsion: Propulsion = Propulsion.ICE,
            batteryCapacity: BatteryCapacity? = null,
            chargeConnector: ChargingConnector? = null,
            photoPath: String? = null,
        ): Vehicle = Vehicle(
            id = id,
            displayName = displayName,
            tankCapacity = tankCapacity,
            fuelProduct = fuelProduct,
            priceSource = priceSource,
            priceDropAlertEnabled = priceDropAlertEnabled,
            sortOrder = sortOrder,
            propulsion = propulsion,
            batteryCapacity = batteryCapacity,
            chargeConnector = chargeConnector,
            photoPath = photoPath,
        )
    }
}
