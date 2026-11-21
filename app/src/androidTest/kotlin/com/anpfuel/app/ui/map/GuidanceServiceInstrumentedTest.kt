package com.anpfuel.app.ui.map

import android.Manifest
import android.os.Build
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.rule.GrantPermissionRule
import com.anpfuel.app.AcceptedTermsRule
import com.anpfuel.app.MainActivity
import com.anpfuel.app.R
import com.anpfuel.domain.routing.NativeRouteRequest
import com.anpfuel.domain.routing.NativeRouteResult
import com.anpfuel.domain.routing.RoutePoint
import com.anpfuel.maps.engine.MapCamera
import com.anpfuel.maps.engine.MapEngineHolder
import com.anpfuel.maps.engine.SharedPreferencesMapCameraStore
import java.io.File
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** MAP-23 foreground guidance from the application's map tab on an owned device. */
@RunWith(AndroidJUnit4::class)
class GuidanceServiceInstrumentedTest {
    @get:Rule(order = 0) val permissions = GrantPermissionRule.grant(*buildList {
        add(Manifest.permission.ACCESS_COARSE_LOCATION)
        add(Manifest.permission.ACCESS_FINE_LOCATION)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            add(Manifest.permission.POST_NOTIFICATIONS)
        }
    }.toTypedArray())
    @get:Rule(order = 1) val acceptedTerms = AcceptedTermsRule()
    @get:Rule(order = 2) val compose = createAndroidComposeRule<MainActivity>()

    @Test fun foregroundServiceStartsStopsAndRecoversExplicitly() {
        val activity = compose.activity
        val appContext = activity.applicationContext
        assertTrue("admitted regional fixture is required", File(appContext.filesDir, "261004/Brazil_Parana_East.mwm").isFile)
        val engine = MapEngineHolder.engine
        SharedPreferencesMapCameraStore(activity).save(MapCamera(-25.4295, -49.2719, 15))
        compose.onNodeWithContentDescription(activity.getString(R.string.nav_map), useUnmergedTree = true).performClick()
        compose.waitUntil(45_000) {
            engine.isInitialized() && engine.isRenderingReady()
        }
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
