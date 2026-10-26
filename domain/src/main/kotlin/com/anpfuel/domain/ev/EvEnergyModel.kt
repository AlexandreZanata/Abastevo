package com.anpfuel.domain.ev

import java.math.BigInteger

/** MAP-26 result: a number with its conservative rounding, or a stable reason; never a zero default. */
sealed interface EnergyEstimate<out T> {
    data class Known<T>(val value: T) : EnergyEstimate<T>
    data class Unknown(val reason: String) : EnergyEstimate<Nothing>
}

enum class EnergyScope { BATTERY, ELECTRIC_PORTION_ONLY }

/**
 * Owner-declared battery facts. Capacity is the USEFUL capacity in Wh (never a nominal label assumed
 * useful); soc/reserve are basis points of it; a PHEV declares the permille of that capacity available
 * to electric driving. A null field is "missing", not zero.
 */
data class BatteryInput(
    val capacityWh: Long?,
    val socBp: Long?,
    val reserveBp: Long?,
    val usablePermille: Long? = null,
)

/** Consumption in milli-Wh per km (150 Wh/km = 150,000) plus an optional non-negative margin in permille. */
data class ConsumptionInput(val consMilliWhPerKm: Long?, val marginPermille: Long? = 0)

data class CalibrationSession(val energyWh: Long, val distanceMetres: Long)

/**
 * Pure conservative energy model (docs/product/MAP_26_CONTRACT.md). Integer/rational arithmetic only:
 * usable energy, reach and arrival state of charge round down; consumed energy and calibrated
 * consumption round up. It estimates; it never guarantees a trip.
 */
object EvEnergyModel {
    const val MAX_ENERGY_WH = 1_000_000_000_000L
    const val MAX_DISTANCE_METRES = 1_000_000_000_000L
    const val MAX_CONSUMPTION_MILLI = 1_000_000_000L
    const val MAX_MARGIN_PERMILLE = 1_000L
    const val MIN_CALIBRATION_METRES = 20_000L

    private fun big(v: Long) = BigInteger.valueOf(v)
    private fun floorDiv(n: BigInteger, d: BigInteger): BigInteger = n.divide(d) // operands are non-negative
    private fun ceilDiv(n: BigInteger, d: BigInteger): BigInteger = n.add(d).subtract(BigInteger.ONE).divide(d)
    private fun unknown(reason: String) = EnergyEstimate.Unknown(reason)

    fun scopeOf(battery: BatteryInput): EnergyScope =
        if (battery.usablePermille != null) EnergyScope.ELECTRIC_PORTION_ONLY else EnergyScope.BATTERY

    private class Checked(val capacity: Long, val soc: Long, val reserve: Long, val portion: Long?)

    private fun check(b: BatteryInput): Pair<Checked?, EnergyEstimate.Unknown?> {
        val capacity = b.capacityWh ?: return null to unknown("capacity-missing")
        val soc = b.socBp ?: return null to unknown("soc-missing")
        val reserve = b.reserveBp ?: return null to unknown("reserve-missing")
        if (capacity !in 1..MAX_ENERGY_WH || soc !in 0..10_000 || reserve !in 0..10_000) return null to unknown("input-out-of-range")
        val portion = b.usablePermille
        if (portion != null && portion !in 1..1_000) return null to unknown("input-out-of-range")
        return Checked(capacity, soc, reserve, portion) to null
    }

    /** capacity x (soc - reserve) / 10,000 [x portion / 1,000], floor; at or below reserve is a known 0. */
    fun usableEnergyWh(battery: BatteryInput): EnergyEstimate<Long> {
        val (c, error) = check(battery)
        if (c == null) return error!!
        val above = maxOf(c.soc - c.reserve, 0L)
        var numerator = big(c.capacity) * big(above)
        var denominator = big(10_000)
        if (c.portion != null) { numerator *= big(c.portion); denominator *= big(1_000) }
        return EnergyEstimate.Known(floorDiv(numerator, denominator).toLong())
    }

    /** Effective consumption as numerator over 1000 x 1000 (milli x permille), or the refusal. */
    private fun effective(c: ConsumptionInput): Pair<BigInteger?, EnergyEstimate.Unknown?> {
        val cons = c.consMilliWhPerKm ?: return null to unknown("consumption-missing")
        if (cons <= 0) return null to unknown("consumption-nonpositive")
        if (cons > MAX_CONSUMPTION_MILLI) return null to unknown("input-out-of-range")
        val margin = c.marginPermille ?: 0L
        if (margin !in 0..MAX_MARGIN_PERMILLE) return null to unknown("margin-invalid")
        return big(cons) * big(1_000 + margin) to null
    }

