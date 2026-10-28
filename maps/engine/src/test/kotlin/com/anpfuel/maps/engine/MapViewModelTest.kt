package com.anpfuel.maps.engine

import android.content.Context
import android.view.View
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class MapViewModelTest {
    private class FakeEngine : MapEngine {
        var viewport: Triple<Double, Double, Int>? = null
        var recenterCalls = 0
        var cancelSearchCalls = 0
        var cancelRouteCalls = 0
        var releaseCalls = 0
        var center: MapCenter? = MapCenter(-25.43, -49.28)
        var camera: MapCamera? = null
        var labelsVisibleCalls = mutableListOf<Boolean>()
        var recenterResult = true
        override fun initialize(context: Context, onReady: () -> Unit): Boolean = true
        override fun newMapView(context: Context): View = throw UnsupportedOperationException()
        override fun setViewport(latitude: Double, longitude: Double, zoom: Int) { viewport = Triple(latitude, longitude, zoom) }
        override fun currentCenter(): MapCenter? = center
        override fun currentCamera(): MapCamera? = camera
        override fun setOverlayLabelsVisible(visible: Boolean) { labelsVisibleCalls += visible }
        override fun recenter(): Boolean { recenterCalls++; return recenterResult }
        override fun isInitialized(): Boolean = true
        override fun search(context: Context, query: String, onResult: (List<MapSearchResult>) -> Unit): Boolean = true
        override fun buildCarRoute(startLatitude: Double, startLongitude: Double, endLatitude: Double, endLongitude: Double, onResult: (MapRouteInfo?) -> Unit): Boolean = true
        override fun setOverlayMarkers(markers: List<MapOverlayMarker>): Boolean = true
        override fun overlayClusters(): List<MapOverlayCluster> = emptyList()
        override fun setOnOverlaySelected(listener: (MapOverlaySelection) -> Unit) = Unit
        override fun cancelSearch() { cancelSearchCalls++ }
        override fun cancelRoute() { cancelRouteCalls++ }
        override fun release() { releaseCalls++; cancelSearch(); cancelRoute() }
    }

    private class FakeStore(var camera: MapCamera? = null) : MapCameraStore {
        override fun load() = camera
        override fun save(camera: MapCamera) { this.camera = camera }
    }

    @Test fun `restore saved camera and apply viewport once`() {
        val engine = FakeEngine()
        val store = FakeStore(MapCamera(-20.0, -40.0, 9))
        val vm = MapViewModel(engine, store)
        vm.onEngineReady()
        vm.onEngineReady()
        assertTrue(vm.state.value.ready)
        assertEquals(Triple(-20.0, -40.0, 9), engine.viewport)
    }

    @Test fun `browsing without location never starts recenter`() {
        val engine = FakeEngine()
        val vm = MapViewModel(engine, FakeStore())
        vm.onEngineReady()
        vm.onPermissionState(fine = false, coarse = false, deniedForever = true)
        vm.onRecenter()
        assertEquals(0, engine.recenterCalls)
        assertFalse(vm.state.value.hasLocationPermission)
        assertTrue(vm.state.value.deniedForever)
    }

    @Test fun `approximate permission allows explicit recenter`() {
        val engine = FakeEngine()
        val vm = MapViewModel(engine, FakeStore())
        vm.onEngineReady()
        vm.onPermissionState(fine = false, coarse = true, deniedForever = false)
        vm.onRecenter()
        assertEquals(1, engine.recenterCalls)
        assertTrue(vm.state.value.approximateLocation)
    }

    @Test fun `host stop persists the current center without changing zoom`() {
        val engine = FakeEngine()
        val store = FakeStore()
        val vm = MapViewModel(engine, store)
        vm.onEngineReady()
        vm.onHostStop()
        assertEquals(MapCamera(-25.43, -49.28, 13), store.camera)
    }

    @Test fun `clearing the view model cancels stale search and route work`() {
        val engine = FakeEngine()
        val vm = MapViewModel(engine, FakeStore())
        vm.dispose()
        assertEquals(1, engine.cancelSearchCalls)
        assertEquals(1, engine.cancelRouteCalls)
        assertEquals(1, engine.releaseCalls)
    }

    @Test fun `late readiness and permission callbacks cannot revive disposed host`() {
        val engine = FakeEngine()
        val store = FakeStore()
        val vm = MapViewModel(engine, store)
        vm.dispose()
        vm.onEngineReady()
        vm.onPermissionState(fine = true, coarse = true, deniedForever = false)
        vm.onRecenter()
        vm.onHostStop()
        assertFalse(vm.state.value.ready)
        assertFalse(vm.state.value.hasLocationPermission)
        assertEquals(null, engine.viewport)
        assertEquals(null, store.camera)
        assertEquals(0, engine.recenterCalls)
    }

    @Test fun `overlay labels hide when zoomed out and return when zoomed in`() {
        val engine = FakeEngine()
        val vm = MapViewModel(engine, FakeStore())
        vm.onEngineReady()
        engine.camera = MapCamera(-12.54, -55.71, 13)
        vm.syncCameraFromEngine()
        assertEquals(listOf(false), engine.labelsVisibleCalls)
        // Same zoom again: no repeat call to the native engine.
        vm.syncCameraFromEngine()
        assertEquals(listOf(false), engine.labelsVisibleCalls)
        // Street zoom: labels come back.
        engine.camera = MapCamera(-12.54, -55.71, 16)
        vm.syncCameraFromEngine()
        assertEquals(listOf(false, true), engine.labelsVisibleCalls)
    }

    @Test fun `repeated disposal releases exactly once`() {
        val engine = FakeEngine()
        val vm = MapViewModel(engine, FakeStore())
        vm.dispose()
        vm.dispose()
        assertEquals(1, engine.releaseCalls)
        assertEquals(1, engine.cancelSearchCalls)
        assertEquals(1, engine.cancelRouteCalls)
    }
}
