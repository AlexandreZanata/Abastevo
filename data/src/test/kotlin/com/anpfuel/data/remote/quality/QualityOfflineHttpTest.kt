package com.anpfuel.data.remote.quality

import com.anpfuel.data.local.quality.QualityPublicCache
import com.anpfuel.domain.quality.QualityReadUnavailable
import java.nio.file.Files
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class QualityOfflineHttpTest {
    private val station="eeeeeeee-5555-4555-8555-000000000001"
    private val revision="eeeeeeee-6666-4666-8666-000000000001"
    private fun summary(id:String=station)="""{"station_id":"$id","fuel_cells":[],"lease":{"source":"pmqc","stream":"pmqc-assay-v1","edition":"2026-09","revision_id":"$revision","available":true,"stale":false,"published_at":"2026-10-18T00:00:00Z","revision_status":"published","is_current_stream":true,"coverage_note":"checked source"},"generated_at":"2026-10-18T00:00:00Z"}"""
    @Test fun `validated body survives client restart conditional refresh and outage as stale only`()=runTest {
        val dir=Files.createTempDirectory("quality-offline-http").toFile()
        try {MockWebServer().use {server ->
            val origin=server.url("/").toString()
            val api=QualityHttpClient(OkHttpClient(),origin,QualityPublicCache(dir))
            server.enqueue(MockResponse().setBody(summary()).setHeader("ETag","\"v1\""))
            assertFalse(api.summary(station,"ETHANOL",null).lease.stale)
            server.takeRequest()
            val restarted=QualityHttpClient(OkHttpClient(),origin,QualityPublicCache(dir))
            server.enqueue(MockResponse().setResponseCode(304))
            assertFalse(restarted.summary(station,"ETHANOL",null).lease.stale)
            assertEquals("\"v1\"",server.takeRequest().getHeader("If-None-Match"))
            server.enqueue(MockResponse().setResponseCode(503))
            assertTrue(restarted.summary(station,"ETHANOL",null).lease.stale)
            server.shutdown()
            assertTrue(restarted.summary(station,"ETHANOL",null).lease.stale)
        }}finally {dir.deleteRecursively()}
    }
    @Test fun `malformed foreign and disabled responses never fall back or poison prior accepted cache`()=runTest {
        val dir=Files.createTempDirectory("quality-offline-refusal").toFile()
        try {MockWebServer().use {server ->
            val api=QualityHttpClient(OkHttpClient(),server.url("/").toString(),QualityPublicCache(dir))
            server.enqueue(MockResponse().setBody(summary()));api.summary(station,"ETHANOL",null)
            for(body in listOf("invalid",summary(revision))) {
                server.enqueue(MockResponse().setBody(body))
                assertThrows(QualityReadUnavailable::class.java){runBlocking {api.summary(station,"ETHANOL",null)}}
                server.enqueue(MockResponse().setResponseCode(503))
                assertEquals(station,api.summary(station,"ETHANOL",null).stationId)
            }
            server.enqueue(MockResponse().setResponseCode(404))
            assertTrue(assertThrows(QualityReadUnavailable::class.java){runBlocking {api.summary(station,"ETHANOL",null)}}.disabled)
            server.enqueue(MockResponse().setResponseCode(503))
            assertThrows(QualityReadUnavailable::class.java){runBlocking {api.summary(station,"ETHANOL",null)}}
        }}finally {dir.deleteRecursively()}
    }
    @Test fun `cache expiry or clock rollback during a stalled refresh is rechecked before fallback`()=runTest {
        for(rollback in listOf(false,true)) {
            val dir=Files.createTempDirectory("quality-offline-clock").toFile()
            try {MockWebServer().use {server ->
                var now=1000L
                val api=QualityHttpClient(OkHttpClient(),server.url("/").toString(),QualityPublicCache(dir){now})
                server.enqueue(MockResponse().setBody(summary()));api.summary(station,"ETHANOL",null)
                server.dispatcher=object:okhttp3.mockwebserver.Dispatcher(){
                    override fun dispatch(request:okhttp3.mockwebserver.RecordedRequest):MockResponse {
                        now=if(rollback)999L else 1000L+QualityPublicCache.RETENTION_MILLIS
                        return MockResponse().setResponseCode(503)
                    }
                }
                assertThrows(QualityReadUnavailable::class.java){runBlocking {api.summary(station,"ETHANOL",null)}}
            }}finally {dir.deleteRecursively()}
        }
    }

    @Test fun `successful no store response removes prior cached public representation`()=runTest {
        val dir=Files.createTempDirectory("quality-no-store").toFile()
        try {MockWebServer().use {server ->
            val api=QualityHttpClient(OkHttpClient(),server.url("/").toString(),QualityPublicCache(dir))
            server.enqueue(MockResponse().setBody(summary()));api.summary(station,"ETHANOL",null)
            server.enqueue(MockResponse().setBody(summary()).setHeader("Cache-Control","no-store"))
            api.summary(station,"ETHANOL",null)
            server.enqueue(MockResponse().setResponseCode(503))
            assertThrows(QualityReadUnavailable::class.java){runBlocking {api.summary(station,"ETHANOL",null)}}
        }}finally {dir.deleteRecursively()}
    }

}
