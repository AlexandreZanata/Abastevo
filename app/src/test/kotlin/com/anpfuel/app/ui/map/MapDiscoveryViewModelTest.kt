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
import com.anpfuel.maps.engine.MapStationLocationQuality
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
        var failNearby: Boolean = false,
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
            if (failNearby) throw java.io.IOException("offline")
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
        val viewports = mutableListOf<Pair<Double, Double>>()
        override fun setViewport(latitude: Double, longitude: Double, zoom: Int) {
            viewports += latitude to longitude
        }
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

    private class FakeStationCache : com.anpfuel.domain.repository.ServerStationCache {
        val pages = mutableMapOf<String, com.anpfuel.domain.discovery.ServerStationPage>()
        override suspend fun saveScopedPage(municipalityCode: String, page: com.anpfuel.domain.discovery.ServerStationPage) {
            pages[municipalityCode] = page
        }
        override suspend fun loadScopedPage(municipalityCode: String) = pages[municipalityCode]
        override suspend fun savePage(page: com.anpfuel.domain.discovery.ServerStationPage) = Unit
        override suspend fun loadPage() = null
        override suspend fun saveDetail(station: com.anpfuel.domain.discovery.ServerStation) = Unit
        override suspend fun loadDetail(stationId: String) = null
        override suspend fun clear() = Unit
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

    @Test fun `unpinned and coordinate-less stations are selectable by uuid with route disabled`() = runTest {
        val reviewedId = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
        val unknownId = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
        val centroidId = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
        val nullCoordId = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
        val gateway = FakeGateway(
            stations = listOf(
                station(reviewedId, quality = StationLocationQuality.REVIEWED),
                station(unknownId, quality = StationLocationQuality.UNKNOWN),
                station(centroidId, quality = StationLocationQuality.CITY_CENTROID),
                station(nullCoordId, lat = null, lon = null, quality = StationLocationQuality.UNKNOWN),
            ),
        )
        val engine = FakeEngine()
        val vm = MapDiscoveryViewModel(gateway, ChargingMarkersSource { emptyList() }, engine)
        vm.onViewport(-25.42, -49.27)
        advanceTimeBy(400)
        dispatcher.scheduler.advanceUntilIdle()

        assertEquals(4, vm.ui.value.listItems.size)
        assertEquals(1, engine.markers.size)

        vm.onSelection(reviewedId)
        assertEquals(reviewedId, vm.ui.value.selectedUuid)
        assertTrue(vm.ui.value.selectedMarker!!.canRouteByCoordinates)

        vm.onSelection(unknownId)
        assertEquals(unknownId, vm.ui.value.selectedUuid)
        org.junit.jupiter.api.Assertions.assertNotNull(vm.ui.value.selectedMarker)
        org.junit.jupiter.api.Assertions.assertFalse(vm.ui.value.selectedMarker!!.canRouteByCoordinates)

        vm.onSelection(centroidId)
        assertEquals(centroidId, vm.ui.value.selectedUuid)
        org.junit.jupiter.api.Assertions.assertNotNull(vm.ui.value.selectedMarker)
        org.junit.jupiter.api.Assertions.assertFalse(vm.ui.value.selectedMarker!!.canRouteByCoordinates)

        vm.onSelection(nullCoordId)
        assertEquals(nullCoordId, vm.ui.value.selectedUuid)
        org.junit.jupiter.api.Assertions.assertNotNull(vm.ui.value.selectedMarker)
        org.junit.jupiter.api.Assertions.assertFalse(vm.ui.value.selectedMarker!!.hasValidCoordinates)
        org.junit.jupiter.api.Assertions.assertFalse(vm.ui.value.selectedMarker!!.canRouteByCoordinates)
    }

    @Test fun `cluster expansion reaches second and last member and reconciles across generations`() = runTest {
        val aId = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
        val bId = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
        val cId = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
        val gateway = FakeGateway(
            stations = listOf(
                station(aId, lat = -25.4200, lon = -49.2700, name = "Posto Alpha"),
                station(bId, lat = -25.4201, lon = -49.2701, name = "Posto Beta"),
                station(cId, lat = -25.4202, lon = -49.2702, name = "Posto Gamma"),
            ),
        )
        val engine = FakeEngine()
        val vm = MapDiscoveryViewModel(gateway, ChargingMarkersSource { emptyList() }, engine)
        vm.onViewport(-25.42, -49.27)
        advanceTimeBy(400)
        dispatcher.scheduler.advanceUntilIdle()

        val cluster = vm.ui.value.clusters.single()
        assertEquals(3, cluster.count)

        // Native cluster tap expands without auto-selecting the first member.
        engine.listener?.invoke(MapOverlaySelection.Cluster(cluster.markers))
        org.junit.jupiter.api.Assertions.assertNull(vm.ui.value.selectedUuid)
        assertEquals(cluster.id, vm.ui.value.expandedClusterId)
        assertEquals(3, vm.ui.value.selectedMembers.size)

        // Select 2nd member.
        val secondUuid = vm.ui.value.selectedMembers[1].uuid
        vm.onSelection(secondUuid)
        assertEquals(bId, vm.ui.value.selectedUuid)
        assertEquals(bId, vm.ui.value.selectedMarker?.uuid)
        assertEquals(3, vm.ui.value.selectedMembers.size)

        // Expand again via UI control and select last (3rd) member.
        vm.onClusterExpanded(cluster.id)
        org.junit.jupiter.api.Assertions.assertNull(vm.ui.value.selectedUuid)
        val lastUuid = vm.ui.value.selectedMembers.last().uuid
        vm.onSelection(lastUuid)
        assertEquals(cId, vm.ui.value.selectedUuid)
        assertEquals(cId, vm.ui.value.selectedMarker?.uuid)

        // Trigger a new viewport generation: selection of last member is reconciled & preserved.
        vm.onViewport(-25.42, -49.27)
        advanceTimeBy(400)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(cId, vm.ui.value.selectedUuid)
        assertEquals(cId, vm.ui.value.selectedMarker?.uuid)
        assertEquals(3, vm.ui.value.selectedMembers.size)
    }

    @Test fun `unchanged native viewport refresh does not replace an active search`() = runTest {
        val gateway = FakeGateway(
            stations = listOf(
                station("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", name = "Posto Central"),
                station("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", name = "Posto Bairro"),
            ),
        )
        val vm = MapDiscoveryViewModel(gateway, ChargingMarkersSource { emptyList() }, FakeEngine())
        vm.onSearch("4106902", "Central")
        advanceTimeBy(400)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(listOf("Posto Central"), vm.ui.value.listItems.map { it.title })
        val callsAfterSearch = gateway.calls

        vm.onViewport(-25.43, -49.28, 15_000)
        advanceTimeBy(400)
        dispatcher.scheduler.advanceUntilIdle()

        assertEquals(listOf("Posto Central"), vm.ui.value.listItems.map { it.title })
        assertEquals(callsAfterSearch, gateway.calls, "viewport sync must not refetch while search is active")

        vm.onSearch("4106902", "")
        advanceTimeBy(400)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals("", vm.ui.value.searchQuery)
        assertTrue(vm.ui.value.listItems.size == 2, "clearing search must return to nearby results")
    }

    @Test fun `offline viewport uses the last saved public map page and marks it cached`() = runTest {
        val station = station("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", name = "Posto Offline")
        val cache = FakeStationCache()
        val gateway = FakeGateway(stations = listOf(station), failNearby = true)
        val vm = MapDiscoveryViewModel(
            gateway, ChargingMarkersSource { emptyList() }, FakeEngine(),
            null, null, cache,
        )
        cache.saveScopedPage(
            "map:4106902",
            com.anpfuel.domain.discovery.ServerStationPage(listOf(station), null),
        )
        vm.onViewport(-25.42, -49.27, 10_000)
        advanceTimeBy(400)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(listOf("Posto Offline"), vm.ui.value.listItems.map { it.title })
        assertTrue(vm.ui.value.offlineCached)
        assertEquals(null, vm.ui.value.error)
    }

    @Test fun `clearing or shortening search during debounce or in-flight cancels prior query`() = runTest {
        val gateway = FakeGateway(
            stations = listOf(
                station("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", name = "Posto Central Curitiba"),
            ),
            delayMs = 300,
        )
        val engine = FakeEngine()
        val vm = MapDiscoveryViewModel(gateway, ChargingMarkersSource { emptyList() }, engine)

        // Case 1: Clear search during the 300ms debounce window.
        vm.onSearch("4106902", "Central")
        advanceTimeBy(150)
        vm.onSearch("4106902", "")
        advanceTimeBy(1_000)
        dispatcher.scheduler.advanceUntilIdle()
        // Clearing search returns to viewport mode, but the canceled search
        // must never have reached the gateway.
        org.junit.jupiter.api.Assertions.assertNull(gateway.lastSearch)
        assertEquals("", vm.ui.value.searchQuery)
        assertEquals(1, vm.ui.value.listItems.size)

        // Case 2: Clear search while network call is already in-flight (after debounce).
        val callsBeforeCase2 = gateway.calls
        val listBeforeCase2 = vm.ui.value.listItems.map { it.title }
        vm.onSearch("4106902", "Central")
        advanceTimeBy(350) // Past 300ms debounce, inside 300ms gateway delay.
        assertEquals(callsBeforeCase2 + 1, gateway.calls)
        vm.onSearch("4106902", "c") // Shortened to < 2 chars while in-flight.
        advanceTimeBy(1_000)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals("c", vm.ui.value.searchQuery)
        assertEquals(listBeforeCase2, vm.ui.value.listItems.map { it.title }, "In-flight search must not overwrite cleared state")
    }

    @Test fun `charging markers source paginates beyond first 100 rows and preserves unknown power`() = runTest {
        val profile = com.anpfuel.data.repository.ChargingDiscoveryRepository.OCM_PROFILE
        val now = 1_700_000_000_000L
        val allPlaces = (0 until 125).map { idx ->
            val id = "%08d-2222-4222-8222-%012d".format(idx, idx)
            val isCuritiba = idx >= 120
            val lat = if (isCuritiba) "-25.420${idx - 120}" else "-23.5500"
            val lon = if (isCuritiba) "-49.270${idx - 120}" else "-46.6300"
            val powerWJson = when (idx) {
                120 -> "60000"
                121 -> "null" // unknown power must remain visible when minPowerKw is set
                122 -> "11000" // known power < 50kW must be filtered out
                else -> "50000"
            }
            val groupId = "%08d-3333-4333-8333-%012d".format(idx, idx)
            val entityKey = "%064x".format(idx + 1)
            val groupsJson = """[{"id":"$groupId","entity_key":"$entityKey","external_id":"grp-$idx","connector_code":"CCS2","raw_connector_code":"CCS2","current":"DC","power_w":$powerWJson,"reported_quantity":2}]"""
            com.anpfuel.data.local.entity.ChargingSnapshotPlaceEntity(
                placeId = id,
                entityKey = entityKey,
                externalId = "ocm-$idx",
                generation = 1L,
                profile = profile,
                sourceRevision = "rev-1",
                origin = "https://teste.abastevo.com.br",
                license = com.anpfuel.data.repository.ChargingDiscoveryRepository.OCM_LICENSE,
                name = if (isCuritiba) "Eletroposto Curitiba $idx" else "Eletroposto SP $idx",
                country = "BR",
                latitude = lat,
                longitude = lon,
                publicAccess = "PUBLIC",
                reportedPoints = 2,
                groupsJson = groupsJson,
                tombstone = false,
                savedAtMillis = now,
            )
        }
        val metaEntity = com.anpfuel.data.local.entity.ChargingSnapshotMetaEntity(
            key = com.anpfuel.data.local.entity.ChargingSnapshotMetaEntity.packKey(profile),
            generation = 1L,
            profile = profile,
            sourceRevision = "rev-1",
            origin = "https://teste.abastevo.com.br",
            license = com.anpfuel.data.repository.ChargingDiscoveryRepository.OCM_LICENSE,
            schemaVersion = 1,
            baseGeneration = 0L,
            placeCount = allPlaces.size.toLong(),
            groupCount = allPlaces.size.toLong(),
            cursor = null,
            state = "ACTIVE",
            savedAtMillis = now,
        )
        val fakeDao = object : com.anpfuel.data.local.dao.ChargingSnapshotDao {
            override suspend fun upsertPlaces(places: List<com.anpfuel.data.local.entity.ChargingSnapshotPlaceEntity>) = Unit
            override suspend fun upsertMeta(meta: com.anpfuel.data.local.entity.ChargingSnapshotMetaEntity) = Unit
            override suspend fun deleteOlderGenerations(profile: String, generation: Long) = Unit
            override suspend fun deleteProfilePlaces(profile: String) = Unit
            override suspend fun clearMeta(key: String) = Unit
            override suspend fun markTombstones(ids: List<String>, generation: Long) = Unit
            override suspend fun findMeta(key: String) = metaEntity.takeIf { it.key == key }
            override suspend fun readPlaces(profile: String, generation: Long, limit: Int, offset: Int) =
                allPlaces.drop(offset).take(limit)
            override suspend fun findPlace(placeId: String, profile: String, generation: Long) =
                allPlaces.firstOrNull { it.placeId == placeId }
            override suspend fun readPlacesInBounds(
                profile: String,
                generation: Long,
                minLatitude: Double,
                maxLatitude: Double,
                minLongitude: Double,
                maxLongitude: Double,
                limit: Int,
            ) = allPlaces.filter {
                it.latitude?.toDoubleOrNull()?.let { lat -> lat in minLatitude..maxLatitude } == true &&
                    it.longitude?.toDoubleOrNull()?.let { lon -> lon in minLongitude..maxLongitude } == true
            }.take(limit)

            override suspend fun countPlaces(generation: Long) = allPlaces.size.toLong()
            override suspend fun estimatedBytes(generation: Long) = 1024L
        }
        val repo = com.anpfuel.data.repository.ChargingDiscoveryRepository(
            client = com.anpfuel.data.remote.charging.ChargingSnapshotHttpClient(okhttp3.OkHttpClient(), "https://teste.abastevo.com.br"),
            dao = fakeDao,
            environment = com.anpfuel.data.remote.ApiEnvironment.STAGING,
        )
        val source = MapDiscoveryModule.provideChargingMarkersSource(repo)
        val result = source.query(
            ChargingMarkersQuery(
                latitude = -25.42,
                longitude = -49.27,
                radiusMeters = 10_000,
                minPowerKw = 50,
                limit = 100,
            )
        )
        assertTrue(result.snapshotPresent)
        // Indices 120 (60kW), 121 (unknown power), 123 (50kW), 124 (50kW) match; 122 (11kW) is excluded.
        assertEquals(4, result.places.size)
        assertTrue(result.places.any { it.name == "Eletroposto Curitiba 120" })
        assertTrue(result.places.any { it.name == "Eletroposto Curitiba 121" })
        assertTrue(result.places.none { it.name == "Eletroposto Curitiba 122" })
    }
    @Test fun `a city outside the shortcuts uses the national scoped directory without fabricating pins`() = runTest {
        val gateway = io.mockk.mockk<ServerStationGateway>()
        val id = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
        val townStation = station(id, lat = null, lon = null, quality = StationLocationQuality.UNKNOWN)
        io.mockk.coEvery { gateway.listInMunicipality("5103403", 100, null) } returns ServerStationPage(listOf(townStation), null)
        val vm = MapDiscoveryViewModel(gateway, ChargingMarkersSource { emptyList() }, FakeEngine())
        vm.onSelectCity("5103403", "Cuiabá")
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals("5103403", vm.ui.value.municipalityCode)
        assertTrue(vm.ui.value.cityLocationUnknown)
        assertTrue(vm.ui.value.clusters.isEmpty())
        vm.onSelection(id)
        assertEquals(id, vm.ui.value.selectedMarker?.uuid)
        assertEquals(false, vm.ui.value.selectedMarker?.canRouteByCoordinates)
    }

    @Test fun `every pin tier reaches the native overlay with its own tier`() = runTest {
        val gateway = FakeGateway(
            stations = listOf(
                station("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", lat = -25.40, quality = StationLocationQuality.REVIEWED),
                station("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", lat = -25.41, quality = StationLocationQuality.OSM_MAPPED),
                station("cccccccc-cccc-4ccc-8ccc-cccccccccccc", lat = -25.42, quality = StationLocationQuality.ANP_REPORTED),
                station("dddddddd-dddd-4ddd-8ddd-dddddddddddd", lat = -25.43, quality = StationLocationQuality.ADDRESS_POINT),
                station("eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", lat = -25.44, quality = StationLocationQuality.CITY_CENTROID),
                station("ffffffff-ffff-4fff-8fff-ffffffffffff", lat = null, lon = null, quality = StationLocationQuality.OSM_MAPPED),
            ),
        )
        val engine = FakeEngine()
        val vm = MapDiscoveryViewModel(gateway, ChargingMarkersSource { emptyList() }, engine)
        vm.onViewport(-25.42, -49.27)
        advanceTimeBy(400)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(
            mapOf(
                "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" to MapStationLocationQuality.REVIEWED,
                "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb" to MapStationLocationQuality.OSM_MAPPED,
                "cccccccc-cccc-4ccc-8ccc-cccccccccccc" to MapStationLocationQuality.ANP_REPORTED,
                "dddddddd-dddd-4ddd-8ddd-dddddddddddd" to MapStationLocationQuality.ADDRESS_POINT,
            ),
            engine.markers.associate { it.uuid to it.locationQuality },
        )
        // City-centroid and coordinate-less stations stay list-only.
        assertEquals(6, vm.ui.value.listItems.size)
        assertTrue(engine.markers.all { it.canRouteByCoordinates })
    }

    @Test fun `city viewport centers on the best available pin tier`() = runTest {
        val gateway = io.mockk.mockk<ServerStationGateway>()
        val address = station("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", lat = -15.60, lon = -56.10, quality = StationLocationQuality.ADDRESS_POINT)
        val anp = station("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", lat = -15.61, lon = -56.11, quality = StationLocationQuality.ANP_REPORTED)
        val centroid = station("cccccccc-cccc-4ccc-8ccc-cccccccccccc", lat = -15.62, lon = -56.12, quality = StationLocationQuality.CITY_CENTROID)
        io.mockk.coEvery { gateway.listInMunicipality("5103403", 100, null) } returns
            ServerStationPage(listOf(centroid, address, anp), null)
        val engine = FakeEngine()
        val vm = MapDiscoveryViewModel(gateway, ChargingMarkersSource { emptyList() }, engine)
        vm.onSelectCity("5103403", "Cuiabá")
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(false, vm.ui.value.cityLocationUnknown)
        assertEquals(-15.61 to -56.11, engine.viewports.last())
        assertEquals(setOf(address.stationId, anp.stationId), engine.markers.map { it.uuid }.toSet())
    }

    @Test fun `opened city point follows the camera centered on the city, then stations, then built-in`() = runTest {
        val gateway = io.mockk.mockk<ServerStationGateway>(relaxed = true)
        val anp = station("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", lat = -12.54, lon = -55.71, quality = StationLocationQuality.ANP_REPORTED)
        val centroidA = station("cccccccc-cccc-4ccc-8ccc-cccccccccccc", lat = -12.0, lon = -55.0, quality = StationLocationQuality.CITY_CENTROID)
        val centroidB = station("dddddddd-dddd-4ddd-8ddd-dddddddddddd", lat = -13.0, lon = -56.0, quality = StationLocationQuality.CITY_CENTROID)
        io.mockk.coEvery { gateway.listInMunicipality("5107925", 100, null) } returns ServerStationPage(listOf(centroidA, anp), null)
        io.mockk.coEvery { gateway.listInMunicipality("5100250", 100, null) } returns ServerStationPage(listOf(centroidA, centroidB), null)
        val vm = MapDiscoveryViewModel(gateway, ChargingMarkersSource { emptyList() }, FakeEngine())
        // Default city (Curitiba) is known before any selection.
        assertEquals(-25.4284 to -49.2733, vm.ui.value.cityLatitude to vm.ui.value.cityLongitude)
        // Pinned station: the camera centers on it, so it represents the city.
        vm.onSelectCity("5107925", "Sorriso")
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(-12.54 to -55.71, vm.ui.value.cityLatitude to vm.ui.value.cityLongitude)
        // No pin: the centroid of the city's loaded stations.
        vm.onSelectCity("5100250", "Alta Floresta")
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(-12.5 to -55.5, vm.ui.value.cityLatitude to vm.ui.value.cityLongitude)
        // Built-in city coordinates.
        vm.onSelectCity("1302603", "Manaus")
        assertEquals(-3.1190 to -60.0217, vm.ui.value.cityLatitude to vm.ui.value.cityLongitude)
        // Panning elsewhere does not move the opened city's point.
        vm.onViewport(-23.55, -46.63)
        assertEquals(-3.1190 to -60.0217, vm.ui.value.cityLatitude to vm.ui.value.cityLongitude)
        dispatcher.scheduler.advanceUntilIdle()
    }

    @Test fun `unknown city without any station coordinate has no city point`() = runTest {
        val gateway = io.mockk.mockk<ServerStationGateway>()
        val bare = station("dddddddd-dddd-4ddd-8ddd-dddddddddddd", lat = null, lon = null, quality = StationLocationQuality.UNKNOWN)
        io.mockk.coEvery { gateway.listInMunicipality("5103403", 100, null) } returns ServerStationPage(listOf(bare), null)
        val vm = MapDiscoveryViewModel(gateway, ChargingMarkersSource { emptyList() }, FakeEngine())
        vm.onSelectCity("5103403", "Cuiabá")
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(null, vm.ui.value.cityLatitude)
        assertEquals(null, vm.ui.value.cityLongitude)
    }
}
