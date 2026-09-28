package com.anpfuel.domain.valueobject

import com.anpfuel.domain.exception.DomainException
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

class DeviceLocationTest {

    @Test
    fun createsValidCoordinates() {
        val location = DeviceLocation.of(-25.4284, -49.2733)

        assertEquals(-25.4284, location.latitude)
        assertEquals(-49.2733, location.longitude)
    }

    @Test
    fun rejectsLatitudeOutOfRange() {
        assertThrows<DomainException> {
            DeviceLocation.of(91.0, 0.0)
        }
    }

    @Test
    fun rejectsLongitudeOutOfRange() {
        assertThrows<DomainException> {
            DeviceLocation.of(0.0, 181.0)
        }
    }
}
