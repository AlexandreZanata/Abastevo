package com.anpfuel.domain.portable

/**
 * Portable identity and time representations (P12-T02).
 *
 * Pure Kotlin with zero `java.*` imports so this file moves unchanged into a
 * future `commonMain` source set. Canonical forms shared with the backend and
 * the future Swift wrapper: UUIDs are lowercase `8-4-4-4-12` hex strings and
 * instants are UTC `YYYY-MM-DDTHH:MM:SSZ`. No `java.util.UUID` or `java.time`
 * anywhere in portable code; Android adapters keep those APIs behind ports.
 */
object PortableIdTime {

    /** Lowercase canonical UUID only; uppercase is rejected, not coerced. */
    fun isUuid(text: String): Boolean {
        if (text.length != 36) return false
        for (i in text.indices) {
            val c = text[i]
            if (i == 8 || i == 13 || i == 18 || i == 23) {
                if (c != '-') return false
            } else if (!isLowerHex(c)) {
                return false
            }
        }
        return true
    }

    /** Strict UTC instant `YYYY-MM-DDTHH:MM:SSZ` with calendar range checks. */
    fun isUtcInstant(text: String): Boolean {
        if (text.length != 20) return false
        if (text[4] != '-' || text[7] != '-' || text[10] != 'T' ||
            text[13] != ':' || text[16] != ':' || text[19] != 'Z'
        ) {
            return false
        }
        val digits = intArrayOf(0, 1, 2, 3, 5, 6, 8, 9, 11, 12, 14, 15, 17, 18)
        for (i in digits) {
            if (text[i] < '0' || text[i] > '9') return false
        }
        val year = part(text, 0, 4)
        val month = part(text, 5, 7)
        val day = part(text, 8, 10)
        val hour = part(text, 11, 13)
        val minute = part(text, 14, 16)
        val second = part(text, 17, 19)
        if (month < 1 || month > 12) return false
        if (day < 1 || day > daysInMonth(year, month)) return false
        if (hour > 23 || minute > 59 || second > 59) return false
        return true
    }

    private fun isLowerHex(c: Char): Boolean {
        return c in '0'..'9' || c in 'a'..'f'
    }

    private fun part(text: String, from: Int, to: Int): Int {
        var value = 0
        for (i in from until to) {
            value = value * 10 + (text[i] - '0')
        }
        return value
    }

    private fun daysInMonth(year: Int, month: Int): Int {
        return when (month) {
            1, 3, 5, 7, 8, 10, 12 -> 31
            4, 6, 9, 11 -> 30
            else -> if (isLeapYear(year)) 29 else 28
        }
    }

    private fun isLeapYear(year: Int): Boolean {
        return year % 4 == 0 && (year % 100 != 0 || year % 400 == 0)
    }
}
