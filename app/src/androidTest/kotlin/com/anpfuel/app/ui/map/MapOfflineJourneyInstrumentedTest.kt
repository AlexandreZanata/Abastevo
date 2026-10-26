package com.anpfuel.app.ui.map

import android.content.Context
import android.net.ConnectivityManager
import android.provider.Settings
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.MainActivity
import com.anpfuel.app.R
import com.anpfuel.domain.routing.NativeRouteRequest
import com.anpfuel.domain.routing.NativeRouteResult
import com.anpfuel.domain.routing.NavigationFix
import com.anpfuel.domain.routing.RoutePoint
import com.anpfuel.maps.engine.MapEngineHolder
import java.io.File
import java.net.InetSocketAddress
import java.net.Socket
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference
import org.junit.Assert.*
import org.junit.Assume.assumeTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * MAP-24 OFFLINE_GATE journey on a real device with the admitted real MWM inputs.
 *
 * Two instrumentation runs separated by a host-side force-stop and airplane mode
 * form the clean-restart proof:
 *  - `mapOfflinePhase=online`: download and publish through the production UI.
 *  - `mapOfflinePhase=offline`: fresh process in airplane mode, last-good map only.
 * The host script compares the proof server's request log before and after the
 * offline run: any endpoint contact would be counted there (the loopback tunnel
 * stays up on purpose). Synthetic coordinates only; no private route or location.
 */
@RunWith(AndroidJUnit4::class)
class MapOfflineJourneyInstrumentedTest {
    @get:Rule val compose = createAndroidComposeRule<MainActivity>()
    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    private val engine get() = MapEngineHolder.engine
    private fun text(id: Int) = compose.activity.getString(id)
    private fun phase() = InstrumentationRegistry.getArguments().getString("mapOfflinePhase")

    private fun waitText(value: String, timeout: Long = 60_000, substring: Boolean = true) {
        compose.waitUntil(timeout) {
            try { compose.onNodeWithText(value, substring = substring).assertExists(); true }
            catch (_: AssertionError) { false }
        }
    }

    private fun openMap() {
        compose.waitUntil(30_000) {
            compose.onAllNodesWithContentDescription(text(R.string.nav_map), useUnmergedTree = true).fetchSemanticsNodes().isNotEmpty()
        }
        compose.onNodeWithContentDescription(text(R.string.nav_map), useUnmergedTree = true).performClick()
        compose.waitUntil(60_000) { engine.isInitialized() && engine.isRenderingReady() }
    }

    private fun viewport(lat: Double, lon: Double) {
        compose.runOnIdle { engine.setViewport(lat, lon, 15) }
    }

    private fun search(query: String): List<com.anpfuel.maps.engine.MapSearchResult> {
        val found = AtomicReference<List<com.anpfuel.maps.engine.MapSearchResult>>()
        val done = CountDownLatch(1)
        compose.runOnIdle {
            assertTrue(engine.search(compose.activity, query) { found.set(it); done.countDown() })
        }
        assertTrue("offline search must finish: $query", done.await(30, TimeUnit.SECONDS))
        compose.runOnIdle { engine.cancelSearch() }
        return found.get()
    }

    @Test fun onlinePhaseDownloadsAndPublishesThroughTheProductionUi() {
        assumeTrue("online phase only", phase() == "online")
        openMap()
        compose.onNodeWithTag("map-open-downloads").performClick()
        compose.waitUntil(60_000) {
            compose.onAllNodesWithTag("map-download-update").fetchSemanticsNodes().isNotEmpty()
        }
        compose.onNodeWithTag("map-download-update").performScrollTo().performClick()
        compose.waitUntil(60_000) {
            compose.onAllNodesWithTag("map-download-start").fetchSemanticsNodes().isNotEmpty()
        }
        compose.onNodeWithTag("map-download-start").performScrollTo().performClick()
        waitText(text(R.string.map_published), 240_000)
        val root = File(compose.activity.filesDir, "261004")
        listOf("Brazil_Parana_East.mwm", "World.mwm", "WorldCoasts.mwm").forEach {
            assertTrue("$it must be published", File(root, it).length() > 1_000_000)
        }
        viewport(-25.4295, -49.2719)
        compose.waitUntil(60_000) { engine.isDownloadedMapAtScreenCenter() }
    }

