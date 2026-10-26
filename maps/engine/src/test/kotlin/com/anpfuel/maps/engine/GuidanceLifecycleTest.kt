package com.anpfuel.maps.engine

import android.content.Context
import android.view.View
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class GuidanceLifecycleTest {
    private class GuidingFake(var guiding: Boolean = true) : MapEngine {
        var cancelSearchCalls = 0
        var cancelRouteCalls = 0
        var releaseCalls = 0
        override fun initialize(context: Context, onReady: () -> Unit) = true
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
        override fun cancelSearch() { cancelSearchCalls++ }
        override fun cancelRoute() { cancelRouteCalls++ }
        override fun isGuiding() = guiding
        override fun release() { releaseCalls++ }
    }

    private class FakeStore : MapCameraStore {
        override fun load(): MapCamera? = null
        override fun save(camera: MapCamera) = Unit
    }

    @Test fun `disposing the map host never destroys an owned guidance session`() {
        val engine = GuidingFake(guiding = true)
        val vm = MapViewModel(engine, FakeStore())
        vm.dispose()
        assertEquals(0, engine.releaseCalls)
        assertEquals(1, engine.cancelSearchCalls)
        assertEquals(1, engine.cancelRouteCalls)
    }

    @Test fun `disposing without guidance still releases the engine once`() {
        val engine = GuidingFake(guiding = false)
        val vm = MapViewModel(engine, FakeStore())
        vm.dispose()
        vm.dispose()
        assertEquals(1, engine.releaseCalls)
    }
}
