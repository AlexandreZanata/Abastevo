package com.anpfuel.data.repository

import com.anpfuel.data.remote.DirectoryStationHttpClient
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class DirectoryStationRepositoryTest {

    private val stationJson = """
        {
          "station_id": "d6c74c23-63db-4c24-a2e5-408cb23bad26",
          "display_name": "Posto Central",
          "cnpj_normalized": "04218406000104",
          "municipality_code": "3550308",
          "state": "SP",
          "location_quality": "reviewed",
          "coordinates": {"lat": -23.55, "lon": -46.63},
          "current_revision_id": "aac027be-d331-40e4-8d63-52ec3b8d2f41"
        }
    """.trimIndent()

    private fun gateway(server: MockWebServer): DirectoryStationGatewayImpl =
        DirectoryStationGatewayImpl(
            DirectoryStationHttpClient(
                client = OkHttpClient.Builder()
                    .connectTimeout(2L, TimeUnit.SECONDS)
                    .readTimeout(2L, TimeUnit.SECONDS)
                    .build(),
                baseUrl = server.url("/").toString(),
            ),
        )

    @Test
    fun `gateway decodes list and detail`() = runTest {
        val server = MockWebServer()
        try {
            server.enqueue(MockResponse().setResponseCode(200).setBody("""{"items": [$stationJson]}"""))
            val page = gateway(server).list(20, null)
            assertEquals(1, page.items.size)
            assertEquals("Posto Central", page.items[0].displayName)

            server.enqueue(MockResponse().setResponseCode(200).setBody(stationJson))
            val detail = gateway(server).detail("d6c74c23-63db-4c24-a2e5-408cb23bad26")
            assertEquals("04218406000104", detail.cnpjNormalized)
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun `gateway propagates transport failure`() = runTest {
        val server = MockWebServer()
        try {
            server.enqueue(MockResponse().setResponseCode(500).setBody("down"))
            assertThrows(java.io.IOException::class.java) {
                kotlinx.coroutines.runBlocking {
                    gateway(server).list(20, null)
                }
            }
        } finally {
            server.shutdown()
        }
    }
}
