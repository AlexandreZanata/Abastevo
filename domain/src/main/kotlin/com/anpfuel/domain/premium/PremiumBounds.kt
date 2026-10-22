package com.anpfuel.domain.premium

import java.math.BigInteger

/**
 * PRM09-T01 exact integer bounds shared by ledger inputs.
 *
 * Mirrors the backend metrics ceilings (`backend/internal/modules/metrics`):
 * quantity 1..1_000_000_000 milli-units, money 0..1_000_000_000_000
 * centavos, distance 0..1_000_000_000_000 metres, notes at most 280 runes.
 * No floats; display rounding happens only at the boundary.
 */
object PremiumBounds {
    const val MAX_QUANTITY_MILLI: Long = 1_000_000_000L
    const val MAX_TOTAL_CENTAVOS: Long = 1_000_000_000_000L
    const val MAX_DISTANCE_METRES: Long = 1_000_000_000_000L
    const val MAX_NOTE_RUNES: Int = 280
    const val MAX_CURSOR_CHARS: Int = 2048
    const val MAX_COMMANDS_PER_BATCH: Int = 50
    const val CALCULATION_VERSION: Int = 1

    fun checkQuantityMilli(value: Long) {
        if (value < 1L || value > MAX_QUANTITY_MILLI) {
            throw PremiumValidationException("input-out-of-bounds")
        }
    }

    fun checkTotalCentavos(value: Long) {
        if (value < 0L || value > MAX_TOTAL_CENTAVOS) {
            throw PremiumValidationException("input-out-of-bounds")
        }
    }

    fun checkDistanceMetres(value: Long) {
        if (value < 0L || value > MAX_DISTANCE_METRES) {
            throw PremiumValidationException("input-out-of-bounds")
        }
    }

    fun checkNote(note: String?) {
        if (note == null) return
        var runes = 0
        var i = 0
        while (i < note.length) {
            val c = note[i]
            if (Character.isHighSurrogate(c) && i + 1 < note.length &&
                Character.isLowSurrogate(note[i + 1])
            ) {
                i += 2
            } else {
                i += 1
            }
            runes += 1
            if (runes > MAX_NOTE_RUNES) {
                throw PremiumValidationException("input-out-of-bounds")
            }
        }
    }

    internal fun addNonNegative(a: Long, b: Long): Long? =
        if (a < 0 || b < 0 || a > Long.MAX_VALUE - b) null else a + b

    /** Exact intermediates may exceed Long even when every input is bounded. */
    fun scaledRational(num: Long, scale: Long, den: Long): String {
        if (den <= 0L) return ""
        val numerator = BigInteger.valueOf(num) * BigInteger.valueOf(scale)
        val denominator = BigInteger.valueOf(den)
        val divisor = numerator.gcd(denominator)
        val n = numerator / divisor
        val d = denominator / divisor
        return if (d == BigInteger.ONE) n.toString() else "$n/$d"
    }

    fun rational(num: Long, den: Long): String = scaledRational(num, 1, den)
}
