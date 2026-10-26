package com.anpfuel.app.ui.map

import android.content.Context
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.domain.routing.NativeRouteRequest
import com.anpfuel.domain.routing.NativeRouteResult
import com.anpfuel.domain.routing.RoutePoint
import com.anpfuel.maps.engine.MapCamera
import com.anpfuel.maps.engine.MapCameraStore
import com.anpfuel.maps.engine.MapEngineHolder
import com.anpfuel.maps.engine.MapHost
import java.io.File
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** MAP-23 foreground service on the owned phone: FGS, notification stop, recovery. */
@RunWith(AndroidJUnit4::class)
class GuidanceServiceInstrumentedTest {
    @get:Rule val compose = createComposeRule()

    private class FakeStore : MapCameraStore {
        override fun load(): MapCamera? = MapCamera(-25.4295, -49.2719, 15)
        override fun save(camera: MapCamera) = Unit
    }

    @Test fun foregroundServiceStartsStopsAndRecoversExplicitly() {
        val appContext = ApplicationProvider.getApplicationContext<Context>()
        assertTrue("admitted regional fixture is required", File(appContext.filesDir, "261004/Brazil_Parana_East.mwm").isFile)
        val engine = MapEngineHolder.engine
        compose.setContent { MapHost(engine, cameraStore = FakeStore()) }
        compose.waitForIdle()
        var ready = false
        for (attempt in 1..90) {
            InstrumentationRegistry.getInstrumentation().runOnMainSync {
                ready = engine.isInitialized() && engine.isRenderingReady()
            }
            if (ready) break
            Thread.sleep(1000)
        }
        assertTrue("map surface ready", ready)
        val request = NativeRouteRequest(
            listOf(
                RoutePoint(-25.4295, -49.2719, "Synthetic origin"),
                RoutePoint(-25.4222, -49.2640, "Synthetic destination"),
            ),
            emptySet(),
            "en",
        )
        val resultRef = AtomicReference<NativeRouteResult>()
        val buildLatch = CountDownLatch(1)
        InstrumentationRegistry.getInstrumentation().runOnMainSync {
            assertTrue(engine.buildRoute(request) { resultRef.set(it); buildLatch.countDown() })
        }
        assertTrue(buildLatch.await(60, TimeUnit.SECONDS))
        assertTrue(resultRef.get() is NativeRouteResult.Ready)
        val intent = GuidanceService.startIntent(appContext, "anonymous", request)
        androidx.core.content.ContextCompat.startForegroundService(appContext, intent)
        var active = false
        for (i in 1..30) {
            Thread.sleep(1000)
            if (GuidanceServiceState.current().active) {
                active = true
                break
            }
        }
        assertTrue("service must become active, state=${GuidanceServiceState.current()}", active)
        appContext.startService(GuidanceService.stopIntent(appContext))
        var stopped = false
        for (i in 1..15) {
            Thread.sleep(1000)
            if (!GuidanceServiceState.current().active) {
                stopped = true
                break
            }
        }
        assertTrue("service must stop via notification action", stopped)
        assertFalse(GuidanceServiceState.current().resumeAvailable)
    }
}
