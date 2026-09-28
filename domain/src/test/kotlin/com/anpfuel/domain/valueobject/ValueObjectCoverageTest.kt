package com.anpfuel.domain.valueobject

import com.anpfuel.domain.exception.DomainException
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ValueObjectCoverageTest {

    @Test
    fun priceAmountSupportsStringAndDoubleFactories() {
        val fromString = PriceAmount.of("4.567")
        val fromDouble = PriceAmount.of(4.567)

        assertEquals(fromString, fromDouble)
        assertEquals(fromString.hashCode(), fromDouble.hashCode())
        assertTrue(fromString.toString().contains("4.57"))
        assertNotEquals(fromString, PriceAmount.of("4.58"))
    }

    @Test
    fun geographicScopeAndBrazilianRegionEnumerateAllValues() {
        assertEquals(5, GeographicScope.entries.size)
        assertEquals(5, BrazilianRegion.entries.size)
        assertTrue(GeographicScope.MUNICIPALITY.name.isNotBlank())
        assertTrue(BrazilianRegion.SOUTHEAST.name.isNotBlank())
    }

    @Test
    fun domainExceptionPreservesCause() {
        val cause = IllegalStateException("root")
        val exception = DomainException("wrapped", cause)

        assertEquals("wrapped", exception.message)
        assertEquals(cause, exception.cause)
    }
}
