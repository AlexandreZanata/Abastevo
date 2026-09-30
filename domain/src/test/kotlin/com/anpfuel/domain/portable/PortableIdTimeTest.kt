package com.anpfuel.domain.portable

import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class PortableIdTimeTest {

    @Test
    fun acceptsBackendKernelUuidVector() {
        assertTrue(PortableIdTime.isUuid("123e4567-e89b-12d3-a456-426614174000"))
    }

    @Test
    fun rejectsUppercaseAndNonHexUuid() {
        assertFalse(PortableIdTime.isUuid("123E4567-E89B-12D3-A456-426614174000"))
        assertFalse(PortableIdTime.isUuid("xyz"))
        assertFalse(PortableIdTime.isUuid(""))
    }

    @Test
    fun acceptsUtcInstant() {
        assertTrue(PortableIdTime.isUtcInstant("2026-06-07T00:00:00Z"))
    }

    @Test
    fun acceptsLeapDayAndRejectsNonLeapFeb29() {
        assertTrue(PortableIdTime.isUtcInstant("2024-02-29T12:00:00Z"))
        assertFalse(PortableIdTime.isUtcInstant("2025-02-29T00:00:00Z"))
    }

    @Test
    fun rejectsBadMonthAndMissingDesignator() {
        assertFalse(PortableIdTime.isUtcInstant("2026-13-01T00:00:00Z"))
        assertFalse(PortableIdTime.isUtcInstant("2026-06-07T00:00:00"))
    }
}
