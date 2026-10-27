package com.anpfuel.data.mapper

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class DriverHttpCodecTest {
    private fun fixture(name: String): String =
        checkNotNull(javaClass.getResourceAsStream("/$name")).bufferedReader().use { it.readText() }

    @Test
    fun `decodes exact Go receipt bytes and new fields`() {
        val receipt = DriverHttpCodec.receipt(fixture("receipt-200.json"))
        assertEquals("11111111-1111-4111-8111-111111111111", receipt.id)
        assertEquals(1L, receipt.revision)
        assertEquals(0L, receipt.epoch)
        assertFalse(receipt.replayed)
    }

    @Test
    fun `decodes conflict and auth error bytes`() {
        val conflict = DriverHttpCodec.error(409, fixture("conflict-409.json"))
        assertEquals("driver.conflict", conflict.code)
        assertEquals("private driver request unavailable or refused", conflict.message)
        val auth = DriverHttpCodec.error(401, fixture("auth-401.json"))
        assertEquals("driver.auth-required", auth.code)
        assertThrows(IllegalArgumentException::class.java) { DriverHttpCodec.error(500, fixture("conflict-409.json")) }
    }
}
