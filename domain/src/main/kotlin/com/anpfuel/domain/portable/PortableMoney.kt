package com.anpfuel.domain.portable

import com.anpfuel.domain.exception.DomainException

/**
 * Portable exact money in integer milli-BRL (P12-T02).
 *
 * Pure Kotlin with zero `java.*` imports so this file moves unchanged into a
 * future `commonMain` source set. Semantics mirror the backend Go kernel
 * (`backend/internal/modules/kernel/price.go`, B-BR-002): no floats, ANP
 * comma decimals, nonzero precision beyond 3 decimals is refused (never
 * rounded) and the range is exactly 1..1000000 milli-BRL.
 */
class PortableMoneyException(
    val code: String,
    message: String,
) : DomainException(message)

object PortableMoney {

    const val MIN_MILLI: Long = 1L
    const val MAX_MILLI: Long = 1_000_000L

    /** Tank capacity bound in milli-liters (200 L, mirrors TankCapacity.MAX_LITERS). */
    const val MAX_CAPACITY_MILLI_LITERS: Long = 200_000L

    /**
     * Parses ANP price text (digits with an optional single ',' decimal
     * separator and surrounding spaces) into integer milli-BRL.
     * Throws [PortableMoneyException] with a stable quarantine-style code.
     */
    fun parse(text: String): Long {
        var trimmed = text.trim()
        if (trimmed.isEmpty()) throw PortableMoneyException("missing-price", "empty price text")
        var negative = false
        if (trimmed.startsWith("-")) {
            negative = true
            trimmed = trimmed.substring(1).trim()
        }
        if (trimmed.startsWith("+")) throw PortableMoneyException("invalid-price", "invalid price text")
        val comma = trimmed.indexOf(',')
        val intPart: String
        val fracPart: String
        if (comma >= 0) {
            intPart = trimmed.substring(0, comma)
            fracPart = trimmed.substring(comma + 1)
            if (intPart.isEmpty() || fracPart.isEmpty()) {
                throw PortableMoneyException("invalid-price", "invalid price text")
            }
        } else {
            intPart = trimmed
            fracPart = ""
        }
        if (!allDigits(intPart) || (fracPart.isNotEmpty() && !allDigits(fracPart))) {
            throw PortableMoneyException("invalid-price", "invalid price text")
        }
        var frac = fracPart
        if (frac.length > 3) {
            for (i in 3 until frac.length) {
                if (frac[i] != '0') throw PortableMoneyException("over-precision", "precision beyond 3 decimals")
            }
            frac = frac.substring(0, 3)
        }
        while (frac.length < 3) frac += "0"
        var milli = 0L
        for (c in intPart) {
            milli = milli * 10 + (c - '0')
            if (milli > MAX_MILLI) throw PortableMoneyException("over-range", "price outside 1..1000000 milli-BRL")
        }
        var fracValue = 0L
        for (c in frac) {
            fracValue = fracValue * 10 + (c - '0')
        }
        milli = milli * 1000 + fracValue
        if (negative) throw PortableMoneyException("negative-price", "negative price")
        if (milli < MIN_MILLI) {
            if ((intPart + frac).all { it == '0' }) {
                throw PortableMoneyException("zero-price", "zero price")
            }
            throw PortableMoneyException("invalid-price", "invalid price text")
        }
        if (milli > MAX_MILLI) throw PortableMoneyException("over-range", "price outside 1..1000000 milli-BRL")
        return milli
    }

    /** Returns "ok" or the stable rejection code for [text]; never throws. */
    fun codeOf(text: String): String {
        return try {
            parse(text)
            "ok"
        } catch (e: PortableMoneyException) {
            e.code
        }
    }

