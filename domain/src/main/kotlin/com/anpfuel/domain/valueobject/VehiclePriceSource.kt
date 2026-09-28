package com.anpfuel.domain.valueobject

import com.anpfuel.domain.exception.DomainException

/**
 * Price reference for tank fill cost and price drop alerts.
 */
class VehiclePriceSource internal constructor(
    val mode: VehiclePriceSourceMode,
    val specificStationCnpj: Cnpj?,
) {
    init {
        when (mode) {
            VehiclePriceSourceMode.SPECIFIC_STATION -> {
                if (specificStationCnpj == null) {
                    throw DomainException("SPECIFIC_STATION requires a CNPJ")
                }
            }
            VehiclePriceSourceMode.CHEAPEST_STATION -> {
                if (specificStationCnpj != null) {
                    throw DomainException("CHEAPEST_STATION must not include a CNPJ")
                }
            }
        }
    }

    override fun equals(other: Any?): Boolean =
        other is VehiclePriceSource &&
            mode == other.mode &&
            specificStationCnpj == other.specificStationCnpj

    override fun hashCode(): Int = 31 * mode.hashCode() + (specificStationCnpj?.hashCode() ?: 0)

    override fun toString(): String = "VehiclePriceSource(mode=$mode, cnpj=$specificStationCnpj)"

    companion object {
        fun cheapest(): VehiclePriceSource =
            VehiclePriceSource(VehiclePriceSourceMode.CHEAPEST_STATION, null)

        fun specific(cnpj: Cnpj): VehiclePriceSource =
            VehiclePriceSource(VehiclePriceSourceMode.SPECIFIC_STATION, cnpj)
    }
}
