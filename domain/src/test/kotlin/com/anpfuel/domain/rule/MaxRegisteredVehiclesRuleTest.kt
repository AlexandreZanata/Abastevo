package com.anpfuel.domain.rule

import com.anpfuel.domain.exception.DomainException
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class MaxRegisteredVehiclesRuleTest {

    @Test
    fun canRegisterUntilLimitReached() {
        assertTrue(MaxRegisteredVehiclesRule.canRegister(0))
        assertTrue(MaxRegisteredVehiclesRule.canRegister(2))
        assertFalse(MaxRegisteredVehiclesRule.canRegister(3))
    }

    @Test
    fun requireCanRegisterThrowsAtLimit() {
        assertThrows(DomainException::class.java) {
            MaxRegisteredVehiclesRule.requireCanRegister(3)
        }
    }
}