    /** Formats milli-BRL canonically as "<int>,<3dp>" (e.g. 5999 -> "5,999"). */
    fun format(milli: Long): String {
        if (milli < MIN_MILLI) {
            if (milli == 0L) throw PortableMoneyException("zero-price", "zero price")
            throw PortableMoneyException("invalid-price", "invalid price text")
        }
        if (milli > MAX_MILLI) throw PortableMoneyException("over-range", "price outside 1..1000000 milli-BRL")
        val intPart = milli / 1000
        val fracPart = (milli % 1000).toString().padStart(3, '0')
        return "$intPart,$fracPart"
    }

    /**
     * Parses tank capacity text with the same comma grammar into
     * milli-liters. Range is 1..200000 (0 < liters <= 200).
     */
    fun parseCapacityMilliLiters(text: String): Long {
        val milliLiters = parseCapacityRaw(text)
        if (milliLiters < 1L) {
            if (milliLiters == 0L) throw PortableMoneyException("zero-price", "zero capacity")
            throw PortableMoneyException("invalid-price", "invalid capacity text")
        }
        if (milliLiters > MAX_CAPACITY_MILLI_LITERS) {
            throw PortableMoneyException("over-range", "capacity above 200 liters")
        }
        return milliLiters
    }

    /**
     * Exact tank-fill total in milli-BRL: round-half-up of
     * `unitMilli * capacityMilliLiters / 1000`, with overflow refusal.
     */
    fun multiplyTankFill(unitMilli: Long, capacityMilliLiters: Long): Long {
        if (unitMilli < MIN_MILLI || unitMilli > MAX_MILLI) {
            throw PortableMoneyException("over-range", "unit price outside 1..1000000 milli-BRL")
        }
        if (capacityMilliLiters < 1L || capacityMilliLiters > MAX_CAPACITY_MILLI_LITERS) {
            throw PortableMoneyException("over-range", "capacity outside 1..200000 milli-liters")
        }
        if (unitMilli > Long.MAX_VALUE / capacityMilliLiters) {
            throw PortableMoneyException("over-range", "tank-fill multiply overflows")
        }
        val product = unitMilli * capacityMilliLiters
        return (product + 500L) / 1000L
    }

    private fun parseCapacityRaw(text: String): Long {
        var trimmed = text.trim()
        if (trimmed.isEmpty()) throw PortableMoneyException("missing-price", "empty capacity text")
        if (trimmed.startsWith("-")) throw PortableMoneyException("negative-price", "negative capacity")
        if (trimmed.startsWith("+")) throw PortableMoneyException("invalid-price", "invalid capacity text")
        val comma = trimmed.indexOf(',')
        val intPart: String
        val fracPart: String
        if (comma >= 0) {
            intPart = trimmed.substring(0, comma)
            fracPart = trimmed.substring(comma + 1)
            if (intPart.isEmpty() || fracPart.isEmpty()) {
                throw PortableMoneyException("invalid-price", "invalid capacity text")
            }
        } else {
            intPart = trimmed
            fracPart = ""
        }
        if (!allDigits(intPart) || (fracPart.isNotEmpty() && !allDigits(fracPart))) {
            throw PortableMoneyException("invalid-price", "invalid capacity text")
        }
        var frac = fracPart
        if (frac.length > 3) {
            for (i in 3 until frac.length) {
                if (frac[i] != '0') throw PortableMoneyException("over-precision", "precision beyond 3 decimals")
            }
            frac = frac.substring(0, 3)
        }
        while (frac.length < 3) frac += "0"
        var liters = 0L
        for (c in intPart) {
            liters = liters * 10 + (c - '0')
            if (liters > MAX_CAPACITY_MILLI_LITERS) {
                throw PortableMoneyException("over-range", "capacity above 200 liters")
            }
        }
        var fracValue = 0L
        for (c in frac) {
            fracValue = fracValue * 10 + (c - '0')
        }
        return liters * 1000 + fracValue
    }

    private fun allDigits(s: String): Boolean {
        if (s.isEmpty()) return false
        for (c in s) {
            if (c < '0' || c > '9') return false
        }
        return true
    }
}
