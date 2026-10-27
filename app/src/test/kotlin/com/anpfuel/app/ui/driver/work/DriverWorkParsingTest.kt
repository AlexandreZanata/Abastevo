package com.anpfuel.app.ui.driver.work

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class DriverWorkParsingTest {
    @Test fun `pt-BR money and odometer convert exactly`() {
        assertEquals(123456L, DriverWorkViewModel.parseMoney("1.234,56"))
        assertEquals(12345600L, DriverWorkViewModel.parseKm("12.345,6"))
        assertEquals(0L, DriverWorkViewModel.parseMoney("0,00"))
        assertNull(DriverWorkViewModel.parseMoney("1,234"))
        assertNull(DriverWorkViewModel.parseKm("abc"))
    }
}
