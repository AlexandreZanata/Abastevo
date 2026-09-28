package com.anpfuel.domain.valueobject

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class BrazilianStateTest {

    @Test
    fun enumContainsTwentySevenStates() {
        assertEquals(27, BrazilianState.entries.size)
    }

    @Test
    fun eachStateHasUniqueAbbreviation() {
        val abbreviations = BrazilianState.entries.map { it.abbreviation }
        assertEquals(abbreviations.size, abbreviations.toSet().size)
    }

    @Test
    fun saoPauloHasSpAbbreviationAndSoutheastRegion() {
        assertEquals("SP", BrazilianState.SAO_PAULO.abbreviation)
        assertEquals(BrazilianRegion.SOUTHEAST, BrazilianState.SAO_PAULO.region)
    }

    @Test
    fun fromAbbreviationResolvesKnownState() {
        assertEquals(BrazilianState.PARANA, BrazilianState.fromAbbreviation("PR"))
        assertEquals(BrazilianState.PARANA, BrazilianState.fromAbbreviation("pr"))
    }

    @Test
    fun fromAbbreviationReturnsNullForUnknownAbbreviation() {
        assertNull(BrazilianState.fromAbbreviation("XX"))
    }
}
