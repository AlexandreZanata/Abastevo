package com.anpfuel.maps.engine

import io.mockk.every
import io.mockk.mockk
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class MapCameraSyncTest {
    private class Store : MapCameraStore {
        var saved: MapCamera? = null
        override fun load(): MapCamera? = saved
        override fun save(camera: MapCamera) { saved = camera }
    }

    @Test fun `zoom gesture at unchanged center updates query radius`() {
        val engine = mockk<MapEngine>(relaxed = true)
        val store = Store()
        val vm = MapViewModel(engine, store)
        vm.onEngineReady()
        every { engine.currentCamera() } returns MapCamera(-25.42, -49.27, 17)
        every { engine.currentCenter() } returns MapCenter(-25.42, -49.27)
        val camera = vm.syncCameraFromEngine()!!
        assertEquals(17, camera.zoom)
        assertEquals(2000, vm.radiusForZoom(camera.zoom))
    }

    @Test fun `stopping host stores the actual native zoom and center`() {
        val engine = mockk<MapEngine>(relaxed = true)
        val store = Store()
        val vm = MapViewModel(engine, store)
        vm.onEngineReady()
        every { engine.currentCamera() } returns MapCamera(-15.79, -47.88, 16)
        every { engine.currentCenter() } returns MapCenter(-15.79, -47.88)
        vm.onHostStop()
        assertEquals(MapCamera(-15.79, -47.88, 16), store.saved)
    }
}
