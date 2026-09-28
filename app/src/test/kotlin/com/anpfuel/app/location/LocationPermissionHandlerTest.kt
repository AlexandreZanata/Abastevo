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
}
