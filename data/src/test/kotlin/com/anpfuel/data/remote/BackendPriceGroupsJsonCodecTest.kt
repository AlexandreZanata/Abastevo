package com.anpfuel.data.remote

import com.anpfuel.domain.exception.DomainException
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class BackendPriceGroupsJsonCodecTest {

    private val stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26"

    private fun payload(
        fuel: String = "GASOLINE_REGULAR",
        communityNull: Boolean = true,
    ): String = """
        {"items": [{
          "station_id": "$stationId",
          "fuel_product": "$fuel",
          "unit": "L",
          "condition": {"kind": "STANDARD", "qualifier_id": null},
          "official": {
            "source": "ANP",
            "amount_milli_brl": 5999,
            "currency": "BRL",
            "collected_on": "2026-09-28",
            "survey_week": {"start": "2026-09-21", "end": "2026-09-27"},
            "revision_id": "123e4567-e89b-12d3-a456-426614174000"
          },
          "community": ${if (communityNull) "null" else """{"source":"COMMUNITY"}"""}
        }], "generated_at": "2026-09-28T00:00:00Z"}
    """.trimIndent()

    @Test
    fun `decodes official group with source version expiry`() {
        val groups = BackendPriceGroupsJsonCodec.decode(
            payload = payload(),
            stationId = stationId,
            fuelFilterWire = "GASOLINE_REGULAR",
            fetchedAtMillis = 1_000_000L,
            expiresAtMillis = 1_060_000L,
        )

        assertEquals("backend", groups.source)
        assertEquals("v1", groups.version)
        assertEquals(1, groups.groups.size)
        assertEquals(5999L, groups.groups.single().official?.amountMilliBrl)
    }

    @Test
    fun `additived round-trips losslessly`() {
        val decoded = BackendPriceGroupsJsonCodec.decode(
            payload = payload(fuel = "GASOLINE_ADDITIVED"),
            stationId = stationId,
            fuelFilterWire = "GASOLINE_ADDITIVED",
            fetchedAtMillis = 1_000_000L,
            expiresAtMillis = 1_060_000L,
        )
        val encoded = BackendPriceGroupsJsonCodec.encode(decoded)
        val again = BackendPriceGroupsJsonCodec.decode(
            payload = encoded,
            stationId = stationId,
            fuelFilterWire = "GASOLINE_ADDITIVED",
            fetchedAtMillis = 1_000_000L,
            expiresAtMillis = 1_060_000L,
        )

        assertEquals("GASOLINE_ADDITIVED", again.groups.single().fuelProductWire)
        assertTrue(encoded.contains("GASOLINE_ADDITIVED"))
    }

    @Test
    fun `refuses legacy premium unknown fuel and non-null community`() {
        assertThrows(DomainException::class.java) {
            BackendPriceGroupsJsonCodec.decode(
                payload = payload(fuel = "GASOLINE_PREMIUM"),
                stationId = stationId,
                fuelFilterWire = null,
                fetchedAtMillis = 1_000_000L,
                expiresAtMillis = 1_060_000L,
            )
        }
        assertThrows(DomainException::class.java) {
            BackendPriceGroupsJsonCodec.decode(
                payload = payload(fuel = "JET"),
                stationId = stationId,
                fuelFilterWire = null,
                fetchedAtMillis = 1_000_000L,
                expiresAtMillis = 1_060_000L,
            )
        }
        assertThrows(DomainException::class.java) {
            BackendPriceGroupsJsonCodec.decode(
                payload = payload(communityNull = false),
                stationId = stationId,
                fuelFilterWire = null,
                fetchedAtMillis = 1_000_000L,
                expiresAtMillis = 1_060_000L,
            )
        }
    }
}
