package com.anpfuel.data.map

import kotlinx.coroutines.runBlocking
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.Assertions.assertArrayEquals
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class MapDistributionTest {
    private val keys = mapOf("staging-20261027" to ByteArray(32) { 7 })

    @Test fun `manifest lives at the static signed region path`() {
        assertEquals(
            "https://teste.abastevo.com.br/dados/mapa-dist/regions/brazil-mato-grosso/manifest",
            mapRegionManifestUrl("https://teste.abastevo.com.br", "brazil-mato-grosso"),
        )
        assertEquals(
            "https://teste.abastevo.com.br/dados/mapa-dist/regions/brazil-sao-paulo-city/manifest",
            mapRegionManifestUrl("https://teste.abastevo.com.br/", "brazil-sao-paulo-city"),
        )
    }

    @Test fun `unsafe or blank region slugs never reach the network`() {
        for (slug in listOf("", "Brazil_Mato Grosso", "brazil mato", "../x", "brazil/x", "a".repeat(65))) {
            val error = assertThrows(MapDownloadException::class.java) { mapRegionManifestUrl("https://x.test", slug) }
            assertEquals("map-region-unknown", error.message)
        }
    }

    @Test fun `configured depends on origin and keys only`() {
        val runtime = MapDistributionConfig("https://teste.abastevo.com.br", "", keys, keysInvalid = false)
        assertTrue(runtime.configured)
        assertNull(runtime.defaultRegion)
        assertEquals("brazil-bahia", runtime.copy(region = "brazil-bahia").defaultRegion)
        assertNull(runtime.copy(region = "Brazil_Bahia").defaultRegion)
        assertFalse(runtime.copy(baseUrl = "").configured)
        assertTrue(runtime.copy(baseUrl = "").missingUrl)
        assertFalse(runtime.copy(trustedKeys = emptyMap()).configured)
        assertTrue(runtime.copy(trustedKeys = emptyMap()).missingKeys)
        assertFalse(runtime.copy(trustedKeys = emptyMap(), keysInvalid = true).configured)
    }

    @Test fun `fetch requests the selected region manifest`() = runBlocking {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setBody("{\"envelope\":true}"))
            server.start()
            val config = MapDistributionConfig(server.url("/").toString().trimEnd('/'), "", keys, false)
            val body = MapDistribution(OkHttpClient(), config).fetchEnvelope("brazil-northeast-region-west")
            assertArrayEquals("{\"envelope\":true}".toByteArray(), body)
            assertEquals("/dados/mapa-dist/regions/brazil-northeast-region-west/manifest", server.takeRequest().path)
        }
    }

    @Test fun `unconfigured distribution refuses before any request`() {
        MockWebServer().use { server ->
            server.start()
            val config = MapDistributionConfig(server.url("/").toString(), "brazil-bahia", emptyMap(), false)
            val error = assertThrows(MapDownloadException::class.java) {
                runBlocking { MapDistribution(OkHttpClient(), config).fetchEnvelope("brazil-bahia") }
            }
            assertEquals("map-distribution-unconfigured", error.message)
            assertEquals(0, server.requestCount)
        }
    }
}
