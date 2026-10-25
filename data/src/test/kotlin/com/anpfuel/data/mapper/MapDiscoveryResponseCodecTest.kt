package com.anpfuel.data.mapper

import com.anpfuel.domain.map.MapContractException
import java.io.File
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

class MapDiscoveryResponseCodecTest {
    private fun fixture(name: String): String = File("../contracts/testdata/map/$name").readText()

    @Test
    fun `actual API bytes preserve identity and exact price`() {
        val page = MapDiscoveryResponseCodec.decode(fixture("assertions-response-v1.json"))
        assertEquals("3ca88db9-19cb-4f86-969d-d69fb3fb506f", page.generation?.id)
        assertEquals("ACTIVE", page.generation?.status)
        assertEquals(3, page.items.size)
        assertEquals("11111111-1111-4111-8111-111111111111", page.items[0].id)
        assertEquals("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", page.items[0].identities.stationId)
        assertEquals(5890L, page.items[0].prices.single().amountMilliBrl)
        assertEquals("L", page.items[0].prices.single().unit.wire)
        assertTrue(page.items[2].location.pinEligible)
        assertNull(page.nextCursor)
    }

    @Test
    fun `API page and prepared corpus agree on the same assertion`() {
        val page = MapDiscoveryResponseCodec.decode(fixture("assertions-response-v1.json"))
        val corpus = MapPoiCodec.decodeCorpus(fixture("poi-assertions-v1.json"))
        val fromPage = page.items.first { it.id == "11111111-1111-4111-8111-111111111111" }
        val fromCorpus = corpus.assertions.first { it.id == fromPage.id }
        assertEquals(fromCorpus.identities.stationId, fromPage.identities.stationId)
        assertEquals(fromCorpus.prices.single(), fromPage.prices.single())
    }

    @Test
    fun `unknown fields and malformed identity fail closed`() {
        val root = JSONObject(fixture("assertions-response-v1.json"))
        root.getJSONArray("items").getJSONObject(0).put("invented", true)
        assertThrows<MapContractException> { MapDiscoveryResponseCodec.decode(root.toString()) }

        val missingStation = JSONObject(fixture("assertions-response-v1.json"))
        missingStation.getJSONArray("items").getJSONObject(0).put("station_uuid", JSONObject.NULL)
        assertThrows<MapContractException> { MapDiscoveryResponseCodec.decode(missingStation.toString()) }
    }
}
