package com.anpfuel.app.map

import android.app.Activity
import android.os.Bundle
import com.anpfuel.maps.engine.ComapsMapEngine
import com.anpfuel.maps.engine.MapOverlayMarker
import com.anpfuel.maps.engine.MapOverlaySelection
import com.anpfuel.maps.engine.MapStationLocationQuality
import com.anpfuel.maps.engine.overlayListItems

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
        val overlayOnly = intent.getBooleanExtra("overlayOnly", false)
        engine.initialize(this) {
            runOnUiThread {
                val view = engine.newMapView(this)
                setContentView(view)
                engine.setOnOverlaySelected { selection ->
                    when (selection) {
                        is MapOverlaySelection.Station -> android.util.Log.i("MapProbe", "selected=${selection.uuid}")
                        is MapOverlaySelection.Cluster ->
                            android.util.Log.i(
                                "MapProbe",
                                "cluster=${selection.members.size} uuids=${selection.members.map { it.uuid }}",
                            )
                    }
                }
                view.postDelayed({
                    engine.setViewport(if (overlayOnly) -25.46 else -25.42, if (overlayOnly) -49.31 else -49.27, if (overlayOnly) 16 else 13)
                    engine.setOverlayMarkers(fixture(1))
                }, 2500)
                view.postDelayed({ logOverlay("first") }, 4500)
                if (overlayOnly) {
                    view.postDelayed({ engine.setOverlayMarkers(fixture(2)) }, 8000)
                    view.postDelayed({ logOverlay("second") }, 10000)
                } else {
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

    private fun logOverlay(label: String) {
        val clusters = engine.overlayClusters()
        val items = overlayListItems(clusters)
        android.util.Log.i(
            "MapProbe",
            "$label clusters=${clusters.size} singles=${clusters.count { !it.isCluster }} " +
                "multi=${clusters.count { it.isCluster }} list=${items.size}",
        )
        items.take(4).forEach { android.util.Log.i("MapProbe", "$label list ${it.uuid} ${it.contentDescription}") }
    }

    private fun fixture(generation: Int): List<MapOverlayMarker> {
        val markers = mutableListOf(
            marker("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", -25.4600, -49.3100, "Posto Central", reviewed = true),
            marker(
                "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
                -25.4601,
                -49.3101,
                "Recarga Central",
                fuel = false,
                charging = true,
                reviewed = true,
            ),
            marker(
                "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
                -25.4602,
                -49.3102,
                "Posto Ambos",
                charging = true,
                cityCentroid = true,
            ),
            marker("dddddddd-dddd-4ddd-8ddd-dddddddddddd", -25.4300, -49.2800, "Posto Sem Local"),
            marker(
                "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee",
                -25.4301,
                -49.2801,
                "Recarga Sem Local",
                fuel = false,
                charging = true,
            ),
            marker("ffffffff-ffff-4fff-8fff-ffffffffffff", -25.4400, -49.2900, "Posto Revisado", reviewed = true),
            marker(
                "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
                -25.4600,
                -49.3100,
                "Posto Central duplicado",
                charging = true,
                reviewed = true,
            ),
        )
        if (generation == 2) {
            markers += marker("11111111-1111-4111-8111-111111111111", -25.4401, -49.2901, "Posto Novo", reviewed = true)
        }
        return markers
    }

    private fun marker(
        uuid: String,
        latitude: Double,
        longitude: Double,
        title: String,
        fuel: Boolean = true,
        charging: Boolean = false,
        reviewed: Boolean = false,
        cityCentroid: Boolean = false,
    ) = MapOverlayMarker(
        uuid = uuid,
        latitude = latitude,
        longitude = longitude,
        title = title,
        fuel = fuel,
        charging = charging,
        locationQuality = when {
            reviewed -> MapStationLocationQuality.REVIEWED
            cityCentroid -> MapStationLocationQuality.CITY_CENTROID
            else -> MapStationLocationQuality.UNKNOWN
        },
    )
}
