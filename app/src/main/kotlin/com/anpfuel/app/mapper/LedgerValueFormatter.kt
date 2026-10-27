package com.anpfuel.app.mapper

import com.anpfuel.domain.valueobject.PriceAmount
import java.math.BigDecimal
import java.math.RoundingMode
import java.text.NumberFormat
import java.util.Locale

/** Format exact internal ratios only at the presentation boundary. */
object LedgerValueFormatter {
    private val rational = Regex("([0-9]+)(?:/([0-9]+))?")

    private fun parts(value: String): Pair<BigDecimal, BigDecimal>? {
        if (value.length > 128) return null
        val match = rational.matchEntire(value) ?: return null
        val numerator = match.groupValues[1].toBigDecimal()
        val denominator = match.groupValues[2].ifEmpty { "1" }.toBigDecimal()
        return if (denominator.signum() > 0) numerator to denominator else null
    }

    fun costPerKilometre(centavosPerKm: String, locale: Locale): String? {
        val (n, d) = parts(centavosPerKm) ?: return null
        // Divide by 100 before the only monetary rounding, including fractional centavos.
        val brl = n.divide(d.multiply(BigDecimal(100)), 2, RoundingMode.HALF_UP)
        return PriceFormatter.formatAmount(PriceAmount.of(brl), locale)
    }

    fun efficiency(kmPerUnit: String, locale: Locale): String? {
        val (n, d) = parts(kmPerUnit) ?: return null
        val value = n.divide(d, 2, RoundingMode.HALF_UP)
        return NumberFormat.getNumberInstance(locale).apply {
            isGroupingUsed = false
            minimumFractionDigits = 0
            maximumFractionDigits = 2
            roundingMode = RoundingMode.HALF_UP
        }.format(value)
    }
}
