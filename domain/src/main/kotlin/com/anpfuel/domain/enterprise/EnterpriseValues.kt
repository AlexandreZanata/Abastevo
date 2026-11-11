package com.anpfuel.domain.enterprise

import com.anpfuel.domain.portable.PortableMoney
import com.anpfuel.domain.premium.PremiumBounds
import com.anpfuel.domain.premium.PremiumUnit
import java.math.BigInteger

class EnterpriseValueException : IllegalArgumentException("orgvalue: invalid-value")

data class ExactRecord(
    val paidCentavos: Long?,
    val quantityMilli: Long?,
    val unitPriceMilliBRL: Long?,
    val discountCentavos: Long?,
    val unit: String,
    val sourcePriceText: String = "",
)

data class Reconciliation(
    val paidCentavos: Long?,
    val grossCentavos: Long? = null,
    val expectedCentavos: Long? = null,
    val differenceCentavos: Long? = null,
    val unit: String,
    val sourcePriceText: String = "",
    val derivedPriceMilliBRL: String? = null,
    val warning: String = "",
    val unknown: String = "",
)

data class AllocationLine(val order: Long, val weight: Long?)
data class AllocationPart(val order: Long, val centavos: Long)
data class Allocation(val parts: List<AllocationPart>? = null, val unknown: String = "")

/** Pure financial-v1 integer rules; no client storage, routes or workflow. */
object EnterpriseValues {
    const val MAX_ALLOCATION_LINES = 1000

    private fun parse(raw: String, minimum: Long, maximum: Long): Long {
        if (raw.isEmpty() || raw.length > 19 || (raw.length > 1 && raw[0] == '0') ||
            raw.any { it !in '0'..'9' }
        ) throw EnterpriseValueException()
        val value = raw.toLongOrNull() ?: throw EnterpriseValueException()
        if (value !in minimum..maximum) throw EnterpriseValueException()
        return value
    }

    fun parseCentavos(raw: String): Long = parse(raw, 0, PremiumBounds.MAX_TOTAL_CENTAVOS)
    fun parseQuantityMilli(raw: String): Long = parse(raw, 1, PremiumBounds.MAX_QUANTITY_MILLI)
    fun parseUnitPriceMilliBRL(raw: String): Long = parse(raw, PortableMoney.MIN_MILLI, PortableMoney.MAX_MILLI)

    private fun valid(value: Long?, minimum: Long, maximum: Long): Boolean =
        value == null || value in minimum..maximum

    /** Source text is retained, not interpreted or silently quantized here. */
    fun reconcile(input: ExactRecord): Reconciliation {
        if (!valid(input.paidCentavos, 0, PremiumBounds.MAX_TOTAL_CENTAVOS) ||
            !valid(input.discountCentavos, 0, PremiumBounds.MAX_TOTAL_CENTAVOS) ||
            !valid(input.quantityMilli, 1, PremiumBounds.MAX_QUANTITY_MILLI) ||
            !valid(input.unitPriceMilliBRL, PortableMoney.MIN_MILLI, PortableMoney.MAX_MILLI) ||
            PremiumUnit.entries.none { it.wire == input.unit }
        ) throw EnterpriseValueException()
        val gross = if (input.paidCentavos != null && input.discountCentavos != null) {
            input.paidCentavos + input.discountCentavos // <= 2e12, no Long overflow.
        } else null
        val expected = if (input.quantityMilli != null && input.unitPriceMilliBRL != null) {
            (input.quantityMilli * input.unitPriceMilliBRL + 5000) / 10000 // <= 1e15 intermediate.
        } else null
        val derived = if (input.unitPriceMilliBRL == null && input.paidCentavos != null && input.quantityMilli != null) {
            PremiumBounds.scaledRational(input.paidCentavos, 10000, input.quantityMilli)
        } else null
        val unknown = when {
            input.paidCentavos == null -> "paid-total-missing"
            input.quantityMilli == null -> "quantity-missing"
            input.discountCentavos == null -> "discount-missing"
            input.unitPriceMilliBRL == null -> "unit-price-missing"
            else -> ""
        }
        val difference = if (gross != null && expected != null) kotlin.math.abs(gross - expected) else null
        return Reconciliation(
            paidCentavos = input.paidCentavos,
            grossCentavos = gross,
            expectedCentavos = expected,
            differenceCentavos = difference,
            unit = input.unit,
            sourcePriceText = input.sourcePriceText,
            derivedPriceMilliBRL = derived,
            warning = if (difference != null && difference > 1) "ARITHMETIC_MISMATCH" else "",
            unknown = unknown,
        )
    }

    /** Existing driver allocation uses ID ties; enterprise ties use line order. */
    fun allocate(source: Long?, lines: List<AllocationLine>): Allocation {
        if (!valid(source, 0, PremiumBounds.MAX_TOTAL_CENTAVOS) || lines.size > MAX_ALLOCATION_LINES) {
            throw EnterpriseValueException()
        }
        val seen = mutableSetOf<Long>()
        if (lines.any { it.order < 0 || !seen.add(it.order) || (it.weight != null && it.weight < 0) }) {
            throw EnterpriseValueException()
        }
        if (source == null) return Allocation(unknown = "source-amount-missing")
        if (lines.any { it.weight == null }) return Allocation(unknown = "allocation-basis-missing")
        val total = lines.fold(BigInteger.ZERO) { sum, line -> sum + line.weight!!.toBigInteger() }
        if (total == BigInteger.ZERO) return Allocation(unknown = "allocation-basis-missing")
        data class Share(val order: Long, var centavos: Long, val remainder: BigInteger)
        val shares = lines.map {
            val (q, r) = (source.toBigInteger() * it.weight!!.toBigInteger()).divideAndRemainder(total)
            Share(it.order, q.longValueExact(), r)
        }.sortedWith(compareByDescending<Share> { it.remainder }.thenBy { it.order })
        val residual = source - shares.sumOf { it.centavos }
        repeat(residual.toInt()) { shares[it].centavos++ }
        return Allocation(shares.sortedBy { it.order }.map { AllocationPart(it.order, it.centavos) })
    }
}
