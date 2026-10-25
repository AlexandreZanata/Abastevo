package com.anpfuel.maps.engine

import android.content.Context
import android.view.View
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.assertFalse
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class MapHostInstrumentedTest {
    @get:Rule val compose = createComposeRule()

    private class FakeEngine(
        private val readyImmediately: Boolean = true,
        var initResult: Boolean = true,
    ) : MapEngine {
        var readiness: (() -> Unit)? = null
        var initialized = false
        var initCalls = 0
        var viewCreated = false
        var recenterCalls = 0
        override fun initialize(context: Context, onReady: () -> Unit): Boolean {
            initCalls++
            if (!initResult) return false
            initialized = true
            readiness = onReady
            if (readyImmediately) onReady()
            return true
        }
        override fun newMapView(context: Context): View {
            viewCreated = true
            return View(context)
        }
        override fun setViewport(latitude: Double, longitude: Double, zoom: Int) = Unit
        override fun currentCenter(): MapCenter? = MapCenter(-25.42, -49.27)
        override fun recenter(): Boolean { recenterCalls++; return true }
        override fun isInitialized(): Boolean = initialized
        override fun search(context: Context, query: String, onResult: (List<MapSearchResult>) -> Unit): Boolean = true
        override fun buildCarRoute(startLatitude: Double, startLongitude: Double, endLatitude: Double, endLongitude: Double, onResult: (MapRouteInfo?) -> Unit): Boolean = true
        override fun setOverlayMarkers(markers: List<MapOverlayMarker>): Boolean = true
        override fun overlayClusters(): List<MapOverlayCluster> = emptyList()
        override fun setOnOverlaySelected(listener: (MapOverlaySelection) -> Unit) = Unit
        override fun cancelSearch() = Unit
        override fun cancelRoute() = Unit
        override fun release() = Unit
    }

    private class FakeStore : MapCameraStore {
        var camera: MapCamera? = null
        override fun load(): MapCamera? = camera
        override fun save(camera: MapCamera) { this.camera = camera }
    }

    @Test
    fun surfaceWaitsForNativeInitialization() {
        val engine = FakeEngine(readyImmediately = false)
        compose.setContent { MapHost(engine, cameraStore = FakeStore()) }
        compose.waitForIdle()
        assertFalse(engine.viewCreated)
        compose.runOnIdle { engine.readiness?.invoke() }
        compose.waitForIdle()
        assertTrue(engine.viewCreated)
    }

    @Test
    fun hostCreatesMapAndDoesNotStartLocationOnEntry() {
        val engine = FakeEngine()
        compose.setContent { MapHost(engine, cameraStore = FakeStore()) }
        compose.waitForIdle()
        assertTrue(engine.initialized)
        assertTrue(engine.viewCreated)
        compose.onNodeWithContentDescription("Recenter map").assertIsDisplayed()
        assertEquals(0, engine.recenterCalls)
    }

    @Test
    fun initFailureShowsErrorAndRetryStartsEngineOnce() {
        val engine = FakeEngine(readyImmediately = false, initResult = false)
        compose.setContent {
            MapHost(
                engine,
                cameraStore = FakeStore(),
                initErrorMessage = "Map unavailable",
                retryLabel = "Retry",
            )
        }
        compose.waitForIdle()
        assertEquals(1, engine.initCalls)
        assertFalse(engine.viewCreated)
        compose.onNodeWithText("Map unavailable").assertIsDisplayed()
        compose.onNodeWithContentDescription("Retry").assertIsDisplayed()
    }

    @Test
    fun retryAfterFailureReachesReadySurface() {
        val engine = FakeEngine(readyImmediately = true, initResult = false)
        compose.setContent {
            MapHost(
                engine,
                cameraStore = FakeStore(),
                initErrorMessage = "Map unavailable",
                retryLabel = "Retry",
            )
        }
        compose.waitForIdle()
        compose.onNodeWithText("Map unavailable").assertIsDisplayed()
        compose.runOnIdle { engine.initResult = true }
        compose.onNodeWithContentDescription("Retry").performClick()
        compose.waitForIdle()
        assertEquals(2, engine.initCalls)
        assertTrue(engine.viewCreated)
        compose.onNodeWithContentDescription("Recenter map").assertIsDisplayed()
    }
}
