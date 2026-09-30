package com.anpfuel.domain.portable

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class PortableMoneyTest {

    @Test
    fun parsesExactThreeDecimalAnpText() {
        assertEquals(5999L, PortableMoney.parse("5,999"))
        assertEquals(4599L, PortableMoney.parse("4,599"))
        assertEquals(109900L, PortableMoney.parse("109,90"))
    }

    @Test
    fun parsesTwoDecimalIntegerAndPaddedText() {
        assertEquals(5990L, PortableMoney.parse("5,99"))
        assertEquals(6000L, PortableMoney.parse("6"))
        assertEquals(5999L, PortableMoney.parse("  5,999  "))
        assertEquals(5999L, PortableMoney.parse("5,9990"))
    }

    @Test
    fun acceptsRangeBoundaries() {
        assertEquals(1L, PortableMoney.parse("0,001"))
        assertEquals(1000000L, PortableMoney.parse("1000,00"))
        assertEquals(1000000L, PortableMoney.parse("1000,000"))
    }

    @Test
    fun rejectsEmptyZeroAndNegative() {
        assertEquals("missing-price", PortableMoney.codeOf(""))
        assertEquals("missing-price", PortableMoney.codeOf("   "))
        assertEquals("zero-price", PortableMoney.codeOf("0,00"))
        assertEquals("negative-price", PortableMoney.codeOf("-1,00"))
        assertEquals("negative-price", PortableMoney.codeOf("-0,00"))
    }

    @Test
    fun rejectsOverPrecisionDotSeparatorsAndSigns() {
        assertEquals("over-precision", PortableMoney.codeOf("5,9999"))
        assertEquals("invalid-price", PortableMoney.codeOf("5.999"))
        assertEquals("invalid-price", PortableMoney.codeOf("1.099,90"))
        assertEquals("invalid-price", PortableMoney.codeOf("+5,00"))
        assertEquals("invalid-price", PortableMoney.codeOf("cinco"))
        assertEquals("invalid-price", PortableMoney.codeOf("5,"))
        assertEquals("invalid-price", PortableMoney.codeOf(",5"))
        assertEquals("over-range", PortableMoney.codeOf("1000,01"))
    }

    @Test
    fun parseThrowsWithStableCode() {
        val error = assertThrows(PortableMoneyException::class.java) {
            PortableMoney.parse("5,9999")
        }
        assertEquals("over-precision", error.code)
    }

    @Test
    fun formatsCanonicalCommaText() {
        assertEquals("5,999", PortableMoney.format(5999L))
        assertEquals("5,990", PortableMoney.format(5990L))
        assertEquals("6,000", PortableMoney.format(6000L))
        assertEquals("0,001", PortableMoney.format(1L))
        assertEquals("1000,000", PortableMoney.format(1000000L))
    }

    @Test
    fun multipliesTankFillExactly() {
        assertEquals(274950L, PortableMoney.multiplyTankFill(5499L, 50_000L))
        assertEquals(299500L, PortableMoney.multiplyTankFill(5990L, 50_000L))
        assertEquals(124000L, PortableMoney.multiplyTankFill(3100L, 40_000L))
    }

    @Test
    fun multipliesTankFillWithHalfUpRounding() {
        // 1001 * 500 / 1000 = 500.5 -> 501
        assertEquals(501L, PortableMoney.multiplyTankFill(1001L, 500L))
    }

    @Test
    fun rejectsOutOfRangeMilliAndOverflow() {
        assertThrows(PortableMoneyException::class.java) {
            PortableMoney.format(0L)
        }
        assertThrows(PortableMoneyException::class.java) {
            PortableMoney.multiplyTankFill(1000001L, 1000L)
        }
        assertThrows(PortableMoneyException::class.java) {
            PortableMoney.multiplyTankFill(Long.MAX_VALUE, 200_000L)
        }
    }

    @Test
    fun parsesTankCapacityUpTo200Liters() {
        assertEquals(50_000L, PortableMoney.parseCapacityMilliLiters("50"))
        assertEquals(50_500L, PortableMoney.parseCapacityMilliLiters("50,5"))
        assertEquals(200_000L, PortableMoney.parseCapacityMilliLiters("200"))
        assertThrows(PortableMoneyException::class.java) {
            PortableMoney.parseCapacityMilliLiters("0")
        }
        assertThrows(PortableMoneyException::class.java) {
            PortableMoney.parseCapacityMilliLiters("200,001")
        }
    }
}
