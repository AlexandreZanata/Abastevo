package com.anpfuel.app.mapper

import java.util.Locale
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class LedgerValueFormatterTest {
    private val pt = Locale.forLanguageTag("pt-BR")
    private fun plain(value: String?) = value?.replace('\u00a0',' ')
    @Test fun `centavos and fractions format as BRL with one final half-up rounding`() {
        assertEquals("R$ 0,52",plain(LedgerValueFormatter.costPerKilometre("52",pt)))
        assertEquals("R$ 0,58",plain(LedgerValueFormatter.costPerKilometre("115/2",pt)))
        assertEquals("R$ 0,33",plain(LedgerValueFormatter.costPerKilometre("100/3",pt)))
        assertEquals("R$ 0,00",plain(LedgerValueFormatter.costPerKilometre("0",pt)))
        assertEquals("R$ 1,00",plain(LedgerValueFormatter.costPerKilometre("999/10",pt)))
    }
    @Test fun `efficiency uses localized decimal rather than internal rational`() {
        assertEquals("12,5",LedgerValueFormatter.efficiency("25/2",pt))
        assertEquals("12.5",LedgerValueFormatter.efficiency("25/2",Locale.US))
        assertEquals("14,29",LedgerValueFormatter.efficiency("100/7",pt))
    }
    @Test fun `invalid or unknown internal values do not display a fake zero`() {
        for (value in listOf("", "1/0", "-1", "NaN", "1//2", "1/-2", "9".repeat(129))) {
            assertNull(LedgerValueFormatter.costPerKilometre(value,pt))
            assertNull(LedgerValueFormatter.efficiency(value,pt))
        }
    }
}
