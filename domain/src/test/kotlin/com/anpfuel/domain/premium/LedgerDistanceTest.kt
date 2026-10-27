package com.anpfuel.domain.premium

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class LedgerDistanceTest {
    @Test fun `decimal kilometres convert exactly with inclusive existing bounds`() {
        assertEquals(10000000L, LedgerDistance.parseKilometres("10000"))
        assertEquals(12345600L, LedgerDistance.parseKilometres("12345,6"))
        assertEquals(1L, LedgerDistance.parseKilometres("0.001"))
        assertEquals(0L, LedgerDistance.parseKilometres("0"))
        assertEquals(1000000000000L, LedgerDistance.parseKilometres("1000000000"))
        assertNull(LedgerDistance.parseKilometres("1000000000.001"))
        assertEquals(1000000000L, LedgerDistance.parseKilometres("1000000", 1000000000))
        assertNull(LedgerDistance.parseKilometres("1000000.001", 1000000000))
    }
    @Test fun `ambiguous nonfinite overflowing signed or sub-metre inputs refuse`() {
        for (input in listOf("", "NaN", "Infinity", "1e9", "-1", "+1", "1,000.2", "1.000,2", "1 000", "0.0001", "9223372036854775808", "9".repeat(65))) {
            assertNull(LedgerDistance.parseKilometres(input), input)
        }
    }
    @Test fun `all representable edit values round trip without multiplying twice`() {
        for (m in listOf(0L,1L,999L,1000L,12345600L,1000000000000L)) {
            for (separator in listOf('.',',')) assertEquals(m,LedgerDistance.parseKilometres(LedgerDistance.kilometresInput(m,separator)))
        }
        assertThrows(PremiumValidationException::class.java) { LedgerDistance.kilometresInput(-1) }
    }
}
