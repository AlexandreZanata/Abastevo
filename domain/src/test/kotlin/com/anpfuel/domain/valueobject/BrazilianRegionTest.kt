package com.anpfuel.domain.valueobject

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class BrazilianRegionTest {

    @Test
    fun enumContainsFiveRegions() {
        assertEquals(5, BrazilianRegion.entries.size)
        assertEquals(
            setOf(
                BrazilianRegion.NORTH,
                BrazilianRegion.NORTHEAST,
                BrazilianRegion.CENTRAL_WEST,
                BrazilianRegion.SOUTHEAST,
                BrazilianRegion.SOUTH,
            ),
            BrazilianRegion.entries.toSet(),
        )
    }
}
