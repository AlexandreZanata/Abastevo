package com.anpfuel.domain.driver

enum class DriverDistanceBasis { ODOMETER, GPS_ESTIMATE, PAID_REPORTED, MANUAL_TRIP, ROUTE_PREDICTION }
data class DriverDistanceObservation(val id: String, val window: DriverWindow, val segment: String, val basis: DriverDistanceBasis,
                                     val metres: Long?, val complete: Boolean, val hasGap: Boolean = false) {
    init { require(id.isNotBlank() && segment.isNotBlank()); metres?.let(::amount) }
}
data class DriverDistanceResult(val metres: Long? = null, val basis: DriverDistanceBasis, val unknown: String = "")
data class DriverDistancePartition(val emptyMetres: Long? = null, val paidPercent: String? = null, val unknown: String = "")
object DriverDistance {
    fun select(window: DriverWindow, basis: DriverDistanceBasis, observations: List<DriverDistanceObservation>): DriverDistanceResult {
        require(observations.size <= DRIVER_MAX_ITEMS && observations.map { it.id }.toSet().size == observations.size)
        fun unknown(reason: String) = DriverDistanceResult(basis = basis, unknown = reason)
        if (basis == DriverDistanceBasis.ROUTE_PREDICTION) return unknown("prediction-not-measured")
        if (observations.any { it.window != window }) return unknown("unmatched-period-window")
        val candidates = observations.filter { it.basis == basis }
        if (candidates.size != 1) return unknown("distance-source-ambiguous-or-missing")
        val selected = candidates.single()
        if (selected.hasGap || !selected.complete || selected.metres == null) return unknown("distance-coverage-incomplete")
        return DriverDistanceResult(selected.metres, basis)
    }
    fun odometer(start: Long, end: Long, startSegment: String, endSegment: String): DriverDistanceResult {
        amount(start); amount(end); require(startSegment.isNotBlank() && endSegment.isNotBlank())
        return when {
            startSegment != endSegment -> DriverDistanceResult(basis = DriverDistanceBasis.ODOMETER, unknown = "odometer-segment-boundary")
            end < start -> DriverDistanceResult(basis = DriverDistanceBasis.ODOMETER, unknown = "reversed-odometer")
            else -> DriverDistanceResult(end - start, DriverDistanceBasis.ODOMETER)
        }
    }
    fun partition(total: Long, work: Long?, paid: Long?, personal: Long?, complete: Boolean): DriverDistancePartition {
        amount(total)
        listOfNotNull(work, paid, personal).forEach { amount(it); require(it <= total) }
        if (paid != null && work != null) require(paid <= work)
        if (!complete || work == null || paid == null || personal == null) return DriverDistancePartition(unknown = "distance-classification-incomplete")
        require(work.toBigInteger() + personal.toBigInteger() == total.toBigInteger())
        if (work == 0L) return DriverDistancePartition(work - paid, unknown = "work-distance-nonpositive")
        val numerator = paid.toBigInteger() * 100.toBigInteger(); val denominator = work.toBigInteger(); val gcd = numerator.gcd(denominator)
        val percent = if (denominator / gcd == java.math.BigInteger.ONE) (numerator / gcd).toString() else "${numerator / gcd}/${denominator / gcd}"
        return DriverDistancePartition(work - paid, percent)
    }
}
