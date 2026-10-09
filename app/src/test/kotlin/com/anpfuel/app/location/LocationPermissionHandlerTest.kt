package com.anpfuel.app.location

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import androidx.core.content.ContextCompat
import io.mockk.every
import io.mockk.mockk
import io.mockk.mockkStatic
import io.mockk.unmockkStatic
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class LocationPermissionHandlerTest {

    private val context = mockk<Context>(relaxed = true)
    private lateinit var handler: LocationPermissionHandler

    @BeforeEach
    fun setUp() {
        mockkStatic(ContextCompat::class)
        handler = LocationPermissionHandler(context)
    }

    @AfterEach
    fun tearDown() {
        unmockkStatic(ContextCompat::class)
    }

    @Test
    fun hasLocationPermissionWhenFineGranted() {
        every {
            ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION)
        } returns PackageManager.PERMISSION_GRANTED
        every {
            ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_COARSE_LOCATION)
        } returns PackageManager.PERMISSION_DENIED

        assertTrue(handler.hasLocationPermission())
    }

    @Test
    fun hasLocationPermissionWhenCoarseGranted() {
        every {
            ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION)
        } returns PackageManager.PERMISSION_DENIED
        every {
            ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_COARSE_LOCATION)
        } returns PackageManager.PERMISSION_GRANTED

        assertTrue(handler.hasLocationPermission())
    }

    @Test
    fun lacksLocationPermissionWhenBothDenied() {
        every {
            ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION)
        } returns PackageManager.PERMISSION_DENIED
        every {
            ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_COARSE_LOCATION)
        } returns PackageManager.PERMISSION_DENIED

        assertFalse(handler.hasLocationPermission())
    }
    @Test
    fun revokedPermissionDuringCachedFixReturnsNoLocation() {
        every { ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION) } returns PackageManager.PERMISSION_GRANTED
        val manager = mockk<android.location.LocationManager>()
        every { context.getSystemService(android.location.LocationManager::class.java) } returns manager
        every { manager.allProviders } returns listOf("gps")
        every { manager.getLastKnownLocation("gps") } throws SecurityException("revoked")
        org.junit.jupiter.api.Assertions.assertNull(handler.getLastKnownLocation())
    }
    @Test fun captureSnapshotPreservesAccuracyTimeProviderAndRejectsFutureOrMissingAccuracy() = kotlinx.coroutines.test.runTest {
        every { ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION) } returns PackageManager.PERMISSION_GRANTED
        val manager = mockk<android.location.LocationManager>()
        every { context.getSystemService(android.location.LocationManager::class.java) } returns manager
        every { manager.allProviders } returns listOf("gps")
        every { manager.getProviders(true) } returns emptyList()
        val location = mockk<android.location.Location>()
        val now = System.currentTimeMillis()
        every { manager.getLastKnownLocation("gps") } returns location
        every { location.time } returns now
        every { location.hasAccuracy() } returns true
        every { location.latitude } returns -12.5
        every { location.longitude } returns -55.7
        every { location.accuracy } returns 12.0f
        every { location.provider } returns "gps"
        every { location.isFromMockProvider } returns false
        every { location.isMock } returns false
        val fix = handler.getFreshCaptureFix()!!
        org.junit.jupiter.api.Assertions.assertEquals(now, fix.capturedAtMillis)
        org.junit.jupiter.api.Assertions.assertEquals(12.0, fix.accuracyMeters)
        assertTrue(fix.permissionGranted && fix.sourceInfoPresent && !fix.simulated)
        every { location.hasAccuracy() } returns false
        org.junit.jupiter.api.Assertions.assertNull(handler.getFreshCaptureFix())
        every { location.hasAccuracy() } returns true
        every { location.time } returns System.currentTimeMillis() + 60000
        org.junit.jupiter.api.Assertions.assertNull(handler.getFreshCaptureFix())
    }

}