    @Test fun offlineJourneyAfterCleanRestartUsesOnlyLastGoodLocalData() {
        assumeTrue("offline phase only", phase() == "offline")
        // 1. The environment really is offline.
        val context = compose.activity.applicationContext
        assertEquals("airplane mode must be on", 1, Settings.Global.getInt(context.contentResolver, Settings.Global.AIRPLANE_MODE_ON, 0))
        val connectivity = context.getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager
        assertNull("no active network", connectivity.activeNetwork)
        try {
            Socket().use { it.connect(InetSocketAddress("192.0.2.1", 443), 2_000) }
            fail("a public endpoint must be unreachable")
        } catch (_: java.io.IOException) {
        }

        // 2. Clean restart: last-good generation is still published and registered.
        openMap()
        viewport(-25.4295, -49.2719)
        compose.waitUntil(60_000) { engine.isDownloadedMapAtScreenCenter() }
        assertTrue("regional map registered after restart", engine.downloadedMapsCount() >= 3)
        compose.onNodeWithTag("map-open-downloads").performClick()
        waitText(text(R.string.map_published))
        compose.onNodeWithTag("map-download-back").performScrollTo().performClick()

        // 3. Offline browse/search/POI: real features, names and coordinates.
        val street = search("Barão do Serro Azul")
        assertTrue("street search returns features", street.isNotEmpty())
        assertTrue(street.any { kotlin.math.abs(it.latitude + 25.43) < 0.05 && kotlin.math.abs(it.longitude + 49.27) < 0.05 })
        val poi = search("Praça Tiradentes")
        assertTrue("named place search returns features: $poi", poi.isNotEmpty())
        assertTrue(poi.all { it.name.isNotBlank() })

        // 4. Real route sheet: search result as origin, coordinates as destination.
        compose.onNodeWithTag("native-route-open").performScrollTo().performClick()
        compose.onNodeWithText(text(R.string.map22_origin), substring = false).performScrollTo().performClick()
        compose.onNodeWithText(text(R.string.map22_search_hint)).performTextInput("Barão do Serro Azul")
        // The route sheet is the last composed Search button (map shell has two more).
        compose.onAllNodesWithText(text(R.string.map_search)).onLast().performScrollTo().performClick()
        compose.waitUntil(30_000) {
            compose.onAllNodesWithText(street.first().name, substring = true).fetchSemanticsNodes().isNotEmpty()
        }
        compose.onAllNodesWithText(street.first().name, substring = true).onFirst().performScrollTo().performClick()
        compose.onNodeWithText(text(R.string.map22_destination), substring = false).performScrollTo().performClick()
        compose.onNodeWithText(text(R.string.map22_latitude)).performTextInput("-25.4222")
        compose.onNodeWithText(text(R.string.map22_longitude)).performTextInput("-49.2640")
        compose.onNodeWithText(text(R.string.map22_coordinates), substring = false).performScrollTo().performClick()
        compose.onNodeWithTag("native-route-build").performScrollTo().performClick()
        compose.waitUntil(90_000) {
            compose.onAllNodesWithTag("native-route-preview").fetchSemanticsNodes().isNotEmpty() ||
                compose.onAllNodesWithTag("native-route-error").fetchSemanticsNodes().isNotEmpty()
        }
        if (compose.onAllNodesWithTag("native-route-preview").fetchSemanticsNodes().isEmpty()) {
            fail(
                "offline route must preview; first street result=${street.first()}; sheet=" +
                    compose.onAllNodes(isRoot()).onLast().printToString(maxDepth = 14).take(3000),
            )
        }
        compose.onNodeWithTag("native-route-preview").performScrollTo().assertIsDisplayed()

        // 5. Cross-region/missing-data journey: never a drivable false success.
        compose.onNodeWithText(text(R.string.map22_destination), substring = false).performScrollTo().performClick()
        compose.onNodeWithText(text(R.string.map22_latitude)).performTextReplacement("-27.5949")
        compose.onNodeWithText(text(R.string.map22_longitude)).performTextReplacement("-48.5482")
        compose.onNodeWithText(text(R.string.map22_coordinates), substring = false).performScrollTo().performClick()
        compose.onNodeWithTag("native-route-build").performScrollTo().performClick()
        compose.waitUntil(90_000) { compose.onAllNodesWithTag("native-route-error").fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithTag("native-route-error").performScrollTo().assertIsDisplayed()
        // Explicit recovery action inside the sheet (the shell has its own Downloads entry).
        compose.onAllNodesWithText(text(R.string.map_downloads)).onLast().performScrollTo().assertIsDisplayed()
        compose.runOnIdle { engine.cancelRoute() }

        // 6. Guidance on the same downloaded data (synthetic fixes, no location service).
        val request = NativeRouteRequest(
            listOf(RoutePoint(-25.4295, -49.2719, "Synthetic origin"), RoutePoint(-25.4222, -49.2640, "Synthetic destination")),
            emptySet(), "en",
        )
        val built = AtomicReference<NativeRouteResult>()
        val latch = CountDownLatch(1)
        instrumentation.runOnMainSync { assertTrue(engine.buildRoute(request) { built.set(it); latch.countDown() }) }
        assertTrue(latch.await(60, TimeUnit.SECONDS))
        assertTrue("offline route ready: ${built.get()}", built.get() is NativeRouteResult.Ready)
        var started = false
        instrumentation.runOnMainSync { started = engine.startGuidance(request) }
        assertTrue("guidance starts offline", started)
        try {
            val now = android.os.SystemClock.elapsedRealtimeNanos()
            var feed: com.anpfuel.maps.engine.GuidanceFeed? = null
            instrumentation.runOnMainSync {
                feed = engine.feedGuidanceFix(NavigationFix(-25.4295, -49.2719, 5f, System.currentTimeMillis(), now), now)
            }
            assertNotNull("a good synthetic fix advances native guidance", feed?.progress)
        } finally {
            instrumentation.runOnMainSync { engine.stopGuidance(); engine.cancelRoute() }
        }

        // 7. Outside the downloaded region the engine does not invent street coverage.
        viewport(-27.5949, -48.5482)
        val missing = search("Rua Felipe Schmidt")
        assertTrue(
            "no street-level feature may appear for the missing region: $missing",
            missing.none { kotlin.math.abs(it.latitude + 27.59) < 0.1 && kotlin.math.abs(it.longitude + 48.55) < 0.1 },
        )
    }
}
