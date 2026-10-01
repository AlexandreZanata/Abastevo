package com.anpfuel.data.local.location

import com.anpfuel.application.portable.LocationSignal
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class LocationReadingTest {

    @Test
    fun deniedPermissionShortCircuits() {
        val signal = LocationReading.signal(
            permissionGranted = false,
            locationPresent = false,
            sourceInfoAvailable = false,
            simulated = false,
            accuracyMeters = null,
            fixAgeSeconds = null,
        )
        assertFalse(signal.permissionGranted)
        assertEquals(LocationSignal.denied(), signal)
    }

    @Test
    fun absentFixWithoutPermission() {
        val signal = LocationReading.signal(
            permissionGranted = true,
            locationPresent = false,
            sourceInfoAvailable = false,
            simulated = false,
            accuracyMeters = null,
            fixAgeSeconds = null,
        )
        assertTrue(signal.permissionGranted)
        assertFalse(signal.sourceInfoAvailable)
        assertEquals(LocationSignal.absent(), signal)
    }

    @Test
    fun mapsRealFixVerbatim() {
        val signal = LocationReading.signal(
            permissionGranted = true,
            locationPresent = true,
            sourceInfoAvailable = true,
            simulated = false,
            accuracyMeters = 25.0,
            fixAgeSeconds = 30L,
            clockSkewSeconds = 10L,
        )
        assertTrue(signal.permissionGranted)
        assertTrue(signal.sourceInfoAvailable)
        assertFalse(signal.simulated)
        assertTrue(signal.hasAccuracy)
        assertEquals(25.0, signal.accuracyMeters)
        assertEquals(30L, signal.fixAgeSeconds)
        assertEquals(10L, signal.clockSkewSeconds)
        assertFalse(signal.testInjected)
    }

    @Test
    fun negativeAccuracyFoldsToMissing() {
        val signal = LocationReading.signal(
            permissionGranted = true,
            locationPresent = true,
            sourceInfoAvailable = true,
            simulated = false,
            accuracyMeters = -1.0,
            fixAgeSeconds = 5L,
        )
        assertFalse(signal.hasAccuracy, "negative accuracy is invalid data, not a precise fix")
    }

    @Test
    fun mockFlagPassesThroughUntouched() {
        val signal = LocationReading.signal(
            permissionGranted = true,
            locationPresent = true,
            sourceInfoAvailable = true,
            simulated = true,
            accuracyMeters = 10.0,
            fixAgeSeconds = 5L,
        )
        assertTrue(signal.simulated)
        assertTrue(signal.sourceInfoAvailable)
    }

    @Test
    fun unknownAgePassesThroughForContract() {
        val signal = LocationReading.signal(
            permissionGranted = true,
            locationPresent = true,
            sourceInfoAvailable = true,
            simulated = false,
            accuracyMeters = 25.0,
            fixAgeSeconds = null,
        )
        assertNull(signal.fixAgeSeconds, "unknown age reaches the contract, which refuses it")
    }
}
