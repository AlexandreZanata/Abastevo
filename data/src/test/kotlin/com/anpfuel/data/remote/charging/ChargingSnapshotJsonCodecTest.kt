package com.anpfuel.data.remote.charging

import org.json.JSONObject
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class ChargingSnapshotJsonCodecTest {

    private val placeId = "11111111-1111-4111-8111-111111111111"
    private val groupId = "22222222-2222-4222-8222-222222222222"
    private val key = "b".repeat(64)

    private fun group(connector: String = "CCS2", current: String = "DC") =
        """{"id":"$groupId","entity_key":"$key","external_id":"OCM-7","connector_code":"$connector","raw_connector_code":"$connector","current":"$current","power_w":150000,"reported_quantity":2}"""

    private fun place(extra: String = "") =
        """{"id":"$placeId","entity_key":"$key","external_id":"OCM-7","name":"Eletroposto DF-7","country":"BR","latitude":"-15.7939","longitude":"-47.8828","public_access":"PUBLIC","reported_points":2$extra}"""

    private fun detail() =
        JSONObject(place().dropLast(1) + ""","groups":[${group()}] }""")

    private fun page(items: String = place(), next: String = "null") =
        JSONObject("""{"items":[$items],"next_cursor":$next,"generated_at":"2026-10-22T00:00:00Z"}""")

    @Test
    fun `valid page detail and coverage decode with raw retention`() {
        val parsed = ChargingSnapshotJsonCodec.decodePlacePage(page())
        assertEquals(1, parsed.items.size)
        assertEquals("Eletroposto DF-7", parsed.items.single().name)
        val withFuture = ChargingSnapshotJsonCodec.decodePlaceDetail(
            JSONObject(place().dropLast(1) + ""","groups":[${group("MEGAWATT_X", "AC3")}] }"""),
        )
        assertEquals(com.anpfuel.domain.charging.ChargingConnector.UNKNOWN, withFuture.groups.single().connector)
        assertEquals("MEGAWATT_X", withFuture.groups.single().rawConnectorCode)
        assertEquals(com.anpfuel.domain.charging.ChargingCurrent.UNKNOWN, withFuture.groups.single().current)
        val coverage = ChargingSnapshotJsonCodec.decodeCoverage(
            JSONObject("""{"profile":"ocm-br-20261021-v1","source_revision":"rev-1","places":1298,"groups":3387,"vehicles":181,"generated_at":"2026-10-22T00:00:00Z"}"""),
        )
        assertEquals(3387L, coverage.groups)
    }

    @Test
    fun `corrupt oversize foreign and private bodies refuse`() {
        assertThrows(ChargingSnapshotUnavailable::class.java) {
            ChargingSnapshotJsonCodec.decodePlacePage(page(place().replace(placeId, "not-a-uuid")))
        }
        assertThrows(ChargingSnapshotUnavailable::class.java) {
            ChargingSnapshotJsonCodec.decodePlacePage(page(place().replace("\"reported_points\":2", "\"reported_points\":-1")))
        }
        val many = (1..101).joinToString(",") { place() }
        assertThrows(ChargingSnapshotUnavailable::class.java) {
            ChargingSnapshotJsonCodec.decodePlacePage(JSONObject("""{"items":[$many],"next_cursor":null,"generated_at":"2026-10-22T00:00:00Z"}"""))
        }
        val dup = place() + "," + place()
        assertThrows(ChargingSnapshotUnavailable::class.java) { ChargingSnapshotJsonCodec.decodePlacePage(page(dup)) }
        // Private operator/contributor payload: refused, never stored or returned.
        val private = place().dropLast(1) + ""","reporter":"user-9","price":5.99 }"""
        assertThrows(ChargingSnapshotUnavailable::class.java) { ChargingSnapshotJsonCodec.decodePlacePage(page(private)) }
        assertThrows(ChargingSnapshotUnavailable::class.java) {
            ChargingSnapshotJsonCodec.decodeCoverage(JSONObject("""{"profile":"x","places":-3}"""))
        }
        assertThrows(ChargingSnapshotUnavailable::class.java) {
            ChargingSnapshotJsonCodec.decodePlaceDetail(detail().put("groups", "not-an-array"))
        }
    }
}
