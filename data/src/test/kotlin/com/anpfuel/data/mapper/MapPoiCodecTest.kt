package com.anpfuel.data.mapper

import com.anpfuel.domain.map.MapContractException
import java.io.File
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

class MapPoiCodecTest {

    private fun fixture(name: String): String =
        File("../contracts/testdata/map/$name").readText()

    @Test
    fun `valid corpus preserves exact money identity and unknown charging facts`() {
        val corpus = MapPoiCodec.decodeCorpus(fixture("poi-assertions-v1.json"))
        assertEquals(4, corpus.assertions.size)

        val anp = corpus.assertions[0]
        assertEquals(5890L, anp.prices.single().amountMilliBrl)
        assertEquals("L", anp.prices.single().unit.wire)
        assertEquals("01122233300018", anp.identities.cnpj)
        assertTrue(anp.capabilities.pinEligible)

        val community = corpus.assertions[1]
        assertEquals(1850L, community.charging?.tariffMilliBrlPerKwh)
        assertEquals("CCS2", community.charging?.connectorCode)

        val ocm = corpus.assertions[2]
        assertNull(ocm.charging?.powerW)
        assertNull(ocm.charging?.tariffMilliBrlPerKwh)
        assertFalse(ocm.capabilities.priceKnown)

        val osm = corpus.assertions[3]
        assertFalse(osm.location.pinEligible)
        assertTrue(osm.prices.isEmpty())
    }

    @Test
    fun `invalid money unit pin and rights are rejected`() {
        val base = JSONObject(fixture("poi-assertions-v1.json"))
        val cases = listOf<Pair<String, (JSONObject) -> Unit>>(
            "wrong-product-unit" to { root ->
                root.getJSONArray("assertions").getJSONObject(0)
                    .getJSONArray("prices").getJSONObject(0)
                    .put("product", "LPG_P13")
            },
            "price-overflow" to { root ->
                root.getJSONArray("assertions").getJSONObject(0)
                    .getJSONArray("prices").getJSONObject(0)
                    .put("amount_milli_brl", 1_000_001)
            },
            "city-centroid-as-pin" to { root ->
                val item = root.getJSONArray("assertions").getJSONObject(3)
                item.getJSONObject("location")
                    .put("latitude_deg", "-15.79")
                    .put("longitude_deg", "-47.88")
                    .put("quality", "city-centroid")
                    .put("display_point", "station")
                item.getJSONObject("capabilities").put("pin_eligible", true)
            },
            "rights-source-mismatch" to { root ->
                root.getJSONArray("assertions").getJSONObject(0)
                    .getJSONObject("rights").put("source", "COMMUNITY")
            },
            "duplicate-id" to { root ->
                val list = root.getJSONArray("assertions")
                list.getJSONObject(1).put("id", list.getJSONObject(0).getString("id"))
            },
            "unknown-field" to { root ->
                root.getJSONArray("assertions").getJSONObject(0).put("invented", true)
            },
        )
        cases.forEach { (name, mutate) ->
            val copy = JSONObject(base.toString())
            mutate(copy)
            assertThrows<MapContractException>(name) { MapPoiCodec.decodeCorpus(copy.toString()) }
        }
    }

    @Test
    fun `unknown charging power and tariff stay null instead of zero`() {
        val ocm = MapPoiCodec.decodeCorpus(fixture("poi-assertions-v1.json")).assertions[2]
        assertNull(ocm.charging?.powerW)
        assertNull(ocm.charging?.tariffMilliBrlPerKwh)
        assertEquals("unknown", ocm.charging?.status)
    }

    @Test
    fun `native manifest pins engine and dependencies`() {
        val manifest = MapPoiCodec.decodeManifest(fixture("native-map-manifest-v1.json"))
        assertEquals("4587b8ad4d12c9f59982709fdaf03cc925948827", manifest.upstreamSha)
        assertEquals("ODbL-1.0", manifest.license)
        assertTrue(manifest.dependencies.contains("br-go"))

        val root = JSONObject(fixture("native-map-manifest-v1.json"))
        root.put("dependencies", org.json.JSONArray().put("world").put("br-sp"))
        assertThrows<MapContractException> { MapPoiCodec.decodeManifest(root.toString()) }
    }
}
