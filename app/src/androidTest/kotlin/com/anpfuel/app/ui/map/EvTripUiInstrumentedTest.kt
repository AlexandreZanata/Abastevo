package com.anpfuel.app.ui.map

import android.content.Context
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.performTextReplacement
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.app.MainActivity
import com.anpfuel.app.R
import com.anpfuel.app.debug.InstrumentedDbEntryPoint
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.data.local.entity.ChargingSnapshotMetaEntity
import com.anpfuel.data.local.entity.ChargingSnapshotPlaceEntity
import com.anpfuel.data.mapper.VehicleMapper
import com.anpfuel.data.repository.ChargingDiscoveryRepository
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.valueobject.BatteryCapacity
import com.anpfuel.domain.valueobject.Propulsion
import com.anpfuel.domain.valueobject.VehiclePriceSource
import com.anpfuel.domain.ev.EvRoadLeg
import com.anpfuel.domain.routing.RoutePoint
import com.anpfuel.maps.engine.MapEngineHolder
import com.anpfuel.maps.engine.MapCamera
import com.anpfuel.maps.engine.NativeEvRoadRouter
import com.anpfuel.maps.engine.SharedPreferencesMapCameraStore
import dagger.hilt.android.EntryPointAccessors
import java.math.BigDecimal
import java.math.RoundingMode
import java.util.UUID
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertNotNull
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.json.JSONArray
import org.json.JSONObject

/**
 * MAP-28 UI proof on the real app and native core. The vehicle and charging
 * places are synthetic fixtures inserted into the disposable test database;
 * they are not a coverage or live-availability claim.
 */
@RunWith(AndroidJUnit4::class)
class EvTripUiInstrumentedTest {
    @get:Rule val compose = createAndroidComposeRule<MainActivity>()
    private val context = ApplicationProvider.getApplicationContext<Context>()
    private val profile = ChargingDiscoveryRepository.OCM_PROFILE
    private val vehicleId = "11111111-1111-4111-8111-111111111111"
    private val primaryPlaceId = "22222222-2222-4222-8222-222222222222"
    private val altPlaceId = "33333333-3333-4333-8333-333333333333"
    private val entityKey = "a".repeat(64)

    private fun db(): AnpFuelDatabase =
        EntryPointAccessors.fromApplication(context, InstrumentedDbEntryPoint::class.java).database()

    private fun coordinate(value: Double): String =
        BigDecimal.valueOf(value).setScale(6, RoundingMode.HALF_UP).toPlainString()

    private fun place(id: String, name: String, latitude: String, longitude: String): ChargingSnapshotPlaceEntity {
        val groupId = UUID.nameUUIDFromBytes("group:$id".toByteArray()).toString()
        val group = JSONObject()
            .put("id", groupId)
            .put("entity_key", entityKey)
            .put("external_id", "SYNTHETIC-$id")
            .put("connector_code", "CCS2")
            .put("raw_connector_code", "CCS2")
            .put("current", "DC")
            .put("power_w", 50_000)
            .put("reported_quantity", 1)
        return ChargingSnapshotPlaceEntity(
            placeId = id,
            entityKey = entityKey,
            externalId = "SYNTHETIC-$id",
            generation = 1,
            profile = profile,
            sourceRevision = "synthetic-ui-fixture-v1",
            origin = "https://teste.abastevo.com.br",
            license = ChargingDiscoveryRepository.OCM_LICENSE,
            name = name,
            country = "BR",
            latitude = latitude,
            longitude = longitude,
            publicAccess = "PUBLIC",
            reportedPoints = 1,
            groupsJson = JSONArray().put(group).toString(),
            tombstone = false,
            savedAtMillis = 1,
        )
    }

    private fun seedVehicle() = runBlocking {
        db().vehicleDao().listAll().forEach { db().vehicleDao().deleteById(it.id) }
        db().vehicleDao().upsert(
            VehicleMapper.toEntity(
                Vehicle.create(
                    displayName = "Synthetic EV",
                    tankCapacity = null,
                    fuelProduct = null,
                    priceSource = VehiclePriceSource.cheapest(),
                    propulsion = Propulsion.BEV,
                    batteryCapacity = BatteryCapacity.ofWh(60_000),
                    chargeConnector = ChargingConnector.CCS2,
                    id = com.anpfuel.domain.valueobject.DomainId.from(vehicleId),
                ),
            ),
        )
    }

    private fun seedCharging(primaryPoint: RoutePoint, altPoint: RoutePoint) = runBlocking {
        val meta = ChargingSnapshotMetaEntity(
                key = ChargingSnapshotMetaEntity.packKey(profile),
                generation = 1,
                profile = profile,
                sourceRevision = "synthetic-ui-fixture-v1",
                origin = "https://teste.abastevo.com.br",
                license = ChargingDiscoveryRepository.OCM_LICENSE,
                schemaVersion = 1,
                baseGeneration = 0,
                placeCount = 2,
                groupCount = 2,
                cursor = null,
                state = ChargingSnapshotMetaEntity.STATE_COMPLETE,
            savedAtMillis = 1,
        )
        db().chargingSnapshotDao().replaceGeneration(
            meta,
            listOf(
                place(primaryPlaceId, "Synthetic primary charger", coordinate(primaryPoint.latitude), coordinate(primaryPoint.longitude)),
                place(altPlaceId, "Synthetic alternative charger", coordinate(altPoint.latitude), coordinate(altPoint.longitude)),
            ),
        )
    }

