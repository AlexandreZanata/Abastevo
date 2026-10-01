package com.anpfuel.data.local.ocr

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P10-T04: default OCR adapter delegates to the portable parser without
 * network, sorting or minimum-picking.
 */
class LocalRegexPriceOcrTest {

    private val ocr = LocalRegexPriceOcr()

    @Test
    fun `parses marked price exactly`() {
        val out = ocr.candidatesFromText("R$ 5,89")
        assertEquals(1, out.size)
        assertEquals(5890L, out[0].priceMilli)
    }

    @Test
    fun `keeps multiple candidates in order`() {
        val out = ocr.candidatesFromText("R$ 6,19\nR$ 5,89")
        assertEquals(2, out.size)
        assertEquals(6190L, out[0].priceMilli)
        assertEquals(5890L, out[1].priceMilli)
    }

    @Test
    fun `empty and invalid text yields empty set`() {
        assertTrue(ocr.candidatesFromText("").isEmpty())
        assertTrue(ocr.candidatesFromText("sem preco").isEmpty())
    }
}
