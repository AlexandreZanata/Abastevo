package com.anpfuel.app.ui.map

import android.content.Context
import android.view.View
import com.anpfuel.domain.discovery.NearbyServerStation
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.ServerStationPage
import com.anpfuel.domain.discovery.StationLocationQuality
import com.anpfuel.domain.repository.ServerStationGateway
import com.anpfuel.maps.engine.MapEngine
import com.anpfuel.maps.engine.MapOverlayMarker
import com.anpfuel.maps.engine.MapOverlaySelection
import com.anpfuel.maps.engine.MapSearchResult
import com.anpfuel.maps.engine.MapRouteInfo
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class MapDiscoveryViewModelTest {
    private val dispatcher = StandardTestDispatcher()

    private class FakeGateway(
        var stations: List<ServerStation> = emptyList(),
        var delayMs: Long = 0,
    ) : ServerStationGateway {
        var lastRadius: Int? = null
        var calls = 0
        var lastSearch: Pair<String, String>? = null
        override suspend fun list(limit: Int, cursor: String?): ServerStationPage =
            ServerStationPage(emptyList(), null)
        override suspend fun listInMunicipality(municipalityCode: String, limit: Int, cursor: String?): ServerStationPage =
            ServerStationPage(emptyList(), null)
        override suspend fun search(municipalityCode: String, query: String, limit: Int, cursor: String?): ServerStationPage {
            calls++
            lastSearch = municipalityCode to query
            if (delayMs > 0) kotlinx.coroutines.delay(delayMs)
            val filtered = stations.filter { it.displayName.contains(query, ignoreCase = true) }
            return ServerStationPage(filtered, null)
        }
        override suspend fun nearby(lat: Double, lon: Double, radiusMeters: Int, limit: Int): List<NearbyServerStation> {
            calls++
            lastRadius = radiusMeters
            if (delayMs > 0) kotlinx.coroutines.delay(delayMs)
            return stations.map { NearbyServerStation(it, 100.0) }
        }
        override suspend fun byCnpj(cnpj: String): ServerStation? = null
        override suspend fun detail(stationId: String): ServerStation = error("no detail in viewport test")
    }

    private class FakeEngine : MapEngine {
        var markers: List<MapOverlayMarker> = emptyList()
        var listener: ((MapOverlaySelection) -> Unit)? = null
        override fun initialize(context: Context, onReady: () -> Unit) = true
        override fun newMapView(context: Context): View = throw UnsupportedOperationException()
        override fun setViewport(latitude: Double, longitude: Double, zoom: Int) = Unit
        override fun currentCenter() = null
        override fun recenter() = false
        override fun isInitialized() = true
        override fun search(context: Context, query: String, onResult: (List<MapSearchResult>) -> Unit) = true
        override fun buildCarRoute(sLat: Double, sLon: Double, eLat: Double, eLon: Double, onResult: (MapRouteInfo?) -> Unit) = true
        override fun setOverlayMarkers(markers: List<MapOverlayMarker>): Boolean {
            this.markers = markers
            return true
        }
        override fun overlayClusters() = emptyList<com.anpfuel.maps.engine.MapOverlayCluster>()
        override fun setOnOverlaySelected(listener: (MapOverlaySelection) -> Unit) {
            this.listener = listener
        }
        override fun cancelSearch() = Unit
        override fun cancelRoute() = Unit
        override fun release() = Unit
    }

    private fun station(
        id: String,
        lat: Double? = -25.42,
        lon: Double? = -49.27,
        quality: StationLocationQuality = StationLocationQuality.REVIEWED,
        name: String = "Posto $id",
    ) = ServerStation.create(
        stationId = id,
        displayName = name,
        locationQuality = quality,
        latitude = lat,
        longitude = lon,
        cnpjNormalized = null,
        municipalityCode = "4106902",
        state = "PR",
        currentRevisionId = null,
    )

    @BeforeEach fun setup() {
        Dispatchers.setMain(dispatcher)
    }

    @AfterEach fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test fun `viewport queries are bounded and debounced with stale discard`() = runTest {
        val gateway = FakeGateway(
            stations = listOf(station("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")),
        )
        val engine = FakeEngine()
        val vm = MapDiscoveryViewModel(gateway, ChargingMarkersSource { emptyList() }, engine)
        // Rapid viewport moves: only the last survives debounce.
        vm.onViewport(-25.42, -49.27, 100_000)
        vm.onViewport(-25.43, -49.28, 10_000)
        advanceTimeBy(400)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(1, gateway.calls)
        assertEquals(10_000, gateway.lastRadius)
        assertEquals(1, vm.ui.value.clusters.single().markers.size)
        assertEquals(1, vm.ui.value.listItems.size)
    }

    @Test fun `unknown and city-centroid never become pins but stay in list`() = runTest {
        val gateway = FakeGateway(
            stations = listOf(
                station("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", quality = StationLocationQuality.REVIEWED),
                station("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", quality = StationLocationQuality.UNKNOWN),
                station("cccccccc-cccc-4ccc-8ccc-cccccccccccc", quality = StationLocationQuality.CITY_CENTROID),
            ),
        )
        val engine = FakeEngine()
        val vm = MapDiscoveryViewModel(gateway, ChargingMarkersSource { emptyList() }, engine)
        vm.onViewport(-25.42, -49.27)
        advanceTimeBy(400)
        dispatcher.scheduler.advanceUntilIdle()
        // Only REVIEWED reaches the native overlay.
        assertEquals(1, engine.markers.size)
        assertEquals(3, vm.ui.value.listItems.size)
        assertTrue(vm.ui.value.listItems.any { it.locationLabel.contains("unknown", ignoreCase = true) })
    }

    @Test fun `clusters are deterministic and selection is preserved`() = runTest {
        val a = station("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", lat = -25.4200, lon = -49.2700)
        val b = station("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", lat = -25.4201, lon = -49.2701)
        val gateway = FakeGateway(stations = listOf(a, b))
        val engine = FakeEngine()
        val vm = MapDiscoveryViewModel(gateway, ChargingMarkersSource { emptyList() }, engine)
        vm.onViewport(-25.42, -49.27)
        advanceTimeBy(400)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(1, vm.ui.value.clusters.size)
        assertTrue(vm.ui.value.clusters.single().isCluster)
        vm.onSelection("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
        assertEquals("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", vm.ui.value.selectedUuid)
        // New generation with the same UUID preserves selection.
        vm.onViewport(-25.42, -49.27)
        advanceTimeBy(400)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", vm.ui.value.selectedUuid)
    }

    @Test fun `native cluster callback expands without technical ids in ui`() = runTest {
        val gateway = FakeGateway(
            stations = listOf(
                station("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", lat = -25.4200, lon = -49.2700),
                station("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", lat = -25.4201, lon = -49.2701),
            ),
        )
        val engine = FakeEngine()
        val vm = MapDiscoveryViewModel(gateway, ChargingMarkersSource { emptyList() }, engine)
        vm.onViewport(-25.42, -49.27)
        advanceTimeBy(400)
        dispatcher.scheduler.advanceUntilIdle()
        val cluster = vm.ui.value.clusters.single()
        // UI list never exposes ABASTEVO_CLUSTER ids.
        assertTrue(vm.ui.value.listItems.none { it.contentDescription.contains("ABASTEVO_CLUSTER") })
        vm.onClusterExpanded(cluster.id)
        assertEquals(2, vm.ui.value.selectedMembers.size)
    }

    @Test fun `fuel charging and combined sites stay distinguishable`() = runTest {
        val fuel = station("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", lat = -25.42, lon = -49.27)
        val gateway = FakeGateway(stations = listOf(fuel))
        val chargingPlace = com.anpfuel.domain.charging.ChargingPlace(
            id = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
            entityKey = "a".repeat(64),
            externalId = "ext-1",
            name = "Recarga Central",
            country = "BR",
            latitude = "-25.4201",
            longitude = "-49.2701",
            publicAccess = com.anpfuel.domain.charging.ChargingAccess.PUBLIC,
            reportedPoints = 2,
            groups = emptyList(),
        )
        val engine = FakeEngine()
        val vm = MapDiscoveryViewModel(gateway, ChargingMarkersSource { listOf(chargingPlace) }, engine)
        vm.onViewport(-25.42, -49.27)
        advanceTimeBy(400)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(1, vm.ui.value.clusters.size)
        val combined = vm.ui.value.clusters.single()
        assertTrue(combined.fuel)
        assertTrue(combined.charging)
        val items = vm.ui.value.listItems
        assertTrue(items.any { it.type == com.anpfuel.maps.engine.MapStationType.FUEL })
        assertTrue(items.any { it.type == com.anpfuel.maps.engine.MapStationType.CHARGING })
    }

    @Test fun `dense viewport clusters deterministically`() = runTest {
        val stations = (0 until 50).map { i ->
            station(
                "%08x-1111-4111-8111-%012d".format(i, i),
                lat = -25.42 + (i % 10) * 0.0001,
                lon = -49.27 + (i / 10) * 0.0001,
            )
        }
        val gateway = FakeGateway(stations = stations)
        val engine = FakeEngine()
        val vm = MapDiscoveryViewModel(gateway, ChargingMarkersSource { emptyList() }, engine)
        vm.onViewport(-25.42, -49.27)
        advanceTimeBy(400)
        dispatcher.scheduler.advanceUntilIdle()
        val first = vm.ui.value.clusters.map { it.markers.map { m -> m.uuid }.sorted() }.sortedBy { it.firstOrNull() }
        vm.onViewport(-25.42, -49.27)
        advanceTimeBy(400)
        dispatcher.scheduler.advanceUntilIdle()
        val second = vm.ui.value.clusters.map { it.markers.map { m -> m.uuid }.sorted() }.sortedBy { it.firstOrNull() }
        assertEquals(first, second)
        assertEquals(50, vm.ui.value.listItems.size)
    }

    @Test fun `search by city and name is bounded with stale discard`() = runTest {
        val gateway = FakeGateway(
            stations = listOf(
                station("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", name = "Posto Central Curitiba"),
                station("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", name = "Recarga Norte"),
            ),
        )
        val engine = FakeEngine()
        val vm = MapDiscoveryViewModel(gateway, ChargingMarkersSource { emptyList() }, engine)
        vm.onSearch("4106902", "Central")
        advanceTimeBy(400)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals("4106902" to "Central", gateway.lastSearch)
        assertEquals(1, vm.ui.value.listItems.size)
        assertEquals("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", vm.ui.value.listItems.single().uuid)
        // Short queries never hit the wire.
        val before = gateway.calls
        vm.onSearch("4106902", "x")
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(before, gateway.calls)
    }

    @Test fun `service filters keep unknown metadata visible`() = runTest {
        val gateway = FakeGateway(
            stations = listOf(
                station("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", name = "Posto"),
            ),
        )
        val chargingPlace = com.anpfuel.domain.charging.ChargingPlace(
            id = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
            entityKey = "a".repeat(64),
            externalId = "ext-1",
            name = "Recarga",
            country = "BR",
            latitude = "-25.4201",
            longitude = "-49.2701",
            publicAccess = com.anpfuel.domain.charging.ChargingAccess.PUBLIC,
            reportedPoints = 2,
            groups = emptyList(),
        )
        val engine = FakeEngine()
        val vm = MapDiscoveryViewModel(gateway, ChargingMarkersSource { listOf(chargingPlace) }, engine)
        vm.onViewport(-25.42, -49.27)
        advanceTimeBy(400)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(2, vm.ui.value.listItems.size)
        // Filter to fuel only: charging hidden, fuel stays, unknown power not excluded elsewhere.
        vm.onServiceFilter(showFuel = true, showCharging = false)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(1, vm.ui.value.listItems.size)
        assertEquals(com.anpfuel.maps.engine.MapStationType.FUEL, vm.ui.value.listItems.single().type)
        // At least one service always stays visible.
        vm.onServiceFilter(showFuel = false, showCharging = false)
        assertTrue(vm.ui.value.showFuel || vm.ui.value.showCharging)
    }
}
