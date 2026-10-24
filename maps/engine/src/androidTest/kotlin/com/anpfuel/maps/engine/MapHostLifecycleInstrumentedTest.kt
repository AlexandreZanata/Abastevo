package com.anpfuel.maps.engine

import android.content.Context
import android.view.View
import androidx.activity.ComponentActivity
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class MapHostLifecycleInstrumentedTest {
    @get:Rule val compose = createAndroidComposeRule<ComponentActivity>()

    private class FakeEngine : MapEngine {
        var views = 0
        override fun initialize(context: Context, onReady: () -> Unit): Boolean { onReady(); return true }
        override fun newMapView(context: Context): View { views++; return View(context) }
        override fun setViewport(latitude: Double, longitude: Double, zoom: Int) = Unit
        override fun currentCenter(): MapCenter = MapCenter(-25.42, -49.27)
        override fun recenter(): Boolean = true
        override fun isInitialized(): Boolean = true
        override fun search(context: Context, query: String, onResult: (List<MapSearchResult>) -> Unit): Boolean = true
        override fun buildCarRoute(startLatitude: Double, startLongitude: Double, endLatitude: Double, endLongitude: Double, onResult: (MapRouteInfo?) -> Unit): Boolean = true
        override fun setOverlayMarkers(markers: List<MapOverlayMarker>): Boolean = true
        override fun setOnMarkerSelected(listener: (String) -> Unit) = Unit
        override fun cancelSearch() = Unit
        override fun cancelRoute() = Unit
        override fun release() = Unit
    }

    private class FakeStore : MapCameraStore {
        var camera: MapCamera? = null
        override fun load() = camera
        override fun save(camera: MapCamera) { this.camera = camera }
    }

    @Test
    fun hostSurvivesActivityRecreation() {
        val engine = FakeEngine()
        compose.setContent { MapHost(engine, cameraStore = FakeStore()) }
        compose.waitForIdle()
        compose.activityRule.scenario.recreate()
        compose.waitForIdle()
        assertTrue(engine.views >= 1)
    }
}
