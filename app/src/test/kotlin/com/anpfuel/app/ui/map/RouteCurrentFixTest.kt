package com.anpfuel.app.ui.map

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import android.location.Location
import android.location.LocationManager
import android.os.Looper
import android.os.SystemClock
import androidx.core.content.ContextCompat
import androidx.core.location.LocationListenerCompat
import androidx.core.location.LocationManagerCompat
import com.anpfuel.domain.routing.RoutePoint
import io.mockk.every
import io.mockk.mockk
import io.mockk.mockkStatic
import io.mockk.slot
import io.mockk.unmockkStatic
import io.mockk.verify
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.async
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class RouteCurrentFixTest {
    private val context = mockk<Context>()
    private val manager = mockk<LocationManager>()
    private val listener = slot<LocationListenerCompat>()
    private val now = 60_000_000_000L
    private val origin = RoutePoint(-15.54, -55.17, "Current position")

    @BeforeEach fun setup() {
        mockkStatic(ContextCompat::class, LocationManagerCompat::class, SystemClock::class, Looper::class)
        every { ContextCompat.checkSelfPermission(context, any()) } returns PackageManager.PERMISSION_GRANTED
        every { context.getSystemService(LocationManager::class.java) } returns manager
        every { LocationManagerCompat.isLocationEnabled(manager) } returns true
        every { manager.getProviders(true) } returns listOf(LocationManager.GPS_PROVIDER, "fused")
        every { manager.getLastKnownLocation(any()) } returns null
        every { SystemClock.elapsedRealtimeNanos() } returns now
        every { Looper.getMainLooper() } returns mockk()
        every { LocationManagerCompat.requestLocationUpdates(manager, any(), any(), capture(listener), any<Looper>()) } returns Unit
        every { LocationManagerCompat.removeUpdates(manager, any()) } returns Unit
    }

    @AfterEach fun teardown() {
        unmockkStatic(ContextCompat::class, LocationManagerCompat::class, SystemClock::class, Looper::class)
    }

    @Test fun `recent map position is usable immediately without requesting GPS`() = runTest {
        assertEquals(origin, findCurrentRoutePoint(context, origin.label) { fix() })
        verify(exactly = 0) { manager.getProviders(any()) }
    }

    @Test fun `expired map and inaccurate GPS fixes fall back to the fused cache`() = runTest {
        every { manager.getLastKnownLocation(LocationManager.GPS_PROVIDER) } returns fix(accuracy = 300f)
        every { manager.getLastKnownLocation("fused") } returns fix(sampledAt = now - 1)
        assertEquals(origin, findCurrentRoutePoint(context, origin.label) { fix(sampledAt = now - 16_000_000_000L) })
        verify(exactly = 0) { LocationManagerCompat.requestLocationUpdates(manager, any(), any(), any(), any<Looper>()) }
    }

    @Test fun `fresh lookup skips bad updates and unregisters both sources on success`() = runTest {
        val lookup = async { findCurrentRoutePoint(context, origin.label) { null } }
        runCurrent()
        verify { LocationManagerCompat.requestLocationUpdates(manager, LocationManager.GPS_PROVIDER, any(), any(), any<Looper>()) }
        verify { LocationManagerCompat.requestLocationUpdates(manager, "fused", any(), any(), any<Looper>()) }
        listener.captured.onLocationChanged(fix(accuracy = 101f))
        assertFalse(lookup.isCompleted)
        listener.captured.onLocationChanged(fix())
        assertEquals(origin, lookup.await())
        verify(exactly = 1) { LocationManagerCompat.removeUpdates(manager, listener.captured) }
    }

    @Test fun `timeout unregisters the lookup and does not route from an old position`() = runTest {
        val lookup = async { findCurrentRoutePoint(context, origin.label) { fix(sampledAt = 1) } }
        runCurrent()
        advanceTimeBy(15_001)
        assertNull(lookup.await())
        verify(exactly = 1) { LocationManagerCompat.removeUpdates(manager, listener.captured) }
    }

    @Test fun `coarse permission cannot use a saved precise map fix`() = runTest {
        every { ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION) } returns
            PackageManager.PERMISSION_DENIED
        every { manager.getLastKnownLocation("fused") } returns fix()
        assertEquals(origin, findCurrentRoutePoint(context, origin.label) { error("Precise map fix was read") })
        verify(exactly = 0) { manager.getLastKnownLocation(LocationManager.GPS_PROVIDER) }
    }

    private fun fix(sampledAt: Long = now, accuracy: Float = 15f): Location = mockk {
        every { elapsedRealtimeNanos } returns sampledAt
        every { hasAccuracy() } returns true
        every { this@mockk.accuracy } returns accuracy
        every { latitude } returns origin.latitude
        every { longitude } returns origin.longitude
    }
}
