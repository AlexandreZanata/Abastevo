package com.anpfuel.domain.portable

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P10-T04: portable price-OCR parsing (no product/condition inference,
 * no auto-pick, exact milli-BRL core).
 */
class PortablePriceOcrTest {

    @Test
    fun `parses single marked price in milli-BRL`() {
        val out = PortablePriceOcr.parseCandidates("R$ 5,89")
        assertEquals(1, out.size)
        assertEquals(5890L, out[0].priceMilli)
        assertTrue(out[0].hasCurrencyMarker)
        assertFalse(PortablePriceOcr.isLowConfidence(out[0]))
    }

    @Test
    fun `dot separator is tolerated and normalized`() {
        val out = PortablePriceOcr.parseCandidates("GASOLINA R$ 5.89")
        assertEquals(1, out.size)
        assertEquals(5890L, out[0].priceMilli)
    }

    @Test
    fun `multiple prices keep text order without picking minimum`() {
        val out = PortablePriceOcr.parseCandidates("R$ 6,19\nR$ 5,89")
        assertEquals(2, out.size)
        assertEquals(6190L, out[0].priceMilli)
        assertEquals(5890L, out[1].priceMilli)
    }

    @Test
    fun `bare number without marker is low confidence`() {
        val out = PortablePriceOcr.parseCandidates("5,89")
        assertEquals(1, out.size)
        assertFalse(out[0].hasCurrencyMarker)
        assertTrue(PortablePriceOcr.isLowConfidence(out[0]))
    }

    @Test
    fun `over-precision and over-range fragments are skipped`() {
        assertTrue(PortablePriceOcr.parseCandidates("R$ 5,8999").isEmpty())
        assertTrue(PortablePriceOcr.parseCandidates("R$ 99999,00").isEmpty())
        assertTrue(PortablePriceOcr.parseCandidates("").isEmpty())
        assertTrue(PortablePriceOcr.parseCandidates("   ").isEmpty())
    }

    @Test
    fun `candidate set is bounded and never carries product`() {
        val text = (1..20).joinToString("\n") { "R$ 5,89" }
        val out = PortablePriceOcr.parseCandidates(text)
        assertEquals(PortablePriceOcr.MAX_CANDIDATES, out.size)
        val fields = PortablePriceOcr.OcrCandidate::class.java.declaredFields
            .map { it.name }.toSet()
        assertFalse(fields.contains("product"))
        assertFalse(fields.contains("condition"))
    }
}
