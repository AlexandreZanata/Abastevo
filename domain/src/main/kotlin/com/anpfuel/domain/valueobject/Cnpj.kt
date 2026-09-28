package com.anpfuel.domain.valueobject

import com.anpfuel.domain.exception.DomainException

/**
 * Brazilian company tax identifier (CNPJ) — 14 digits after normalization.
 */
class Cnpj private constructor(
    val digits: String,
) {
    init {
        if (digits.length != DIGIT_COUNT || digits.any { !it.isDigit() }) {
            throw DomainException("CNPJ must contain exactly $DIGIT_COUNT digits")
        }
    }

    fun formatted(): String =
        "${digits.substring(0, 2)}.${digits.substring(2, 5)}.${digits.substring(5, 8)}/" +
            "${digits.substring(8, 12)}-${digits.substring(12, 14)}"

    override fun equals(other: Any?): Boolean =
        other is Cnpj && digits == other.digits

    override fun hashCode(): Int = digits.hashCode()

    override fun toString(): String = "Cnpj(digits=$digits)"

    companion object {
        private const val DIGIT_COUNT = 14

        fun parse(raw: String): Cnpj {
            val normalized = raw.filter(Char::isDigit)
            return Cnpj(normalized)
        }
    }
}
