package com.anpfuel.domain.premium

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class LedgerSessionContextTest {

    @Test
    fun `contexts parse and reject unknown`() {
        assertEquals(LedgerSessionContext.HOME, LedgerSessionContext.parse("home"))
        assertEquals(LedgerSessionContext.PUBLIC, LedgerSessionContext.parse("public"))
        assertEquals("work", LedgerSessionContext.WORK.wire)
        assertThrows(PremiumValidationException::class.java) {
            LedgerSessionContext.parse("gps-precise")
        }
        assertThrows(PremiumValidationException::class.java) {
            LedgerSessionContext.parse("")
        }
    }
}
