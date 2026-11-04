package com.anpfuel.app.ui.map

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import android.location.Location
import android.location.LocationManager
import android.os.SystemClock
import androidx.core.content.ContextCompat
import androidx.core.location.LocationManagerCompat
import androidx.core.os.CancellationSignal
import androidx.core.util.Consumer
import com.anpfuel.domain.routing.RoutePoint
import io.mockk.every
import io.mockk.mockk
import io.mockk.mockkStatic
import io.mockk.unmockkStatic
import io.mockk.verify
import java.util.concurrent.Executor
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.async
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
@Suppress("DEPRECATION")
class RouteCurrentFixTest {
    private val context = mockk<Context>()
    private val manager = mockk<LocationManager>()
    private data class Lookup(val cancel: CancellationSignal, val callback: Consumer<Location>)

    private val requests = mutableMapOf<String, MutableList<Lookup>>()
    private val now = 60_000_000_000L
    private val origin = RoutePoint(-15.54, -55.17, "Current position")

    @BeforeEach fun setup() {
        mockkStatic(ContextCompat::class, LocationManagerCompat::class, SystemClock::class)
        every { ContextCompat.checkSelfPermission(context, any()) } returns PackageManager.PERMISSION_GRANTED
        every { context.getSystemService(LocationManager::class.java) } returns manager
        every { LocationManagerCompat.isLocationEnabled(manager) } returns true
        every { manager.getProviders(true) } returns listOf(LocationManager.GPS_PROVIDER, "fused")
        every { manager.getLastKnownLocation(any()) } returns null
        every { SystemClock.elapsedRealtimeNanos() } returns now
        every { ContextCompat.getMainExecutor(context) } returns Executor { it.run() }
        every { LocationManagerCompat.getCurrentLocation(manager, any(), any<CancellationSignal>(), any(), any()) } answers {
            val provider = secondArg<String>()
            val signal = thirdArg<CancellationSignal>()
            val callback = arg<Consumer<Location>>(4)
            requests.getOrPut(provider) { mutableListOf() }.add(Lookup(signal, callback))
        }
    }

    @AfterEach fun teardown() {
        unmockkStatic(ContextCompat::class, LocationManagerCompat::class, SystemClock::class)
    }

    @Test fun `recent map position is usable immediately without requesting GPS`() = runTest {
        assertEquals(origin, findCurrentRoutePoint(context, origin.label) { fix() })
        verify(exactly = 0) { manager.getProviders(any()) }
    }

    @Test fun `expired map and inaccurate GPS fixes fall back to the fused cache`() = runTest {
        every { manager.getLastKnownLocation(LocationManager.GPS_PROVIDER) } returns fix(accuracy = 300f)
        every { manager.getLastKnownLocation("fused") } returns fix(sampledAt = now - 1)
        assertEquals(origin, findCurrentRoutePoint(context, origin.label) { fix(sampledAt = now - 16_000_000_000L) })
        assertTrue(requests.isEmpty())
    }

    @Test fun `fresh lookup retries an inaccurate fused fix and cancels the pending GPS lookup`() = runTest {
        val lookup = async { findCurrentRoutePoint(context, origin.label) { null } }
        runCurrent()
        val gps = requests.getValue(LocationManager.GPS_PROVIDER).single()
        val fused = requests.getValue("fused")
        fused.single().callback.accept(fix(accuracy = 101f))
        runCurrent()
        assertFalse(lookup.isCompleted)
        assertEquals(1, fused.size)
        advanceTimeBy(1_001)
        assertEquals(2, fused.size)
        fused.last().callback.accept(fix())
        assertEquals(origin, lookup.await())
        assertTrue(gps.cancel.isCanceled)
    }

    @Test fun `timeout cancels both lookups and does not route from an old position`() = runTest {
        val lookup = async { findCurrentRoutePoint(context, origin.label) { fix(sampledAt = 1) } }
        runCurrent()
        advanceTimeBy(15_001)
        assertNull(lookup.await())
        assertTrue(requests.getValue(LocationManager.GPS_PROVIDER).single().cancel.isCanceled)
        assertTrue(requests.getValue("fused").single().cancel.isCanceled)
    }

    @Test fun `closing the preview cancels all outstanding provider calls`() = runTest {
        val lookup = async { findCurrentRoutePoint(context, origin.label) { null } }
        runCurrent()
        lookup.cancelAndJoin()
        assertTrue(requests.getValue(LocationManager.GPS_PROVIDER).single().cancel.isCanceled)
        assertTrue(requests.getValue("fused").single().cancel.isCanceled)
    }

    @Test fun `a provider removed before the request does not block the other provider`() = runTest {
        every {
            LocationManagerCompat.getCurrentLocation(manager, LocationManager.GPS_PROVIDER, any<CancellationSignal>(), any(), any())
        } throws IllegalArgumentException("removed")
        val lookup = async { findCurrentRoutePoint(context, origin.label) { null } }
        runCurrent()
        requests.getValue("fused").single().callback.accept(fix())
        assertEquals(origin, lookup.await())
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
