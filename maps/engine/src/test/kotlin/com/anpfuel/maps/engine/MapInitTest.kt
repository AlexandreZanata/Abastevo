package com.anpfuel.maps.engine

import android.content.Context
import android.view.View
import io.mockk.every
import io.mockk.mockk
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * B-BR-MAP-HOST-INIT-01: the ViewModel owns native initialization exactly
 * once, on the application context, with error + retry. Covers the
 * production Hilt path (`MapHost()` without an explicit engine), which the
 * fake-host Compose tests do not exercise.
 */
class MapInitTest {
    private class InitFakeEngine(var result: Boolean = true) : MapEngine {
        var calls = 0
        val contexts = mutableListOf<Context>()
        var pending: (() -> Unit)? = null
        override fun initialize(context: Context, onReady: () -> Unit): Boolean {
            calls++
            contexts += context
            if (result) onReady() else pending = onReady
            return result
        }
        override fun newMapView(context: Context): View = throw UnsupportedOperationException()
        override fun setViewport(latitude: Double, longitude: Double, zoom: Int) = Unit
        override fun currentCenter(): MapCenter? = null
        override fun recenter() = false
        override fun isInitialized() = true
        override fun search(context: Context, query: String, onResult: (List<MapSearchResult>) -> Unit) = true
        override fun buildCarRoute(sLat: Double, sLon: Double, eLat: Double, eLon: Double, onResult: (MapRouteInfo?) -> Unit) = true
        override fun setOverlayMarkers(markers: List<MapOverlayMarker>) = true
        override fun overlayClusters() = emptyList<MapOverlayCluster>()
        override fun setOnOverlaySelected(listener: (MapOverlaySelection) -> Unit) = Unit
        override fun cancelSearch() = Unit
        override fun cancelRoute() = Unit
        override fun release() = Unit
    }

    private class FakeStore : MapCameraStore {
        override fun load(): MapCamera? = null
        override fun save(camera: MapCamera) = Unit
    }

    private fun contexts(): Pair<Context, Context> {
        val app = mockk<Context>()
        val activity = mockk<Context>()
        every { activity.applicationContext } returns app
        return activity to app
    }

    @Test fun `ensureInitialized starts engine once on the application context`() {
        val engine = InitFakeEngine()
        val vm = MapViewModel(engine, FakeStore())
        val (activity, app) = contexts()
        vm.ensureInitialized(activity)
        vm.ensureInitialized(activity)
        assertEquals(1, engine.calls)
        assertTrue(engine.contexts.single() === app)
        assertTrue(vm.state.value.ready)
    }

    @Test fun `synchronous init failure surfaces error and retry re-invokes once`() {
        val engine = InitFakeEngine(result = false)
        val vm = MapViewModel(engine, FakeStore())
        val (activity, _) = contexts()
        vm.ensureInitialized(activity)
        assertFalse(vm.state.value.ready)
        assertEquals("map-init-failed", vm.state.value.initError)
        engine.result = true
        vm.retryInit(activity)
        assertEquals(2, engine.calls)
        assertTrue(vm.state.value.ready)
        assertEquals(null, vm.state.value.initError)
    }

    @Test fun `late readiness after disposal never revives the host`() {
        val engine = InitFakeEngine(result = false)
        val vm = MapViewModel(engine, FakeStore())
        val (activity, _) = contexts()
        vm.ensureInitialized(activity)
        vm.dispose()
        engine.pending?.invoke()
        assertFalse(vm.state.value.ready)
    }
}
