package com.anpfuel.app.mapper

import com.anpfuel.domain.valueobject.PriceAmount
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.util.Locale

class PriceFormatterTest {

    @Test
    fun formatsBrlWithRealSymbolRegardlessOfUiLocale() {
        val formatted = PriceFormatter.formatAmount(PriceAmount.of("3.99"), Locale.US)

        assertTrue(formatted!!.startsWith("R$"))
        assertEquals("R$ 3,99", formatted)
    }

    @Test
    fun formatsLargeLpgPriceInBrl() {
        val formatted = PriceFormatter.formatAmount(PriceAmount.of("138.00"), Locale.ENGLISH)

        assertEquals("R$ 138,00", formatted)
    }
}
