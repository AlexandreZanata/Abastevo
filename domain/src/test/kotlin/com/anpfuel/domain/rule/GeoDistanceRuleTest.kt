package com.anpfuel.domain.rule

import com.anpfuel.domain.valueobject.GeoCoordinates
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class GeoDistanceRuleTest {

    private val curitiba = GeoCoordinates.of(-25.4284, -49.2733)

    @Test
    fun distanceFromPointToItselfIsZero() {
        assertEquals(0.0, GeoDistanceRule.distanceMeters(curitiba, curitiba), 0.0001)
    }

    @Test
    fun measuresKnownDistanceBetweenCities() {
        val saoPaulo = GeoCoordinates.of(-23.5505, -46.6333)

        val distance = GeoDistanceRule.distanceMeters(curitiba, saoPaulo)

        assertTrue(distance in 330_000.0..350_000.0, "Expected ~338 km, got $distance")
    }

    @Test
    fun isSymmetric() {
        val saoPaulo = GeoCoordinates.of(-23.5505, -46.6333)

        assertEquals(
            GeoDistanceRule.distanceMeters(curitiba, saoPaulo),
            GeoDistanceRule.distanceMeters(saoPaulo, curitiba),
            0.0001,
        )
    }

    @Test
    fun measuresSmallLocalDistances() {
        val oneThousandthDegreeNorth = GeoCoordinates.of(-25.4274, -49.2733)

        val distance = GeoDistanceRule.distanceMeters(curitiba, oneThousandthDegreeNorth)

        assertTrue(distance in 100.0..120.0, "Expected ~111 m, got $distance")
    }
}
