package com.anpfuel.maps.engine

import android.content.Context
import android.os.SystemClock
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import app.organicmaps.sdk.location.TrackRecorder
import com.anpfuel.domain.routing.GuidancePolicy
import com.anpfuel.domain.routing.NativeRouteRequest
import com.anpfuel.domain.routing.NativeRouteResult
import com.anpfuel.domain.routing.NavigationFix
import com.anpfuel.domain.routing.RoutePoint
import java.io.File
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** MAP-23 native guidance proof: no trace, binding, progress, MWM fence, stop. */
@RunWith(AndroidJUnit4::class)
class GuidanceEngineInstrumentedTest {
    @get:Rule val compose = createComposeRule()
    private val instrumentation = InstrumentationRegistry.getInstrumentation()

    private class FakeStore : MapCameraStore {
        override fun load(): MapCamera? = MapCamera(-25.4295, -49.2719, 15)
        override fun save(camera: MapCamera) = Unit
    }

    @Test fun guidanceDisablesTrackBindsPreviewFeedsProgressAndStops() {
        val appContext = ApplicationProvider.getApplicationContext<Context>()
        assertTrue("admitted regional fixture is required", File(appContext.filesDir, "261004/Brazil_Parana_East.mwm").isFile)
        val engine = ComapsMapEngine()
        compose.setContent { MapHost(engine, cameraStore = FakeStore()) }
        compose.waitForIdle()
        // Wait for native readiness via polling on the main thread.
        var ready = false
        for (attempt in 1..90) {
            instrumentation.runOnMainSync { ready = engine.isInitialized() && engine.isRenderingReady() }
            if (ready) break
            Thread.sleep(1000)
        }
        assertTrue("map surface ready", ready)
        val start = RoutePoint(-25.4295, -49.2719, "Synthetic origin")
        val destination = RoutePoint(-25.4222, -49.2640, "Synthetic destination")
        val request = NativeRouteRequest(listOf(start, destination), emptySet(), "en")
        val result = AtomicReference<NativeRouteResult>()
        val latch = CountDownLatch(1)
        instrumentation.runOnMainSync {
            assertTrue(engine.buildRoute(request) { result.set(it); latch.countDown() })
        }
        assertTrue(latch.await(60, TimeUnit.SECONDS))
        assertTrue("preview expected: ${result.get()}", result.get() is NativeRouteResult.Ready)
        val preview = (result.get() as NativeRouteResult.Ready).preview
        val midpoint = preview.geometry[midpointIndex(preview)]
        var started = false
        instrumentation.runOnMainSync { started = engine.startGuidance(request) }
        assertTrue("guidance start", started)
        instrumentation.runOnMainSync { assertTrue(engine.isGuiding()) }
        instrumentation.runOnMainSync {
            assertFalse("navigation must not record a trace", TrackRecorder.nativeIsTrackRecordingEnabled())
            assertFalse("MWM swap must refuse while guiding", engine.suspendMapReads())
        }

        val now = SystemClock.elapsedRealtimeNanos()
        val fix = NavigationFix(-25.4295, -49.2719, 5f, System.currentTimeMillis(), now)
        var feed: GuidanceFeed? = null
        instrumentation.runOnMainSync { feed = engine.feedGuidanceFix(fix, now) }
        assertTrue("feed expected", feed != null)
        assertFalse(feed!!.rerouteRecoverable)
        try {
            val baseElapsed = SystemClock.elapsedRealtimeNanos()
            val replaySeconds = 150L
            val baseMillis = System.currentTimeMillis() - replaySeconds * 1_000L
            var offFeed: GuidanceFeed? = null
            var sawOffRoute = false
            var sawRecoverable = false
            // Fresh 1 Hz replay beside the road for >60 s. A single 61 s-old fix is
            // rejected by NavigationFixPolicy and cannot start the timeout.
            for (index in 0..replaySeconds.toInt()) {
                val simulatedNow = baseElapsed + index * 1_000_000_000L
                val offRoute = NavigationFix(
                    midpoint.latitude, midpoint.longitude + 0.003 + index * 0.00005, 5f,
                    baseMillis + index * 1_000L, simulatedNow,
                    speedMetersPerSecond = 5f, bearingDegrees = 0f,
                )
                instrumentation.runOnMainSync { offFeed = engine.feedGuidanceFix(offRoute, simulatedNow) }
                val state = offFeed?.progress?.nativeState
                if (state == "OffRoute" || state == "RouteRebuilding" || state == "RouteNeedsRebuild") {
                    sawOffRoute = true
                }
                if (offFeed?.rerouteRecoverable == true) {
                    sawRecoverable = true
                    android.util.Log.d("GuidanceOffRoute", "recoverable at index=$index state=$state")
                }
            }
            assertTrue("native guidance must report off-route/rebuild", sawOffRoute)
            assertTrue("off-route beyond timeout must surface recoverable failure", sawRecoverable)
        } finally {
            instrumentation.runOnMainSync {
                assertFalse("trace started while feeding", TrackRecorder.nativeIsTrackRecordingEnabled())
                engine.stopGuidance()
                assertFalse(engine.isGuiding())
                engine.cancelRoute()
            }
        }
    }

    private fun midpointIndex(preview: com.anpfuel.domain.routing.NativeRoutePreview): Int =
        (preview.geometry.size / 2).coerceIn(0, preview.geometry.lastIndex)
}
