package com.anpfuel.data.mapper

/**
 * P10-T01 backend-compatible CNPJ mapping (A06).
 *
 * The legacy domain [com.anpfuel.domain.valueobject.Cnpj] stays
 * numeric-only. This adapter preserves alphanumeric identifiers and
 * leading zeroes as text, never coerces letters to digits, and applies
 * the same check-digit rule as the backend Go kernel
 * (`backend/internal/modules/kernel/cnpj.go`, Receita Federal
 * alphanumeric program: 'A'-'Z' map to 17-42).
 */
object BackendCnpjMapper {

    fun parse(raw: String): Result<String> {
        val normalized = normalize(raw) ?: return Result.failure(
            IllegalArgumentException("invalid CNPJ: $raw"),
        )
        return if (checkDigits(normalized)) {
            Result.success(normalized)
        } else {
            Result.failure(IllegalArgumentException("invalid CNPJ checksum: $raw"))
        }
    }

    fun isAlphanumeric(normalized: String): Boolean =
        normalized.any { it in 'A'..'Z' }

    private fun normalize(raw: String): String? {
        val builder = StringBuilder()
        for (char in raw) {
            when {
                char in '0'..'9' || char in 'A'..'Z' -> builder.append(char)
                char in 'a'..'z' -> builder.append(char.uppercaseChar())
                char == '.' || char == '/' || char == '-' || char == ' ' -> Unit
                else -> return null
            }
        }
        val normalized = builder.toString()
        if (normalized.length != CNPJ_LENGTH) return null
        if (!normalized.all { it in '0'..'9' || it in 'A'..'Z' }) return null
        return normalized
    }

    private fun digitValue(char: Char): Int =
        if (char <= '9') char - '0' else char - 'A' + 17

    private fun checkDigits(normalized: String): Boolean {
        val firstWeights = intArrayOf(5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2)
        var sum = 0
        for (i in 0 until 12) {
            sum += digitValue(normalized[i]) * firstWeights[i]
        }
        val remainder = sum % 11
        val first = if (remainder >= 2) 11 - remainder else 0
        val secondWeights = intArrayOf(6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2)
        sum = 0
        for (i in 0 until 12) {
            sum += digitValue(normalized[i]) * secondWeights[i]
        }
        sum += first * secondWeights[12]
        val secondRemainder = sum % 11
        val second = if (secondRemainder >= 2) 11 - secondRemainder else 0
        return normalized[12] - '0' == first && normalized[13] - '0' == second
    }

    private const val CNPJ_LENGTH = 14
}
