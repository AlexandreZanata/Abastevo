package com.anpfuel.domain.premium

import java.math.BigDecimal

/** Exact UI kilometre boundary. Stored/wire distance remains integer metres. */
object LedgerDistance {
    private val decimal = Regex("[0-9]+(?:[.,][0-9]+)?")

    fun parseKilometres(input: String, maximumMetres: Long = PremiumBounds.MAX_DISTANCE_METRES): Long? {
        val text = input.trim()
        if (text.length > 64 || !decimal.matches(text) || maximumMetres < 0) return null
        return try {
            BigDecimal(text.replace(',', '.')).movePointRight(3).longValueExact()
                .takeIf { it in 0..maximumMetres }
        } catch (_: ArithmeticException) {
            null
        } catch (_: NumberFormatException) {
            null
        }
    }

    /** Ungrouped decimal, at most three fractional km digits; safe for edit -> save. */
    fun kilometresInput(metres: Long, decimalSeparator: Char = '.'): String {
        PremiumBounds.checkDistanceMetres(metres)
        return BigDecimal.valueOf(metres, 3).stripTrailingZeros().toPlainString()
            .replace('.', decimalSeparator)
    }
}
