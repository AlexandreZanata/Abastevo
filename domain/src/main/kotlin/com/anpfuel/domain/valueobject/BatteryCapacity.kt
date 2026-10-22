package com.anpfuel.domain.valueobject

import com.anpfuel.domain.exception.DomainException

/**
 * Usable battery capacity in watt-hours (EE-19). Only BEV and PHEV
 * carry one; HEV/MHEV/ICE refuse it. Bounds mirror the backend energy
 * engine (1..1,000,000,000 Wh); display converts to kWh with the UI
 * locale, never here.
 */
class BatteryCapacity private constructor(
    val wattHours: Long,
) {
    init {
        if (wattHours < 1 || wattHours > MAX_WH) {
            throw DomainException("BatteryCapacity must be within 1..$MAX_WH Wh")
        }
    }

    override fun equals(other: Any?): Boolean =
        other is BatteryCapacity && wattHours == other.wattHours

    override fun hashCode(): Int = wattHours.hashCode()

    override fun toString(): String = "BatteryCapacity(wattHours=$wattHours)"

    companion object {
        const val MAX_WH = 1_000_000_000L

        fun ofWh(wattHours: Long): BatteryCapacity = BatteryCapacity(wattHours)
    }
}
