package com.anpfuel.data.mapper

import com.anpfuel.domain.valueobject.Cnpj
import java.math.BigDecimal

/**
 * Normalizes ANP CNPJ cell values (numeric Excel cells, missing leading zeros).
 */
object AnpCnpjMapper {

    fun parse(raw: String): Cnpj = Cnpj.parse(normalizeDigits(raw))

    fun normalizeDigits(raw: String): String {
        val trimmed = raw.trim()
        val digits = if (trimmed.contains('E', ignoreCase = true)) {
            BigDecimal(trimmed).toPlainString().filter(Char::isDigit)
        } else {
            trimmed.filter(Char::isDigit)
        }
        return when {
            digits.length == CNPJ_LENGTH -> digits
            digits.length < CNPJ_LENGTH -> digits.padStart(CNPJ_LENGTH, '0')
            else -> digits.takeLast(CNPJ_LENGTH)
        }
    }

    private const val CNPJ_LENGTH = 14
}
