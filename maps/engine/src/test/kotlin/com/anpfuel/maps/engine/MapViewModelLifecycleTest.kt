package com.anpfuel.maps.engine

import android.content.Context
import android.view.View
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class MapViewModelLifecycleTest {
    private class FakeEngine(
        var center: MapCenter? = MapCenter(-25.43, -49.28),
        var recenterResult: Boolean = true,
    ) : MapEngine {
        var viewport: Triple<Double, Double, Int>? = null
        var recenterCalls = 0
        var releaseCalls = 0
        override fun initialize(context: Context, onReady: () -> Unit): Boolean = true
        override fun newMapView(context: Context): View = throw UnsupportedOperationException()
        override fun setViewport(latitude: Double, longitude: Double, zoom: Int) {
            viewport = Triple(latitude, longitude, zoom)
        }
        override fun currentCenter(): MapCenter? = center
        override fun recenter(): Boolean {
            recenterCalls++
            return recenterResult
        }
        override fun isInitialized(): Boolean = true
        override fun search(context: Context, query: String, onResult: (List<MapSearchResult>) -> Unit) = true
        override fun buildCarRoute(sLat: Double, sLon: Double, eLat: Double, eLon: Double, onResult: (MapRouteInfo?) -> Unit) = true
        override fun setOverlayMarkers(markers: List<MapOverlayMarker>) = true
        override fun overlayClusters(): List<MapOverlayCluster> = emptyList()
        override fun setOnOverlaySelected(listener: (MapOverlaySelection) -> Unit) = Unit
        override fun cancelSearch() = Unit
        override fun cancelRoute() = Unit
        override fun release() { releaseCalls++ }
    }

    private class FakeStore(var camera: MapCamera? = null) : MapCameraStore {
        override fun load() = camera
        override fun save(camera: MapCamera) { this.camera = camera }
    }

    @Test fun `gps off blocks recenter but keeps browsing`() {
        val vm = MapViewModel(FakeEngine(), FakeStore())
        vm.onEngineReady()
        vm.onPermissionState(fine = true, coarse = true, deniedForever = false)
        vm.onGpsState(false)
        vm.onRecenter()
        assertEquals("gps-off", vm.state.value.recenterError)
        assertTrue(vm.state.value.ready)
    }

    @Test fun `no fix surfaces explicit error without moving camera`() {
        val engine = FakeEngine(recenterResult = false)
        val vm = MapViewModel(engine, FakeStore(MapCamera(-25.0, -49.0, 12)))
        vm.onEngineReady()
        vm.onPermissionState(fine = true, coarse = true, deniedForever = false)
        vm.onGpsState(true)
        vm.onRecenter()
        assertEquals("no-fix", vm.state.value.recenterError)
        assertEquals(1, engine.recenterCalls)
        assertEquals(MapCamera(-25.0, -49.0, 12), vm.state.value.camera)
    }

    @Test fun `denied forever never triggers recenter`() {
        val engine = FakeEngine()
        val vm = MapViewModel(engine, FakeStore())
        vm.onEngineReady()
        vm.onPermissionState(fine = false, coarse = false, deniedForever = true)
        vm.onRecenter()
        assertEquals(0, engine.recenterCalls)
        assertEquals("no-permission", vm.state.value.recenterError)
        assertTrue(vm.state.value.deniedForever)
    }

    @Test fun `approximate location permits explicit recenter`() {
        val engine = FakeEngine()
        val vm = MapViewModel(engine, FakeStore())
        vm.onEngineReady()
        vm.onPermissionState(fine = false, coarse = true, deniedForever = false)
        vm.onGpsState(true)
        vm.onRecenter()
        assertEquals(1, engine.recenterCalls)
        assertEquals(null, vm.state.value.recenterError)
        assertTrue(vm.state.value.approximateLocation)
    }

    @Test fun `camera restores after recreation and persists on stop`() {
        val store = FakeStore(MapCamera(-25.42, -49.27, 13))
        val first = MapViewModel(FakeEngine(), store)
        first.onEngineReady()
        first.onHostStop()
        assertEquals(MapCamera(-25.43, -49.28, 13), store.camera)
        // Recreation: a new host restores the persisted camera.
        val second = MapViewModel(FakeEngine(), store)
        second.onEngineReady()
        assertEquals(MapCamera(-25.43, -49.28, 13), second.state.value.camera)
    }

    @Test fun `singleton holder returns one engine per process`() {
        MapEngineHolder.resetForTests()
        val first = MapEngineHolder.engine
        val second = MapEngineHolder.engine
        assertTrue(first === second)
        MapEngineHolder.resetForTests()
        val third = MapEngineHolder.engine
        assertFalse(first === third)
        MapEngineHolder.resetForTests()
    }
}
