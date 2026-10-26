package com.anpfuel.domain.driver

import java.math.BigInteger
import java.time.LocalDate
import java.time.ZoneId

private val costId = Regex("[A-Za-z0-9._-]{1,64}")
data class DriverWeight(val id: String, val value: Long)
data class DriverAllocation(val amounts: Map<String, Long> = emptyMap(), val unknown: String = "")
data class DriverAllocationSpan(val id: String, val span: DriverTimeSpan)
data class DriverCalendarSpan(val id: String, val from: String, val to: String)
object DriverAllocationRules {
    fun allocate(source: Long?, weights: List<DriverWeight>): DriverAllocation {
        if (source == null) return DriverAllocation(unknown = "source-amount-missing")
        amount(source); require(weights.size <= DRIVER_MAX_ITEMS && weights.map { it.id }.toSet().size == weights.size)
        weights.forEach { require(costId.matches(it.id)); amount(it.value) }
        val denominator = weights.fold(BigInteger.ZERO) { a, w -> a + w.value.toBigInteger() }
        if (denominator == BigInteger.ZERO) return DriverAllocation(unknown = "allocation-basis-missing")
        val amounts = mutableMapOf<String, Long>()
        val remainders = weights.map { w ->
            val (q, r) = (source.toBigInteger() * w.value.toBigInteger()).divideAndRemainder(denominator)
            amounts[w.id] = q.driverLongExact()
            w.id to r
        }.sortedWith(compareByDescending<Pair<String, BigInteger>> { it.second }.thenBy { it.first })
        val remaining = source - amounts.values.sum()
        repeat(remaining.toInt()) { i -> val id = remainders[i].first; amounts[id] = amounts.getValue(id) + 1 }
        return DriverAllocation(amounts.toMap())
    }
    fun prorate(source: Long?, period: DriverTimeSpan, parts: List<DriverAllocationSpan>): DriverAllocation {
        require(parts.size < DRIVER_MAX_ITEMS)
        var previous = period.from
        var covered = 0L
        val weights = parts.sortedBy { it.span.from }.map {
            require(it.id != "_unallocated" && it.span.from >= previous && it.span.to <= period.to)
            previous = it.span.to
            covered = Math.addExact(covered, it.span.to - it.span.from)
            DriverWeight(it.id, it.span.to - it.span.from)
        } + DriverWeight("_unallocated", period.to - period.from - covered)
        return allocate(source, weights)
    }
    fun days(source: Long?, from: String, to: String, timezone: String, parts: List<DriverCalendarSpan>): DriverAllocation {
        require(timezone in ZoneId.getAvailableZoneIds())
        fun day(value: String): Long = civilDay(value).toEpochDay() + 719162
        return prorate(source, DriverTimeSpan(day(from), day(to)), parts.map { DriverAllocationSpan(it.id, DriverTimeSpan(day(it.from), day(it.to))) })
    }
}
private fun civilDay(value: String) = LocalDate.parse(value).also { require(it.year in 1..9999 && it.toString() == value) }
private fun period(from: String, until: String?): Boolean {
    val a = civilDay(from)
    return until == null || civilDay(until) > a
}
data class DriverProfileCost(val id: String, val kind: String, val provenance: String, val amount: Long?, val from: String,
                             val until: String? = null, val ledgerSourceId: String? = null, val sourceRevision: Long = 0)
data class DriverCostBuckets(val operating: List<DriverProfileCost> = emptyList(), val capital: List<DriverProfileCost> = emptyList(),
                             val modeled: List<DriverProfileCost> = emptyList(), val unknown: String = "")
data class DriverVehicleCostProfile(val vehicle: String, val ownership: String, val timezone: String, val from: String,
                                    val until: String?, val version: Long, val costs: List<DriverProfileCost>) {
    init {
        require(vehicle.isNotBlank() && ownership in setOf("OWNED", "LEASED", "FINANCED") && version > 0 && period(from, until) && timezone in ZoneId.getAvailableZoneIds())
        require(costs.size <= DRIVER_MAX_ITEMS && costs.map { it.id }.toSet().size == costs.size)
        val ledger = mutableSetOf<String>()
        costs.forEach { c ->
            require(costId.matches(c.id) && period(c.from, c.until) && c.from >= from && (until == null || (c.until != null && c.until <= until)))
            c.amount?.let(::amount)
            if (c.kind in setOf("RESERVE", "DEPRECIATION")) {
                require(c.provenance == "MODELED" && c.ledgerSourceId == null && c.sourceRevision == 0L)
            } else require(c.provenance == "ACTUAL" && c.ledgerSourceId != null && costId.matches(c.ledgerSourceId) && ledger.add(c.ledgerSourceId) && c.sourceRevision > 0)
            require(when(c.kind) {
                "LEASE" -> ownership == "LEASED"
                "DEPRECIATION" -> ownership != "LEASED"
                "PRINCIPAL", "INTEREST" -> ownership == "FINANCED"
                "FEE", "MAINTENANCE", "RESERVE" -> true
                else -> false
            })
        }
    }
    fun at(date: String): DriverCostBuckets {
        civilDay(date)
        if (date < from || (until != null && date >= until)) return DriverCostBuckets(unknown = "profile-not-effective")
        val eligible = costs.filter { date >= it.from && (it.until == null || date < it.until) }
        return DriverCostBuckets(eligible.filter { it.provenance == "ACTUAL" && it.kind != "PRINCIPAL" },
            eligible.filter { it.kind == "PRINCIPAL" }, eligible.filter { it.provenance == "MODELED" })
    }
}
data class DriverReserve(val id: String, val amount: Long?, val window: DriverWindow)
data class DriverReserveReplacement(val reserveId: String, val actualId: String)
data class DriverReserveResult(val additionalModeled: Long?, val actualRecorded: Long, val replaced: List<String>, val unknown: String = "")
object DriverReserveRules {
    fun reconcile(window: DriverWindow, reserves: List<DriverReserve>, actual: List<DriverCost>, links: List<DriverReserveReplacement>): DriverReserveResult {
        require(reserves.size <= DRIVER_MAX_ITEMS && actual.size <= DRIVER_MAX_ITEMS && links.size <= DRIVER_MAX_ITEMS)
        require(reserves.map { it.id }.toSet().size == reserves.size && actual.map { it.id }.toSet().size == actual.size)
        reserves.forEach { require(costId.matches(it.id) && it.window == window); it.amount?.let(::amount) }
        val rmap = reserves.associateBy { it.id }
        actual.forEach { require(costId.matches(it.id) && it.window == window && it.id !in rmap); amount(it.amount) }
        val amap = actual.associateBy { it.id }
        require(links.map { it.reserveId }.toSet().size == links.size && links.map { it.actualId }.toSet().size == links.size)
        links.forEach { require(it.reserveId in rmap && it.actualId in amap) }
        val replaced = links.map { it.reserveId }.sorted()
        val remaining = reserves.filter { it.id !in replaced }
        val actualTotal = bounded(actual.fold(BigInteger.ZERO) { a, c -> a + c.amount.toBigInteger() })
        if (remaining.any { it.amount == null }) return DriverReserveResult(null, actualTotal, replaced, "model-assumptions-missing")
        val extra = bounded(remaining.fold(BigInteger.ZERO) { a, c -> a + requireNotNull(c.amount).toBigInteger() })
        return DriverReserveResult(extra, actualTotal, replaced)
    }
}