    private fun checkDistance(d: Long?): EnergyEstimate.Unknown? = when {
        d == null -> unknown("distance-missing")
        d < 0 -> unknown("negative-input")
        d > MAX_DISTANCE_METRES -> unknown("input-out-of-range")
        else -> null
    }

    /** Energy for a segment: distance x consumption x (1 + margin) / 10^6, rounded up. */
    fun segmentEnergyWh(distanceMetres: Long?, consumption: ConsumptionInput): EnergyEstimate<Long> {
        val (eff, error) = effective(consumption)
        if (eff == null) return error!!
        checkDistance(distanceMetres)?.let { return it }
        val wh = ceilDiv(big(distanceMetres!!) * eff, big(1_000_000_000L))
        return if (wh > big(MAX_ENERGY_WH)) unknown("out-of-bounds") else EnergyEstimate.Known(wh.toLong())
    }

    /** Distance the usable energy covers at the effective consumption, rounded down; not a guarantee. */
    fun reachableDistanceMetres(battery: BatteryInput, consumption: ConsumptionInput): EnergyEstimate<Long> {
        val usable = when (val u = usableEnergyWh(battery)) {
            is EnergyEstimate.Unknown -> return u
            is EnergyEstimate.Known -> u.value
        }
        val (eff, error) = effective(consumption)
        if (eff == null) return error!!
        val metres = floorDiv(big(usable) * big(1_000_000_000L), eff)
        return if (metres > big(MAX_DISTANCE_METRES)) unknown("out-of-bounds") else EnergyEstimate.Known(metres.toLong())
    }

    /** State of charge (bp of useful capacity) after a segment, floor; may fall below any reserve. */
    fun arrivalSocBp(capacityWh: Long?, socBp: Long?, distanceMetres: Long?, consumption: ConsumptionInput): EnergyEstimate<Long> {
        val capacity = capacityWh ?: return unknown("capacity-missing")
        val soc = socBp ?: return unknown("soc-missing")
        val (eff, error) = effective(consumption)
        if (eff == null) return error!!
        if (distanceMetres == null) return unknown("distance-missing")
        if (capacity !in 1..MAX_ENERGY_WH || soc !in 0..10_000) return unknown("input-out-of-range")
        checkDistance(distanceMetres)?.let { return it }
        val needed = ceilDiv(big(distanceMetres) * eff, big(1_000_000_000L))
        // stored energy in units of 1/10,000 Wh to stay exact: capacity x soc
        val storedScaled = big(capacity) * big(soc)
        val neededScaled = needed * big(10_000)
        if (neededScaled > storedScaled) return unknown("energy-exceeds-battery")
        return EnergyEstimate.Known(floorDiv((storedScaled - neededScaled), big(capacity)).toLong())
    }

    /**
     * LOWER BOUND on charging time: delivered power cannot exceed the declared power, so energy x 3,600 /
     * power (floor) is the fastest conceivable time. Curve, acceptance and state of charge are unknown.
     */
    fun minimumChargeSeconds(energyWh: Long?, declaredPowerW: Long?): EnergyEstimate<Long> {
        if (energyWh == null) return unknown("energy-missing")
        if (declaredPowerW == null) return unknown("power-missing")
        if (energyWh < 0) return unknown("negative-input")
        if (declaredPowerW <= 0) return unknown("power-nonpositive")
        if (energyWh > MAX_ENERGY_WH) return unknown("input-out-of-range")
        return EnergyEstimate.Known(floorDiv(big(energyWh) * big(3_600), big(declaredPowerW)).toLong())
    }

    /** Sum-first calibration from completed private sessions: total Wh / total km, rounded up, >= 20 km. */
    fun calibrateConsumptionMilli(sessions: List<CalibrationSession>): EnergyEstimate<Long> {
        if (sessions.isEmpty()) return unknown("calibration-insufficient")
        if (sessions.any { it.energyWh <= 0 || it.distanceMetres <= 0 || it.energyWh > MAX_ENERGY_WH || it.distanceMetres > MAX_DISTANCE_METRES }) {
            return unknown("session-invalid")
        }
        val energy = sessions.fold(BigInteger.ZERO) { a, s -> a + big(s.energyWh) }
        val distance = sessions.fold(BigInteger.ZERO) { a, s -> a + big(s.distanceMetres) }
        if (distance < big(MIN_CALIBRATION_METRES)) return unknown("calibration-insufficient")
        val milli = ceilDiv(energy * big(1_000_000L), distance)
        return if (milli > big(MAX_CONSUMPTION_MILLI)) unknown("out-of-bounds") else EnergyEstimate.Known(milli.toLong())
    }
}