    @After
    fun clearFixtures() {
        runBlocking {
            db().chargingSnapshotDao().removePack(profile)
            db().vehicleDao().deleteById(vehicleId)
        }
    }

    private fun text(id: Int) = compose.activity.getString(id)

    @Test
    fun electricPlanShowsAlternativeReplansFromFailureAndSaves() {
        SharedPreferencesMapCameraStore(compose.activity).save(MapCamera(-25.43, -49.27, 13))
        compose.onNodeWithContentDescription(text(R.string.nav_map), useUnmergedTree = true).performClick()
        compose.waitUntil(45_000) {
            MapEngineHolder.engine.isInitialized() && MapEngineHolder.engine.isRenderingReady()
        }
        val origin = RoutePoint(-25.4295, -49.2719, "synthetic origin")
        val destination = RoutePoint(-25.4222, -49.2640, "synthetic destination")
        val road = runBlocking {
            NativeEvRoadRouter(MapEngineHolder.engine, timeoutMillis = 60_000)
                .leg(listOf(origin, destination), emptySet(), "en") as? EvRoadLeg.Ready
        }
        assertNotNull("real route for synthetic charging fixtures", road)
        val geometry = road!!.geometry
        val primaryPoint = geometry[(geometry.size * 55 / 100).coerceIn(1, geometry.lastIndex - 1)]
        val altPoint = geometry[(geometry.size * 48 / 100).coerceIn(1, geometry.lastIndex - 1)]
        seedVehicle()
        seedCharging(primaryPoint, altPoint)
        compose.onNodeWithTag("native-route-open").performScrollTo().performClick()

        compose.onNodeWithText(text(R.string.map22_origin), substring = false).performScrollTo().performClick()
        compose.onNodeWithText(text(R.string.map22_latitude)).performTextInput(origin.latitude.toString())
        compose.onNodeWithText(text(R.string.map22_longitude)).performTextInput(origin.longitude.toString())
        compose.onNodeWithText(text(R.string.map22_coordinates), substring = false).performScrollTo().performClick()
        compose.onNodeWithText(text(R.string.map22_destination), substring = false).performScrollTo().performClick()
        compose.onNodeWithText(text(R.string.map22_latitude)).performTextReplacement(destination.latitude.toString())
        compose.onNodeWithText(text(R.string.map22_longitude)).performTextReplacement(destination.longitude.toString())
        compose.onNodeWithText(text(R.string.map22_coordinates), substring = false).performScrollTo().performClick()

        compose.waitUntil(15_000) {
            compose.onAllNodesWithText("Synthetic EV").fetchSemanticsNodes().isNotEmpty()
        }
        compose.onNodeWithTag("ev-vehicle-0").performScrollTo().performClick()
        compose.onNodeWithTag("ev-soc").performScrollTo().performTextReplacement("60")
        compose.onNodeWithTag("ev-reserve").performScrollTo().performTextReplacement("10")
        compose.onNodeWithTag("ev-consumption").performScrollTo().performTextReplacement("20000")
        compose.onNodeWithTag("ev-target").performScrollTo().performTextReplacement("100")
        compose.onNodeWithTag("ev-plan").performScrollTo().performClick()
        compose.waitUntil(60_000) {
            compose.onAllNodesWithTag("ev-stop-0").fetchSemanticsNodes().isNotEmpty() ||
                compose.onAllNodesWithTag("ev-plan-status").fetchSemanticsNodes().isNotEmpty()
        }
        assertNotNull(compose.onAllNodesWithTag("ev-stop-0").fetchSemanticsNodes().firstOrNull())
        val failedSite = compose.onNodeWithTag("ev-stop-site-0").fetchSemanticsNode()
            .config[SemanticsProperties.Text].first().text
        compose.onNodeWithTag("ev-alternative").performScrollTo().assertIsDisplayed()
        compose.onNodeWithTag("ev-fail-primary").performScrollTo().performClick()
        compose.waitUntil(60_000) {
            compose.onAllNodesWithTag("ev-replanned").fetchSemanticsNodes().isNotEmpty() &&
                compose.onAllNodesWithTag("ev-stop-0").fetchSemanticsNodes().isNotEmpty()
        }
        compose.onNodeWithTag("ev-replanned").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText(failedSite).assertDoesNotExist()
        compose.onNodeWithTag("ev-save").performScrollTo().performClick()
        compose.waitUntil(15_000) {
            compose.onAllNodesWithTag("ev-saved").fetchSemanticsNodes().isNotEmpty()
        }
        compose.onNodeWithTag("ev-saved").performScrollTo().assertIsDisplayed()

        compose.onNodeWithTag("native-route-close").performScrollTo().performClick()
        compose.waitUntil(15_000) {
            compose.onAllNodesWithTag("native-route-close").fetchSemanticsNodes().isEmpty()
        }
        compose.onNodeWithTag("map-open-saved").performClick()
        compose.waitUntil(15_000) {
            compose.onAllNodesWithTag("saved-open-trip").fetchSemanticsNodes().isNotEmpty()
        }
        compose.onAllNodesWithTag("saved-open-trip")[0].performScrollTo().performClick()
        compose.waitUntil(20_000) {
            compose.onAllNodesWithTag("native-route-close").fetchSemanticsNodes().isNotEmpty()
        }
        compose.onNodeWithTag("native-route-close").assertIsDisplayed()
    }
}
