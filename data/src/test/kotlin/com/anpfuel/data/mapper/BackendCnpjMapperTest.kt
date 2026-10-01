package com.anpfuel.data.mapper

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P10-T01: backend-compatible CNPJ mapping (A06).
 *
 * Legacy [com.anpfuel.domain.valueobject.Cnpj] stays numeric-only; this
 * adapter preserves alphanumeric identifiers and leading zeroes, never
 * coerces letters to digits, and applies the same check-digit rule as the
 * backend Go kernel (`backend/internal/modules/kernel/cnpj.go`).
 */
class BackendCnpjMapperTest {

    @Test
    fun numericCnpjPreservesLeadingZeroes() {
        val result = BackendCnpjMapper.parse("04.218.406/0001-04")
        assertTrue(result.isSuccess)
        assertEquals("04218406000104", result.getOrNull())
        assertFalse(BackendCnpjMapper.isAlphanumeric(requireNotNull(result.getOrNull())))
    }

    @Test
    fun alphanumericCnpjPreservesLetters() {
        val result = BackendCnpjMapper.parse("12ABC345/01DE-35")
        assertTrue(result.isSuccess)
        assertEquals("12ABC34501DE35", result.getOrNull())
        assertTrue(BackendCnpjMapper.isAlphanumeric(requireNotNull(result.getOrNull())))
    }

    @Test
    fun lowercaseAlphanumericUppercased() {
        val result = BackendCnpjMapper.parse("12abc345/01de-35")
        assertTrue(result.isSuccess)
        assertEquals("12ABC34501DE35", result.getOrNull())
    }

    @Test
    fun lettersAreNeverCoercedToDigits() {
        // The legacy numeric-only path strips letters; this adapter must not.
        val result = BackendCnpjMapper.parse("12ABC34501DE35")
        assertTrue(result.isSuccess)
        val normalized = requireNotNull(result.getOrNull())
        assertTrue(normalized.any { it in 'A'..'Z' })
        assertEquals(14, normalized.length)
    }

    @Test
    fun invalidChecksumQuarantined() {
        assertTrue(BackendCnpjMapper.parse("04.218.406/0001-00").isFailure)
        assertTrue(BackendCnpjMapper.parse("12ABC345/01DE-30").isFailure)
        assertTrue(BackendCnpjMapper.parse("11222333000182").isFailure)
    }

    @Test
    fun malformedInputQuarantined() {
        assertTrue(BackendCnpjMapper.parse("").isFailure)
        assertTrue(BackendCnpjMapper.parse("123").isFailure)
        assertTrue(BackendCnpjMapper.parse("12abc345/01de-3!").isFailure)
        assertTrue(BackendCnpjMapper.parse("AAAAAAAAAAAAAA").isFailure)
        assertTrue(BackendCnpjMapper.parse("04.218.406/0001-044").isFailure)
    }
}
