package com.anpfuel.app.ui.map

import android.content.Context
import android.os.Debug
import android.os.SystemClock
import android.util.Log
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.domain.ev.EvRoadLeg
import com.anpfuel.domain.routing.NativeRouteRequest
import com.anpfuel.domain.routing.RoutePoint
import com.anpfuel.maps.engine.ComapsMapEngine
import com.anpfuel.maps.engine.NativeEvRoadRouter
import java.io.File
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/**
 * MAP-29 local proxy measurements on the reference 16 KB emulator. Debug build,
 * translated ARM64, admitted regional map. These are not low-end physical, national,
 * frame-pacing, battery or PostGIS measurements; see MAP_29_PERFORMANCE_CONTRACT.md.
 */
@RunWith(AndroidJUnit4::class)
class MapPerformanceInstrumentedTest {
    private val instrumentation = InstrumentationRegistry.getInstrumentation()

    private fun pssKb(): Int {
        val info = Debug.MemoryInfo()
        Debug.getMemoryInfo(info)
        return info.totalPss
    }

    @Test
    fun engineInitRouteCancellationAndProcessMemoryStayWithinDeclaredLocalBudgets() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        assertTrue("admitted regional fixture is required", File(context.filesDir, "261004/Brazil_Parana_East.mwm").isFile)
        val engine = ComapsMapEngine()
        val ready = CountDownLatch(1)
        val initStart = SystemClock.elapsedRealtime()
        instrumentation.runOnMainSync {
            assertTrue(engine.initialize(context) { ready.countDown() })
        }
        assertTrue("native readiness", ready.await(45, TimeUnit.SECONDS))
        val initMs = SystemClock.elapsedRealtime() - initStart
        Log.d("MapPerf", "engineInitMs=$initMs pssAfterInitKb=${pssKb()}")

        val origin = RoutePoint(-25.4295, -49.2719, "synthetic perf origin")
        val destination = RoutePoint(-25.4222, -49.2640, "synthetic perf destination")
        val router = NativeEvRoadRouter(engine, timeoutMillis = 60_000)
        val routeTimes = mutableListOf<Long>()
        repeat(3) { index ->
            val start = SystemClock.elapsedRealtime()
            val result = runBlocking { router.leg(listOf(origin, destination), emptySet(), "en") }
            val elapsed = SystemClock.elapsedRealtime() - start
            routeTimes += elapsed
            assertTrue("route $index must be a real road leg", result is EvRoadLeg.Ready)
            Log.d("MapPerf", "routeMs[$index]=$elapsed distance=${(result as EvRoadLeg.Ready).distanceMeters}")
        }
        val pssAfterRoutes = pssKb()
        Log.d("MapPerf", "pssAfterRoutesKb=$pssAfterRoutes")

        var callbackCount = 0
        var cancelMs = 0L
        instrumentation.runOnMainSync {
            val accepted = engine.buildRoute(
                NativeRouteRequest(listOf(destination, origin), emptySet(), "en"),
            ) { callbackCount++ }
            assertTrue("native route accepted", accepted)
            val cancelStart = SystemClock.elapsedRealtime()
            engine.cancelRoute()
            cancelMs = SystemClock.elapsedRealtime() - cancelStart
        }
        Thread.sleep(300)
        Log.d("MapPerf", "cancelMs=$cancelMs callbacksAfterCancel=$callbackCount")
        val pssAfterCancel = pssKb()
        Log.d("MapPerf", "pssAfterCancelKb=$pssAfterCancel")

        assertTrue("engine init budget: ${initMs}ms", initMs <= 15_000)
        assertTrue("route budget: $routeTimes", routeTimes.all { it <= 5_000 })
        assertTrue("PSS budget: ${pssAfterRoutes}KiB", pssAfterRoutes <= 512 * 1024)
        assertTrue("cancel budget: ${cancelMs}ms", cancelMs <= 1_000)
        instrumentation.runOnMainSync { engine.release() }
    }
}
