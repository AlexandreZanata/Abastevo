package com.anpfuel.application.capture

import com.anpfuel.application.port.CaptureFix
import com.anpfuel.application.port.isEligibleForPhotoCapture
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class CaptureFixTest {
    private val now = 1_000_000L
    private val valid = CaptureFix(-12.5, -55.7, 12.0, now, true, true, false)

    @Test fun `fresh finite non mocked OS snapshot is required`() {
        assertTrue(valid.isEligibleForPhotoCapture(now))
        assertTrue(valid.copy(accuracyMeters = 100.0, capturedAtMillis = now - 120_000).isEligibleForPhotoCapture(now))
        listOf(
            valid.copy(permissionGranted = false), valid.copy(sourceInfoPresent = false),
            valid.copy(simulated = true), valid.copy(latitude = Double.NaN),
            valid.copy(longitude = Double.POSITIVE_INFINITY), valid.copy(latitude = 90.1),
            valid.copy(longitude = -180.1), valid.copy(accuracyMeters = -1.0),
            valid.copy(accuracyMeters = Double.NaN), valid.copy(accuracyMeters = 100.1),
            valid.copy(capturedAtMillis = now + 1), valid.copy(capturedAtMillis = now - 120_001),
        ).forEach { assertFalse(it.isEligibleForPhotoCapture(now)) }
    }
}
