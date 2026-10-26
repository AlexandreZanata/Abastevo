package com.anpfuel.app.ui.map

import android.content.Context
import android.os.SystemClock
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.domain.routing.GuidancePolicy
import com.anpfuel.domain.routing.NativeRouteRequest
import com.anpfuel.domain.routing.NativeRouteResult
import com.anpfuel.domain.routing.NavigationFix
import com.anpfuel.domain.routing.NavigationFixPolicy
import com.anpfuel.domain.routing.RoadAvoidance
import com.anpfuel.domain.routing.RoutePoint
import com.anpfuel.maps.engine.MapEngineHolder
import java.io.File
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

/** MAP-23 real native guidance on the owned phone; synthetic fixes only, no trace. */
@RunWith(AndroidJUnit4::class)
class GuidanceInstrumentedTest {
    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    private val engine get() = MapEngineHolder.engine
    private fun main(action: () -> Unit) = instrumentation.runOnMainSync(action)

    private fun initialize() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        assertTrue("admitted regional fixture is required", File(context.filesDir, "261004/Brazil_Parana_East.mwm").isFile)
        val ready = CountDownLatch(1)
        main { assertTrue(engine.initialize(context) { ready.countDown() }) }
        assertTrue("native readiness", ready.await(45, TimeUnit.SECONDS))
        main { assertTrue(engine.reloadMaps()) }
    }

    private fun build(points: List<RoutePoint>): NativeRouteRequest {
        val request = NativeRouteRequest(points, emptySet(), "en")
        val result = AtomicReference<NativeRouteResult>()
        val latch = CountDownLatch(1)
        main { assertTrue(engine.buildRoute(request) { result.set(it); latch.countDown() }) }
        assertTrue("native routing callback", latch.await(60, TimeUnit.SECONDS))
        assertTrue("preview expected: ${result.get()}", result.get() is NativeRouteResult.Ready)
        return request
    }

    private fun fix(lat: Double = -25.4295, lon: Double = -49.2719, accuracy: Float = 5f, ageNanos: Long = 0): Pair<NavigationFix, Long> {
        val now = SystemClock.elapsedRealtimeNanos()
        return NavigationFix(lat, lon, accuracy, System.currentTimeMillis(), now - ageNanos) to now
    }

    @Test fun guidanceBindsPreviewRefusesAlteredRouteWithoutNativeFollow() {
        initialize()
        val start = RoutePoint(-25.4295, -49.2719, "Synthetic origin")
        val destination = RoutePoint(-25.4222, -49.2640, "Synthetic destination")
        val request = build(listOf(start, destination))
        // Altered route must refuse before any native following starts; full
        // follow/progress/stop with a rendered map is proven in the engine module.
        val altered = NativeRouteRequest(listOf(start, RoutePoint(-25.43, -49.27, "Other")), emptySet(), "en")
        var alteredAccepted = true
        main { alteredAccepted = engine.startGuidance(altered) }
        assertFalse("altered launch must refuse", alteredAccepted)
        assertFalse(engine.isGuiding())
        // Locale mapping never invents native codes.
        assertEquals("pt-BR", GuidancePolicy.nativeLocaleFor("pt"))
        assertNull(GuidancePolicy.nativeLocaleFor("xx"))
        // Poor fix policy never advances native progress or speech.
        val (poor, poorNow) = fix(accuracy = 500f)
        assertFalse(NavigationFixPolicy.accepts(poor, poorNow))
        val (good, goodNow) = fix()
        assertTrue(NavigationFixPolicy.accepts(good, goodNow))
        main { engine.cancelRoute() }
    }

    @Test fun localVoicePolicyAcceptsOnlyInstalledOfflineVoices() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        val voice = LocalGuidanceVoice(context)
        assertEquals(java.util.Locale("pt", "BR"), voice.ttsLocaleFor("pt-BR"))
        assertEquals(java.util.Locale.SIMPLIFIED_CHINESE, voice.ttsLocaleFor("zh-CN"))
        assertNull(voice.ttsLocaleFor("xx"))
        // No engine must be started by this policy check; start with unknown language refuses fast.
        assertFalse(voice.start("xx"))
        voice.stop()
    }

    @Test fun localVoiceSpeaksOfflineWhenInstalledEngineAvailable() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        val voice = LocalGuidanceVoice(context)
        try {
            // Google TTS on the owned phone provides offline English; if no local
            // voice is installed this stays a textual-only PASS and does not claim speech.
            val started = voice.start("en")
            if (!started) {
                assertFalse(voice.available)
                return
            }
            assertTrue(voice.available)
            assertTrue("offline speech must enqueue", voice.speak("Turn right"))
            Thread.sleep(1500)
            voice.pause()
        } finally {
            voice.stop()
        }
    }
}
