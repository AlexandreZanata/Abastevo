package com.anpfuel.domain.valueobject

import com.anpfuel.domain.exception.DomainException
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

class GeoCoordinatesTest {

    @Test
    fun createsValidCoordinates() {
        val coordinates = GeoCoordinates.of(-25.4284, -49.2733)

        assertEquals(-25.4284, coordinates.latitude)
        assertEquals(-49.2733, coordinates.longitude)
    }

    @Test
    fun acceptsBoundaryValues() {
        assertEquals(90.0, GeoCoordinates.of(90.0, 180.0).latitude)
        assertEquals(-180.0, GeoCoordinates.of(-90.0, -180.0).longitude)
    }

    @Test
    fun rejectsLatitudeOutOfRange() {
        assertThrows<DomainException> { GeoCoordinates.of(90.1, 0.0) }
        assertThrows<DomainException> { GeoCoordinates.of(-90.1, 0.0) }
    }

    @Test
    fun rejectsLongitudeOutOfRange() {
        assertThrows<DomainException> { GeoCoordinates.of(0.0, 180.1) }
        assertThrows<DomainException> { GeoCoordinates.of(0.0, -180.1) }
    }

    @Test
    fun rejectsNotANumber() {
        assertThrows<DomainException> { GeoCoordinates.of(Double.NaN, 0.0) }
    }

    @Test
    fun equalityIsValueBased() {
        val coordinates = GeoCoordinates.of(-25.4284, -49.2733)

        assertEquals(coordinates, GeoCoordinates.of(-25.4284, -49.2733))
        assertNotEquals(coordinates, GeoCoordinates.of(-25.4284, -49.2734))
        assertEquals(coordinates.hashCode(), GeoCoordinates.of(-25.4284, -49.2733).hashCode())
    }

    @Test
    fun deviceLocationDelegatesToGeoCoordinates() {
        val location = DeviceLocation.of(-25.4284, -49.2733)

        assertEquals(GeoCoordinates.of(-25.4284, -49.2733), location.coordinates)
        assertEquals(location.coordinates.hashCode(), DeviceLocation.of(-25.4284, -49.2733).hashCode())
        assertThrows<DomainException> { DeviceLocation.of(91.0, 0.0) }
    }
}
