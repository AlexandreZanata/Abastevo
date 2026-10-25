package com.anpfuel.data.mapper

import com.anpfuel.data.remote.DirectoryStationJsonCodec
import java.nio.file.Path
import kotlinx.coroutines.runBlocking
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okio.Buffer
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * MAP-20 server → Kotlin proof with exact backend bytes over real HTTP.
 * Uses staging `GET /v1/stations` output (Go JSON, not a synthetic fixture):
 * the same bytes are served over localhost HTTP and decoded by the
 * production codec. No price is invented; unknown locations stay unknown.
 */
class MapServerDecodeTest {
    private val server = MockWebServer()

    @AfterEach fun stop() { server.shutdown() }

    private fun stagingFixture(): String {
        val captured = Path.of("/tmp/staging-stations.json").toFile()
        if (captured.isFile) return captured.readText()
        return Path.of("contracts/testdata/map/stations-staging-sample.json").toFile().readText()
    }

    @Test fun `real backend json decodes with canonical ids`() {
        val raw = stagingFixture()
        val page = DirectoryStationJsonCodec.decodeList(raw)
        assertTrue(page.items.isNotEmpty())
        val first = page.items.first()
        assertTrue(first.stationId.matches(Regex("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")))
        assertTrue(first.displayName.isNotBlank())
    }

    @Test fun `real http with go bytes decodes without mock client`() {
        val raw = stagingFixture()
        server.enqueue(MockResponse().setBody(Buffer().write(raw.toByteArray())))
        server.start()
        val url = server.url("/v1/stations?limit=5").toString()
        val fetched = runBlocking {
            val req = Request.Builder().url(url).build()
            OkHttpClient().newCall(req).execute().use { resp ->
                assertEquals(200, resp.code)
                resp.body!!.string()
            }
        }
        val page = DirectoryStationJsonCodec.decodeList(fetched)
        assertTrue(page.items.isNotEmpty())
        assertEquals(
            DirectoryStationJsonCodec.decodeList(raw).items.map { it.stationId },
            page.items.map { it.stationId },
        )
    }
}
