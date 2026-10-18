package com.anpfuel.data.remote.quality

import com.anpfuel.domain.quality.QualityReadUnavailable
import java.io.IOException
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

@OptIn(kotlinx.coroutines.ExperimentalCoroutinesApi::class)
class QualityHttpClientTest {
    private val station="eeeeeeee-5555-4555-8555-000000000001"
    private val revision="eeeeeeee-6666-4666-8666-000000000001"
    private val assay="eeeeeeee-6666-4666-8666-000000000002"
    private fun lease(id:String=revision)="""{"source":"pmqc","stream":"pmqc-assay-v1","edition":"2026-09","revision_id":"$id","available":true,"stale":false,"published_at":"2026-10-18T00:00:00Z","revision_status":"published","is_current_stream":true,"coverage_note":"checked source"}"""
    private fun page(id:String=station,fuel:String="ETHANOL",rev:String=revision,next:String="null")="""{"station_id":"$id","items":[{"id":"$assay","sample_ref":"native:sample","assay_ref":"Aspecto","observed_on":"2026-09-01","fuel_product":"$fuel","result_text":"LII","unit":"","conformity":"FUTURE_STATUS"}],"lease":${lease(rev)},"next_cursor":$next}"""

    @Test fun `guest reads carry no credentials and unknown conformity is retained`()=runTest {
        MockWebServer().use {server ->
            server.enqueue(MockResponse().setBody("""{"station_id":"$station","fuel_cells":[{"fuel_product":"ETHANOL","conformity":"FUTURE_STATUS","count":1,"last_observed":"2026-09-01"}],"lease":${lease()},"generated_at":"2026-10-18T00:00:00Z"}"""))
            val api=QualityHttpClient(OkHttpClient(),server.url("/").toString())
            val summary=api.summary(station,"ETHANOL",null)
            assertEquals("FUTURE_STATUS",summary.cells.single().conformity)
            assertFalse(summary.dimensionsAvailable)
            val request=server.takeRequest()
            assertNull(request.getHeader("Authorization"));assertNull(request.getHeader("Signature"))
            assertEquals(0L,request.bodySize)
            assertEquals(station,request.requestUrl?.queryParameter("station_id"))
            server.enqueue(MockResponse().setBody(page(next="\"opaque-cursor\"")))
            val result=api.assays(station,"ETHANOL",revision,null)
            assertEquals("opaque-cursor",result.nextCursor)
            assertEquals("FUTURE_STATUS",result.items.single().conformity)
            assertEquals(revision,server.takeRequest().requestUrl?.queryParameter("revision_id"))
        }
    }

    @Test fun `foreign station fuel revision and repeated cursor fail closed`()=runTest {
        MockWebServer().use {server ->
            val api=QualityHttpClient(OkHttpClient(),server.url("/").toString())
            for(body in listOf(page(id=revision),page(fuel="DIESEL_S10"),page(rev=station),page(next="\"repeat\""))) {
                server.enqueue(MockResponse().setBody(body))
                assertThrows(QualityReadUnavailable::class.java) {runBlocking {api.assays(station,"ETHANOL",revision,"repeat")}}
            }
        }
    }

    @Test fun `oversize errors and redirects never disclose raw upstream data`()=runTest {
        MockWebServer().use {server ->
            val api=QualityHttpClient(OkHttpClient(),server.url("/").toString())
            server.enqueue(MockResponse().setBody("x".repeat(1024*1024+1)))
            assertThrows(QualityReadUnavailable::class.java) {runBlocking {api.summary(station,"ETHANOL",null)}}
            server.enqueue(MockResponse().setResponseCode(503).setBody("private diagnostic should not escape"))
            val error=assertThrows(QualityReadUnavailable::class.java) {runBlocking {api.summary(station,"ETHANOL",null)}}
            assertFalse(error.message.orEmpty().contains("private"))
            server.enqueue(MockResponse().setResponseCode(302).setHeader("Location","https://example.invalid/private"))
            assertThrows(QualityReadUnavailable::class.java) {runBlocking {api.summary(station,"ETHANOL",null)}}
            assertEquals(3,server.requestCount)
        }
    }

    @Test fun `cancellation closes the actual public HTTP request`()=runTest {
        val server=MockWebServer()
        val failed=CountDownLatch(1)
        try {
            server.enqueue(MockResponse().setSocketPolicy(okhttp3.mockwebserver.SocketPolicy.NO_RESPONSE))
            val http=OkHttpClient.Builder().eventListener(object:okhttp3.EventListener(){
                override fun callFailed(call:okhttp3.Call,ioe:IOException){failed.countDown()}
            }).build()
            val api=QualityHttpClient(http,server.url("/").toString())
            val pending=launch {api.summary(station,"ETHANOL",null)}
            runCurrent()
            assertNotNull(server.takeRequest(3,TimeUnit.SECONDS))
            pending.cancel();pending.join()
            assertTrue(failed.await(3,TimeUnit.SECONDS))
        } finally {server.shutdown()}
    }

    @Test fun `public source link cannot escape the official ANP origin`()=runTest {
        MockWebServer().use {server ->
            val api=QualityHttpClient(OkHttpClient(),server.url("/").toString())
            server.enqueue(MockResponse().setBody("""{"lease":${lease()},"resources":[{"url":"https://example.invalid/foreign","sha256":"${"a".repeat(64)}"}]}"""))
            assertThrows(QualityReadUnavailable::class.java) {runBlocking {api.provenance(revision)}}
            server.enqueue(MockResponse().setBody("""{"lease":${lease()},"resources":[{"url":"https://www.gov.br:8443/anp/pmqc.zip","sha256":"${"a".repeat(64)}"}]}"""))
            assertThrows(QualityReadUnavailable::class.java) {runBlocking {api.provenance(revision)}}
            server.enqueue(MockResponse().setBody("""{"lease":${lease()},"resources":[{"url":"https://www.gov.br/anp/pmqc.zip","sha256":"${"a".repeat(64)}","member_name":"PMQC_2026_09.csv"}]}"""))
            assertEquals("PMQC_2026_09.csv",api.provenance(revision).resources.single().member)
        }
    }
}
