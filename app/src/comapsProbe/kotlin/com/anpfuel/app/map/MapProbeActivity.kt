package com.anpfuel.app.map

import android.app.Activity
import android.os.Bundle
import com.anpfuel.maps.engine.ComapsMapEngine

/**
 * Debug-only host used to prove the extracted engine inside the real app
 * package. It is not reachable from the production navigation graph.
 */
class MapProbeActivity : Activity() {
    companion object {
        private val engine = ComapsMapEngine()
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        engine.initialize(this) {
            runOnUiThread {
                val view = engine.newMapView(this)
                setContentView(view)
                engine.setOnMarkerSelected { uuid -> android.util.Log.i("MapProbe", "selected=$uuid") }
                view.postDelayed({
                    engine.setViewport(-25.42, -49.27, 13)
                    engine.setOverlayMarkers(listOf(
                        com.anpfuel.maps.engine.MapOverlayMarker(
                            "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", -25.42, -49.27, "Posto Abastevo"),
                        com.anpfuel.maps.engine.MapOverlayMarker(
                            "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", -25.43, -49.26, "Recarga Abastevo", charging = true),
                    ))
                }, 2500)
                view.postDelayed({
                    engine.search(this, "Museu") { results ->
                        android.util.Log.i("MapProbe", "search=${results.size} first=${results.firstOrNull()?.name}")
                    }
                }, 5000)
                view.postDelayed({
                    engine.buildCarRoute(-25.42, -49.27, -25.43, -49.27) { route ->
                        android.util.Log.i("MapProbe", "route=${route?.distance}${route?.unit} eta=${route?.seconds}")
                    }
                }, 7000)
            }
        }
    }
}
