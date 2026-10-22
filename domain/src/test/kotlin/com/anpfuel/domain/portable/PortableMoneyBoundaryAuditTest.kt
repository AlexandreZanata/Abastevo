package com.anpfuel.domain.portable

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

class PortableMoneyBoundaryAuditTest {
    @Test
    fun capacityGrammarAndBoundsRefuseInsteadOfRounding() {
        for ((text, code) in listOf(
            "" to "missing-price", "-1" to "negative-price", "+1" to "invalid-price",
            ",1" to "invalid-price", "1," to "invalid-price", "1.2" to "invalid-price",
            "١" to "invalid-price", "1,0x" to "invalid-price", "1,0001" to "over-precision",
            "200,001" to "over-range", "200001" to "over-range", "0" to "zero-price",
        )) {
            assertEquals(code, assertThrows<PortableMoneyException>(text) { PortableMoney.parseCapacityMilliLiters(text) }.code)
        }
        assertEquals(1L, PortableMoney.parseCapacityMilliLiters("0,001"))
        assertEquals(200000L, PortableMoney.parseCapacityMilliLiters("200,0000"))
        assertEquals(40500L, PortableMoney.parseCapacityMilliLiters(" 0040,5000 "))
        assertEquals(1000L, PortableMoney.parseCapacityMilliLiters("1"))
    }

    @Test
    fun classificationFormattingAndTankEstimateBoundaries() {
        for ((text, code) in listOf("5,999" to "ok", "" to "missing-price", "0" to "zero-price", "5,9999" to "over-precision", "1000,001" to "over-range")) {
            assertEquals(code, PortableMoney.codeOf(text))
        }
        for ((value, code) in listOf(-1L to "invalid-price", 0L to "zero-price", 1000001L to "over-range")) {
            assertEquals(code, assertThrows<PortableMoneyException> { PortableMoney.format(value) }.code)
        }
        for ((unit, capacity) in listOf(0L to 1L, 1000001L to 1L, 1L to 0L, 1L to 200001L)) {
            assertEquals("over-range", assertThrows<PortableMoneyException> { PortableMoney.multiplyTankFill(unit, capacity) }.code)
        }
        assertEquals(242960L, PortableMoney.multiplyTankFill(5999, 40500))
        assertEquals(1L, PortableMoney.multiplyTankFill(500, 1))
    }
}
