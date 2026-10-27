package com.anpfuel.domain.driver

import java.math.BigInteger
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows

class DriverIntegerTest {
    @Test fun exactLongBoundsAndOutsideValues() {
        for (value in listOf(Long.MIN_VALUE, -1L, 0L, 1L, Long.MAX_VALUE)) {
            assertEquals(value, BigInteger.valueOf(value).driverLongExact())
        }
        assertThrows(ArithmeticException::class.java) {
            (BigInteger.valueOf(Long.MAX_VALUE) + BigInteger.ONE).driverLongExact()
        }
        assertThrows(ArithmeticException::class.java) {
            (BigInteger.valueOf(Long.MIN_VALUE) - BigInteger.ONE).driverLongExact()
        }
    }
}
