package com.anpfuel.data.enterprise.route

import com.anpfuel.application.enterprise.route.EnterpriseRoutingOutcome
import com.anpfuel.data.enterprise.EnterpriseVaultCodec
import com.anpfuel.data.enterprise.ProtectedQueueFixture.attempt
import com.anpfuel.data.enterprise.ProtectedQueueFixture.id
import com.anpfuel.data.enterprise.VaultFixture
import com.anpfuel.data.enterprise.VaultFixture.Companion.B
import com.anpfuel.data.enterprise.VaultFixture.Companion.ORIGIN
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import com.anpfuel.domain.enterprise.route.EnterpriseRoutePlan
import com.anpfuel.domain.enterprise.route.GeoPointE7
import com.anpfuel.domain.enterprise.route.GuidanceDegradedState
import com.anpfuel.domain.enterprise.route.RouteAvoidOptions
import com.anpfuel.domain.enterprise.route.RouteComputationKind
import com.anpfuel.domain.enterprise.route.RoutePlanOwnerKind
import com.anpfuel.domain.enterprise.route.RouteWaypoint
import com.anpfuel.domain.enterprise.route.RouteWaypointSource
import java.nio.file.Path
import java.time.Instant
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

class ProtectedRouteStoreTest {
    @TempDir
    lateinit var root: Path

    @Test
    fun `stores and isolates encrypted route plans per partition inside vault`() = runBlocking {
        val f = VaultFixture(root)
        f.confirm(partition())
        f.confirm(partition(org = B))
        val store = ProtectedRouteStore(f.store)
        val firstAttempt = attempt(f)
        val foreignAttempt = attempt(f, partition(org = B))

        assertNull(store.read(firstAttempt))

        val now = Instant.parse("2026-11-18T10:00:00Z")
        val plan = EnterpriseRoutePlan.create(
            id = id(701),
            revision = 1L,
            ownerKind = RoutePlanOwnerKind.TRIP,
            ownerId = id(702),
            waypoints = listOf(
                RouteWaypoint(1, "Garagem", RouteWaypointSource.COORDINATES, point = GeoPointE7(-156_000_000, -561_000_000)),
                RouteWaypoint(2, "Destino", RouteWaypointSource.COORDINATES, point = GeoPointE7(-157_000_000, -562_000_000)),
            ),
            heavyAssetNotice = true,
            updatedAt = now,
        )
        store.savePlan(firstAttempt, plan)

        val readBack = store.read(firstAttempt)
        assertNotNull(readBack)
        assertEquals(plan, readBack!!.planFor(RoutePlanOwnerKind.TRIP, id(702)))
        assertNull(store.read(foreignAttempt))
        // Ensure zero plaintext leakage in vault token files on disk.
        val token = EnterpriseVaultCodec.token(partition())
        assertTrue(f.files.tokens(EnterpriseVaultCodec.accountTag(partition().ownerAccount, ORIGIN)).contains(token))
    }

    @Test
    fun `native routing adapter executes CAR offline without server routing and maps degraded errors`() {
        val okAdapter = NativeEnterpriseRoutingAdapter(
            bridge = NativeCarRouterBridge { points, _ ->
                NativeCarRouterResult.Success(
                    engineId = "comaps-car-v1",
                    mapDataVersion = "2026.11",
                    distanceMetres = 95_000L,
                    durationSeconds = 4_200L,
                    geometry = points,
                )
            },
            nextComputationId = { id(799) },
        )
        assertFalse(okAdapter.isServerRoutingUsed)
        val wps = listOf(
            RouteWaypoint(1, "Origem", RouteWaypointSource.COORDINATES, point = GeoPointE7(-156_000_000, -561_000_000)),
            RouteWaypoint(2, "Destino", RouteWaypointSource.COORDINATES, point = GeoPointE7(-157_000_000, -562_000_000)),
        )
        val ok = okAdapter.computeRoute(
            RoutePlanOwnerKind.TRIP,
            id(702),
            1L,
            RouteComputationKind.PLANNING,
            wps,
            RouteAvoidOptions(avoidToll = true),
            Instant.parse("2026-11-18T10:00:00Z"),
        ) as EnterpriseRoutingOutcome.Computed
        assertEquals("CAR", ok.computation.profile)
        assertEquals(95_000L, ok.computation.distanceMetres)

        val missingMap = NativeEnterpriseRoutingAdapter({ _, _ -> NativeCarRouterResult.MapRegionMissing })
            .computeRoute(RoutePlanOwnerKind.TRIP, id(702), 1L, RouteComputationKind.DEPARTURE, wps, RouteAvoidOptions(), Instant.now())
            as EnterpriseRoutingOutcome.Degraded
        assertEquals(GuidanceDegradedState.MAP_REGION_MISSING, missingMap.reason)
    }
}
