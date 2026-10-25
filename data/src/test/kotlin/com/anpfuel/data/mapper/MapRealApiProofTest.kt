package com.anpfuel.data.mapper

import com.anpfuel.data.remote.BackendPriceGroupsJsonCodec
import com.anpfuel.data.remote.DirectoryStationJsonCodec
import java.util.concurrent.TimeUnit
import okhttp3.OkHttpClient
import okhttp3.Request
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Assumptions.assumeTrue
import org.junit.jupiter.api.Test

/**
 * Opt-in MAP-20 proof against a real local API process. The normal unit suite
 * skips it; run with ANPFUEL_MAP_PROOF_BASE_URL pointing at the authorized
 * backend (for example an adb-reversed loopback proof server).
 */
class MapRealApiProofTest {
    private val base = System.getenv("ANPFUEL_MAP_PROOF_BASE_URL")?.trimEnd('/')

    private fun get(path: String): String {
        val client = OkHttpClient.Builder().callTimeout(15, TimeUnit.SECONDS).build()
        val request = Request.Builder().url("$base$path").build()
        return client.newCall(request).execute().use { response ->
            assertEquals(200, response.code, path)
            response.body!!.string()
        }
    }

    @Test
    fun `real reviewed nearby and exact official price decode`() {
        assumeTrue(!base.isNullOrBlank(), "ANPFUEL_MAP_PROOF_BASE_URL not configured")
        val nearby = DirectoryStationJsonCodec.decodeNearby(
            get("/v1/stations/nearby?lat=-25.412557&lon=-49.279663&radius_m=3000&limit=100"),
        )
        assertTrue(nearby.isNotEmpty(), "proof area returned no stations")
        val reviewed = nearby.filter { item ->
            item.station.locationQuality == com.anpfuel.domain.discovery.StationLocationQuality.REVIEWED &&
                item.station.latitude != null && item.station.longitude != null
        }
        assertTrue(reviewed.isNotEmpty(), "proof area returned no reviewed coordinates")

        var decodedPrice: Long? = null
        for (item in reviewed) {
            val payload = get("/v1/stations/${item.station.stationId}/prices?fuel_product=GASOLINE_REGULAR")
            val groups = BackendPriceGroupsJsonCodec.decode(
                payload = payload,
                stationId = item.station.stationId,
                fuelFilterWire = "GASOLINE_REGULAR",
                fetchedAtMillis = 1,
                expiresAtMillis = 2,
            )
            val official = groups.groups.firstOrNull { it.official != null }?.official
            if (official != null) {
                decodedPrice = official.amountMilliBrl
                assertEquals("ANP", official.source)
                assertEquals("BRL", official.currency)
                assertTrue(official.surveyWeekStart.isNotBlank())
                assertTrue(official.surveyWeekEnd.isNotBlank())
                assertTrue(official.collectedOn.isNotBlank())
                break
            }
        }
        assertTrue(decodedPrice != null && decodedPrice!! > 0, "no exact official price decoded")
    }
}
