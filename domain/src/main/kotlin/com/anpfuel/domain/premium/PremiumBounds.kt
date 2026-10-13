package com.anpfuel.domain.premium

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

    internal fun gcd(a: Long, b: Long): Long {
        var x = if (a < 0) -a else a
        var y = if (b < 0) -b else b
        while (y != 0L) {
            val t = x % y
            x = y
            y = t
        }
        return if (x == 0L) 1L else x
    }

    /**
     * Reduced rational string mirroring Go `rational()`: `10` when the
     * denominator reduces to 1, otherwise `num/den`. Empty when den <= 0.
     */
    fun rational(num: Long, den: Long): String {
        if (den <= 0L) return ""
        val g = gcd(num, den)
        val n = num / g
        val d = den / g
        return if (d == 1L) n.toString() else "$n/$d"
    }
}
