package com.anpfuel.domain.driver

import java.math.BigInteger

const val DRIVER_MAX_AMOUNT = 1_000_000_000_000L
const val DRIVER_MAX_ITEMS = 1000
internal fun bounded(value: BigInteger): Long {
    require(value.abs() <= BigInteger.valueOf(DRIVER_MAX_AMOUNT)) { "driver-money-out-of-bounds" }
    return value.driverLongExact()
}
internal fun amount(value: Long) { require(value in 0..DRIVER_MAX_AMOUNT) { "driver-money-out-of-bounds" } }
internal fun signed(value: Long) { require(value in -DRIVER_MAX_AMOUNT..DRIVER_MAX_AMOUNT) { "driver-money-out-of-bounds" } }

data class DriverEarnings(
    val basis: String, val gross: Long? = null, val fees: Long? = null,
    val tips: Long? = null, val bonus: Long? = null, val refund: Long? = null, val net: Long? = null,
) {
    fun earned(): Long {
        val parts = listOf(gross, fees, tips, bonus, refund)
        return when (basis) {
            "INCLUSIVE_NET" -> requireNotNull(net).also { amount(it); require(parts.all { p -> p == null }) }
            "DETAILED" -> {
                require(net == null && parts.all { it != null })
                parts.forEach { amount(it!!) }
                bounded(gross!!.toBigInteger() - fees!!.toBigInteger() + tips!!.toBigInteger() + bonus!!.toBigInteger() - refund!!.toBigInteger())
            }
            else -> throw IllegalArgumentException("driver-unknown-earnings-basis")
        }
    }
}
data class DriverAdjustment(val id: String, val targetId: String, val kind: String, val targetRevision: Long, val delta: Long) {
    init {
        require(id.isNotBlank() && targetId.isNotBlank() && targetRevision > 0 && delta != 0L)
        signed(delta)
        require(kind == "CORRECTION" || (kind == "REVERSAL" && delta < 0))
    }
}
data class DriverWindow(val owner: String, val vehicle: String, val currency: String, val fromMillis: Long, val toMillis: Long) {
    init { require(owner.isNotBlank() && vehicle.isNotBlank() && currency == "BRL" && toMillis > fromMillis) }
}
data class DriverCashMovement(val id: String, val kind: String, val account: String, val amount: Long)
data class DriverMoneyViews(val earned: Long, val received: Map<String, Long>)
data class DriverRatios(val perKm: String? = null, val perHour: String? = null, val unknown: String = "")
data class DriverCost(val id: String, val amount: Long, val window: DriverWindow)
data class DriverOperatingResult(
    val recorded: Long? = null, val modeled: Long? = null,
    val recordedRatios: DriverRatios = DriverRatios(), val modeledRatios: DriverRatios = DriverRatios(), val unknown: String = "",
)
object DriverMoney {
    fun views(earned: List<Long>, cash: List<DriverCashMovement>): DriverMoneyViews {
        require(earned.size <= DRIVER_MAX_ITEMS && cash.size <= DRIVER_MAX_ITEMS)
        val total = bounded(earned.fold(BigInteger.ZERO) { acc, v -> signed(v); acc + v.toBigInteger() })
        val received = mutableMapOf<String, Long>()
        require(cash.map { it.id }.toSet().size == cash.size)
        cash.forEach {
            require(it.id.isNotBlank() && it.kind in setOf("PAYOUT", "WALLET_TRANSFER", "CASH_RECEIPT") && it.account in setOf("BANK", "CASH", "WALLET"))
            amount(it.amount)
            received[it.account] = bounded(received.getOrDefault(it.account, 0).toBigInteger() + it.amount.toBigInteger())
        }
        return DriverMoneyViews(total, received.toMap())
    }
    fun ratios(value: Long, distanceMetres: Long?, workSeconds: Long?): DriverRatios {
        signed(value)
        val reasons = mutableListOf<String>()
        fun ratio(denominator: Long?, scale: Long, name: String): String? {
            if (denominator == null) { reasons.add("$name-missing"); return null }
            if (denominator <= 0) { reasons.add("$name-nonpositive"); return null }
            require(denominator <= DRIVER_MAX_AMOUNT)
            val n = value.toBigInteger() * scale.toBigInteger()
            val d = denominator.toBigInteger()
            val gcd = n.gcd(d)
            return if (d / gcd == BigInteger.ONE) (n / gcd).toString() else "${n / gcd}/${d / gcd}"
        }
        val km = ratio(distanceMetres, 1000, "distance")
        val hour = ratio(workSeconds, 3600, "time")
        return DriverRatios(km, hour, reasons.joinToString(","))
    }
    fun result(earned: Long, earningsWindow: DriverWindow, reportWindow: DriverWindow, costs: List<DriverCost>, complete: Boolean,
               estimates: List<DriverCost>?, distance: Long?, seconds: Long?): DriverOperatingResult {
        signed(earned)
        require(costs.size <= DRIVER_MAX_ITEMS && (estimates?.size ?: 0) <= DRIVER_MAX_ITEMS)
        if (earningsWindow != reportWindow) return DriverOperatingResult(unknown = "unmatched-period-window")
        if (!complete) return DriverOperatingResult(unknown = "cost-coverage-incomplete")
        val seen = mutableSetOf<String>()
        fun subtract(start: BigInteger, items: List<DriverCost>) = items.fold(start) { total, cost ->
            require(cost.id.isNotBlank() && seen.add(cost.id) && cost.window == reportWindow) { "driver-cost-coverage-conflict" }
            amount(cost.amount)
            total - cost.amount.toBigInteger()
        }
        val recorded = bounded(subtract(earned.toBigInteger(), costs))
        val recordedRatios = ratios(recorded, distance, seconds)
        if (estimates == null) return DriverOperatingResult(recorded = recorded, recordedRatios = recordedRatios, unknown = "model-assumptions-missing")
        val modeled = bounded(subtract(recorded.toBigInteger(), estimates))
        return DriverOperatingResult(recorded, modeled, recordedRatios, ratios(modeled, distance, seconds))
    }
    fun weightedRatios(amounts: List<Long>, distances: List<Long>, seconds: List<Long>): DriverRatios {
        require(amounts.isNotEmpty() && amounts.size <= DRIVER_MAX_ITEMS && amounts.size == distances.size && amounts.size == seconds.size)
        amounts.forEach(::signed); distances.forEach(::amount); seconds.forEach(::amount)
        fun sum(items: List<Long>) = bounded(items.fold(BigInteger.ZERO) { a, v -> a + v.toBigInteger() })
        return ratios(sum(amounts), sum(distances), sum(seconds))
    }
}
