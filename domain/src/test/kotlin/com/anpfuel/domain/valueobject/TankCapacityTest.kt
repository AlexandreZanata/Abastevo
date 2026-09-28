package com.anpfuel.domain.valueobject

import com.anpfuel.domain.exception.DomainException
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test
import java.math.BigDecimal

class TankCapacityTest {

    @Test
    fun validCapacityIsAccepted() {
        val capacity = TankCapacity.of(50.0)

        assertEquals(BigDecimal("50.00"), capacity.liters)
    }

    @Test
    fun zeroCapacityThrowsDomainException() {
        assertThrows(DomainException::class.java) {
            TankCapacity.of(0.0)
        }
    }

    @Test
    fun negativeCapacityThrowsDomainException() {
        assertThrows(DomainException::class.java) {
            TankCapacity.of(-1.0)
        }
    }

    @Test
    fun capacityAboveMaxThrowsDomainException() {
        assertThrows(DomainException::class.java) {
            TankCapacity.of((TankCapacity.MAX_LITERS + 1).toDouble())
        }
    }

    @Test
    fun maxCapacityIsAccepted() {
        val capacity = TankCapacity.of(TankCapacity.MAX_LITERS.toDouble())

        assertEquals(BigDecimal("200.00"), capacity.liters)
    }
}
