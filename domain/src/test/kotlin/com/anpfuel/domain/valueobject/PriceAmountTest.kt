package com.anpfuel.domain.valueobject

import com.anpfuel.domain.exception.DomainException
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test
import java.math.BigDecimal

class PriceAmountTest {

    @Test
    fun nonNegativeValueWithTwoDecimalsIsAccepted() {
        val amount = PriceAmount.of(BigDecimal("5.49"))

        assertEquals(BigDecimal("5.49"), amount.value)
    }

    @Test
    fun valueIsRoundedToTwoDecimals() {
        val amount = PriceAmount.of(BigDecimal("5.499"))

        assertEquals(BigDecimal("5.50"), amount.value)
    }

    @Test
    fun negativeValueThrowsDomainException() {
        assertThrows(DomainException::class.java) {
            PriceAmount.of(BigDecimal("-0.01"))
        }
    }

    @Test
    fun excessivePrecisionIsRoundedToTwoDecimals() {
        val amount = PriceAmount.of(BigDecimal("1.234"))

        assertEquals(2, amount.value.scale())
        assertEquals(BigDecimal("1.23"), amount.value)
    }

    @Test
    fun zeroValueIsAccepted() {
        val amount = PriceAmount.of(BigDecimal.ZERO)

        assertEquals(BigDecimal("0.00"), amount.value)
    }
}
